package iwmonitor

import (
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/nl80211"
)

// liveBudget is how long a GET waits for the kernel station tables.
// Most drivers answer from memory, some ask the firmware for every
// station, and a slow firmware must not hold up the interface list.
const liveBudget = 200 * time.Millisecond

// liveDeadline is when a station dump is given up on.
const liveDeadline = 10 * time.Second

// LiveStations returns the leaves of each entry in the kernel station
// table of the interfaces, by interface and MAC address.  Signal,
// rates and counters change without any event, so the interface
// provider asks on every GET.  All interfaces are asked at once and
// share one liveBudget.  A dump that does not finish in time is left
// to run and the previous answer returned, the late one is there for
// the next GET.
func (m *IWMonitor) LiveStations(ifnames []string) map[string]map[string]map[string]any {
	var pending []<-chan struct{}

	m.liveMu.Lock()
	for _, ifname := range ifnames {
		if m.liveBusy[ifname] {
			continue
		}
		m.liveBusy[ifname] = true
		done := make(chan struct{})
		go m.dumpStations(ifname, m.liveGen[ifname], done)
		pending = append(pending, done)
	}
	m.liveMu.Unlock()

	timer := time.NewTimer(liveBudget)
	defer timer.Stop()
wait:
	for _, done := range pending {
		select {
		case <-done:
		case <-timer.C:
			break wait
		}
	}

	m.liveMu.Lock()
	defer m.liveMu.Unlock()
	live := make(map[string]map[string]map[string]any, len(ifnames))
	for _, ifname := range ifnames {
		if leaves := m.live[ifname]; leaves != nil {
			live[ifname] = leaves
		}
	}
	return live
}

// dropLive forgets the last station table of an interface.  Called
// when its stations change, so an answer from before a station left
// and came back is never laid over the new connection.
func (m *IWMonitor) dropLive(ifname string) {
	m.liveMu.Lock()
	delete(m.live, ifname)
	m.liveGen[ifname]++
	m.liveMu.Unlock()
}

func (m *IWMonitor) dumpStations(ifname string, gen uint64, done chan<- struct{}) {
	var leaves map[string]map[string]any
	stas, err := m.stations(ifname)
	if len(stas) > 0 {
		leaves = make(map[string]map[string]any, len(stas))
		for _, st := range stas {
			leaves[strings.ToLower(st.MAC)] = stationLeaves(st)
		}
	}

	m.liveMu.Lock()
	if leaves == nil || gen != m.liveGen[ifname] {
		delete(m.live, ifname)
	} else {
		m.live[ifname] = leaves
	}
	failed := m.liveFailed[ifname]
	if err != nil {
		m.liveFailed[ifname] = true
	} else {
		delete(m.liveFailed, ifname)
	}
	delete(m.liveBusy, ifname)
	m.liveMu.Unlock()
	close(done)

	if err != nil && !failed && m.log != nil {
		m.log.Warn("nl80211: station dump failed", "iface", ifname, "err", err)
	}
}

func stationLeaves(st nl80211.Station) map[string]any {
	leaves := map[string]any{
		"connected-time": st.ConnectedTime,
		"rx-packets":     strconv.FormatUint(st.RxPackets, 10),
		"tx-packets":     strconv.FormatUint(st.TxPackets, 10),
		"rx-bytes":       strconv.FormatUint(st.RxBytes, 10),
		"tx-bytes":       strconv.FormatUint(st.TxBytes, 10),
	}
	if st.HasSignal {
		leaves["signal-strength"] = int16(st.Signal)
	}
	if st.RxBitrate > 0 {
		leaves["rx-speed"] = st.RxBitrate
	}
	if st.TxBitrate > 0 {
		leaves["tx-speed"] = st.TxBitrate
	}
	if st.HasTxStatus {
		leaves["tx-retries"] = st.TxRetries
		leaves["tx-failed"] = st.TxFailed
	}
	return leaves
}

func kernelStations(ifname string) ([]nl80211.Station, error) {
	iface, err := net.InterfaceByName(ifname)
	if err != nil {
		return nil, nil
	}
	client, err := nl80211.Dial()
	if err != nil {
		return nil, err
	}
	defer client.Close()

	if err := client.SetDeadline(time.Now().Add(liveDeadline)); err != nil {
		return nil, err
	}
	return client.Stations(iface.Index)
}
