// SPDX-License-Identifier: MIT

package handlers

import (
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/kernelkit/webui/internal/restconf"
)

// RESTCONF JSON structures for ietf-interfaces:interfaces.

type interfacesWrapper struct {
	Interfaces struct {
		Interface []ifaceJSON `json:"interface"`
	} `json:"ietf-interfaces:interfaces"`
}

type ifaceJSON struct {
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	OperStatus  string          `json:"oper-status"`
	PhysAddress string          `json:"phys-address"`
	IfIndex     int             `json:"if-index"`
	IPv4        *ipCfg          `json:"ietf-ip:ipv4"`
	IPv6        *ipCfg          `json:"ietf-ip:ipv6"`
	Statistics  *ifaceStats     `json:"statistics"`
	Ethernet    *ethernetJSON   `json:"ieee802-ethernet-interface:ethernet"`
	BridgePort  *bridgePortJSON `json:"infix-interfaces:bridge-port"`
	Vlan        *vlanJSON       `json:"infix-interfaces:vlan"`
	WiFi        *wifiJSON       `json:"infix-interfaces:wifi"`
	WireGuard   *wireGuardJSON  `json:"infix-interfaces:wireguard"`
}

type vlanJSON struct {
	ID           int    `json:"id"`
	LowerLayerIf string `json:"lower-layer-if"`
}

type bridgePortJSON struct {
	Bridge string `json:"bridge"`
	STP    *struct {
		CIST *struct {
			State string `json:"state"`
		} `json:"cist"`
	} `json:"stp"`
}

type wifiJSON struct {
	Radio       string           `json:"radio"`
	AccessPoint *wifiAPJSON      `json:"access-point"`
	Station     *wifiStationJSON `json:"station"`
}

type wifiAPJSON struct {
	SSID     string `json:"ssid"`
	Stations struct {
		Station []wifiStaJSON `json:"station"`
	} `json:"stations"`
}

type wifiStaJSON struct {
	MACAddress     string    `json:"mac-address"`
	SignalStrength *int      `json:"signal-strength"`
	ConnectedTime  yangInt64 `json:"connected-time"`
	RxPackets      yangInt64 `json:"rx-packets"`
	TxPackets      yangInt64 `json:"tx-packets"`
	RxBytes        yangInt64 `json:"rx-bytes"`
	TxBytes        yangInt64 `json:"tx-bytes"`
	RxSpeed        yangInt64 `json:"rx-speed"`
	TxSpeed        yangInt64 `json:"tx-speed"`
}

type wifiStationJSON struct {
	SSID           string               `json:"ssid"`
	SignalStrength *int                 `json:"signal-strength"`
	RxSpeed        yangInt64            `json:"rx-speed"`
	TxSpeed        yangInt64            `json:"tx-speed"`
	ScanResults    []wifiScanResultJSON `json:"scan-results"`
}

type wifiScanResultJSON struct {
	SSID           string   `json:"ssid"`
	BSSID          string   `json:"bssid"`
	SignalStrength *int     `json:"signal-strength"`
	Channel        int      `json:"channel"`
	Encryption     []string `json:"encryption"`
}

// WiFi radio survey RESTCONF structures (from ietf-hardware:hardware).

type wifiRadioJSON struct {
	Survey *wifiSurveyJSON `json:"survey"`
}

type wifiSurveyJSON struct {
	Channel []surveyChanJSON `json:"channel"`
}

type surveyChanJSON struct {
	Frequency    int      `json:"frequency"`
	InUse        yangBool `json:"in-use"`
	Noise        int      `json:"noise"`
	ActiveTime   int      `json:"active-time"`
	BusyTime     int      `json:"busy-time"`
	ReceiveTime  int      `json:"receive-time"`
	TransmitTime int      `json:"transmit-time"`
}

type wireGuardJSON struct {
	PeerStatus *struct {
		Peer []wgPeerJSON `json:"peer"`
	} `json:"peer-status"`
}

type wgPeerJSON struct {
	PublicKey        string `json:"public-key"`
	ConnectionStatus string `json:"connection-status"`
	EndpointAddress  string `json:"endpoint-address"`
	EndpointPort     int    `json:"endpoint-port"`
	LatestHandshake  string `json:"latest-handshake"`
	Transfer         *struct {
		TxBytes yangInt64 `json:"tx-bytes"`
		RxBytes yangInt64 `json:"rx-bytes"`
	} `json:"transfer"`
}

type ipCfg struct {
	Address []ipAddr `json:"address"`
	MTU     int      `json:"mtu"`
}

