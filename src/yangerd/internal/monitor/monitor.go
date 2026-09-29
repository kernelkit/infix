package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/iface"
	"github.com/kernelkit/infix/src/yangerd/internal/ipbatch"
	"github.com/kernelkit/infix/src/yangerd/internal/stpquery"
	"github.com/kernelkit/infix/src/yangerd/internal/tree"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
)

// treeKey is the single YANG module key where the complete
// ietf-interfaces document is stored.
const treeKey = "ietf-interfaces:interfaces"

// NLMonitor subscribes to netlink link/address/neighbor events and
// keeps interface operational data in the in-memory tree up to date.
// It is the central coordinator for all interface data — raw ip-json
// staging data is transformed via iface.Transform() and augmented
// with ethernet/wifi/bridge data before being stored as a single
// complete YANG document.
//
// Staged link and address rows are keyed by ifindex, which survives a
// rename; everything keyed by name is dropped when the name changes.
type NLMonitor struct {
	linkBatch  *ipbatch.Batch
	addrBatch  *ipbatch.Batch
	neighBatch *ipbatch.Batch
	brBatch    *ipbatch.Batch
	tree       *tree.Tree
	ethRefresh func(string)
	linkSet    func()
	log        *slog.Logger
	fc         iface.FileChecker

	// initDone is closed after the first initialDump completes.
	// initDoneOnce ensures it is only closed once across restarts.
	initDone     chan struct{}
	initDoneOnce sync.Once

	// redumpCh asks the event loop to run initialDump, e.g. after a
	// batch subprocess died.  Buffer of 1 so requests coalesce.
	redumpCh chan struct{}

	// staging holds raw ip-json data used as input to iface.Transform().
	// Protected by mu.
	mu        sync.Mutex
	links     json.RawMessage // ip -json -s -d link show (includes stats+details)
	addrs     json.RawMessage // ip -json -d addr show (details only, no stats)
	neighs    json.RawMessage // ip -json neigh show
	fdb       map[string]json.RawMessage
	mdb       map[string]json.RawMessage
	ethernet  map[string]json.RawMessage // ifname → ethtool JSON
	wifi      map[string]json.RawMessage // ifname → wifi JSON
	wireguard map[string]json.RawMessage // ifname → WireGuard peer-status JSON
	lastWG    string

	lastOperStatus map[string]string

	// stpMu guards lastSTP, a fingerprint of the most recent mstpd STP
	// query, letting the periodic poll rebuild only on actual change.
	stpMu   sync.Mutex
	lastSTP string
}

// coalesceDelay is how long the event loop collects further netlink
// events before rebuilding the document once for all of them.
const coalesceDelay = 20 * time.Millisecond

const (
	// redumpSettle lets a burst of failures and the batch restart that
	// caused them settle before one re-dump, redumpRetry follows a
	// failed one.
	redumpSettle = 200 * time.Millisecond
	redumpRetry  = 2 * time.Second
)

// New creates a netlink monitor backed by ip/bridge batch query workers.
// linkBatch should include -s -d flags; addrBatch should include -d only
// (no -s, which causes multi-line output for link commands).
func New(linkBatch, addrBatch, neighBatch, brBatch *ipbatch.Batch, t *tree.Tree, fc iface.FileChecker, log *slog.Logger) *NLMonitor {
	return &NLMonitor{
		linkBatch:      linkBatch,
		addrBatch:      addrBatch,
		neighBatch:     neighBatch,
		brBatch:        brBatch,
		tree:           t,
		fc:             fc,
		log:            log,
		initDone:       make(chan struct{}),
		redumpCh:       make(chan struct{}, 1),
		fdb:            make(map[string]json.RawMessage),
		mdb:            make(map[string]json.RawMessage),
		ethernet:       make(map[string]json.RawMessage),
		wifi:           make(map[string]json.RawMessage),
		wireguard:      make(map[string]json.RawMessage),
		lastOperStatus: make(map[string]string),
	}
}

// SetEthRefresh sets an optional callback used to refresh ethtool data
// when an ethernet interface sees a link event.
func (m *NLMonitor) SetEthRefresh(fn func(string)) {
	m.ethRefresh = fn
}

// SetLinkSetChange sets an optional callback run when an interface
// appears, goes away or is renamed.
func (m *NLMonitor) SetLinkSetChange(fn func()) {
	m.linkSet = fn
}

func (m *NLMonitor) linkSetChanged() {
	if m.linkSet != nil {
		m.linkSet()
	}
}

// WaitReady returns a channel that is closed after initialDump completes.
func (m *NLMonitor) WaitReady() <-chan struct{} {
	return m.initDone
}

