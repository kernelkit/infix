// Package ethmonitor subscribes to ethtool genetlink notifications and
// keeps per-interface ethernet settings updated via a callback.
//
// Data is fetched by shelling out to `ethtool --json <ifname>` (matching
// the Python yanger approach) while genetlink provides reactive change
// notifications.
package ethmonitor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"strings"

	"github.com/mdlayher/genetlink"
	"github.com/mdlayher/netlink"
)

const (
	ETHTOOL_MSG_LINKINFO_NTF  = 28
	ETHTOOL_MSG_LINKMODES_NTF = 29

	ethtoolFamilyName       = "ethtool"
	ethtoolMonitorGroupName = "monitor"

	nlaHeaderIfindex = 1

	ethtoolSpeedUnknown = (1 << 32) - 1
)

// CommandRunner executes external commands and returns stdout.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// EthMonitor listens for ethtool genetlink monitor events and updates
// interface ethernet operational state via a callback.
type EthMonitor struct {
	conn    *genetlink.Conn
	family  genetlink.Family
	groupID uint32
	cmd     CommandRunner
	ctx     context.Context
	log     *slog.Logger
	onUpdate func(ifname string, data json.RawMessage)
}

// New creates an EthMonitor, resolves the ethtool genetlink family,
// and joins its "monitor" multicast group.
func New(log *slog.Logger, cmd CommandRunner) (*EthMonitor, error) {
	conn, err := genetlink.Dial(nil)
	if err != nil {
		return nil, fmt.Errorf("dial genetlink: %w", err)
	}

	family, err := conn.GetFamily(ethtoolFamilyName)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("resolve %q genetlink family: %w", ethtoolFamilyName, err)
	}

	var groupID uint32
	for _, g := range family.Groups {
		if g.Name == ethtoolMonitorGroupName {
			groupID = g.ID
			break
		}
	}
	if groupID == 0 {
		_ = conn.Close()
		return nil, fmt.Errorf("multicast group %q not found in family %q", ethtoolMonitorGroupName, ethtoolFamilyName)
	}

	if err := conn.JoinGroup(groupID); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("join ethtool monitor group %d: %w", groupID, err)
	}

	return &EthMonitor{
		conn:    conn,
		family:  family,
		groupID: groupID,
		cmd:     cmd,
		log:     log,
	}, nil
}

// SetOnUpdate sets the callback invoked when ethernet data changes.
func (m *EthMonitor) SetOnUpdate(fn func(string, json.RawMessage)) {
	m.onUpdate = fn
}

// Run starts the ethtool genetlink receive loop and updates interface
// ethernet settings when link info or link mode notifications are seen.
func (m *EthMonitor) Run(ctx context.Context) error {
	m.ctx = ctx
	defer func() {
		if err := m.conn.Close(); err != nil {
			m.log.Warn("ethmonitor: close genetlink conn", "err", err)
		}
	}()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		msgs, _, err := m.conn.Receive()
		if err != nil {
			if cerr := ctx.Err(); cerr != nil {
				return cerr
			}
			return fmt.Errorf("receive ethtool genetlink message: %w", err)
		}

		for _, msg := range msgs {
			switch msg.Header.Command {
			case ETHTOOL_MSG_LINKINFO_NTF, ETHTOOL_MSG_LINKMODES_NTF:
				ifname, err := extractIfname(msg.Data)
				if err != nil {
					m.log.Warn("ethmonitor: extract interface name", "err", err)
					continue
				}
				m.refreshEthernetSettings(ifname)
			}
		}
	}
}

// RefreshInterface refreshes ethernet settings for ifname. It is intended
// to be called by other subsystems (for example nlmonitor RTM_NEWLINK).
func (m *EthMonitor) RefreshInterface(ifname string) {
	m.refreshEthernetSettings(ifname)
}

// ethtoolJSON represents the relevant fields from `ethtool --json <ifname>`.
type ethtoolJSON struct {
	Speed              int      `json:"speed"`
	Duplex             string   `json:"duplex"`
	Port               string   `json:"port"`
	AutoNegotiation    bool     `json:"auto-negotiation"`
	SupportedLinkModes []string `json:"supported-link-modes"`
	AdvertisedLinkModes []string `json:"advertised-link-modes"`
}

