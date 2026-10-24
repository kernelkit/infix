package iwmonitor

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

// hostapd sets its radios up one after the other, so an access point can
// still be without a control socket when the monitor starts.  Its netdev
// already exists, so no nl80211 event follows: it must get an attach loop
// from the start, which finds the socket once hostapd creates it.
func TestStartAttachesInterfacesWithoutSocket(t *testing.T) {
	m := New(slog.Default())
	m.meshState = func(ifname string) meshState {
		if ifname == "wds0" {
			return meshState{iftype: "AP_VLAN"}
		}
		return meshState{iftype: "AP"}
	}
	m.wdsPorts = func() []string { return []string{"wds0"} }
	m.wifiIfaces = func() []string { return []string{"wifi2-ap", "wds0"} }
	m.SetOnUpdate(func(string, json.RawMessage) {})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.refreshAllInterfaces(ctx)

	m.mu.Lock()
	_, ap := m.attached["wifi2-ap"]
	_, wds := m.attached["wds0"]
	m.mu.Unlock()
	if !ap {
		t.Error("wifi2-ap has no attach loop although its netdev exists")
	}
	if wds {
		t.Error("wds0 got an attach loop, a wds-link port never has a socket")
	}
}