// SetEthernetData updates the staged ethernet data for an interface
// and triggers a full rebuild of the YANG document.
func (m *NLMonitor) SetEthernetData(ifname string, data json.RawMessage) {
	m.mu.Lock()
	m.ethernet[ifname] = data
	m.mu.Unlock()
	m.rebuild()
}

// SetWifiData updates the staged wifi data for an interface
// and triggers a full rebuild of the YANG document.
func (m *NLMonitor) SetWifiData(ifname string, data json.RawMessage) {
	m.mu.Lock()
	m.wifi[ifname] = data
	m.mu.Unlock()
	m.rebuild()
}

// SetWireguardAll replaces the WireGuard peer-status data of every
// interface, so a tunnel that lost its last peer loses its status too.
// The document is rebuilt only when something changed.
func (m *NLMonitor) SetWireguardAll(data map[string]json.RawMessage) {
	var b strings.Builder
	writeSortedRaw(&b, "w", data)
	fp := b.String()

	m.mu.Lock()
	changed := fp != m.lastWG
	m.lastWG = fp
	m.wireguard = copyStringMap(data)
	if m.wireguard == nil {
		m.wireguard = make(map[string]json.RawMessage)
	}
	m.mu.Unlock()

	if changed {
		m.rebuild()
	}
}

// Links returns a copy of the current staged links data.
func (m *NLMonitor) Links() json.RawMessage {
	m.mu.Lock()
	cp := append(json.RawMessage{}, m.links...)
	m.mu.Unlock()
	return cp
}

// Run starts the netlink monitor loop and returns on context
// cancellation, channel closure, or subscription errors.  The netlink
// library ends a subscription on any receive error, ENOBUFS included,
// so every error means resubscribe and re-dump: return and let the
// caller restart us.
func (m *NLMonitor) Run(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan struct{})
	defer close(done)

	errorCallback := func(err error) {
		if err == nil {
			return
		}
		if strings.Contains(err.Error(), syscall.ENOBUFS.Error()) {
			m.log.Warn("netlink events dropped, resubscribing", "err", err)
		} else {
			m.log.Error("netlink subscription error", "err", err)
		}
		cancel()
	}

	linkCh := make(chan netlink.LinkUpdate, 64)
	addrCh := make(chan netlink.AddrUpdate, 64)
	neighCh := make(chan netlink.NeighUpdate, 64)
	mdbCh := make(chan struct{}, 32)

	if err := netlink.LinkSubscribeWithOptions(linkCh, done, netlink.LinkSubscribeOptions{
		ErrorCallback:          errorCallback,
		ReceiveBufferSize:      32 * 1024 * 1024,
		ReceiveBufferForceSize: true,
	}); err != nil {
		return fmt.Errorf("subscribe link updates: %w", err)
	}
	if err := netlink.AddrSubscribeWithOptions(addrCh, done, netlink.AddrSubscribeOptions{
		ErrorCallback:          errorCallback,
		ReceiveBufferSize:      32 * 1024 * 1024,
		ReceiveBufferForceSize: true,
	}); err != nil {
		return fmt.Errorf("subscribe addr updates: %w", err)
	}
	if err := netlink.NeighSubscribeWithOptions(neighCh, done, netlink.NeighSubscribeOptions{
		ErrorCallback:          errorCallback,
		ReceiveBufferSize:      32 * 1024 * 1024,
		ReceiveBufferForceSize: true,
	}); err != nil {
		return fmt.Errorf("subscribe neigh updates: %w", err)
	}
	if err := m.subscribeBridgeMDB(runCtx, mdbCh, errorCallback); err != nil {
		return fmt.Errorf("subscribe bridge mdb updates: %w", err)
	}

	if err := m.initialDump(); err != nil {
		m.log.Error("initial dump failed", "err", err)
	}
	m.initDoneOnce.Do(func() { close(m.initDone) })

	var flush, redump <-chan time.Time
	for {
		changed := false
		select {
		case <-runCtx.Done():
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return runCtx.Err()
		case lu, ok := <-linkCh:
			if !ok {
				return fmt.Errorf("link update channel closed")
			}
			changed = m.handleLinkUpdate(lu)
		case au, ok := <-addrCh:
			if !ok {
				return fmt.Errorf("addr update channel closed")
			}
			changed = m.handleAddrUpdate(au)
		case nu, ok := <-neighCh:
			if !ok {
				return fmt.Errorf("neigh update channel closed")
			}
			changed = m.handleNeighUpdate(nu)
		case _, ok := <-mdbCh:
			if !ok {
				return fmt.Errorf("bridge mdb update channel closed")
			}
			changed = m.handleMDBUpdate()
		case <-flush:
			flush = nil
			m.rebuild()
		case <-m.redumpCh:
			// Every event that hits a dead batch asks for this, so
			// requests while one is pending are the same request.
			if redump == nil {
				redump = time.After(redumpSettle)
			}
		case <-redump:
			redump = nil
			if !m.batchesAlive() {
				redump = time.After(redumpSettle)
				break
			}
			m.log.Warn("re-dumping all interfaces")
			if err := m.initialDump(); err != nil {
				m.log.Error("re-dump failed, will retry", "err", err, "in", redumpRetry)
				redump = time.After(redumpRetry)
			}
		}
		if changed && flush == nil {
			flush = time.After(coalesceDelay)
		}
	}
}