func (m *EthMonitor) refreshEthernetSettings(ifname string) {
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	out, err := m.cmd.Run(ctx, "ethtool", "--json", ifname)
	if err != nil {
		m.log.Warn("ethmonitor: run ethtool", "ifname", ifname, "err", err)
		return
	}

	var results []ethtoolJSON
	if err := json.Unmarshal(out, &results); err != nil {
		m.log.Warn("ethmonitor: parse ethtool json", "ifname", ifname, "err", err)
		return
	}
	if len(results) == 0 {
		return
	}

	data := results[0]
	eth, speedBPS := buildEthernetContainer(data)

	// Marshal the result; include interface-level speed as a special key
	// that mergeAugments will lift onto the interface object.
	result := map[string]any{"ethernet": eth}
	if speedBPS > 0 {
		result["speed"] = fmt.Sprintf("%d", speedBPS)
	}

	raw, err := json.Marshal(result)
	if err != nil {
		m.log.Warn("ethmonitor: marshal ethernet settings", "ifname", ifname, "err", err)
		return
	}

	if m.onUpdate != nil {
		m.onUpdate(ifname, json.RawMessage(raw))
	}
}

// buildEthernetContainer builds the ieee802-ethernet-interface:ethernet
// container and returns (container, interface speed in bits/s or 0).
func buildEthernetContainer(data ethtoolJSON) (map[string]any, int64) {
	autoneg := map[string]any{"enable": data.AutoNegotiation}
	eth := map[string]any{"auto-negotiation": autoneg}

	duplex := strings.ToLower(data.Duplex)
	if duplex == "full" || duplex == "half" {
		eth["duplex"] = duplex
	}

	// Supported PMD types (config-false leaf-list).
	supported := ethtoolModesToPMD(data.SupportedLinkModes)
	if len(supported) > 0 {
		eth["infix-ethernet-interface:supported-pmd-types"] = supported
	}

	// Advertised PMD types — suppress when identical to supported (default).
	advertised := ethtoolModesToPMD(data.AdvertisedLinkModes)
	if len(advertised) > 0 && !stringSliceEqual(advertised, supported) {
		autoneg["infix-ethernet-interface:advertised-pmd-types"] = advertised
	}

	// Speed, phy-type, pmd-type.
	var speedBPS int64
	speedMbps := data.Speed
	if speedMbps > 0 && speedMbps < ethtoolSpeedUnknown {
		speedBPS = int64(speedMbps) * 1_000_000

		// Speed inside the ethernet container (decimal64, Gb/s).
		eth["speed"] = fmt.Sprintf("%.3f", float64(speedMbps)/1000.0)

		key := linkModeKey{Port: data.Port, SpeedMbps: speedMbps, Duplex: duplex}
		if mapping, ok := linkModes[key]; ok {
			eth["phy-type"] = "ieee802-ethernet-phy-type:phy-type-" + mapping.PhyType
			if mapping.PMDType != "" {
				eth["pmd-type"] = "ieee802-ethernet-phy-type:pmd-type-" + mapping.PMDType
			}
		}

		// Refine pmd-type when exactly one supported mode (specific SFP).
		if len(supported) == 1 {
			eth["pmd-type"] = supported[0]
		}
	}

	return eth, speedBPS
}

func extractIfname(data []byte) (string, error) {
	ad, err := netlink.NewAttributeDecoder(data)
	if err != nil {
		return "", fmt.Errorf("new decoder: %w", err)
	}

	for ad.Next() {
		nested, err := netlink.NewAttributeDecoder(ad.Bytes())
		if err != nil {
			continue
		}

		for nested.Next() {
			if nested.Type() != nlaHeaderIfindex {
				continue
			}

			ifindex := int(nested.Uint32())
			iface, err := net.InterfaceByIndex(ifindex)
			if err != nil {
				return "", fmt.Errorf("lookup interface index %d: %w", ifindex, err)
			}
			return iface.Name, nil
		}
		if err := nested.Err(); err != nil {
			return "", fmt.Errorf("decode nested attrs: %w", err)
		}
	}

	if err := ad.Err(); err != nil {
		return "", fmt.Errorf("decode attrs: %w", err)
	}

	return "", fmt.Errorf("header ifindex attribute not found")
}

// linkModeKey is the lookup key for phy-type/pmd-type mapping.
type linkModeKey struct {
	Port      string
	SpeedMbps int
	Duplex    string
}

// linkModeMapping holds the IEEE identity suffixes.
type linkModeMapping struct {
	PhyType string
	PMDType string // empty means "cannot determine from this tuple alone"
}

