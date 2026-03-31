package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"syscall"

	"github.com/kernelkit/infix/src/yangerd/internal/bridgebatch"
	"github.com/kernelkit/infix/src/yangerd/internal/iface"
	"github.com/kernelkit/infix/src/yangerd/internal/ipbatch"
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
type NLMonitor struct {
	linkBatch  *ipbatch.IPBatch
	addrBatch  *ipbatch.IPBatch
	brBatch    *bridgebatch.BridgeBatch
	tree       *tree.Tree
	ethRefresh func(string)
	log        *slog.Logger
	fc         iface.FileChecker

	// initDone is closed after the first initialDump completes.
	initDone chan struct{}

	// staging holds raw ip-json data used as input to iface.Transform().
	// Protected by mu.
	mu       sync.Mutex
	links    json.RawMessage // ip -json -s -d link show (includes stats+details)
	addrs    json.RawMessage // ip -json -d addr show (details only, no stats)
	fdb      map[string]json.RawMessage
	mdb      map[string]json.RawMessage
	ethernet map[string]json.RawMessage // ifname → ethtool JSON
	wifi     map[string]json.RawMessage // ifname → wifi JSON

	lastOperStatus map[string]string
}

// New creates a netlink monitor backed by ip/bridge batch query workers.
// linkBatch should include -s -d flags; addrBatch should include -d only
// (no -s, which causes multi-line output for link commands).
func New(linkBatch, addrBatch *ipbatch.IPBatch, brBatch *bridgebatch.BridgeBatch, t *tree.Tree, fc iface.FileChecker, log *slog.Logger) *NLMonitor {
	return &NLMonitor{
		linkBatch:      linkBatch,
		addrBatch:      addrBatch,
		brBatch:        brBatch,
		tree:           t,
		fc:             fc,
		log:            log,
		initDone:       make(chan struct{}),
		fdb:            make(map[string]json.RawMessage),
		mdb:            make(map[string]json.RawMessage),
		ethernet:       make(map[string]json.RawMessage),
		wifi:           make(map[string]json.RawMessage),
		lastOperStatus: make(map[string]string),
	}
}

// SetEthRefresh sets an optional callback used to refresh ethtool data
// when interface link events are received.
func (m *NLMonitor) SetEthRefresh(fn func(string)) {
	m.ethRefresh = fn
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

// Run starts the netlink monitor loop and returns on context cancellation,
// channel closure, or subscription errors.
func (m *NLMonitor) Run(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan struct{})
	defer close(done)

	errorCallback := func(err error) {
		if err == nil {
			return
		}
		m.log.Error("netlink subscription error", "err", err)
		cancel()
	}

	linkCh := make(chan netlink.LinkUpdate, 64)
	addrCh := make(chan netlink.AddrUpdate, 64)
	neighCh := make(chan netlink.NeighUpdate, 64)
	mdbCh := make(chan struct{}, 32)

	if err := netlink.LinkSubscribeWithOptions(linkCh, done, netlink.LinkSubscribeOptions{
		ErrorCallback: errorCallback,
	}); err != nil {
		return fmt.Errorf("subscribe link updates: %w", err)
	}
	if err := netlink.AddrSubscribeWithOptions(addrCh, done, netlink.AddrSubscribeOptions{
		ErrorCallback: errorCallback,
	}); err != nil {
		return fmt.Errorf("subscribe addr updates: %w", err)
	}
	if err := netlink.NeighSubscribeWithOptions(neighCh, done, netlink.NeighSubscribeOptions{
		ErrorCallback: errorCallback,
	}); err != nil {
		return fmt.Errorf("subscribe neigh updates: %w", err)
	}
	if err := m.subscribeBridgeMDB(runCtx, mdbCh, errorCallback); err != nil {
		return fmt.Errorf("subscribe bridge mdb updates: %w", err)
	}

	if err := m.initialDump(); err != nil {
		m.log.Error("initial dump failed", "err", err)
	}
	close(m.initDone)

	for {
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
			m.handleLinkUpdate(lu)
		case au, ok := <-addrCh:
			if !ok {
				return fmt.Errorf("addr update channel closed")
			}
			m.handleAddrUpdate(au)
		case nu, ok := <-neighCh:
			if !ok {
				return fmt.Errorf("neigh update channel closed")
			}
			m.handleNeighUpdate(nu)
		case _, ok := <-mdbCh:
			if !ok {
				return fmt.Errorf("bridge mdb update channel closed")
			}
			m.handleMDBUpdate()
		}
	}
}

func (m *NLMonitor) initialDump() error {
	linkRaw, err := m.queryLink("link show")
	if err != nil {
		return err
	}
	addrRaw, err := m.queryAddr("addr show")
	if err != nil {
		return err
	}

	m.log.Debug("initialDump", "linkBytes", len(linkRaw), "addrBytes", len(addrRaw))
	m.validateAddrData("initialDump", addrRaw)

	m.mu.Lock()
	m.links = linkRaw
	m.addrs = addrRaw
	for _, name := range interfaceNames(linkRaw) {
		if st, ok := extractOperStatus(filterByIfName(linkRaw, name)); ok {
			m.lastOperStatus[name] = st
		}
	}
	m.mu.Unlock()

	m.rebuild()
	return nil
}