func (m *NLMonitor) initialDump() error {
	linkRaw, err := m.query(m.linkBatch, "link show")
	if err != nil {
		return err
	}
	addrRaw, err := m.query(m.addrBatch, "addr show")
	if err != nil {
		return err
	}
	neighRaw, err := m.query(m.neighBatch, "neigh show")
	if err != nil {
		neighRaw = json.RawMessage(`[]`)
	}
	mdbRaw, err := m.query(m.brBatch, "mdb show")
	if err != nil {
		mdbRaw = json.RawMessage(`[]`)
	}

	m.log.Debug("initialDump", "linkBytes", len(linkRaw), "addrBytes", len(addrRaw), "neighBytes", len(neighRaw))
	m.validateAddrData("initialDump", addrRaw)

	m.mu.Lock()
	m.links = linkRaw
	m.addrs = addrRaw
	m.neighs = neighRaw
	m.mdb = mdbByBridge(mdbRaw)
	for _, row := range decodeRows(linkRaw) {
		if name := rowString(row, "ifname"); name != "" {
			if st := rowString(row, "operstate"); st != "" {
				m.lastOperStatus[name] = st
			}
		}
	}
	m.mu.Unlock()

	m.rebuild()

	// Ports that are up when yangerd starts send no link event, so ask
	// for their ethtool data here or they stay without speed and duplex.
	if m.ethRefresh != nil {
		for _, name := range ethernetNames(linkRaw, m.fc) {
			m.ethRefresh(name)
		}
	}
	return nil
}

// ethernetNames lists the interfaces in an `ip -json link` dump that
// carry ethtool data.
func ethernetNames(linkRaw json.RawMessage, fc iface.FileChecker) []string {
	var rows []json.RawMessage
	if json.Unmarshal(linkRaw, &rows) != nil {
		return nil
	}

	var names []string
	for _, row := range rows {
		one := append(append(json.RawMessage{'['}, row...), ']')
		if !iface.IsEthernet(one, fc) {
			continue
		}
		var link struct {
			Name string `json:"ifname"`
		}
		if json.Unmarshal(row, &link) == nil && link.Name != "" {
			names = append(names, link.Name)
		}
	}
	return names
}

func (m *NLMonitor) handleLinkUpdate(update netlink.LinkUpdate) bool {
	index := int(update.Index)
	if update.Header.Type == syscall.RTM_DELLINK {
		defer m.linkSetChanged()
		return m.removeInterface(index)
	}

	name, ok := linkNameFromUpdate(update)
	if !ok || name == "" {
		m.log.Warn("link update without interface name", "index", index)
		return false
	}
	return m.refreshInterface(index, name)
}

func (m *NLMonitor) handleAddrUpdate(update netlink.AddrUpdate) bool {
	ifname, err := ifNameByIndex(update.LinkIndex)
	if err != nil {
		m.log.Debug("addr update: interface gone", "index", update.LinkIndex, "err", err)
		return false
	}

	raw, err := m.query(m.addrBatch, "addr show dev "+devRef(update.LinkIndex))
	if err != nil {
		return false
	}
	if !m.validateAddrData("handleAddrUpdate/"+ifname, raw) {
		m.log.Error("handleAddrUpdate: REFUSING to store invalid addr data", "ifname", ifname)
		return false
	}

	m.mu.Lock()
	m.addrs = replaceRows(m.addrs, "ifindex", update.LinkIndex, raw)
	m.mu.Unlock()
	return true
}

