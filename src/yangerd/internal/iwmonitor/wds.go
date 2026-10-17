package iwmonitor

import "context"

// buildWDSData assembles the wds-link container of an AP_VLAN port.
// The station bound to the port is the only entry in its station
// table, so that is both the connected flag and the link quality.
func buildWDSData(ms meshState) map[string]any {
	if len(ms.peers) == 0 {
		return map[string]any{"connected": false}
	}

	sta := ms.peers[0]
	wds := map[string]any{"connected": true}
	if sta.HasSignal {
		wds["signal-strength"] = int16(sta.Signal)
	}
	if sta.RxBitrate > 0 {
		wds["rx-speed"] = sta.RxBitrate
	}
	if sta.TxBitrate > 0 {
		wds["tx-speed"] = sta.TxBitrate
	}
	return wds
}

// refreshWDSPorts re-reads every wds-link port.  hostapd binds a
// station to its port without any nl80211 event for the port, the
// AP-STA-CONNECTED on the access point is the cue.
func (m *IWMonitor) refreshWDSPorts(ctx context.Context) {
	for _, port := range m.wdsPorts() {
		m.refreshInterface(ctx, port)
	}
}

// isWDSPort tells whether ifname is a wds-link port (AP_VLAN).
func (m *IWMonitor) isWDSPort(ifname string) bool {
	return m.meshState(ifname).iftype == "AP_VLAN"
}
