package iwmonitor

import (
	"errors"
	"testing"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/nl80211"
)

func TestLiveStations(t *testing.T) {
	m := New(nil)
	m.stations = func(ifname string) ([]nl80211.Station, error) {
		if ifname != "wifi0" {
			return nil, nil
		}
		return []nl80211.Station{{
			MAC: "02:00:00:00:00:02", Signal: -47, HasSignal: true, ConnectedTime: 60,
			RxBytes: 5000000000, TxBitrate: 8667, TxRetries: 7, TxFailed: 1, HasTxStatus: true,
		}, {
			MAC: "02:00:00:00:00:03",
		}}, nil
	}

	all := m.LiveStations([]string{"wifi0", "wifi1"})
	live := all["wifi0"]
	sta := live["02:00:00:00:00:02"]
	if sta["signal-strength"] != int16(-47) || sta["rx-bytes"] != "5000000000" ||
		sta["tx-speed"] != uint32(8667) || sta["tx-retries"] != uint32(7) || sta["tx-failed"] != uint32(1) {
		t.Errorf("station leaves = %v", sta)
	}
	if _, ok := sta["rx-speed"]; ok {
		t.Errorf("no receive rate yet, no rx-speed: %v", sta)
	}

	bare := live["02:00:00:00:00:03"]
	for _, leaf := range []string{"signal-strength", "tx-retries", "tx-failed"} {
		if _, ok := bare[leaf]; ok {
			t.Errorf("a driver that does not report %s must not get one: %v", leaf, bare)
		}
	}

	if all["wifi1"] != nil {
		t.Error("an interface without stations has nothing live")
	}
}

// A firmware that is slow to answer must not hold up the GET: the
// previous answer is returned, and the late one serves the next GET.
func TestLiveStationsSlowKernel(t *testing.T) {
	m := New(nil)
	release := make(chan struct{})
	signal := int8(-50)
	m.stations = func(string) ([]nl80211.Station, error) {
		<-release
		return []nl80211.Station{{MAC: "02:00:00:00:00:02", Signal: signal, HasSignal: true}}, nil
	}

	// Two slow interfaces share the budget, they do not add up.
	start := time.Now()
	if live := m.LiveStations([]string{"wifi0", "wifi1"}); len(live) != 0 {
		t.Errorf("nothing known yet, got %v", live)
	}
	if waited := time.Since(start); waited < liveBudget || waited >= 2*liveBudget {
		t.Errorf("waited %v, the budget is %v", waited, liveBudget)
	}

	// The dump is still under way, a second GET does not start another
	// and does not wait.
	start = time.Now()
	m.LiveStations([]string{"wifi0", "wifi1"})
	if waited := time.Since(start); waited >= liveBudget {
		t.Errorf("waited %v behind a dump already under way", waited)
	}

	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for {
		m.liveMu.Lock()
		busy := m.liveBusy["wifi0"] || m.liveBusy["wifi1"]
		m.liveMu.Unlock()
		if !busy {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the late dump never finished")
		}
		time.Sleep(time.Millisecond)
	}

	if live := m.LiveStations([]string{"wifi0"}); live["wifi0"]["02:00:00:00:00:02"]["signal-strength"] != int16(-50) {
		t.Errorf("the late answer was lost: %v", live)
	}
}

// A dump that fails drops the old answer and frees the interface for
// the next GET.
func TestLiveStationsFailedDump(t *testing.T) {
	m := New(nil)
	var err error
	m.stations = func(string) ([]nl80211.Station, error) {
		if err != nil {
			return nil, err
		}
		return []nl80211.Station{{MAC: "02:00:00:00:00:02", Signal: -50, HasSignal: true}}, nil
	}

	if live := m.LiveStations([]string{"wifi0"}); live["wifi0"] == nil {
		t.Fatal("no answer from a working dump")
	}

	err = errors.New("i/o timeout")
	if live := m.LiveStations([]string{"wifi0"}); live["wifi0"] != nil {
		t.Errorf("the answer from before the failure was kept: %v", live)
	}
	if !m.liveFailed["wifi0"] || m.liveBusy["wifi0"] {
		t.Errorf("failed = %v, busy = %v after a failed dump", m.liveFailed["wifi0"], m.liveBusy["wifi0"])
	}

	err = nil
	if live := m.LiveStations([]string{"wifi0"}); live["wifi0"] == nil || m.liveFailed["wifi0"] {
		t.Errorf("no recovery after the dump works again: %v", live)
	}
}

// An answer that was asked for before the stations changed is not
// kept, it may describe a connection that is gone.
func TestLiveStationsDroppedOnChange(t *testing.T) {
	m := New(nil)
	release := make(chan struct{})
	m.stations = func(string) ([]nl80211.Station, error) {
		<-release
		return []nl80211.Station{{MAC: "02:00:00:00:00:02", ConnectedTime: 5000}}, nil
	}

	m.LiveStations([]string{"wifi0"})
	m.dropLive("wifi0")
	close(release)

	deadline := time.Now().Add(5 * time.Second)
	for {
		m.liveMu.Lock()
		busy, live := m.liveBusy["wifi0"], m.live["wifi0"]
		m.liveMu.Unlock()
		if !busy {
			if live != nil {
				t.Errorf("an answer from before the change was kept: %v", live)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the dump never finished")
		}
		time.Sleep(time.Millisecond)
	}

	if live := m.LiveStations([]string{"wifi0"}); live["wifi0"]["02:00:00:00:00:02"]["connected-time"] != uint32(5000) {
		t.Errorf("no fresh answer after the change: %v", live)
	}
}
