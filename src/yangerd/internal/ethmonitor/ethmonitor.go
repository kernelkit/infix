// Package ethmonitor subscribes to ethtool genetlink notifications and
// keeps per-interface ethernet settings updated via a callback.
package ethmonitor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"

	"github.com/mdlayher/ethtool"
	"github.com/mdlayher/genetlink"
	"github.com/mdlayher/netlink"
)

const (
	// ETHTOOL_MSG_LINKINFO_NTF is the ethtool generic netlink notification
	// command for link info changes.
	ETHTOOL_MSG_LINKINFO_NTF = 28
	// ETHTOOL_MSG_LINKMODES_NTF is the ethtool generic netlink notification
	// command for link mode changes.
	ETHTOOL_MSG_LINKMODES_NTF = 29

	ethtoolFamilyName       = "ethtool"
	ethtoolMonitorGroupName = "monitor"

	nlaHeaderIfindex = 1
)

// EthMonitor listens for ethtool genetlink monitor events and updates
// interface ethernet operational state via a callback.
type EthMonitor struct {
	conn     *genetlink.Conn
	family   genetlink.Family
	groupID  uint32
	etClient *ethtool.Client
	log      *slog.Logger
	onUpdate func(ifname string, data json.RawMessage)
}

// New creates an EthMonitor, resolves the ethtool genetlink family,
// and joins its "monitor" multicast group.
func New(log *slog.Logger) (*EthMonitor, error) {
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

	etClient, err := ethtool.New()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("create ethtool client: %w", err)
	}

	return &EthMonitor{
		conn:     conn,
		family:   family,
		groupID:  groupID,
		etClient: etClient,
		log:      log,
	}, nil
}

// SetOnUpdate sets the callback invoked when ethernet data changes.
func (m *EthMonitor) SetOnUpdate(fn func(string, json.RawMessage)) {
	m.onUpdate = fn
}

// Run starts the ethtool genetlink receive loop and updates interface
// ethernet settings when link info or link mode notifications are seen.
func (m *EthMonitor) Run(ctx context.Context) error {
	defer func() {
		if err := m.etClient.Close(); err != nil {
			m.log.Warn("ethmonitor: close ethtool client", "err", err)
		}
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

func (m *EthMonitor) refreshEthernetSettings(ifname string) {
	iface, err := net.InterfaceByName(ifname)
	if err != nil {
		m.log.Warn("ethmonitor: lookup interface", "ifname", ifname, "err", err)
		return
	}

	ethIface := ethtool.Interface{Index: iface.Index, Name: iface.Name}

	if _, err := m.etClient.LinkInfo(ethIface); err != nil {
		m.log.Warn("ethmonitor: query link info", "ifname", ifname, "err", err)
		return
	}

	mode, err := m.etClient.LinkMode(ethIface)
	if err != nil {
		m.log.Warn("ethmonitor: query link mode", "ifname", ifname, "err", err)
		return
	}

	eth := map[string]any{
		"speed":  speedString(mode.SpeedMegabits),
		"duplex": duplexString(mode.Duplex),
		"auto-negotiation": map[string]any{
			"enable": mode.Autoneg == ethtool.AutonegOn,
		},
	}

	raw, err := json.Marshal(eth)
	if err != nil {
		m.log.Warn("ethmonitor: marshal ethernet settings", "ifname", ifname, "err", err)
		return
	}

	if m.onUpdate != nil {
		m.onUpdate(ifname, json.RawMessage(raw))
	}
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

func duplexString(d ethtool.Duplex) string {
	switch d {
	case ethtool.Full:
		return "full"
	case ethtool.Half:
		return "half"
	default:
		return "unknown"
	}
}

// speedString converts a speed in megabits to a decimal64 string in
// Gb/s with 3 fraction digits, matching the ieee802-ethernet-interface
// YANG model's eth-if-speed-type (decimal64, fraction-digits 3, units Gb/s).
func speedString(megabits int) string {
	return fmt.Sprintf("%.3f", float64(megabits)/1000.0)
}