func (m *NLMonitor) handleNeighUpdate(update netlink.NeighUpdate) bool {
	if isBridgeFDB(update) {
		bridgeName, bridgeIndex, ok := bridgeNameFromNeigh(update)
		if !ok {
			m.log.Warn("fdb update: bridge name not found", "link-index", update.LinkIndex)
			return false
		}

		raw, err := m.query(m.brBatch, "fdb show br "+devRef(bridgeIndex))
		if err != nil {
			return false
		}

		m.mu.Lock()
		m.fdb[bridgeName] = raw
		m.mu.Unlock()
		return true
	}

	ifname, err := ifNameByIndex(update.LinkIndex)
	if err != nil {
		raw, err := m.query(m.neighBatch, "neigh show")
		if err != nil {
			return false
		}
		m.mu.Lock()
		m.neighs = raw
		m.mu.Unlock()
		return true
	}

	raw, err := m.query(m.neighBatch, "neigh show dev "+devRef(update.LinkIndex))
	if err != nil {
		return false
	}
	rows := withField(raw, "dev", ifname)

	m.mu.Lock()
	m.neighs = replaceRows(m.neighs, "dev", ifname, rows)
	m.mu.Unlock()
	return true
}

func (m *NLMonitor) handleMDBUpdate() bool {
	raw, err := m.query(m.brBatch, "mdb show")
	if err != nil {
		return false
	}

	m.mu.Lock()
	m.mdb = mdbByBridge(raw)
	m.mu.Unlock()
	return true
}

// removeInterface purges all staged data for the interface at index.
func (m *NLMonitor) removeInterface(index int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	name := nameByIndex(m.links, index)
	m.links = replaceRows(m.links, "ifindex", index, nil)
	m.addrs = replaceRows(m.addrs, "ifindex", index, nil)
	// Link and address events arrive on separate channels, so the delete
	// of an old interface can be handled after a new one took its name.
	if name != "" && !nameInUse(m.links, name) {
		m.forgetName(name)
	}
	m.log.Debug("removeInterface", "index", index, "ifname", name)
	return true
}

// forgetName drops everything staged under an interface name.  Caller
// holds m.mu.
func (m *NLMonitor) forgetName(name string) {
	m.neighs = replaceRows(m.neighs, "dev", name, nil)
	delete(m.fdb, name)
	delete(m.mdb, name)
	delete(m.ethernet, name)
	delete(m.wifi, name)
	delete(m.wireguard, name)
	delete(m.lastOperStatus, name)
}

// batchesAlive tells whether every batch subprocess is up, so a re-dump
// can succeed.
func (m *NLMonitor) batchesAlive() bool {
	for _, b := range []*ipbatch.Batch{m.linkBatch, m.addrBatch, m.neighBatch, m.brBatch} {
		if b != nil && !b.Alive() {
			return false
		}
	}
	return true
}

func (m *NLMonitor) requestRedump() {
	select {
	case m.redumpCh <- struct{}{}:
	default:
	}
}

func (m *NLMonitor) refreshInterface(index int, name string) bool {
	linkRaw, err := m.query(m.linkBatch, "link show dev "+name)
	if errors.Is(err, ipbatch.ErrCommandFailed) {
		if _, gone := net.InterfaceByIndex(index); gone != nil {
			return m.removeInterface(index) // gone before we asked
		}
		// Still there: ip resolved the name from its cache, to an
		// interface that had it before.  See devRef.
		m.log.Debug("link query failed for a live interface, refreshing ip", "ifname", name)
		if rerr := m.linkBatch.Refresh(); rerr != nil {
			m.log.Warn("refreshing ip batch failed", "err", rerr)
			return false
		}
		linkRaw, err = m.query(m.linkBatch, "link show dev "+name)
	}
	if err != nil {
		return false
	}
	if !linkRowFor(linkRaw, index) {
		// ip prints the object before it checks the message, so a
		// link racing a delete or rename can come back as [{}].
		m.log.Warn("link query returned no row for the interface, re-dumping", "ifname", name, "ifindex", index)
		m.requestRedump()
		return false
	}

	addrRaw, err := m.query(m.addrBatch, "addr show dev "+devRef(index))
	if err != nil {
		addrRaw = nil
	}
	if addrRaw != nil && !m.validateAddrData("refreshInterface/"+name, addrRaw) {
		m.log.Error("refreshInterface: REFUSING to store invalid addr data", "ifname", name)
		addrRaw = nil
	}

	m.mu.Lock()
	old := nameByIndex(m.links, index)
	if old != "" && old != name {
		m.log.Info("interface renamed", "from", old, "to", name)
		if !nameInUse(replaceRows(m.links, "ifindex", index, nil), old) {
			m.forgetName(old)
		}
	}
	m.updateOperStatus(name, linkRaw)
	m.links = replaceRows(m.links, "ifindex", index, linkRaw)
	if addrRaw != nil {
		m.addrs = replaceRows(m.addrs, "ifindex", index, addrRaw)
	}
	m.mu.Unlock()

	if old != name {
		m.linkSetChanged()
	}
	if m.ethRefresh != nil && iface.IsEthernet(linkRaw, m.fc) {
		m.ethRefresh(name)
	}
	return true
}

