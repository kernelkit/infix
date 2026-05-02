package iwmonitor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/wpactrl"
	"github.com/mdlayher/genetlink"
	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

const (
	reconnectInitial = 500 * time.Millisecond
	reconnectMax     = 30 * time.Second
	reconnectFactor  = 2.0
	queryTimeout     = 5 * time.Second
)

// IWEvent is retained for ParseIWEvent compatibility (used in tests).
type IWEvent struct {
	Timestamp float64
	Interface string
	Phy       string
	Type      string
	Addr      string
}

type IWMonitor struct {
	log         *slog.Logger
	onUpdate    func(ifname string, data json.RawMessage)
	onPhyChange func()

	mu       sync.Mutex
	attached map[string]context.CancelFunc
}

func New(log *slog.Logger) *IWMonitor {
	return &IWMonitor{
		log:      log,
		attached: make(map[string]context.CancelFunc),
	}
}

func (m *IWMonitor) SetOnUpdate(fn func(string, json.RawMessage)) {
	m.onUpdate = fn
}

func (m *IWMonitor) SetOnPhyChange(fn func()) {
	m.onPhyChange = fn
}

func (m *IWMonitor) Run(ctx context.Context) error {
	conn, family, err := m.dialNL80211()
	if err != nil {
		return fmt.Errorf("nl80211 setup: %w", err)
	}
	defer conn.Close()

	m.refreshAllInterfaces(ctx)

	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	for {
		msgs, _, err := conn.Receive()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("nl80211 receive: %w", err)
		}

		for _, msg := range msgs {
			m.handleNL80211(ctx, msg, family)
		}
	}
}

func (m *IWMonitor) dialNL80211() (*genetlink.Conn, genetlink.Family, error) {
	conn, err := genetlink.Dial(nil)
	if err != nil {
		return nil, genetlink.Family{}, fmt.Errorf("dial genetlink: %w", err)
	}

	family, err := conn.GetFamily(unix.NL80211_GENL_NAME)
	if err != nil {
		conn.Close()
		return nil, genetlink.Family{}, fmt.Errorf("resolve nl80211: %w", err)
	}

	groups := map[string]bool{
		unix.NL80211_MULTICAST_GROUP_MLME:   true,
		unix.NL80211_MULTICAST_GROUP_REG:    true,
		unix.NL80211_MULTICAST_GROUP_CONFIG: true,
	}
	for _, g := range family.Groups {
		if groups[g.Name] {
			if err := conn.JoinGroup(g.ID); err != nil {
				conn.Close()
				return nil, genetlink.Family{}, fmt.Errorf("join %s: %w", g.Name, err)
			}
			m.log.Info("nl80211: joined multicast group", "name", g.Name, "id", g.ID)
		}
	}

	return conn, family, nil
}

func (m *IWMonitor) handleNL80211(ctx context.Context, msg genetlink.Message, family genetlink.Family) {
	ifname := m.extractIfname(msg.Data)
	cmd := msg.Header.Command

	switch cmd {
	case unix.NL80211_CMD_NEW_STATION, unix.NL80211_CMD_DEL_STATION:
		if ifname != "" {
			m.log.Debug("nl80211: station event", "cmd", cmd, "iface", ifname)
			m.refreshInterface(ctx, ifname)
		}
	case unix.NL80211_CMD_CONNECT:
		if ifname != "" {
			m.log.Debug("nl80211: connect", "iface", ifname)
			m.refreshInterface(ctx, ifname)
		}
	case unix.NL80211_CMD_DISCONNECT:
		if ifname != "" {
			m.log.Debug("nl80211: disconnect", "iface", ifname)
			m.publishWifi(ifname, nil)
		}
	case unix.NL80211_CMD_REG_CHANGE:
		m.log.Debug("nl80211: reg_change")
		m.refreshAllInterfaces(ctx)
	case unix.NL80211_CMD_NEW_INTERFACE:
		if ifname != "" {
			m.log.Info("nl80211: new interface", "iface", ifname)
			m.startAttach(ctx, ifname)
			m.refreshInterface(ctx, ifname)
		}
	case unix.NL80211_CMD_DEL_INTERFACE:
		if ifname != "" {
			m.log.Info("nl80211: del interface", "iface", ifname)
			m.stopAttach(ifname)
			m.publishWifi(ifname, nil)
		}
	case unix.NL80211_CMD_NEW_WIPHY, unix.NL80211_CMD_DEL_WIPHY:
		m.log.Info("nl80211: phy change", "cmd", cmd)
		if m.onPhyChange != nil {
			m.onPhyChange()
		}
	}
}