// linkModes maps (port, speed, duplex) → (phy-type, pmd-type) per
// IEEE Std 802.3.2-2025 (ieee802-ethernet-phy-type).
var linkModes = map[linkModeKey]linkModeMapping{
	{"Twisted Pair", 10, "full"}:       {"10BASE-T", "10BASE-T"},
	{"Twisted Pair", 10, "half"}:       {"10BASE-T", "10BASE-T"},
	{"Twisted Pair", 100, "full"}:      {"100BASE-X", "100BASE-TX"},
	{"Twisted Pair", 100, "half"}:      {"100BASE-X", "100BASE-TX"},
	{"Twisted Pair", 1000, "full"}:     {"1000BASE-T", "1000BASE-T"},
	{"Twisted Pair", 1000, "half"}:     {"1000BASE-T", "1000BASE-T"},
	{"Twisted Pair", 2500, "full"}:     {"2.5GBASE-T", "2.5GBASE-T"},
	{"Twisted Pair", 5000, "full"}:     {"5GBASE-T", "5GBASE-T"},
	{"Twisted Pair", 10000, "full"}:    {"10GBASE-T", "10GBASE-T"},
	{"Twisted Pair", 25000, "full"}:    {"25GBASE-T", "25GBASE-T"},
	{"Twisted Pair", 40000, "full"}:    {"40GBASE-T", "40GBASE-T"},
	{"MII", 10, "full"}:               {"10BASE-T", "10BASE-T"},
	{"MII", 10, "half"}:               {"10BASE-T", "10BASE-T"},
	{"MII", 100, "full"}:              {"100BASE-X", "100BASE-TX"},
	{"MII", 100, "half"}:              {"100BASE-X", "100BASE-TX"},
	{"FIBRE", 100, "full"}:            {"100BASE-X", ""},
	{"FIBRE", 1000, "full"}:           {"1000BASE-X", ""},
	{"FIBRE", 10000, "full"}:          {"10GBASE-R", ""},
	{"FIBRE", 25000, "full"}:          {"25GBASE-R", ""},
	{"FIBRE", 40000, "full"}:          {"40GBASE-R", ""},
	{"FIBRE", 100000, "full"}:         {"100GBASE-R", ""},
	{"Direct Attach Copper", 10000, "full"}:  {"10GBASE-R", ""},
	{"Direct Attach Copper", 25000, "full"}:  {"25GBASE-R", "25GBASE-CR"},
	{"Direct Attach Copper", 40000, "full"}:  {"40GBASE-R", "40GBASE-CR4"},
	{"Direct Attach Copper", 100000, "full"}: {"100GBASE-R", "100GBASE-CR4"},
}

// ethtoolToPMD maps kernel link-mode base names to IEEE pmd-type
// identity suffixes. The kernel reports modes like "1000baseT/Full";
// we strip the "/Full" or "/Half" suffix before lookup.
var ethtoolToPMD = map[string]string{
	"10baseT":          "10BASE-T",
	"10baseT1L":        "10BASE-T1L",
	"100baseT":         "100BASE-TX",
	"100baseT1":        "100BASE-T1",
	"100baseFX":        "100BASE-FX",
	"1000baseT":        "1000BASE-T",
	"1000baseT1":       "1000BASE-T1",
	"1000baseX":        "1000BASE-LX",
	"1000baseKX":       "1000BASE-KX",
	"2500baseT":        "2.5GBASE-T",
	"2500baseX":        "2.5GBASE-X",
	"5000baseT":        "5GBASE-T",
	"10000baseT":       "10GBASE-T",
	"10000baseSR":      "10GBASE-SR",
	"10000baseLR":      "10GBASE-LR",
	"10000baseLRM":     "10GBASE-LRM",
	"10000baseER":      "10GBASE-ER",
	"10000baseKR":      "10GBASE-KR",
	"10000baseKX4":     "10GBASE-KX4",
	"25000baseCR":      "25GBASE-CR",
	"25000baseSR":      "25GBASE-SR",
	"25000baseKR":      "25GBASE-KR",
	"40000baseCR4":     "40GBASE-CR4",
	"40000baseSR4":     "40GBASE-SR4",
	"40000baseLR4":     "40GBASE-LR4",
	"40000baseKR4":     "40GBASE-KR4",
	"100000baseCR4":    "100GBASE-CR4",
	"100000baseSR4":    "100GBASE-SR4",
	"100000baseLR4_ER4": "100GBASE-LR4",
	"100000baseKR4":    "100GBASE-KR4",
}

// ethtoolModesToPMD translates a list of ethtool link-mode strings
// (e.g. "1000baseT/Full") into deduped, order-preserving PMD identity
// strings.
func ethtoolModesToPMD(modes []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, entry := range modes {
		base := entry
		if idx := strings.IndexByte(entry, '/'); idx >= 0 {
			base = entry[:idx]
		}
		pmd, ok := ethtoolToPMD[base]
		if !ok || seen[pmd] {
			continue
		}
		seen[pmd] = true
		out = append(out, "ieee802-ethernet-phy-type:pmd-type-"+pmd)
	}
	return out
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]bool, len(a))
	for _, s := range a {
		set[s] = true
	}
	for _, s := range b {
		if !set[s] {
			return false
		}
	}
	return true
}
