package iwmonitor

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/kernelkit/infix/src/yangerd/internal/nl80211"
	"github.com/kernelkit/infix/src/yangerd/internal/wpactrl"
)

func TestBuildWDSData(t *testing.T) {
	for _, tc := range []struct {
		name string
		ms   meshState
		want string
	}{
		{"unbound", meshState{iftype: "AP_VLAN"}, `{"connected":false}`},
		{"bound", meshState{iftype: "AP_VLAN", peers: []nl80211.Station{{
			MAC: "02:00:00:00:00:02", Signal: -42, HasSignal: true,
			RxBitrate: 1560, TxBitrate: 1733,
		}}}, `{"connected":true,"rx-speed":1560,"signal-strength":-42,"tx-speed":1733}`},
		{"bound, no rates yet", meshState{iftype: "AP_VLAN", peers: []nl80211.Station{{
			MAC: "02:00:00:00:00:02",
		}}}, `{"connected":true}`},
	} {
		raw, err := json.Marshal(buildWDSData(tc.ms))
		if err != nil {
			t.Fatalf("%s: marshal: %v", tc.name, err)
		}
		if string(raw) != tc.want {
			t.Errorf("%s: got %s, want %s", tc.name, raw, tc.want)
		}
	}
}

// A wds-link port has no control socket, its state is the kernel's.
func TestRefreshWDSPortPublishesWDSLink(t *testing.T) {
	m := New(slog.Default())
	m.meshState = func(string) meshState { return meshState{iftype: "AP_VLAN"} }

	var got string
	m.SetOnUpdate(func(_ string, raw json.RawMessage) { got = string(raw) })
	m.refreshInterface(context.Background(), "wds-test-none")

	if want := `{"wds-link":{"connected":false}}`; got != want {
		t.Errorf("published %s, want %s", got, want)
	}
}

// hostapd binds the station to its port without an nl80211 event for
// the port, the station events on the access point refresh the ports.
func TestAPStationEventsRefreshWDSPorts(t *testing.T) {
	m := New(slog.Default())
	m.meshState = func(ifname string) meshState {
		if ifname == "wds0" {
			return meshState{iftype: "AP_VLAN", peers: []nl80211.Station{{MAC: "02:00:00:00:00:02"}}}
		}
		return meshState{}
	}
	m.wdsPorts = func() []string { return []string{"wds0"} }

	published := map[string]string{}
	m.SetOnUpdate(func(ifname string, raw json.RawMessage) { published[ifname] = string(raw) })

	for _, line := range []string{
		"<3>AP-STA-CONNECTED 02:00:00:00:00:02",
		"<3>AP-STA-DISCONNECTED 02:00:00:00:00:02",
	} {
		ev, ok := wpactrl.ParseEvent(line)
		if !ok {
			t.Fatalf("ParseEvent(%q) failed", line)
		}
		delete(published, "wds0")
		m.handleAttachEvent(context.Background(), "wifi-test-none", ev)
		if want := `{"wds-link":{"connected":true}}`; published["wds0"] != want {
			t.Errorf("%s: wds0 published %q, want %s", ev.Name, published["wds0"], want)
		}
	}
}