func (m *IWMonitor) extractIfname(data []byte) string {
	ad, err := netlink.NewAttributeDecoder(data)
	if err != nil {
		return ""
	}
	for ad.Next() {
		if ad.Type() == unix.NL80211_ATTR_IFINDEX {
			iface, err := net.InterfaceByIndex(int(ad.Uint32()))
			if err != nil {
				return ""
			}
			return iface.Name
		}
	}
	return ""
}

func (m *IWMonitor) startAttach(ctx context.Context, ifname string) {
	m.mu.Lock()
	if _, exists := m.attached[ifname]; exists {
		m.mu.Unlock()
		return
	}
	attachCtx, cancel := context.WithCancel(ctx)
	m.attached[ifname] = cancel
	m.mu.Unlock()

	go m.attachLoop(attachCtx, ifname)
}

func (m *IWMonitor) stopAttach(ifname string) {
	m.mu.Lock()
	if cancel, ok := m.attached[ifname]; ok {
		cancel()
		delete(m.attached, ifname)
	}
	m.mu.Unlock()
}

func (m *IWMonitor) attachLoop(ctx context.Context, ifname string) {
	delay := reconnectInitial

	for {
		if ctx.Err() != nil {
			return
		}

		socks := wpactrl.ScanSockets()
		si, ok := socks[ifname]
		if !ok {
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			delay = nextDelay(delay)
			continue
		}

		ac, err := wpactrl.Attach(si.Path)
		if err != nil {
			m.log.Debug("attach failed", "iface", ifname, "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			delay = nextDelay(delay)
			continue
		}

		delay = reconnectInitial
		m.log.Info("attached to control socket", "iface", ifname, "daemon", si.Daemon)
		m.refreshInterface(ctx, ifname)

		ac.SetHandler(func(ev wpactrl.Event) {
			m.handleAttachEvent(ctx, ifname, ev)
		})

		err = ac.Run(ctx)
		ac.Close()

		if ctx.Err() != nil {
			return
		}

		m.log.Warn("control socket lost", "iface", ifname, "err", err)
		m.publishWifi(ifname, nil)
	}
}

func (m *IWMonitor) handleAttachEvent(ctx context.Context, ifname string, ev wpactrl.Event) {
	switch {
	case ev.Name == "AP-STA-CONNECTED" || ev.Name == "AP-STA-DISCONNECTED":
		m.refreshInterface(ctx, ifname)
	case ev.Name == "CTRL-EVENT-CONNECTED":
		m.refreshInterface(ctx, ifname)
	case ev.Name == "CTRL-EVENT-DISCONNECTED":
		m.publishWifi(ifname, nil)
	case ev.Name == "CTRL-EVENT-SCAN-RESULTS":
		m.refreshInterface(ctx, ifname)
	case ev.Name == "CTRL-EVENT-SIGNAL-CHANGE":
		m.handleSignalChange(ifname, ev.Data)
	case ev.Name == "CTRL-EVENT-TERMINATING":
		// Daemon is shutting down; the read loop will get an error next
	}
}

func (m *IWMonitor) handleSignalChange(ifname string, data string) {
	// TODO(lazzer): update signal in-place without full rebuild
	// For now, this is a no-op; signal is read during refreshInterface.
	_ = ifname
	_ = data
}

func (m *IWMonitor) refreshInterface(ctx context.Context, iface string) {
	wifi := m.buildWifiData(ctx, iface)
	m.publishWifi(iface, wifi)
}

func (m *IWMonitor) buildWifiData(ctx context.Context, iface string) map[string]any {
	socks := wpactrl.ScanSockets()
	si, ok := socks[iface]

	var status map[string]string
	if ok {
		conn, err := wpactrl.Dial(si.Path)
		if err == nil {
			status, _ = conn.Status()
			conn.Close()
		}
	}

	mode := m.detectMode(si, status)
	result := make(map[string]any)

	if mode == "ap" {
		result["access-point"] = m.buildAPData(ctx, iface, si, status)
	} else {
		result["station"] = m.buildStationData(ctx, iface, si, status)
	}

	return result
}

func (m *IWMonitor) detectMode(si wpactrl.SocketInfo, status map[string]string) string {
	if si.Daemon == "hostapd" {
		return "ap"
	}
	return "station"
}

func (m *IWMonitor) buildAPData(ctx context.Context, iface string, si wpactrl.SocketInfo, status map[string]string) map[string]any {
	ap := make(map[string]any)

	ssid := resolveSSID(iface, si, status)
	if ssid != "" {
		ap["ssid"] = ssid
	}

	if si.Daemon == "hostapd" {
		conn, err := wpactrl.Dial(si.Path)
		if err != nil {
			m.log.Warn("hostapd dial for stations", "iface", iface, "err", err)
		} else {
			stas, err := conn.AllStations()
			conn.Close()
			if err != nil {
				m.log.Warn("hostapd AllStations", "iface", iface, "err", err)
			}
			if err == nil {
				stas = filterAuthorized(stas)
				if len(stas) > 0 {
					ap["stations"] = m.formatStations(stas)
				}
			}
		}
	}

	return ap
}

func (m *IWMonitor) buildStationData(ctx context.Context, iface string, si wpactrl.SocketInfo, status map[string]string) map[string]any {
	sta := make(map[string]any)

	ssid := resolveSSID(iface, si, status)
	if ssid != "" {
		sta["ssid"] = ssid
	}

	if si.Daemon == "wpa_supplicant" {
		conn, err := wpactrl.Dial(si.Path)
		if err == nil {
			poll, err := conn.SignalPoll()
			if err == nil {
				if v, ok := poll["RSSI"]; ok {
					if sig, err := strconv.Atoi(v); err == nil {
						sta["signal-strength"] = sig
					}
				}
				if v, ok := poll["LINKSPEED"]; ok {
					if speed, err := strconv.ParseUint(v, 10, 32); err == nil {
						sta["tx-speed"] = uint32(speed * 10)
					}
				}
			}
			results, err := conn.ScanResults()
			conn.Close()
			if err == nil && len(results) > 0 {
				sta["scan-results"] = formatScanResults(results)
			}
		}
	}

	return sta
}

func (m *IWMonitor) formatStations(stas []map[string]string) map[string]any {
	type stationEntry struct {
		MAC           string `json:"mac-address"`
		Signal        int16  `json:"signal-strength,omitempty"`
		ConnectedTime uint32 `json:"connected-time,omitempty"`
		RxPackets     string `json:"rx-packets,omitempty"`
		TxPackets     string `json:"tx-packets,omitempty"`
		RxBytes       string `json:"rx-bytes,omitempty"`
		TxBytes       string `json:"tx-bytes,omitempty"`
		RxSpeed       uint32 `json:"rx-speed,omitempty"`
		TxSpeed       uint32 `json:"tx-speed,omitempty"`
	}

	var out []stationEntry
	for _, st := range stas {
		s := stationEntry{MAC: st["addr"]}
		if v := st["signal"]; v != "" {
			if sig, err := strconv.ParseInt(v, 10, 16); err == nil {
				s.Signal = int16(sig)
			}
		}
		if v := st["connected_time"]; v != "" {
			if ct, err := strconv.ParseUint(v, 10, 32); err == nil {
				s.ConnectedTime = uint32(ct)
			}
		}
		if v := st["rx_packets"]; v != "" {
			s.RxPackets = v
		}
		if v := st["tx_packets"]; v != "" {
			s.TxPackets = v
		}
		if v := st["rx_bytes"]; v != "" {
			s.RxBytes = v
		}
		if v := st["tx_bytes"]; v != "" {
			s.TxBytes = v
		}
		if v := st["rx_rate_info"]; v != "" {
			if speed := parseBitrate(v); speed > 0 {
				s.RxSpeed = speed
			}
		}
		if v := st["tx_rate_info"]; v != "" {
			if speed := parseBitrate(v); speed > 0 {
				s.TxSpeed = speed
			}
		}
		out = append(out, s)
	}
	return map[string]any{"station": out}
}

func (m *IWMonitor) publishWifi(iface string, data map[string]any) {
	if m.onUpdate == nil {
		return
	}

	if data == nil {
		m.onUpdate(iface, json.RawMessage(`{}`))
		return
	}

	raw, err := json.Marshal(data)
	if err != nil {
		m.log.Warn("marshal wifi data", "iface", iface, "err", err)
		return
	}
	m.onUpdate(iface, json.RawMessage(raw))
}

func (m *IWMonitor) refreshAllInterfaces(ctx context.Context) {
	for ifname := range wpactrl.ScanSockets() {
		m.startAttach(ctx, ifname)
		m.refreshInterface(ctx, ifname)
	}
}

func nextDelay(current time.Duration) time.Duration {
	d := time.Duration(math.Min(float64(current)*reconnectFactor, float64(reconnectMax)))
	return d
}

func filterAuthorized(stas []map[string]string) []map[string]string {
	var out []map[string]string
	for _, st := range stas {
		if strings.Contains(st["flags"], "AUTHORIZED") {
			out = append(out, st)
		}
	}
	return out
}