// rebuild runs iface.Transform on all staged data, merges augments
// (ethernet, wifi, bridge fdb/mdb), and stores the result.
// Caller must NOT hold m.mu.
func (m *NLMonitor) rebuild() {
	m.mu.Lock()
	linksCopy := append(json.RawMessage{}, m.links...)
	addrsCopy := append(json.RawMessage{}, m.addrs...)
	neighsCopy := append(json.RawMessage{}, m.neighs...)
	doc := iface.Transform(linksCopy, addrsCopy, neighsCopy, m.fc)
	eth := copyStringMap(m.ethernet)
	wfi := copyStringMap(m.wifi)
	fdb := copyStringMap(m.fdb)
	mdb := copyStringMap(m.mdb)
	wg := copyStringMap(m.wireguard)
	m.mu.Unlock()

	var brSTP, ptSTP map[string]json.RawMessage
	resolver := stpquery.NewLinksIfIndexResolver(linksCopy)
	brSTP, ptSTP = stpquery.Query(linksCopy, resolver)

	doc = mergeAugments(doc, eth, wfi, fdb, mdb, brSTP, ptSTP, wg)
	m.tree.Set(treeKey, doc)
}

// RefreshSTP re-queries mstpd and rebuilds only when STP data changed.
// mstpd's control socket is request/response with no event channel, and
// the bridge-level root-id settles via BPDU exchange without any netlink
// event, so STP state must be polled to stay current.  An empty result
// is a result too: mstpd going away must drop the stale STP data, and
// its return must bring it back.
func (m *NLMonitor) RefreshSTP() {
	links := m.Links()
	resolver := stpquery.NewLinksIfIndexResolver(links)
	brSTP, ptSTP := stpquery.Query(links, resolver)

	fp := stpFingerprint(brSTP, ptSTP)
	m.stpMu.Lock()
	changed := fp != m.lastSTP
	m.lastSTP = fp
	m.stpMu.Unlock()

	if changed {
		m.rebuild()
	}
}

func stpFingerprint(brSTP, ptSTP map[string]json.RawMessage) string {
	var b strings.Builder
	writeSortedRaw(&b, "b", brSTP)
	writeSortedRaw(&b, "p", ptSTP)
	return b.String()
}

func writeSortedRaw(b *strings.Builder, prefix string, m map[string]json.RawMessage) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString(prefix)
		b.WriteByte(':')
		b.WriteString(k)
		b.WriteByte('=')
		b.Write(m[k])
		b.WriteByte('\n')
	}
}

// mergeAugments adds ethernet, wifi, and bridge data into the
// complete ietf-interfaces document produced by iface.Transform().
func mergeAugments(doc json.RawMessage, ethernet, wifi, fdb, mdb, bridgeSTP, portSTP, wireguard map[string]json.RawMessage) json.RawMessage {
	if len(ethernet) == 0 && len(wifi) == 0 && len(fdb) == 0 && len(mdb) == 0 && len(bridgeSTP) == 0 && len(portSTP) == 0 && len(wireguard) == 0 {
		return doc
	}

	var root map[string]any
	if err := json.Unmarshal(doc, &root); err != nil {
		return doc
	}

	ifaceList, ok := root["interface"]
	if !ok {
		return doc
	}
	ifaceArr, ok := ifaceList.([]any)
	if !ok {
		return doc
	}

	for i, entry := range ifaceArr {
		ifaceObj, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		name, _ := ifaceObj["name"].(string)
		if name == "" {
			continue
		}

		if ethData, ok := ethernet[name]; ok {
			var wrapper map[string]json.RawMessage
			if err := json.Unmarshal(ethData, &wrapper); err == nil {
				if ethRaw, ok := wrapper["ethernet"]; ok {
					var ethObj any
					if err := json.Unmarshal(ethRaw, &ethObj); err == nil {
						ifaceObj["ieee802-ethernet-interface:ethernet"] = ethObj
					}
				}
				if speedRaw, ok := wrapper["speed"]; ok {
					var speed string
					if err := json.Unmarshal(speedRaw, &speed); err == nil {
						ifaceObj["speed"] = speed
					}
				}
			}
		}

		if wifiData, ok := wifi[name]; ok {
			var wifiObj any
			if err := json.Unmarshal(wifiData, &wifiObj); err == nil {
				ifaceObj["infix-interfaces:wifi"] = wifiObj
			}
		}

		if fdbData, ok := fdb[name]; ok {
			bridgeObj := ensureBridgeAugment(ifaceObj)
			var fdbObj any
			if err := json.Unmarshal(fdbData, &fdbObj); err == nil {
				bridgeObj["fdb"] = fdbObj
			}
		}

		if mdbData, ok := mdb[name]; ok {
			bridgeObj := ensureBridgeAugment(ifaceObj)
			if mf := transformMDB(mdbData); mf != nil {
				bridgeObj["multicast-filters"] = mf
			}
		}

		if stpData, ok := bridgeSTP[name]; ok {
			bridgeObj := ensureBridgeAugment(ifaceObj)
			var stpObj any
			if err := json.Unmarshal(stpData, &stpObj); err == nil {
				bridgeObj["stp"] = stpObj
			}
		}

		if stpData, ok := portSTP[name]; ok {
			bpObj := ensureBridgePortAugment(ifaceObj)
			var stpObj any
			if err := json.Unmarshal(stpData, &stpObj); err == nil {
				deepMergeSTP(bpObj, stpObj)
			}
		}

		if wgData, ok := wireguard[name]; ok {
			var wgObj any
			if err := json.Unmarshal(wgData, &wgObj); err == nil {
				ifaceObj["infix-interfaces:wireguard"] = wgObj
			}
		}

		ifaceArr[i] = ifaceObj
	}

	out, err := json.Marshal(root)
	if err != nil {
		return doc
	}
	return json.RawMessage(out)
}

