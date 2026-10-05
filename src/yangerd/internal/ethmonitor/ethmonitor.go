// Package ethmonitor subscribes to ethtool genetlink notifications and
// keeps per-interface ethernet settings updated via a callback.
//
// Data is fetched by shelling out to `ethtool --json <ifname>` (matching
// the Python yanger approach) while genetlink provides reactive change
// notifications.  The exec runs on a worker fed by a set of pending
// interface names, so a burst of link events on many ports costs one
// ethtool per port and never blocks the caller.
package ethmonitor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"

	"github.com/kernelkit/infix/src/yangerd/internal/backoff"
	"github.com/kernelkit/infix/src/yangerd/internal/collector"
	"github.com/mdlayher/genetlink"
	"github.com/mdlayher/netlink"
)

// Kernel-to-user ethtool message types, from
// include/uapi/linux/ethtool_netlink_generated.h.
const (
	ETHTOOL_MSG_LINKINFO_NTF  = 3
	ETHTOOL_MSG_LINKMODES_NTF = 5

	ethtoolFamilyName       = "ethtool"
	ethtoolMonitorGroupName = "monitor"

	nlaHeaderIfindex = 1

	ethtoolSpeedUnknown = (1 << 32) - 1
)

// EthMonitor listens for ethtool genetlink monitor events and updates
// interface ethernet operational state via a callback.
type EthMonitor struct {
	cmd      collector.CommandRunner
	log      *slog.Logger
	onUpdate func(ifname string, data json.RawMessage)

	// ifaceName resolves a kernel ifindex; overridable in tests.
	ifaceName func(index int) (string, error)

	mu      sync.Mutex
	pending map[string]struct{}
	kick    chan struct{}
}

// New creates an EthMonitor.  The genetlink socket is opened by Run.
func New(log *slog.Logger, cmd collector.CommandRunner) *EthMonitor {
	return &EthMonitor{
		cmd:       cmd,
		log:       log,
		ifaceName: ifNameByIndex,
		pending:   make(map[string]struct{}),
		kick:      make(chan struct{}, 1),
	}
}

// SetOnUpdate sets the callback invoked when ethernet data changes.
func (m *EthMonitor) SetOnUpdate(fn func(string, json.RawMessage)) {
	m.onUpdate = fn
}

// Run serves refresh requests and the ethtool notification stream until
// ctx is cancelled, reconnecting with backoff if the socket fails.
func (m *EthMonitor) Run(ctx context.Context) error {
	go m.worker(ctx)
	return backoff.Retry(ctx, m.log, "ethmonitor", m.receive)
}

func (m *EthMonitor) receive(ctx context.Context) error {
	conn, err := genetlink.Dial(nil)
	if err != nil {
		return fmt.Errorf("dial genetlink: %w", err)
	}
	defer conn.Close()

	family, err := conn.GetFamily(ethtoolFamilyName)
	if err != nil {
		return fmt.Errorf("resolve %q genetlink family: %w", ethtoolFamilyName, err)
	}
	var groupID uint32
	for _, g := range family.Groups {
		if g.Name == ethtoolMonitorGroupName {
			groupID = g.ID
			break
		}
	}
	if groupID == 0 {
		return fmt.Errorf("multicast group %q not found in family %q", ethtoolMonitorGroupName, ethtoolFamilyName)
	}
	if err := conn.JoinGroup(groupID); err != nil {
		return fmt.Errorf("join ethtool monitor group %d: %w", groupID, err)
	}

	// Receive has no deadline; closing the socket is what ends it.
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()

	for {
		msgs, _, err := conn.Receive()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("receive ethtool genetlink message: %w", err)
		}
		for _, msg := range msgs {
			m.dispatch(msg)
		}
	}
}

// dispatch queues a refresh for the interface a link notification is
// about.
func (m *EthMonitor) dispatch(msg genetlink.Message) {
	switch msg.Header.Command {
	case ETHTOOL_MSG_LINKINFO_NTF, ETHTOOL_MSG_LINKMODES_NTF:
	default:
		return
	}
	index, err := headerIfindex(msg.Data)
	if err != nil {
		m.log.Warn("ethmonitor: decode notification", "err", err)
		return
	}
	ifname, err := m.ifaceName(index)
	if err != nil {
		m.log.Debug("ethmonitor: notification for unknown ifindex", "index", index, "err", err)
		return
	}
	m.RefreshInterface(ifname)
}

// RefreshInterface queues a refresh of the ethernet settings for ifname.
// Called by the notification stream and by nlmonitor on link events;
// repeated requests before the worker gets to them collapse into one.
func (m *EthMonitor) RefreshInterface(ifname string) {
	m.mu.Lock()
	m.pending[ifname] = struct{}{}
	m.mu.Unlock()

	select {
	case m.kick <- struct{}{}:
	default:
	}
}

func (m *EthMonitor) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.kick:
		}
		for {
			ifname, ok := m.next()
			if !ok {
				break
			}
			m.refreshEthernetSettings(ctx, ifname)
		}
	}
}

func (m *EthMonitor) next() (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for ifname := range m.pending {
		delete(m.pending, ifname)
		return ifname, true
	}
	return "", false
}

// ethtoolJSON represents the relevant fields from `ethtool --json <ifname>`.
type ethtoolJSON struct {
	// Speed must be a 64-bit type: ethtool reports unknown speed as
	// 0xFFFFFFFF, which overflows int on 32-bit targets (arm).
	Speed               int64    `json:"speed"`
	Duplex              string   `json:"duplex"`
	Port                string   `json:"port"`
	AutoNegotiation     bool     `json:"auto-negotiation"`
	SupportedLinkModes  []string `json:"supported-link-modes"`
	AdvertisedLinkModes []string `json:"advertised-link-modes"`
}