type ipAddr struct {
	IP           string    `json:"ip"`
	PrefixLength yangInt64 `json:"prefix-length"`
	Origin       string    `json:"origin"`
}

type ifaceStats struct {
	InOctets         yangInt64 `json:"in-octets"`
	OutOctets        yangInt64 `json:"out-octets"`
	InUnicastPkts    yangInt64 `json:"in-unicast-pkts"`
	InBroadcastPkts  yangInt64 `json:"in-broadcast-pkts"`
	InMulticastPkts  yangInt64 `json:"in-multicast-pkts"`
	InDiscards       yangInt64 `json:"in-discards"`
	InErrors         yangInt64 `json:"in-errors"`
	OutUnicastPkts   yangInt64 `json:"out-unicast-pkts"`
	OutBroadcastPkts yangInt64 `json:"out-broadcast-pkts"`
	OutMulticastPkts yangInt64 `json:"out-multicast-pkts"`
	OutDiscards      yangInt64 `json:"out-discards"`
	OutErrors        yangInt64 `json:"out-errors"`
}

type ethernetJSON struct {
	Speed           string `json:"speed"`
	Duplex          string `json:"duplex"`
	AutoNegotiation *struct {
		Enable bool `json:"enable"`
	} `json:"auto-negotiation"`
	Statistics *struct {
		Frame *ethFrameStats `json:"frame"`
	} `json:"statistics"`
}

type ethFrameStats struct {
	InTotalPkts        yangInt64 `json:"in-total-pkts"`
	InTotalOctets      yangInt64 `json:"in-total-octets"`
	InGoodPkts         yangInt64 `json:"in-good-pkts"`
	InGoodOctets       yangInt64 `json:"in-good-octets"`
	InBroadcast        yangInt64 `json:"in-broadcast"`
	InMulticast        yangInt64 `json:"in-multicast"`
	InErrorFCS         yangInt64 `json:"in-error-fcs"`
	InErrorUndersize   yangInt64 `json:"in-error-undersize"`
	InErrorOversize    yangInt64 `json:"in-error-oversize"`
	InErrorMACInternal yangInt64 `json:"in-error-mac-internal"`
	OutTotalPkts       yangInt64 `json:"out-total-pkts"`
	OutTotalOctets     yangInt64 `json:"out-total-octets"`
	OutGoodPkts        yangInt64 `json:"out-good-pkts"`
	OutGoodOctets      yangInt64 `json:"out-good-octets"`
	OutBroadcast       yangInt64 `json:"out-broadcast"`
	OutMulticast       yangInt64 `json:"out-multicast"`
}

// Template data structures.

type interfacesData struct {
	PageData
	Interfaces []ifaceEntry
	Error      string
}

type ifaceEntry struct {
	HasMembers   bool // is a bridge/LAG master with child ports
	IsMember     bool // is a bridge port or LAG member
	IsLastMember bool // is the last child in its group
	Forwarding   bool // IP forwarding enabled (⇅ flag)
	GroupID      string // bridge/LAG name — set on parent and all its members
	Name         string
	Type         string
	Status       string
	StatusUp     bool
	PhysAddr     string
	Addresses    []addrEntry
	Detail       string // extra info: wifi AP, wireguard peers, etc.
	RxBytes      string
	TxBytes      string
}

type addrEntry struct {
	Address string
	Origin  string
}

// InterfacesHandler serves the interfaces pages.
type InterfacesHandler struct {
	Template         *template.Template
	DetailTemplate   *template.Template
	CountersTemplate *template.Template
	RC               *restconf.Client
}