// ensureBridgeAugment returns the bridge augment object within an
// interface, creating it if necessary.
func ensureBridgeAugment(ifaceObj map[string]any) map[string]any {
	key := "infix-interfaces:bridge"
	if existing, ok := ifaceObj[key]; ok {
		if m, ok := existing.(map[string]any); ok {
			return m
		}
	}
	bridgeObj := map[string]any{}
	ifaceObj[key] = bridgeObj
	return bridgeObj
}

func ensureBridgePortAugment(ifaceObj map[string]any) map[string]any {
	key := "infix-interfaces:bridge-port"
	if existing, ok := ifaceObj[key]; ok {
		if m, ok := existing.(map[string]any); ok {
			return m
		}
	}
	obj := map[string]any{}
	ifaceObj[key] = obj
	return obj
}

// deepMergeSTP merges mstpd STP data into the bridge-port augment.
// The kernel already provides stp.cist.state via iface.Transform;
// mstpd adds role, port-id, designated, etc.  We deep-merge to
// preserve the kernel state field while adding mstpd fields.
func deepMergeSTP(bpObj map[string]any, stpData any) {
	stpMap, ok := stpData.(map[string]any)
	if !ok {
		return
	}

	existing, _ := bpObj["stp"].(map[string]any)
	if existing == nil {
		bpObj["stp"] = stpMap
		return
	}

	if newCist, ok := stpMap["cist"].(map[string]any); ok {
		existingCist, _ := existing["cist"].(map[string]any)
		if existingCist == nil {
			existing["cist"] = newCist
		} else {
			for k, v := range newCist {
				existingCist[k] = v
			}
		}
	}

	for k, v := range stpMap {
		if k != "cist" {
			existing[k] = v
		}
	}
}

func copyStringMap(m map[string]json.RawMessage) map[string]json.RawMessage {
	if len(m) == 0 {
		return nil
	}
	cp := make(map[string]json.RawMessage, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}

func (m *NLMonitor) updateOperStatus(ifname string, raw json.RawMessage) {
	status, ok := extractOperStatus(raw)
	if !ok {
		return
	}

	prev, had := m.lastOperStatus[ifname]
	m.lastOperStatus[ifname] = status
	if had && prev != status {
		m.log.Info("oper-status transition", "ifname", ifname, "from", prev, "to", status)
	}
}

// query runs one command on a batch worker.  A dead worker asks for a
// re-dump once it is back; a rejected command is left to the caller.
func (m *NLMonitor) query(b *ipbatch.Batch, command string) (json.RawMessage, error) {
	raw, err := b.Query(command)
	switch {
	case err == nil:
		return raw, nil
	case errors.Is(err, ipbatch.ErrCommandFailed):
		m.log.Debug("batch command failed", "command", command)
	case errors.Is(err, ipbatch.ErrBatchDead):
		m.log.Warn("batch dead", "command", command)
		m.requestRedump()
	default:
		m.log.Error("batch query failed", "command", command, "err", err)
	}
	return nil, err
}

func (m *NLMonitor) subscribeBridgeMDB(ctx context.Context, ch chan<- struct{}, errorCallback func(error)) error {
	sock, err := nl.Subscribe(syscall.NETLINK_ROUTE, 26)
	if err != nil {
		return err
	}

	// Receive blocks until a message arrives; closing the socket on
	// cancel is what ends it.  Close once, the fd number may be reused.
	var once sync.Once
	closeSock := func() { once.Do(sock.Close) }
	stop := context.AfterFunc(ctx, closeSock)

	go func() {
		defer close(ch)
		defer stop()
		defer closeSock()

		for {
			msgs, _, err := sock.Receive()
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				errorCallback(err)
				return
			}
			if len(msgs) == 0 {
				continue
			}

			select {
			case ch <- struct{}{}:
			default:
			}
		}
	}()

	return nil
}