func (m *EthMonitor) refreshEthernetSettings(ctx context.Context, ifname string) {
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

// headerIfindex returns the ifindex from the request header nest that
// every ethtool notification carries.
func headerIfindex(data []byte) (int, error) {
	ad, err := netlink.NewAttributeDecoder(data)
	if err != nil {
		return 0, fmt.Errorf("new decoder: %w", err)
	}

	for ad.Next() {
		nested, err := netlink.NewAttributeDecoder(ad.Bytes())
		if err != nil {
			continue
		}

		for nested.Next() {
			if nested.Type() == nlaHeaderIfindex {
				return int(nested.Uint32()), nil
			}
		}
		if err := nested.Err(); err != nil {
			return 0, fmt.Errorf("decode nested attrs: %w", err)
		}
	}

	if err := ad.Err(); err != nil {
		return 0, fmt.Errorf("decode attrs: %w", err)
	}

	return 0, fmt.Errorf("header ifindex attribute not found")
}

func ifNameByIndex(index int) (string, error) {
	iface, err := net.InterfaceByIndex(index)
	if err != nil {
		return "", err
	}
	return iface.Name, nil
}

// linkModeKey is the lookup key for phy-type/pmd-type mapping.
type linkModeKey struct {
	Port      string
	SpeedMbps int64
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
	{"Twisted Pair", 10, "full"}:             {"10BASE-T", "10BASE-T"},
	{"Twisted Pair", 10, "half"}:             {"10BASE-T", "10BASE-T"},
	{"Twisted Pair", 100, "full"}:            {"100BASE-X", "100BASE-TX"},
	{"Twisted Pair", 100, "half"}:            {"100BASE-X", "100BASE-TX"},
	{"Twisted Pair", 1000, "full"}:           {"1000BASE-T", "1000BASE-T"},
	{"Twisted Pair", 1000, "half"}:           {"1000BASE-T", "1000BASE-T"},
	{"Twisted Pair", 2500, "full"}:           {"2.5GBASE-T", "2.5GBASE-T"},
	{"Twisted Pair", 5000, "full"}:           {"5GBASE-T", "5GBASE-T"},
	{"Twisted Pair", 10000, "full"}:          {"10GBASE-T", "10GBASE-T"},
	{"Twisted Pair", 25000, "full"}:          {"25GBASE-T", "25GBASE-T"},
	{"Twisted Pair", 40000, "full"}:          {"40GBASE-T", "40GBASE-T"},
	{"MII", 10, "full"}:                      {"10BASE-T", "10BASE-T"},
	{"MII", 10, "half"}:                      {"10BASE-T", "10BASE-T"},
	{"MII", 100, "full"}:                     {"100BASE-X", "100BASE-TX"},
	{"MII", 100, "half"}:                     {"100BASE-X", "100BASE-TX"},
	{"FIBRE", 100, "full"}:                   {"100BASE-X", ""},
	{"FIBRE", 1000, "full"}:                  {"1000BASE-X", ""},
	{"FIBRE", 10000, "full"}:                 {"10GBASE-R", ""},
	{"FIBRE", 25000, "full"}:                 {"25GBASE-R", ""},
	{"FIBRE", 40000, "full"}:                 {"40GBASE-R", ""},
	{"FIBRE", 100000, "full"}:                {"100GBASE-R", ""},
	{"Direct Attach Copper", 10000, "full"}:  {"10GBASE-R", ""},
	{"Direct Attach Copper", 25000, "full"}:  {"25GBASE-R", "25GBASE-CR"},
	{"Direct Attach Copper", 40000, "full"}:  {"40GBASE-R", "40GBASE-CR4"},
	{"Direct Attach Copper", 100000, "full"}: {"100GBASE-R", "100GBASE-CR4"},
}

// ethtoolToPMD maps kernel link-mode base names to IEEE pmd-type
// identity suffixes. The kernel reports modes like "1000baseT/Full";
// we strip the "/Full" or "/Half" suffix before lookup.
var ethtoolToPMD = map[string]string{
	"10baseT":           "10BASE-T",
	"10baseT1L":         "10BASE-T1L",
	"100baseT":          "100BASE-TX",
	"100baseT1":         "100BASE-T1",
	"100baseFX":         "100BASE-FX",
	"1000baseT":         "1000BASE-T",
	"1000baseT1":        "1000BASE-T1",
	"1000baseX":         "1000BASE-LX",
	"1000baseKX":        "1000BASE-KX",
	"2500baseT":         "2.5GBASE-T",
	"2500baseX":         "2.5GBASE-X",
	"5000baseT":         "5GBASE-T",
	"10000baseT":        "10GBASE-T",
	"10000baseSR":       "10GBASE-SR",
	"10000baseLR":       "10GBASE-LR",
	"10000baseLRM":      "10GBASE-LRM",
	"10000baseER":       "10GBASE-ER",
	"10000baseKR":       "10GBASE-KR",
	"10000baseKX4":      "10GBASE-KX4",
	"25000baseCR":       "25GBASE-CR",
	"25000baseSR":       "25GBASE-SR",
	"25000baseKR":       "25GBASE-KR",
	"40000baseCR4":      "40GBASE-CR4",
	"40000baseSR4":      "40GBASE-SR4",
	"40000baseLR4":      "40GBASE-LR4",
	"40000baseKR4":      "40GBASE-KR4",
	"100000baseCR4":     "100GBASE-CR4",
	"100000baseSR4":     "100GBASE-SR4",
	"100000baseLR4_ER4": "100GBASE-LR4",
	"100000baseKR4":     "100GBASE-KR4",
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