func (m *NLMonitor) handleLinkUpdate(update netlink.LinkUpdate) {
	name, ok := linkNameFromUpdate(update)
	if !ok || name == "" {
		m.log.Warn("link update without interface name", "index", int(update.Index))
		return
	}

	m.refreshInterface(name)
	if m.ethRefresh != nil {
		m.ethRefresh(name)
	}
}

func (m *NLMonitor) handleAddrUpdate(update netlink.AddrUpdate) {
	ifname, err := ifNameByIndex(update.LinkIndex)
	if err != nil {
		m.log.Warn("addr update: resolve interface", "index", update.LinkIndex, "err", err)
		return
	}

	m.log.Debug("handleAddrUpdate", "ifname", ifname)
	raw, err := m.queryAddr("addr show dev " + ifname)
	if err != nil {
		m.log.Error("handleAddrUpdate queryAddr failed", "ifname", ifname, "err", err)
		return
	}

	if !m.validateAddrData("handleAddrUpdate/"+ifname, raw) {
		m.log.Error("handleAddrUpdate: REFUSING to store invalid addr data", "ifname", ifname)
		return
	}

	m.mu.Lock()
	m.addrs = replaceByIfName(m.addrs, ifname, raw)
	m.mu.Unlock()

	m.rebuild()
}

func (m *NLMonitor) handleNeighUpdate(update netlink.NeighUpdate) {
	if isBridgeFDB(update) {
		bridgeName, ok := bridgeNameFromNeigh(update)
		if !ok {
			m.log.Warn("fdb update: bridge name not found", "link-index", update.LinkIndex)
			return
		}

		raw, err := m.queryBridge("fdb show br " + bridgeName)
		if err != nil {
			return
		}

		m.mu.Lock()
		m.fdb[bridgeName] = raw
		m.mu.Unlock()

		m.rebuild()
		return
	}

}

func (m *NLMonitor) handleMDBUpdate() {
	raw, err := m.queryBridge("mdb show")
	if err != nil {
		return
	}

	m.mu.Lock()
	for _, bridgeName := range bridgeNames(raw) {
		m.mdb[bridgeName] = filterByBridge(raw, bridgeName)
	}
	m.mu.Unlock()

	m.rebuild()
}

func (m *NLMonitor) refreshInterface(name string) {
	linkRaw, err := m.queryLink("link show dev " + name)
	if err != nil {
		return
	}

	addrRaw, err := m.queryAddr("addr show dev " + name)
	if err != nil {
		addrRaw = nil
	}
	if addrRaw != nil && !m.validateAddrData("refreshInterface/"+name, addrRaw) {
		m.log.Error("refreshInterface: REFUSING to store invalid addr data", "ifname", name)
		addrRaw = nil
	}

	m.mu.Lock()
	m.updateOperStatus(name, linkRaw)
	m.links = replaceByIfName(m.links, name, linkRaw)
	if addrRaw != nil {
		m.addrs = replaceByIfName(m.addrs, name, addrRaw)
	}
	m.mu.Unlock()

	m.rebuild()
}

// rebuild runs iface.Transform on all staged data, merges augments
// (ethernet, wifi, bridge fdb/mdb), and stores the result.
// Caller must NOT hold m.mu.
func (m *NLMonitor) rebuild() {
	m.mu.Lock()
	doc := iface.Transform(m.links, m.addrs, m.links, m.fc)
	eth := copyStringMap(m.ethernet)
	wfi := copyStringMap(m.wifi)
	fdb := copyStringMap(m.fdb)
	mdb := copyStringMap(m.mdb)
	m.mu.Unlock()

	doc = mergeAugments(doc, eth, wfi, fdb, mdb)
	m.tree.Set(treeKey, doc)
}