// Overview renders the interfaces page (GET /interfaces).
func (h *InterfacesHandler) Overview(w http.ResponseWriter, r *http.Request) {
	data := interfacesData{
		PageData: newPageData(r, "interfaces", "Interfaces"),
	}

	var (
		ifaces interfacesWrapper
		ri     struct {
			Routing struct {
				Interfaces struct {
					Interface []string `json:"interface"`
				} `json:"interfaces"`
			} `json:"ietf-routing:routing"`
		}
		ifaceErr error
		wg       sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		ifaceErr = h.RC.Get(r.Context(), "/data/ietf-interfaces:interfaces", &ifaces)
	}()
	go func() {
		defer wg.Done()
		// Best-effort: ignore errors (routing may not be configured).
		// Fetch the full routing object — the /interfaces sub-path returns empty
		// on Infix even when the data is present in the parent resource.
		h.RC.Get(r.Context(), "/data/ietf-routing:routing", &ri) //nolint:errcheck
	}()
	wg.Wait()

	if ifaceErr != nil {
		log.Printf("restconf interfaces: %v", ifaceErr)
		data.Error = "Could not fetch interface information"
	} else {
		fwdSet := make(map[string]bool, len(ri.Routing.Interfaces.Interface))
		for _, name := range ri.Routing.Interfaces.Interface {
			fwdSet[name] = true
		}
		data.Interfaces = buildIfaceList(ifaces.Interfaces.Interface, fwdSet)
	}

	tmplName := "interfaces.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

const (
	ifTypeEthernet = "ethernet"
	ifTypeLoopback = "loopback"
)

// prettyIfType converts a YANG interface type identity to the display
// name used by the Infix CLI (cli_pretty).
func prettyIfType(full string) string {
	pretty := map[string]string{
		"bridge":           "bridge",
		"dummy":            "dummy",
		"ethernet":         "ethernet",
		"gre":              "gre",
		"gretap":           "gretap",
		"vxlan":            "vxlan",
		"wireguard":        "wireguard",
		"lag":              "lag",
		"loopback":         "loopback",
		"veth":             "veth",
		"vlan":             "vlan",
		"wifi":             "wifi",
		"other":            "other",
		"ethernetCsmacd":   "ethernet",
		"softwareLoopback": "loopback",
		"l2vlan":           "vlan",
		"ieee8023adLag":    "lag",
		"ieee80211":        "wifi",
		"ilan":             "veth",
	}

	if i := strings.LastIndex(full, ":"); i >= 0 {
		full = full[i+1:]
	}
	if name, ok := pretty[full]; ok {
		return name
	}
	return full
}

// buildIfaceList converts raw RESTCONF interface data into a flat,
// hierarchically ordered display list. Bridge/LAG members are grouped
// under their parent. fwdSet contains the names of interfaces with IP
// forwarding enabled (the ⇅ flag).
func buildIfaceList(raw []ifaceJSON, fwdSet map[string]bool) []ifaceEntry {
	byName := map[string]*ifaceJSON{}
	children := map[string][]string{}
	childSet := map[string]bool{}

	for i := range raw {
		iface := &raw[i]
		byName[iface.Name] = iface
		if iface.BridgePort != nil && iface.BridgePort.Bridge != "" {
			parent := iface.BridgePort.Bridge
			children[parent] = append(children[parent], iface.Name)
			childSet[iface.Name] = true
		}
	}

	// Collect top-level interfaces (not bridge/LAG members) and sort them:
	// loopback first, then alphabetically.
	var topLevel []ifaceJSON
	for _, iface := range raw {
		if !childSet[iface.Name] {
			topLevel = append(topLevel, iface)
		}
	}
	sort.Slice(topLevel, func(i, j int) bool {
		li := prettyIfType(topLevel[i].Type) == ifTypeLoopback
		lj := prettyIfType(topLevel[j].Type) == ifTypeLoopback
		if li != lj {
			return li
		}
		return topLevel[i].Name < topLevel[j].Name
	})

	// Sort children of each bridge/LAG alphabetically.
	for parent := range children {
		sort.Strings(children[parent])
	}

	var result []ifaceEntry

	for _, iface := range topLevel {
		e := makeIfaceEntry(iface, fwdSet)
		members := children[iface.Name]
		e.HasMembers = len(members) > 0
		if e.HasMembers {
			e.GroupID = iface.Name
		}
		result = append(result, e)

		for i, childName := range members {
			child, ok := byName[childName]
			if !ok {
				continue
			}
			me := makeIfaceEntry(*child, fwdSet)
			me.IsMember = true
			me.IsLastMember = i == len(members)-1
			me.GroupID = iface.Name
			if child.BridgePort != nil && child.BridgePort.STP != nil &&
				child.BridgePort.STP.CIST != nil && child.BridgePort.STP.CIST.State != "" {
				me.Status = child.BridgePort.STP.CIST.State
				me.StatusUp = me.Status == "forwarding"
			}
			result = append(result, me)
		}
	}

	return result
}

func makeIfaceEntry(iface ifaceJSON, fwdSet map[string]bool) ifaceEntry {
	e := ifaceEntry{
		Forwarding: fwdSet[iface.Name],
		Name:       iface.Name,
		Type:       prettyIfType(iface.Type),
		Status:     iface.OperStatus,
		StatusUp:   iface.OperStatus == "up",
		PhysAddr:   iface.PhysAddress,
	}

	if iface.Statistics != nil {
		e.RxBytes = humanBytes(int64(iface.Statistics.InOctets))
		e.TxBytes = humanBytes(int64(iface.Statistics.OutOctets))
	}

	if iface.IPv4 != nil {
		for _, a := range iface.IPv4.Address {
			e.Addresses = append(e.Addresses, addrEntry{
				Address: fmt.Sprintf("%s/%d", a.IP, int(a.PrefixLength)),
				Origin:  a.Origin,
			})
		}
	}
	if iface.IPv6 != nil {
		for _, a := range iface.IPv6.Address {
			e.Addresses = append(e.Addresses, addrEntry{
				Address: fmt.Sprintf("%s/%d", a.IP, int(a.PrefixLength)),
				Origin:  a.Origin,
			})
		}
	}

	if v := iface.Vlan; v != nil {
		if v.LowerLayerIf != "" {
			e.Detail = fmt.Sprintf("vid %d (%s)", v.ID, v.LowerLayerIf)
		} else {
			e.Detail = fmt.Sprintf("vid %d", v.ID)
		}
	}

	if iface.WiFi != nil {
		if ap := iface.WiFi.AccessPoint; ap != nil {
			n := len(ap.Stations.Station)
			e.Detail = fmt.Sprintf("AP, ssid: %s, stations: %d", ap.SSID, n)
		} else if st := iface.WiFi.Station; st != nil {
			e.Detail = fmt.Sprintf("Station, ssid: %s", st.SSID)
		}
	}

	if wg := iface.WireGuard; wg != nil && wg.PeerStatus != nil {
		total := len(wg.PeerStatus.Peer)
		up := 0
		for _, p := range wg.PeerStatus.Peer {
			if p.ConnectionStatus == "up" {
				up++
			}
		}
		e.Detail = fmt.Sprintf("%d peers (%d up)", total, up)
	}

	return e
}

// Template data for the interface detail page.
type ifaceDetailData struct {
	PageData
	Name string
	Type             string
	Status           string
	StatusUp         bool
	PhysAddr         string
	IfIndex          int
	MTU              int
	Speed            string
	Duplex           string
	AutoNeg          string
	Addresses        []addrEntry
	WiFiMode         string // "Access Point" or "Station"
	WiFiSSID         string
	WiFiSignal       string
	WiFiRxSpeed      string
	WiFiTxSpeed      string
	WiFiStationCount string // e.g. "3" for AP mode
	WGPeerSummary    string // e.g. "3 peers (2 up)"
	Counters         ifaceCounters
	EthFrameStats    []kvEntry
	WGPeers          []wgPeerEntry
	WiFiStations     []wifiStaEntry
	ScanResults      []wifiScanEntry
}

type ifaceCounters struct {
	RxBytes     string
	RxUnicast   string
	RxBroadcast string
	RxMulticast string
	RxDiscards  string
	RxErrors    string
	TxBytes     string
	TxUnicast   string
	TxBroadcast string
	TxMulticast string
	TxDiscards  string
	TxErrors    string
}

type kvEntry struct {
	Key   string
	Value string
}

type wgPeerEntry struct {
	PublicKey string
	Status    string
	StatusUp  bool
	Endpoint  string
	Handshake string
	TxBytes   string
	RxBytes   string
}

type wifiStaEntry struct {
	MAC       string
	Signal    string
	SignalCSS string // "excellent", "good", "poor", "bad"
	Time      string
	RxPkts    string
	TxPkts    string
	RxBytes   string
	TxBytes   string
	RxSpeed   string
	TxSpeed   string
}

type wifiScanEntry struct {
	SSID       string
	BSSID      string
	Signal     string
	SignalCSS  string
	Channel    string
	Encryption string
}

// fetchInterface retrieves a single interface by name from RESTCONF.
func (h *InterfacesHandler) fetchInterface(r *http.Request, name string) (*ifaceJSON, error) {
	var all interfacesWrapper
	if err := h.RC.Get(r.Context(), "/data/ietf-interfaces:interfaces", &all); err != nil {
		return nil, err
	}
	for i := range all.Interfaces.Interface {
		if all.Interfaces.Interface[i].Name == name {
			return &all.Interfaces.Interface[i], nil
		}
	}
	return nil, fmt.Errorf("interface %q not found", name)
}

// buildDetailData converts raw RESTCONF interface data to template data.
func buildDetailData(r *http.Request, iface *ifaceJSON) ifaceDetailData {
	d := ifaceDetailData{
		Name: iface.Name,
		Type:      prettyIfType(iface.Type),
		Status:    iface.OperStatus,
		StatusUp:  iface.OperStatus == "up",
		PhysAddr:  iface.PhysAddress,
		IfIndex:   iface.IfIndex,
	}

	if iface.IPv4 != nil {
		if iface.IPv4.MTU > 0 {
			d.MTU = iface.IPv4.MTU
		}
		for _, a := range iface.IPv4.Address {
			d.Addresses = append(d.Addresses, addrEntry{
				Address: fmt.Sprintf("%s/%d", a.IP, int(a.PrefixLength)),
				Origin:  a.Origin,
			})
		}
	}
	if iface.IPv6 != nil {
		for _, a := range iface.IPv6.Address {
			d.Addresses = append(d.Addresses, addrEntry{
				Address: fmt.Sprintf("%s/%d", a.IP, int(a.PrefixLength)),
				Origin:  a.Origin,
			})
		}
	}

	if iface.Ethernet != nil && prettyIfType(iface.Type) == ifTypeEthernet {
		d.Speed = prettySpeed(iface.Ethernet.Speed)
		d.Duplex = iface.Ethernet.Duplex
		if iface.Ethernet.AutoNegotiation != nil {
			if iface.Ethernet.AutoNegotiation.Enable {
				d.AutoNeg = "on"
			} else {
				d.AutoNeg = "off"
			}
		}
		if iface.Ethernet.Statistics != nil && iface.Ethernet.Statistics.Frame != nil {
			d.EthFrameStats = buildEthFrameStats(iface.Ethernet.Statistics.Frame)
		}
	}

	if iface.Statistics != nil {
		d.Counters = buildCounters(iface.Statistics)
	}

	if iface.WiFi != nil {
		if ap := iface.WiFi.AccessPoint; ap != nil {
			d.WiFiMode = "Access Point"
			d.WiFiSSID = ap.SSID
			d.WiFiStationCount = fmt.Sprintf("%d", len(ap.Stations.Station))
			for _, s := range ap.Stations.Station {
				d.WiFiStations = append(d.WiFiStations, buildWifiStaEntry(s))
			}
		} else if st := iface.WiFi.Station; st != nil {
			d.WiFiMode = "Station"
			d.WiFiSSID = st.SSID
			if st.SignalStrength != nil {
				d.WiFiSignal = fmt.Sprintf("%d dBm", *st.SignalStrength)
			}
			if st.RxSpeed > 0 {
				d.WiFiRxSpeed = fmt.Sprintf("%.1f Mbps", float64(st.RxSpeed)/10)
			}
			if st.TxSpeed > 0 {
				d.WiFiTxSpeed = fmt.Sprintf("%.1f Mbps", float64(st.TxSpeed)/10)
			}
			for _, sr := range st.ScanResults {
				d.ScanResults = append(d.ScanResults, buildWifiScanEntry(sr))
			}
		}
	}

	if wg := iface.WireGuard; wg != nil && wg.PeerStatus != nil {
		total := len(wg.PeerStatus.Peer)
		up := 0
		for _, p := range wg.PeerStatus.Peer {
			pe := wgPeerEntry{
				PublicKey: p.PublicKey,
				Status:    p.ConnectionStatus,
				StatusUp:  p.ConnectionStatus == "up",
			}
			if p.EndpointAddress != "" {
				pe.Endpoint = fmt.Sprintf("%s:%d", p.EndpointAddress, p.EndpointPort)
			}
			if p.LatestHandshake != "" {
				pe.Handshake = p.LatestHandshake
			}
			if p.Transfer != nil {
				pe.TxBytes = humanBytes(int64(p.Transfer.TxBytes))
				pe.RxBytes = humanBytes(int64(p.Transfer.RxBytes))
			}
			if p.ConnectionStatus == "up" {
				up++
			}
			d.WGPeers = append(d.WGPeers, pe)
		}
		d.WGPeerSummary = fmt.Sprintf("%d peers (%d up)", total, up)
	}

	return d
}

func buildCounters(s *ifaceStats) ifaceCounters {
	return ifaceCounters{
		RxBytes:     humanBytes(int64(s.InOctets)),
		RxUnicast:   formatCount(int64(s.InUnicastPkts)),
		RxBroadcast: formatCount(int64(s.InBroadcastPkts)),
		RxMulticast: formatCount(int64(s.InMulticastPkts)),
		RxDiscards:  formatCount(int64(s.InDiscards)),
		RxErrors:    formatCount(int64(s.InErrors)),
		TxBytes:     humanBytes(int64(s.OutOctets)),
		TxUnicast:   formatCount(int64(s.OutUnicastPkts)),
		TxBroadcast: formatCount(int64(s.OutBroadcastPkts)),
		TxMulticast: formatCount(int64(s.OutMulticastPkts)),
		TxDiscards:  formatCount(int64(s.OutDiscards)),
		TxErrors:    formatCount(int64(s.OutErrors)),
	}
}

func buildEthFrameStats(f *ethFrameStats) []kvEntry {
	return []kvEntry{
		{"eth-in-frames", formatCount(int64(f.InTotalPkts))},
		{"eth-in-octets", humanBytes(int64(f.InTotalOctets))},
		{"eth-in-good-frames", formatCount(int64(f.InGoodPkts))},
		{"eth-in-good-octets", humanBytes(int64(f.InGoodOctets))},
		{"eth-in-broadcast", formatCount(int64(f.InBroadcast))},
		{"eth-in-multicast", formatCount(int64(f.InMulticast))},
		{"eth-in-fcs-error", formatCount(int64(f.InErrorFCS))},
		{"eth-in-undersize", formatCount(int64(f.InErrorUndersize))},
		{"eth-in-oversize", formatCount(int64(f.InErrorOversize))},
		{"eth-in-mac-error", formatCount(int64(f.InErrorMACInternal))},
		{"eth-out-frames", formatCount(int64(f.OutTotalPkts))},
		{"eth-out-octets", humanBytes(int64(f.OutTotalOctets))},
		{"eth-out-good-frames", formatCount(int64(f.OutGoodPkts))},
		{"eth-out-good-octets", humanBytes(int64(f.OutGoodOctets))},
		{"eth-out-broadcast", formatCount(int64(f.OutBroadcast))},
		{"eth-out-multicast", formatCount(int64(f.OutMulticast))},
	}
}

// prettySpeed converts YANG ethernet speed identities to display strings.
func prettySpeed(s string) string {
	if i := strings.LastIndex(s, ":"); i >= 0 {
		s = s[i+1:]
	}
	return s
}

func buildWifiStaEntry(s wifiStaJSON) wifiStaEntry {
	e := wifiStaEntry{
		MAC:     s.MACAddress,
		Time:    formatDuration(int64(s.ConnectedTime)),
		RxPkts:  formatCount(int64(s.RxPackets)),
		TxPkts:  formatCount(int64(s.TxPackets)),
		RxBytes: humanBytes(int64(s.RxBytes)),
		TxBytes: humanBytes(int64(s.TxBytes)),
		RxSpeed: fmt.Sprintf("%.1f Mbps", float64(s.RxSpeed)/10),
		TxSpeed: fmt.Sprintf("%.1f Mbps", float64(s.TxSpeed)/10),
	}
	if s.SignalStrength != nil {
		sig := *s.SignalStrength
		e.Signal = fmt.Sprintf("%d dBm", sig)
		switch {
		case sig >= -50:
			e.SignalCSS = "excellent"
		case sig >= -60:
			e.SignalCSS = "good"
		case sig >= -70:
			e.SignalCSS = "poor"
		default:
			e.SignalCSS = "bad"
		}
	}
	return e
}

func buildWifiScanEntry(sr wifiScanResultJSON) wifiScanEntry {
	e := wifiScanEntry{
		SSID:    sr.SSID,
		BSSID:   sr.BSSID,
		Channel: fmt.Sprintf("%d", sr.Channel),
	}
	if len(sr.Encryption) > 0 {
		e.Encryption = strings.Join(sr.Encryption, ", ")
	} else {
		e.Encryption = "Open"
	}
	if sr.SignalStrength != nil {
		sig := *sr.SignalStrength
		e.Signal = fmt.Sprintf("%d dBm", sig)
		switch {
		case sig >= -50:
			e.SignalCSS = "excellent"
		case sig >= -60:
			e.SignalCSS = "good"
		case sig >= -70:
			e.SignalCSS = "poor"
		default:
			e.SignalCSS = "bad"
		}
	}
	return e
}

func formatDuration(secs int64) string {
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	if secs < 3600 {
		return fmt.Sprintf("%dm %ds", secs/60, secs%60)
	}
	h := secs / 3600
	m := (secs % 3600) / 60
	return fmt.Sprintf("%dh %dm", h, m)
}

// formatCount formats a packet/frame count with thousand separators.
func formatCount(n int64) string {
	if n == 0 {
		return "0"
	}
	s := fmt.Sprintf("%d", n)
	// Insert thousand separators from the right.
	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}

// Detail renders the interface detail page (GET /interfaces/{name}).
func (h *InterfacesHandler) Detail(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	iface, err := h.fetchInterface(r, name)
	if err != nil {
		log.Printf("restconf interface %s: %v", name, err)
		http.Error(w, "Interface not found", http.StatusNotFound)
		return
	}

	data := buildDetailData(r, iface)
	data.PageData = newPageData(r, "interfaces", "Interface "+name)

	tmplName := "iface-detail.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.DetailTemplate.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// Counters renders the counters fragment for htmx polling (GET /interfaces/{name}/counters).
func (h *InterfacesHandler) Counters(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	iface, err := h.fetchInterface(r, name)
	if err != nil {
		log.Printf("restconf interface %s counters: %v", name, err)
		http.Error(w, "Interface not found", http.StatusNotFound)
		return
	}

	data := buildDetailData(r, iface)

	if err := h.CountersTemplate.ExecuteTemplate(w, "iface-counters", data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// freqToChannel converts a WiFi center frequency (MHz) to a channel number.
func freqToChannel(freq int) int {
	switch {
	case freq == 2484:
		return 14
	case freq >= 2412 && freq <= 2472:
		return (freq - 2407) / 5
	case freq >= 5180 && freq <= 5885:
		return (freq - 5000) / 5
	case freq >= 5955 && freq <= 7115:
		return (freq - 5950) / 5
	default:
		return 0
	}
}

// renderSurveySVG generates an inline SVG bar chart visualizing WiFi channel
// survey data.  Each channel gets a stacked bar showing receive, transmit,
// and other busy time as a percentage of active time.  A dashed noise-floor
// line is overlaid with a right-side dBm axis.  The in-use channel is marked
// with a triangle.
func renderSurveySVG(channels []surveyChanJSON) template.HTML {
	n := len(channels)
	if n == 0 {
		return ""
	}

	sorted := make([]surveyChanJSON, n)
	copy(sorted, channels)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Frequency < sorted[j].Frequency
	})

	// Layout constants.
	const chartH = 200
	padL, padR, padT, padB := 44, 48, 28, 58

	slotW := 600.0 / float64(n)
	if slotW > 44 {
		slotW = 44
	}
	if slotW < 16 {
		slotW = 16
	}
	barW := slotW * 0.65
	chartW := slotW * float64(n)
	svgW := int(chartW) + padL + padR
	svgH := chartH + padT + padB

	// Noise range for right axis.
	hasNoise := false
	noiseMin, noiseMax := 0, 0
	for i, ch := range sorted {
		if ch.Noise != 0 {
			if !hasNoise {
				noiseMin, noiseMax = ch.Noise, ch.Noise
				hasNoise = true
			}
			if ch.Noise < noiseMin {
				noiseMin = ch.Noise
			}
			if ch.Noise > noiseMax {
				noiseMax = ch.Noise
			}
		}
		_ = i
	}
	nFloor := int(math.Floor(float64(noiseMin)/5))*5 - 5
	nCeil := int(math.Ceil(float64(noiseMax)/5))*5 + 5
	nRange := float64(nCeil - nFloor)
	if nRange == 0 {
		nRange = 10
	}

	var b strings.Builder

	fmt.Fprintf(&b, `<svg viewBox="0 0 %d %d" xmlns="http://www.w3.org/2000/svg" class="survey-chart">`,
		svgW, svgH)

	// Y-axis grid lines and labels (utilization %).
	for _, pct := range []int{0, 25, 50, 75, 100} {
		y := padT + chartH - pct*chartH/100
		fmt.Fprintf(&b, `<line x1="%d" y1="%d" x2="%.0f" y2="%d" stroke="#e5e7eb" stroke-width="1"/>`,
			padL, y, float64(padL)+chartW, y)
		fmt.Fprintf(&b, `<text x="%d" y="%d" text-anchor="end" fill="#9ca3af" font-size="10">%d%%</text>`,
			padL-4, y+4, pct)
	}

	// Right Y-axis labels (noise dBm).
	if hasNoise {
		nMid := (nFloor + nCeil) / 2
		for _, db := range []int{nFloor + 5, nMid, nCeil - 5} {
			ny := float64(padT+chartH) - float64(db-nFloor)/nRange*float64(chartH)
			fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" text-anchor="start" `+
				`fill="#ef4444" font-size="10" opacity="0.8">%d</text>`,
				float64(padL)+chartW+4, ny+3, db)
		}
	}

	// Draw bars and collect noise line points.
	var noisePts []string

	for i, ch := range sorted {
		cx := float64(padL) + float64(i)*slotW + slotW/2
		bx := cx - barW/2

		var rxPct, txPct, otherPct float64
		if ch.ActiveTime > 0 {
			act := float64(ch.ActiveTime)
			rxPct = float64(ch.ReceiveTime) / act * 100
			txPct = float64(ch.TransmitTime) / act * 100
			otherPct = float64(ch.BusyTime)/act*100 - rxPct - txPct
			if otherPct < 0 {
				otherPct = 0
			}
			if total := rxPct + txPct + otherPct; total > 100 {
				s := 100 / total
				rxPct *= s
				txPct *= s
				otherPct *= s
			}
		}

		baseY := float64(padT + chartH)

		// Stacked: other busy (bottom), transmit (middle), receive (top).
		if otherPct > 0.5 {
			h := otherPct / 100 * float64(chartH)
			fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="#f59e0b" rx="1.5"/>`,
				bx, baseY-h, barW, h)
			baseY -= h
		}
		if txPct > 0.5 {
			h := txPct / 100 * float64(chartH)
			fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="#22c55e" rx="1.5"/>`,
				bx, baseY-h, barW, h)
			baseY -= h
		}
		if rxPct > 0.5 {
			h := rxPct / 100 * float64(chartH)
			fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="#3b82f6" rx="1.5"/>`,
				bx, baseY-h, barW, h)
		}

		// In-use marker (triangle above bar).
		if ch.InUse {
			totalPct := rxPct + txPct + otherPct
			topY := float64(padT+chartH) - totalPct/100*float64(chartH)
			fmt.Fprintf(&b, `<polygon points="%.1f,%.1f %.1f,%.1f %.1f,%.1f" fill="#2563eb"/>`,
				cx, topY-3, cx-4, topY-11, cx+4, topY-11)
		}

		// Channel label on X-axis.
		chNum := freqToChannel(ch.Frequency)
		label := fmt.Sprintf("%d", chNum)
		if chNum == 0 {
			label = fmt.Sprintf("%d", ch.Frequency)
		}
		fmt.Fprintf(&b, `<text x="%.1f" y="%d" text-anchor="middle" fill="#6b7280" font-size="10">%s</text>`,
			cx, padT+chartH+14, label)

		// Noise line point.
		if hasNoise && ch.Noise != 0 {
			ny := float64(padT+chartH) - float64(ch.Noise-nFloor)/nRange*float64(chartH)
			noisePts = append(noisePts, fmt.Sprintf("%.1f,%.1f", cx, ny))
		}
	}

	// Draw noise floor line.
	if len(noisePts) > 1 {
		fmt.Fprintf(&b, `<polyline points="%s" fill="none" stroke="#ef4444" `+
			`stroke-width="1.5" stroke-dasharray="4,3" opacity="0.7"/>`,
			strings.Join(noisePts, " "))
		for _, pt := range noisePts {
			parts := strings.SplitN(pt, ",", 2)
			fmt.Fprintf(&b, `<circle cx="%s" cy="%s" r="2.5" fill="#ef4444" opacity="0.7"/>`,
				parts[0], parts[1])
		}
	}

	// Legend row.
	ly := svgH - 8
	lx := float64(padL)

	for _, item := range []struct{ color, label string }{
		{"#3b82f6", "Rx"},
		{"#22c55e", "Tx"},
		{"#f59e0b", "Other"},
	} {
		fmt.Fprintf(&b, `<rect x="%.0f" y="%d" width="10" height="10" fill="%s" rx="1.5"/>`,
			lx, ly-9, item.color)
		fmt.Fprintf(&b, `<text x="%.0f" y="%d" fill="#6b7280" font-size="10">%s</text>`,
			lx+13, ly, item.label)
		lx += 13 + float64(len(item.label))*7 + 10
	}

	if hasNoise {
		fmt.Fprintf(&b, `<line x1="%.0f" y1="%d" x2="%.0f" y2="%d" `+
			`stroke="#ef4444" stroke-width="1.5" stroke-dasharray="4,3" opacity="0.7"/>`,
			lx, ly-4, lx+14, ly-4)
		lx += 18
		fmt.Fprintf(&b, `<text x="%.0f" y="%d" fill="#ef4444" font-size="10" opacity="0.8">Noise (dBm)</text>`,
			lx, ly)
		lx += 80
	}

	// In-use legend marker.
	fmt.Fprintf(&b, `<polygon points="%.0f,%d %.0f,%d %.0f,%d" fill="#2563eb"/>`,
		lx+5, ly-9, lx+1, ly-1, lx+9, ly-1)
	fmt.Fprintf(&b, `<text x="%.0f" y="%d" fill="#6b7280" font-size="10">In use</text>`,
		lx+14, ly)

	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