func ifNameByIndex(index int) (string, error) {
	iface, err := net.InterfaceByIndex(index)
	if err != nil {
		return "", err
	}
	return iface.Name, nil
}

func linkNameFromUpdate(update netlink.LinkUpdate) (string, bool) {
	if update.Link != nil && update.Link.Attrs() != nil && update.Link.Attrs().Name != "" {
		return update.Link.Attrs().Name, true
	}
	if update.Index <= 0 {
		return "", false
	}
	name, err := ifNameByIndex(int(update.Index))
	if err != nil {
		return "", false
	}
	return name, true
}

func isBridgeFDB(update netlink.NeighUpdate) bool {
	if update.Family == syscall.AF_BRIDGE {
		return true
	}
	if update.MasterIndex > 0 {
		return true
	}
	if update.Flags&netlink.NTF_MASTER != 0 {
		return true
	}
	return false
}

func bridgeNameFromNeigh(update netlink.NeighUpdate) (string, int, bool) {
	if update.MasterIndex > 0 {
		name, err := ifNameByIndex(update.MasterIndex)
		if err == nil {
			return name, update.MasterIndex, true
		}
	}

	if update.LinkIndex <= 0 {
		return "", 0, false
	}
	link, err := netlink.LinkByIndex(update.LinkIndex)
	if err == nil && link != nil && link.Attrs() != nil && link.Attrs().MasterIndex > 0 {
		name, err := ifNameByIndex(link.Attrs().MasterIndex)
		if err == nil {
			return name, link.Attrs().MasterIndex, true
		}
	}
	return "", 0, false
}

// devRef names a device by index for a batch command.  iproute2 caches
// name-to-index lookups for the life of the process, so once a name is
// reused by a new interface, "dev wifi0" keeps resolving to the deleted
// one.  "if<N>" bypasses that cache.  "link show" is the exception: it
// sends the name to the kernel and does not accept this form.
func devRef(index int) string {
	return "if" + strconv.Itoa(index)
}

// validateAddrData checks whether a JSON response from "addr show" contains
// addr_info entries.  "ip -json addr show" always includes an "addr_info"
// array for every interface object; its absence means we got link-format
// data instead.  Returns true if the data looks valid (has addr_info).
func (m *NLMonitor) validateAddrData(caller string, raw json.RawMessage) bool {
	if len(raw) == 0 {
		m.log.Error("addr data is EMPTY", "caller", caller)
		return false
	}

	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		m.log.Error("addr data unmarshal failed", "caller", caller, "err", err, "raw", string(raw))
		return false
	}

	if len(rows) == 0 {
		// Empty array is valid — interface exists but has no addresses.
		return true
	}

	for _, row := range rows {
		ifnRaw, _ := row["ifname"]
		var ifn string
		json.Unmarshal(ifnRaw, &ifn)

		if _, ok := row["addr_info"]; !ok {
			m.log.Error("addr data MISSING addr_info — got link-format data",
				"caller", caller,
				"ifname", ifn,
				"keys", mapKeys(row),
				"raw", string(raw),
			)
			return false
		}
	}
	return true
}

// mapKeys returns the JSON object keys from a map for diagnostic logging.
func mapKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func extractOperStatus(raw json.RawMessage) (string, bool) {
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil || len(rows) == 0 {
		return "", false
	}
	stateRaw, ok := rows[0]["operstate"]
	if !ok {
		return "", false
	}
	var state string
	if err := json.Unmarshal(stateRaw, &state); err != nil || state == "" {
		return "", false
	}
	return state, true
}

// decodeRows splits an ip/bridge -json array into raw rows.
func decodeRows(raw json.RawMessage) []map[string]json.RawMessage {
	var rows []map[string]json.RawMessage
	if json.Unmarshal(raw, &rows) != nil {
		return nil
	}
	return rows
}

func rowString(row map[string]json.RawMessage, key string) string {
	var s string
	if json.Unmarshal(row[key], &s) != nil {
		return ""
	}
	return s
}

// rowMatches compares a row field against a Go value by its JSON form,
// so an ifindex matches as a number and a dev name as a string.
func rowMatches(row map[string]json.RawMessage, key, want string) bool {
	v, ok := row[key]
	return ok && string(v) == want
}