// mergeAugments adds ethernet, wifi, and bridge data into the
// complete ietf-interfaces document produced by iface.Transform().
func mergeAugments(doc json.RawMessage, ethernet, wifi, fdb, mdb map[string]json.RawMessage) json.RawMessage {
	if len(ethernet) == 0 && len(wifi) == 0 && len(fdb) == 0 && len(mdb) == 0 {
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
			var ethObj any
			if err := json.Unmarshal(ethData, &ethObj); err == nil {
				ifaceObj["ieee802-ethernet-interface:ethernet"] = ethObj
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
			var mdbObj any
			if err := json.Unmarshal(mdbData, &mdbObj); err == nil {
				bridgeObj["mdb"] = mdbObj
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

func (m *NLMonitor) queryLink(command string) (json.RawMessage, error) {
	raw, err := m.linkBatch.Query(command)
	if err != nil {
		if errors.Is(err, ipbatch.ErrBatchDead) {
			m.log.Warn("link batch dead", "command", command, "err", err)
			return nil, err
		}
		m.log.Error("link batch query failed", "command", command, "err", err)
		return nil, err
	}
	return raw, nil
}

func (m *NLMonitor) queryAddr(command string) (json.RawMessage, error) {
	raw, err := m.addrBatch.Query(command)
	if err != nil {
		if errors.Is(err, ipbatch.ErrBatchDead) {
			m.log.Warn("addr batch dead", "command", command, "err", err)
			return nil, err
		}
		m.log.Error("addr batch query failed", "command", command, "err", err)
		return nil, err
	}
	return raw, nil
}

func (m *NLMonitor) queryBridge(command string) (json.RawMessage, error) {
	raw, err := m.brBatch.Query(command)
	if err != nil {
		if errors.Is(err, bridgebatch.ErrBatchDead) {
			m.log.Warn("bridge batch dead", "command", command, "err", err)
			return nil, err
		}
		m.log.Error("bridge batch query failed", "command", command, "err", err)
		return nil, err
	}
	return raw, nil
}

func (m *NLMonitor) subscribeBridgeMDB(ctx context.Context, ch chan<- struct{}, errorCallback func(error)) error {
	sock, err := nl.Subscribe(syscall.NETLINK_ROUTE, 26)
	if err != nil {
		return err
	}

	go func() {
		defer close(ch)
		defer sock.Close()

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

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

func bridgeNameFromNeigh(update netlink.NeighUpdate) (string, bool) {
	if update.MasterIndex > 0 {
		name, err := ifNameByIndex(update.MasterIndex)
		if err == nil {
			return name, true
		}
	}

	if update.LinkIndex <= 0 {
		return "", false
	}
	link, err := netlink.LinkByIndex(update.LinkIndex)
	if err == nil && link != nil && link.Attrs() != nil && link.Attrs().MasterIndex > 0 {
		name, err := ifNameByIndex(link.Attrs().MasterIndex)
		if err == nil {
			return name, true
		}
	}
	return "", false
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

func interfaceNames(raw json.RawMessage) []string {
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil
	}

	names := make([]string, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		ifnRaw, ok := row["ifname"]
		if !ok {
			continue
		}
		var ifname string
		if err := json.Unmarshal(ifnRaw, &ifname); err != nil || ifname == "" {
			continue
		}
		if _, ok := seen[ifname]; ok {
			continue
		}
		seen[ifname] = struct{}{}
		names = append(names, ifname)
	}
	return names
}

func filterByIfName(raw json.RawMessage, ifname string) json.RawMessage {
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return json.RawMessage(`[]`)
	}

	filtered := make([]map[string]json.RawMessage, 0, 1)
	for _, row := range rows {
		ifnRaw, ok := row["ifname"]
		if !ok {
			continue
		}
		var name string
		if err := json.Unmarshal(ifnRaw, &name); err != nil {
			continue
		}
		if name == ifname {
			filtered = append(filtered, row)
		}
	}

	out, err := json.Marshal(filtered)
	if err != nil {
		return json.RawMessage(`[]`)
	}
	return json.RawMessage(out)
}

// replaceByIfName replaces all entries for ifname in the bulk array
// with entries from perIface, and returns the updated full array.
func replaceByIfName(bulk json.RawMessage, ifname string, perIface json.RawMessage) json.RawMessage {
	var bulkRows []json.RawMessage
	if err := json.Unmarshal(bulk, &bulkRows); err != nil {
		return perIface
	}

	kept := make([]json.RawMessage, 0, len(bulkRows))
	for _, row := range bulkRows {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(row, &obj); err != nil {
			kept = append(kept, row)
			continue
		}
		ifnRaw, ok := obj["ifname"]
		if !ok {
			kept = append(kept, row)
			continue
		}
		var name string
		if err := json.Unmarshal(ifnRaw, &name); err != nil || name != ifname {
			kept = append(kept, row)
		}
	}

	var newRows []json.RawMessage
	if err := json.Unmarshal(perIface, &newRows); err == nil {
		kept = append(kept, newRows...)
	}

	out, err := json.Marshal(kept)
	if err != nil {
		return bulk
	}
	return json.RawMessage(out)
}

func bridgeNames(raw json.RawMessage) []string {
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil
	}

	names := make([]string, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		brRaw, ok := row["br"]
		if !ok {
			continue
		}
		var name string
		if err := json.Unmarshal(brRaw, &name); err != nil || name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}

func filterByBridge(raw json.RawMessage, bridgeName string) json.RawMessage {
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return json.RawMessage(`[]`)
	}

	filtered := make([]map[string]json.RawMessage, 0, 1)
	for _, row := range rows {
		brRaw, ok := row["br"]
		if !ok {
			continue
		}
		var br string
		if err := json.Unmarshal(brRaw, &br); err != nil {
			continue
		}
		if br == bridgeName {
			filtered = append(filtered, row)
		}
	}

	out, err := json.Marshal(filtered)
	if err != nil {
		return json.RawMessage(`[]`)
	}
	return json.RawMessage(out)
}