func jsonKey(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// replaceRows drops every row of bulk whose key equals match, appends
// the rows of replacement (nil to only drop), and returns the result.
func replaceRows(bulk json.RawMessage, key string, match any, replacement json.RawMessage) json.RawMessage {
	want := jsonKey(match)
	var kept []json.RawMessage
	if json.Unmarshal(bulk, &kept) != nil {
		kept = nil
	}

	out := kept[:0]
	for _, raw := range kept {
		var row map[string]json.RawMessage
		if json.Unmarshal(raw, &row) == nil && rowMatches(row, key, want) {
			continue
		}
		out = append(out, raw)
	}

	var add []json.RawMessage
	if json.Unmarshal(replacement, &add) == nil {
		out = append(out, add...)
	}
	if out == nil {
		out = []json.RawMessage{}
	}

	merged, err := json.Marshal(out)
	if err != nil {
		return bulk
	}
	return merged
}

// nameByIndex returns the staged ifname for index, "" if none.
// linkRowFor tells whether raw is a link answer for index: one row with
// that ifindex and a name.
func linkRowFor(raw json.RawMessage, index int) bool {
	rows := decodeRows(raw)
	return len(rows) == 1 && rowMatches(rows[0], "ifindex", jsonKey(index)) &&
		rowString(rows[0], "ifname") != ""
}

// nameInUse tells whether any staged link row carries name.
func nameInUse(links json.RawMessage, name string) bool {
	for _, row := range decodeRows(links) {
		if rowString(row, "ifname") == name {
			return true
		}
	}
	return false
}

func nameByIndex(links json.RawMessage, index int) string {
	want := jsonKey(index)
	for _, row := range decodeRows(links) {
		if rowMatches(row, "ifindex", want) {
			return rowString(row, "ifname")
		}
	}
	return ""
}

// withField sets key to value on every row of raw that lacks it.
// `ip neigh show dev X` leaves out the dev it was asked about.
func withField(raw json.RawMessage, key, value string) json.RawMessage {
	rows := decodeRows(raw)
	v := json.RawMessage(jsonKey(value))
	for _, row := range rows {
		if _, ok := row[key]; !ok {
			row[key] = v
		}
	}
	if rows == nil {
		rows = []map[string]json.RawMessage{}
	}
	out, err := json.Marshal(rows)
	if err != nil {
		return raw
	}
	return out
}

// parseMDBEntries extracts the flat list of MDB entries from the
// bridge batch output format: [{"mdb":[{entries...}],"router":{}}]
func parseMDBEntries(raw json.RawMessage) []map[string]any {
	var wrappers []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wrappers); err != nil {
		return nil
	}

	var all []map[string]any
	for _, w := range wrappers {
		mdbRaw, ok := w["mdb"]
		if !ok {
			continue
		}
		var entries []map[string]any
		if err := json.Unmarshal(mdbRaw, &entries); err != nil {
			continue
		}
		all = append(all, entries...)
	}
	return all
}

// mdbByBridge splits `bridge mdb show` into per-bridge entry lists.  A
// bridge without entries is simply absent, which clears its filters.
func mdbByBridge(raw json.RawMessage) map[string]json.RawMessage {
	groups := make(map[string][]map[string]any)
	for _, e := range parseMDBEntries(raw) {
		if dev, _ := e["dev"].(string); dev != "" {
			groups[dev] = append(groups[dev], e)
		}
	}
	out := make(map[string]json.RawMessage, len(groups))
	for dev, entries := range groups {
		if b, err := json.Marshal(entries); err == nil {
			out[dev] = b
		}
	}
	return out
}

func transformMDB(raw json.RawMessage) map[string]any {
	var entries []map[string]any
	if err := json.Unmarshal(raw, &entries); err != nil || len(entries) == 0 {
		return nil
	}

	type portEntry struct {
		Port  string `json:"port"`
		State string `json:"state"`
	}

	groups := make(map[string][]portEntry)
	var order []string

	for _, e := range entries {
		grp, _ := e["grp"].(string)
		port, _ := e["port"].(string)
		state, _ := e["state"].(string)
		if grp == "" || port == "" {
			continue
		}

		if _, seen := groups[grp]; !seen {
			order = append(order, grp)
		}
		groups[grp] = append(groups[grp], portEntry{
			Port:  port,
			State: mdbStateToYANG(state),
		})
	}

	if len(groups) == 0 {
		return nil
	}

	filters := make([]map[string]any, 0, len(order))
	for _, grp := range order {
		filters = append(filters, map[string]any{
			"group": grp,
			"ports": groups[grp],
		})
	}

	return map[string]any{"multicast-filter": filters}
}

func mdbStateToYANG(state string) string {
	switch state {
	case "temp":
		return "temporary"
	case "permanent":
		return "permanent"
	default:
		return state
	}
}
