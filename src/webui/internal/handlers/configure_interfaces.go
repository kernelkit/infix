// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"crypto/ecdh"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"

	"infix/webui/internal/restconf"
	"infix/webui/internal/schema"
)

const ifaceCandPath = candidatePath + "/ietf-interfaces:interfaces"

// ─── RESTCONF JSON structs (configure-only fields) ───────────────────────────

type bridgeCfgJSON struct {
	IEEEGroupFwd []string             `json:"ieee-group-forward"`
	VLANs        *vlanFilterCfgJSON   `json:"vlans"` // non-nil means 802.1Q mode
	STP          bridgeSTPCfgJSON     `json:"stp"`
	Multicast    *bridgeMulticastJSON `json:"multicast"`
}

type bridgeMulticastJSON struct {
	Snooping      *bool  `json:"snooping"`
	Querier       string `json:"querier"`
	QueryInterval *int   `json:"query-interval"`
}

type vlanFilterCfgJSON struct {
	Proto string        `json:"proto"`
	VLANs []bridgeVLAN  `json:"vlan"`
}

type bridgeVLAN struct {
	VID      int      `json:"vid"`
	Untagged []string `json:"untagged"`
	Tagged   []string `json:"tagged"`
}

type bridgeSTPCfgJSON struct {
	ForceProtocol     string `json:"force-protocol"`
	HelloTime         *int   `json:"hello-time"`
	ForwardDelay      *int   `json:"forward-delay"`
	MaxAge            *int   `json:"max-age"`
	TransmitHoldCount *int   `json:"transmit-hold-count"`
	MaxHops           *int   `json:"max-hops"`
}

type lagCfgJSON struct {
	Mode string       `json:"mode"`
	Hash string       `json:"hash"`
	LACP lagLACPJSON  `json:"lacp"`
}

type lagLACPJSON struct {
	Mode           string `json:"mode"`
	Rate           string `json:"rate"`
	SystemPriority *int   `json:"system-priority"`
}

type lagPortCfgJSON struct {
	LAG string `json:"lag"`
}

// ─── Template display types ───────────────────────────────────────────────────

type cfgVLANRow struct {
	VID         int
	UntaggedTxt string
	TaggedTxt   string
	UntaggedSet map[string]bool
	TaggedSet   map[string]bool
}

type cfgIfaceRow struct {
	ifaceJSON
	TypeSlug        string
	TypeDisplay     string
	AdminEnabled    bool // true when enabled leaf absent (YANG default) or explicitly true
	MemberOf        string // bridge or lag name this interface belongs to
	AddrSummary     string
	ConfigTags      []string        // type-aware overview pills (DHCP, SLAAC, vid N, …)
	BridgeMembers   []string        // interface names that are ports of this bridge/lag
	BridgeMemberSet map[string]bool // for checkbox pre-selection
	PortCandidates  []string        // free ports + current members of this bridge/lag
	BridgeIs8021Q   bool
	VLANRows        []cfgVLANRow
	IsBridge        bool
	IsBridgePort    bool
	IsLag           bool
	IsLagPort       bool
	IsVlan          bool
	IsWifi          bool
	WifiMode        string // "station" or "access-point" once known
	HasIP           bool // can carry IP addresses
	// ParentBridgeIs8021Q says whether the bridge this port is attached
	// to has VLAN filtering on. PVID only makes sense in that mode, so
	// the bridge-port editor hides the field when this is false.
	ParentBridgeIs8021Q bool
	// PortRadio is a snapshot of the wifi-radio component mirrored into
	// the interface editor so the user can adjust country/band/channel
	// from the WiFi interface page without bouncing to Configure >
	// Hardware. Nil when the wifi/radio reference is empty or unknown.
	PortRadio *ifaceRadioMirror
	// MDIXState renders the *bool ethernet/mdi-x as a template-friendly
	// string: "" = absent (Auto-MDIX), "true" / "false" = explicit force.
	MDIXState     string
	EthAutoneg    bool     // current candidate value; defaults to YANG default (true)
	EthDuplex     string   // "" / "full" / "half"
	EthAdvertised []string // identityref leaf-list, empty = advertise all
	EthSupported  []string // identityref leaf-list from operational data
	// DHCP enabled flags — captured BEFORE the placeholder DHCP/DHCPv6
	// containers are seeded so the template can tell "configured" from
	// "auto-injected placeholder" apart.
	DHCPv4Enabled bool
	DHCPv6Enabled bool

	// Page-level bits copied onto each row so the per-interface IPv4/IPv6
	// blocks can render as standalone fragments — both inline on the full
	// page and on their own when a save handler re-renders just one block
	// (to surface confd-inferred values without collapsing the page).
	Desc          map[string]string
	DHCPv4Options []schema.IdentityOption
	DHCPv6Options []schema.IdentityOption
	// JustSaved marks a post-save re-render (renderIPBlock) so the DHCP
	// foldout auto-expands to reveal the freshly-inferred options.  False
	// on the normal page render, so foldouts stay collapsed there.
	JustSaved bool
}

// ifaceRadioMirror is the subset of wifi-radio fields we expose on the
// WiFi interface editor. The form posts to /configure/hardware/wifi/{name}
// so the backend stays single-sourced.
type ifaceRadioMirror struct {
	Name        string
	CountryCode string
	Band        string
	Channel     *int
}

type cfgIfacePageData struct {
	PageData
	Loading         bool
	Interfaces      []cfgIfaceRow
	AllNames        []string // every interface name (running config)
	BridgeNames     []string // type=bridge only
	LagNames        []string // type=lag only
	// UnconfiguredPhysical lists physical Ethernet interfaces present in
	// operational state but NOT in running config — i.e. ports that have
	// been deleted and can be re-bound. Shown as datalist suggestions on
	// the Add Interface row so users don't have to remember the name.
	UnconfiguredPhysical []string
	Desc            map[string]string

	// Add Interface modal — pre-computed once per page render so the
	// dialog has no per-tile-click latency. WizardTypes feeds the type
	// pulldown; WizardNames[slug] pre-fills each per-type fieldset's
	// name input (veth peer under key "veth-peer"); FreePortCandidates
	// is the bridge/lag ports checkbox list; VlanDefaultName is the
	// initial value of the VLAN fieldset's name input
	// (<first-parent>.1) which JS keeps in sync when parent/vid change.
	// WizardAsymKeys is the keystore /asymmetric-keys name list for the
	// WireGuard private-key picker.
	WizardTypes        []ifaceTypeOption
	WizardNames        map[string]string
	FreePortCandidates []string
	VlanDefaultName    string
	WizardAsymKeys        []string      // for WireGuard private-key picker
	WizardSymKeys         []symKeyEntry // for WiFi PSK picker (Name + decoded cleartext)
	WizardWifiRadios      []wifiRadioOption // configured (in candidate)
	WizardAvailableRadios []string          // detected (operational) but not yet in candidate
	WizardCountryOptions  []schema.IdentityOption
	WizardBandOptions     []schema.IdentityOption

	STPProtoOptions []schema.IdentityOption
	LagModeOptions  []schema.IdentityOption
	LagHashOptions  []schema.IdentityOption
	DHCPv4Options   []schema.IdentityOption
	DHCPv6Options   []schema.IdentityOption
	// MCRouterOptions populates the per-bridge-port multicast-router
	// select. Schema-driven; falls back to a tiny hardcoded list when
	// the schema lookup misses (e.g. older YANG).
	MCRouterOptions   []schema.IdentityOption
	WiFiSecOptionsAP  []schema.IdentityOption
	WiFiSecOptionsSta []schema.IdentityOption
	WiFiQuerierOptions []schema.IdentityOption
	// CountryOptions / BandOptions reused by the mirrored radio editor
	// on WiFi interface rows (same enums as the wizard).
	CountryOptions []schema.IdentityOption
	BandOptions    []schema.IdentityOption
	// PSKKeys is the keystore symmetric-keys list for the WiFi security
	// secret picker (mirrors WizardSymKeys without the cleartext).
	PSKKeys []symKeyEntry
	Error   string
}

// ─── Handler ─────────────────────────────────────────────────────────────────

// ConfigureInterfacesHandler serves the Configure > Interfaces page.
type ConfigureInterfacesHandler struct {
	Template *template.Template
	RC       restconf.Fetcher
	Schema   *schema.Cache
}

// Overview renders the Configure > Interfaces page.
// GET /configure/interfaces
func (h *ConfigureInterfacesHandler) Overview(w http.ResponseWriter, r *http.Request) {
	data := cfgIfacePageData{
		PageData: newPageData(w, r, "configure-interfaces", "Interfaces"),
	}

	mgr := h.Schema.Manager()
	data.Loading = mgr == nil
	if mgr != nil {
		// The list-entry path is /ietf-interfaces:interfaces/interface —
		// the singular form returns nil from entryAt() and silently
		// empties every DescriptionOf and OptionsFor below, which is
		// why no field-info (i) icons rendered before this change.
		ifPath := "/ietf-interfaces:interfaces/interface"
		bPath := "/infix-interfaces:bridge"
		lPath := "/infix-interfaces:lag"
		ip4 := "/ietf-ip:ipv4"
		ip6 := "/ietf-ip:ipv6"
		data.Desc = map[string]string{
			"name":           descOr(mgr, ifPath+"/name", "The unique name for this interface. Pick one not already in use."),
			"description":    schema.DescriptionOf(mgr, ifPath+"/description"),
			"type":           schema.DescriptionOf(mgr, ifPath+"/type"),
			"enabled":        schema.DescriptionOf(mgr, ifPath+"/enabled"),
			"mac":            descOr(mgr, ifPath+"/infix-interfaces:custom-phys-address/static", "Override the interface's default physical (MAC) address with a static unicast value."),
			"bridge-type":    descOr(mgr, ifPath+bPath+"/vlans", "Presence of bridge/vlans switches the bridge into IEEE 802.1Q VLAN-filtering mode. Pick this if downstream ports need PVID and tagged/untagged membership."),
			"stp-force":      schema.DescriptionOf(mgr, ifPath+bPath+"/stp/force-protocol"),
			"stp-hello":      schema.DescriptionOf(mgr, ifPath+bPath+"/stp/hello-time"),
			"stp-fwd-delay":  schema.DescriptionOf(mgr, ifPath+bPath+"/stp/forward-delay"),
			"stp-max-age":    schema.DescriptionOf(mgr, ifPath+bPath+"/stp/max-age"),
			"stp-hold-count": schema.DescriptionOf(mgr, ifPath+bPath+"/stp/transmit-hold-count"),
			"stp-max-hops":   schema.DescriptionOf(mgr, ifPath+bPath+"/stp/max-hops"),
			"lag-mode":       descOr(mgr, ifPath+lPath+"/mode", "Aggregation mode: static (balanced XOR) or lacp (IEEE 802.3ad)."),
			"lag-hash":       schema.DescriptionOf(mgr, ifPath+lPath+"/hash"),
			"lacp-mode":      schema.DescriptionOf(mgr, ifPath+lPath+"/lacp/mode"),
			"lacp-rate":      schema.DescriptionOf(mgr, ifPath+lPath+"/lacp/rate"),
			"lacp-sysprio":   schema.DescriptionOf(mgr, ifPath+lPath+"/lacp/system-priority"),
			"vlan-id":        descOr(mgr, ifPath+"/infix-interfaces:vlan/id", "VLAN tag (1–4094) carried on the parent interface."),
			"vlan-lower":     descOr(mgr, ifPath+"/infix-interfaces:vlan/lower-layer-if", "Parent (lower-layer) interface on which the VLAN sub-interface sits."),
			"ipv4-address":   schema.DescriptionOf(mgr, ifPath+ip4+"/address/ip"),
			"ipv4-prefix":    schema.DescriptionOf(mgr, ifPath+ip4+"/address/prefix-length"),
			"ipv4-dhcp":      schema.DescriptionOf(mgr, ifPath+ip4+"/infix-dhcp-client:dhcp"),
			"ipv4-autoconf":  schema.DescriptionOf(mgr, ifPath+ip4+"/infix-ip:autoconf"),
			"ipv4-forwarding": schema.DescriptionOf(mgr, ifPath+ip4+"/forwarding"),
			"ipv6-address":   schema.DescriptionOf(mgr, ifPath+ip6+"/address/ip"),
			"ipv6-prefix":    schema.DescriptionOf(mgr, ifPath+ip6+"/address/prefix-length"),
			"ipv6-slaac":     schema.DescriptionOf(mgr, ifPath+ip6+"/autoconf"),
			"ipv6-dhcp":      schema.DescriptionOf(mgr, ifPath+ip6+"/infix-dhcpv6-client:dhcp"),
			"ipv6-forwarding": schema.DescriptionOf(mgr, ifPath+ip6+"/forwarding"),

			// Add Interface wizard — augment paths still don't resolve
			// through goyang (same gap as LAG mode), so each entry is
			// wrapped in descOr() with a fallback string taken straight
			// from the YANG description text. Drop the fallback once
			// the schema layer learns to traverse augments.
			"bridge-port-bridge": descOr(mgr, ifPath+"/infix-interfaces:bridge-port/bridge", "Interfaces enslaved to this bridge. Each becomes a member port carrying L2 traffic for the bridge."),
			"veth-peer":          descOr(mgr, ifPath+"/infix-interfaces:veth/peer", "Peer veth interface to which this interface is connected (the other end of the pair)."),
			"gre-local":          descOr(mgr, ifPath+"/infix-interfaces:gre/local", "Local address used as the tunnel source. Must be the same family (IPv4 or IPv6) as the remote."),
			"gre-remote":         descOr(mgr, ifPath+"/infix-interfaces:gre/remote", "Remote peer address for the tunnel. Must be the same family (IPv4 or IPv6) as the local."),
			"gre-pmtu":           descOr(mgr, ifPath+"/infix-interfaces:gre/pmtu-discovery", "Enable Path MTU Discovery (default). Disable on links with broken ICMP filtering — at the cost of suboptimal performance."),
			"vxlan-local":        descOr(mgr, ifPath+"/infix-interfaces:vxlan/local", "Local address used as the tunnel source. Must be the same family (IPv4 or IPv6) as the remote."),
			"vxlan-remote":       descOr(mgr, ifPath+"/infix-interfaces:vxlan/remote", "Remote unicast or multicast address for the VXLAN overlay."),
			"vxlan-vni":          descOr(mgr, ifPath+"/infix-interfaces:vxlan/vni", "VXLAN Network Identifier (0–16777215). Pick a unique VNI per overlay segment."),
			"vxlan-port":         descOr(mgr, ifPath+"/infix-interfaces:vxlan/remote-port", "Destination UDP port. IANA-assigned VXLAN port is 4789 (default)."),
			"wg-key":             descOr(mgr, ifPath+"/infix-interfaces:wireguard/private-key", "Reference to the WireGuard private key (X25519/Curve25519) stored in the keystore."),
			"wg-port":            descOr(mgr, ifPath+"/infix-interfaces:wireguard/listen-port", "Local UDP port to listen on for incoming WireGuard traffic (default 51820)."),
			"wifi-radio":         descOr(mgr, ifPath+"/infix-interfaces:wifi/radio", "Parent WiFi radio (hardware component, class=wifi). Configure the radio's band, channel, and country code in Configure › Hardware first."),
			"wifi-mode":          "Station (client) connects to an existing AP. Access Point creates a network that clients join. Only one Station per radio; multiple APs per radio supported.",
			"wifi-ssid":          descOr(mgr, ifPath+"/infix-interfaces:wifi/station/ssid", "WiFi network name (1–32 characters). Case-sensitive; must match the target network for Station mode."),
			"wifi-sec-mode":      descOr(mgr, ifPath+"/infix-interfaces:wifi/access-point/security/mode", "Security mode. Open is unencrypted (insecure). For AP: wpa2-wpa3-personal is recommended for compatibility + security."),
			"wifi-secret":        descOr(mgr, ifPath+"/infix-interfaces:wifi/access-point/security/secret", "Pre-shared key reference — a symmetric key in the keystore. 8–63 characters per the WPA spec."),
			"wifi-hidden":        descOr(mgr, ifPath+"/infix-interfaces:wifi/access-point/hidden", "Hide the SSID from broadcast beacons. Minimal security benefit and may cause compatibility issues with some clients."),
			"eth-autoneg":     descOr(mgr, ifPath+"/ieee802-ethernet-interface:ethernet/auto-negotiation/enable", "Enable IEEE 802.3 auto-negotiation.  When off, the link must come up via parallel detection or the forced PMD picked below."),
			"eth-advertised":  descOr(mgr, ifPath+"/ieee802-ethernet-interface:ethernet/auto-negotiation/infix-ethernet-interface:advertised-pmd-types", "Restrict auto-negotiation to advertise only these PMD types.  Leave empty to advertise every mode the PHY supports."),
			"eth-duplex":      descOr(mgr, ifPath+"/ieee802-ethernet-interface:ethernet/duplex", "Force half- or full-duplex.  Leave on Auto to let auto-negotiation pick.  Modern PMDs are full-duplex only."),
			"eth-mdix":        descOr(mgr, ifPath+"/ieee802-ethernet-interface:ethernet/infix-ethernet-interface:mdi-x", "Force the copper MDI/MDI-X crossover pinout.  Leave on Auto-MDIX (default) for any link that negotiates.  Force MDI/MDI-X only when negotiation is disabled and the two ends must use opposite values."),
			"bp-bridge":       descOr(mgr, ifPath+"/infix-interfaces:bridge-port/bridge", "Bridge that this port joins as a member, carrying L2 traffic on its behalf."),
			"bp-pvid":         descOr(mgr, ifPath+"/infix-interfaces:bridge-port/pvid", "Port VLAN ID — VLAN assigned to untagged frames arriving on this port. Only meaningful when the parent bridge is in IEEE 802.1Q VLAN-filtering mode."),
			"bp-flood":        descOr(mgr, ifPath+"/infix-interfaces:bridge-port/flood", "Per-traffic-class control of how unknown destinations are flooded out this port. Unticking suppresses flooding of that traffic class."),
			"bp-mc-router":    descOr(mgr, ifPath+"/infix-interfaces:bridge-port/multicast/router", "Multicast router behaviour on this port. Auto (default) lets IGMP/MLD snooping decide; permanent forces the port to always receive multicast; off blocks it."),
			"bp-mc-fast-leave": descOr(mgr, ifPath+"/infix-interfaces:bridge-port/multicast/fast-leave", "Drop the port from a multicast group immediately on IGMP/MLD leave instead of waiting for the next query. Suitable when each port has at most one receiver."),
			"lp-lag":          descOr(mgr, ifPath+"/infix-interfaces:lag-port/lag", "LAG that this port joins as a slave. The LAG itself is configured on its own row."),
			"mc-snoop":        descOr(mgr, ifPath+bPath+"/multicast/snooping", "Enable IGMP/MLD snooping on the bridge so multicast is forwarded only to ports with active receivers."),
			"mc-querier":      descOr(mgr, ifPath+bPath+"/multicast/querier", "Querier role: auto (only when no other querier is heard), on (always send queries), or off."),
			"mc-query-int":    descOr(mgr, ifPath+bPath+"/multicast/query-interval", "Interval (seconds) between IGMP/MLD general queries when this bridge is acting as querier."),
		}
		if data.Desc["ipv6-slaac"] == "" {
			data.Desc["ipv6-slaac"] = "SLAAC (Stateless Address Autoconfiguration, RFC 4862) " +
				"automatically assigns an IPv6 address using the network prefix " +
				"advertised by the local router — no DHCPv6 server required."
		}
		if data.Desc["ipv4-autoconf"] == "" {
			data.Desc["ipv4-autoconf"] = "Link-Local / Zeroconf (RFC 3927) automatically assigns " +
				"a 169.254.x.x address when no DHCP server is reachable, enabling " +
				"local network communication without any manual configuration."
		}
		data.STPProtoOptions = schema.OptionsFor(mgr, ifPath+bPath+"/stp/force-protocol")
		data.LagModeOptions = schema.OptionsFor(mgr, ifPath+lPath+"/mode")
		if len(data.LagModeOptions) == 0 {
			// FIXME: schema augment lookup for /ietf-interfaces:interfaces/
			// interface/infix-interfaces:lag/mode returns empty (augment
			// targets aren't being merged into the goyang Entry tree).
			// Fall back to the YANG-defined lag-mode enum so the Add
			// Interface wizard's LAG fieldset isn't unusable.
			data.LagModeOptions = []schema.IdentityOption{
				{Value: "static", Label: "static"},
				{Value: "lacp", Label: "lacp"},
			}
		}
		data.LagHashOptions = schema.OptionsFor(mgr, ifPath+lPath+"/hash")
		data.DHCPv4Options = schema.OptionsFor(mgr, ifPath+ip4+"/infix-dhcp-client:dhcp/option/id")
		data.DHCPv6Options = schema.OptionsFor(mgr, ifPath+ip6+"/infix-dhcpv6-client:dhcp/option/id")
		data.WizardTypes = buildIfaceTypeList(mgr)
		// WiFi-radio enums sourced from infix-hardware:wifi-radio. Used
		// by the inline "+ New radio" form in the WiFi fieldset and by
		// the WiFi interface row's mirrored radio editor.
		const radioSchemaPath = "/ietf-hardware:hardware/component/infix-hardware:wifi-radio"
		data.WizardCountryOptions = schema.OptionsFor(mgr, radioSchemaPath+"/country-code")
		data.WizardBandOptions = schema.OptionsFor(mgr, radioSchemaPath+"/band")
		data.CountryOptions = data.WizardCountryOptions
		data.BandOptions = data.WizardBandOptions
		// Cross-module typedef enums (iwcc:country-code, wifi-band)
		// don't always resolve through goyang's Entry tree, so the
		// schema lookup can return empty. Fall back to a small set so
		// the WiFi radio editor at least shows pickable values.
		if len(data.CountryOptions) == 0 {
			for _, cc := range []string{"00", "US", "GB", "DE", "SE", "FI", "JP", "AU"} {
				data.CountryOptions = append(data.CountryOptions, schema.IdentityOption{Value: cc, Label: cc})
			}
		}
		if len(data.BandOptions) == 0 {
			data.BandOptions = []schema.IdentityOption{
				{Value: "2.4GHz", Label: "2.4 GHz"},
				{Value: "5GHz", Label: "5 GHz"},
				{Value: "6GHz", Label: "6 GHz"},
			}
		}

		// Bridge-port multicast-router enum + IGMP querier mode +
		// WiFi security modes. Schema-driven with small fallbacks.
		data.MCRouterOptions = schema.OptionsFor(mgr, ifPath+"/infix-interfaces:bridge-port/multicast/router")
		if len(data.MCRouterOptions) == 0 {
			data.MCRouterOptions = []schema.IdentityOption{
				{Value: "auto", Label: "auto"},
				{Value: "off", Label: "off"},
				{Value: "permanent", Label: "permanent"},
			}
		}
		data.WiFiQuerierOptions = schema.OptionsFor(mgr, ifPath+bPath+"/multicast/querier")
		if len(data.WiFiQuerierOptions) == 0 {
			data.WiFiQuerierOptions = []schema.IdentityOption{
				{Value: "auto", Label: "auto"},
				{Value: "off", Label: "off"},
				{Value: "on", Label: "on"},
			}
		}
		data.WiFiSecOptionsAP = schema.OptionsFor(mgr, ifPath+"/infix-interfaces:wifi/access-point/security/mode")
		if len(data.WiFiSecOptionsAP) == 0 {
			data.WiFiSecOptionsAP = []schema.IdentityOption{
				{Value: "wpa2-wpa3-personal", Label: "wpa2-wpa3-personal"},
				{Value: "wpa3-personal", Label: "wpa3-personal"},
				{Value: "wpa2-personal", Label: "wpa2-personal"},
				{Value: "disabled", Label: "disabled (open)"},
			}
		}
		data.WiFiSecOptionsSta = schema.OptionsFor(mgr, ifPath+"/infix-interfaces:wifi/station/security/mode")
		if len(data.WiFiSecOptionsSta) == 0 {
			data.WiFiSecOptionsSta = []schema.IdentityOption{
				{Value: "auto", Label: "auto"},
				{Value: "disabled", Label: "disabled (open)"},
			}
		}
	}

	// Fetch the candidate (running) interface list, operational view,
	// keystore, and hardware (both candidate and operational) in
	// parallel — on yanger-backed targets each takes ~1s, so serial
	// fetches multiply the page latency. Keystore feeds the WG
	// private-key + WiFi PSK pickers; candidate hardware feeds the
	// configured WiFi radio picker; operational hardware feeds the
	// "available radios" list shown in the inline "+ New radio" form.
	ctx := context.WithoutCancel(r.Context())
	var (
		ifaces  []ifaceJSON
		operWrap interfacesWrapper
		ks      keystoreWrapper
		hw      hardwareWrapper // operational
		hwCand  hardwareWrapper // candidate
		ifaceErr, operErr, ksErr, hwErr, hwCandErr error
		wg      sync.WaitGroup
	)
	wg.Add(5)
	go func() { defer wg.Done(); ifaces, ifaceErr = h.fetchAllInterfaces(ctx) }()
	go func() { defer wg.Done(); operErr = h.RC.Get(ctx, "/data/ietf-interfaces:interfaces", &operWrap) }()
	go func() {
		defer wg.Done()
		// Candidate first, /data/ on 404 — so uncommitted keystore
		// edits show up in the wizard's pickers and Edit can pre-fill
		// values from keys the user has just added in candidate.
		ksErr = h.RC.Get(ctx, keystoreCandPath, &ks)
		if ksErr != nil && restconf.IsNotFound(ksErr) {
			ksErr = h.RC.Get(ctx, "/data/ietf-keystore:keystore", &ks)
		}
	}()
	go func() {
		defer wg.Done()
		hwErr = h.RC.Get(ctx, "/data/ietf-hardware:hardware", &hw)
	}()
	go func() {
		defer wg.Done()
		hwCandErr = h.RC.Get(ctx, candidatePath+"/ietf-hardware:hardware", &hwCand)
	}()
	wg.Wait()

	if ifaceErr != nil {
		log.Printf("configure interfaces: %v", ifaceErr)
		data.Error = "Could not read interface configuration"
	}
	if ksErr != nil && !restconf.IsNotFound(ksErr) {
		// Keystore unavailable just means the WG / WiFi pickers will
		// show an empty list — not a fatal page error.
		log.Printf("configure interfaces: keystore fetch: %v", ksErr)
	} else {
		for _, k := range ks.Keystore.AsymmetricKeys.AsymmetricKey {
			data.WizardAsymKeys = append(data.WizardAsymKeys, k.Name)
		}
		for _, k := range ks.Keystore.SymmetricKeys.SymmetricKey {
			data.WizardSymKeys = append(data.WizardSymKeys, symKeyEntry{
				Name:  k.Name,
				Value: decodeSymmetricValue(k),
			})
		}
		sort.Strings(data.WizardAsymKeys)
		sort.SliceStable(data.WizardSymKeys, func(i, j int) bool {
			return data.WizardSymKeys[i].Name < data.WizardSymKeys[j].Name
		})
		data.PSKKeys = data.WizardSymKeys
	}
	if hwErr != nil && !restconf.IsNotFound(hwErr) {
		log.Printf("configure interfaces: hardware fetch: %v", hwErr)
	}
	if hwCandErr != nil && !restconf.IsNotFound(hwCandErr) {
		log.Printf("configure interfaces: candidate hardware fetch: %v", hwCandErr)
	}
	// Configured WiFi radios live in candidate (so the picker reflects
	// uncommitted edits the user is in the middle of). Available radios
	// = detected (operational class=wifi) - those already in candidate.
	data.WizardWifiRadios = buildWifiRadioOptions(hwCand.Hardware.Component)
	configuredRadioNames := make(map[string]bool, len(data.WizardWifiRadios))
	for _, r := range data.WizardWifiRadios {
		configuredRadioNames[r.Name] = true
	}
	for _, c := range hw.Hardware.Component {
		if shortClass(c.Class) != classWiFi {
			continue
		}
		if configuredRadioNames[c.Name] {
			continue
		}
		data.WizardAvailableRadios = append(data.WizardAvailableRadios, c.Name)
	}
	sort.Strings(data.WizardAvailableRadios)

	data.Interfaces = h.buildRows(ifaces, operWrap.Interfaces.Interface)
	// Copy page-level descriptions + DHCP option enums onto each row so the
	// IPv4/IPv6 blocks render standalone (see cfgIfaceRow.Desc).
	for i := range data.Interfaces {
		data.Interfaces[i].Desc = data.Desc
		data.Interfaces[i].DHCPv4Options = data.DHCPv4Options
		data.Interfaces[i].DHCPv6Options = data.DHCPv6Options
	}
	// Populate the mirrored radio editor for WiFi interface rows from
	// the already-fetched candidate hardware tree (no extra fetch).
	radios := indexWifiRadios(hwCand.Hardware.Component)
	for i := range data.Interfaces {
		row := &data.Interfaces[i]
		if !row.IsWifi || row.WiFi == nil || row.WiFi.Radio == "" {
			continue
		}
		if rh, ok := radios[row.WiFi.Radio]; ok {
			row.PortRadio = rh
		}
	}

	taken := make(map[string]bool, len(ifaces))
	for _, iface := range ifaces {
		slug := typeSlug(iface.Type)
		data.AllNames = append(data.AllNames, iface.Name)
		taken[iface.Name] = true
		switch slug {
		case "bridge":
			data.BridgeNames = append(data.BridgeNames, iface.Name)
		case "lag":
			data.LagNames = append(data.LagNames, iface.Name)
		}
	}
	sort.Strings(data.AllNames)
	data.WizardNames = buildWizardNames(taken)

	// Bridge port candidates for the Add Interface wizard: any interface
	// not currently enslaved to a bridge/lag and of a type that can be
	// a bridge port. Computed by reusing portCandidatesFor("", …) which
	// the bridge edit-form path already relies on (line 831).
	memberOf := make(map[string]membership, len(ifaces))
	for _, iface := range ifaces {
		if iface.BridgePort != nil && iface.BridgePort.Bridge != "" {
			memberOf[iface.Name] = membership{"bridge", iface.BridgePort.Bridge}
		} else if iface.LagPort != nil && iface.LagPort.LAG != "" {
			memberOf[iface.Name] = membership{"lag", iface.LagPort.LAG}
		}
	}
	data.FreePortCandidates = portCandidatesFor("", ifaces, memberOf)

	// VLAN default name = <first-parent>.1; AllNames is sorted above.
	if len(data.AllNames) > 0 {
		data.VlanDefaultName = data.AllNames[0] + ".1"
	}

	if operErr != nil {
		log.Printf("configure interfaces: operational fetch: %v", operErr)
	} else {
		data.UnconfiguredPhysical = unconfiguredPhysical(ifaces, operWrap.Interfaces.Interface)
	}

	tmplName := "configure-interfaces.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// CreateInterface creates a new interface of the chosen type.
// POST /configure/interfaces
func (h *ConfigureInterfacesHandler) CreateInterface(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	ifType := r.FormValue("type")
	if name == "" || ifType == "" {
		renderSaveError(w, fmt.Errorf("name and type are required"))
		return
	}

	iface := map[string]any{
		"name":    name,
		"type":    ifType,
		"enabled": true,
	}

	switch typeSlug(ifType) {
	case "bridge":
		bridge := map[string]any{}
		if r.FormValue("bridge-type") == "ieee8021q" {
			// Presence of `vlans` container switches the bridge into
			// IEEE 802.1Q VLAN-filtering mode; empty container is fine
			// since list `vlan` has no min-elements. User adds VLANs
			// through the bridge's fold-out form after Create.
			bridge["vlans"] = map[string]any{}
		}
		iface["infix-interfaces:bridge"] = bridge

		// Bridge ports: pre-assign existing free interfaces to this
		// new bridge in the same write. PATCH on the parent collection
		// merges the bridge-port augmentation into each port's
		// existing entry without overwriting other config.
		if members := r.Form["members"]; len(members) > 0 {
			ifs := []map[string]any{iface}
			for _, port := range members {
				ifs = append(ifs, map[string]any{
					"name": port,
					"infix-interfaces:bridge-port": map[string]any{
						"bridge": name,
					},
				})
			}
			body := map[string]any{
				"ietf-interfaces:interfaces": map[string]any{"interface": ifs},
			}
			if err := h.RC.Patch(r.Context(), ifaceCandPath, body); err != nil {
				log.Printf("configure interfaces create bridge %s with ports: %v", name, err)
				renderSaveError(w, err)
				return
			}
			renderSavedRedirect(w, fmt.Sprintf("%s created with %d port(s)", name, len(members)), "/configure/interfaces")
			return
		}
	case "lag":
		mode := strings.TrimSpace(r.FormValue("mode"))
		if mode == "" {
			mode = "static"
		}
		iface["infix-interfaces:lag"] = map[string]any{"mode": mode}

		// LAG ports: enslave selected interfaces to this new LAG in one
		// PATCH on the parent collection — same atomic-create pattern
		// as bridge ports above.
		if members := r.Form["members"]; len(members) > 0 {
			ifs := []map[string]any{iface}
			for _, port := range members {
				ifs = append(ifs, map[string]any{
					"name": port,
					"infix-interfaces:lag-port": map[string]any{
						"lag": name,
					},
				})
			}
			body := map[string]any{
				"ietf-interfaces:interfaces": map[string]any{"interface": ifs},
			}
			if err := h.RC.Patch(r.Context(), ifaceCandPath, body); err != nil {
				log.Printf("configure interfaces create lag %s with ports: %v", name, err)
				renderSaveError(w, err)
				return
			}
			renderSavedRedirect(w, fmt.Sprintf("%s created with %d port(s)", name, len(members)), "/configure/interfaces")
			return
		}
	case "vlan":
		vid, err := strconv.Atoi(r.FormValue("vid"))
		if err != nil || vid < 1 || vid > 4094 {
			renderSaveError(w, fmt.Errorf("VID must be 1–4094"))
			return
		}
		lowerIf := strings.TrimSpace(r.FormValue("lower-layer-if"))
		if lowerIf == "" {
			renderSaveError(w, fmt.Errorf("lower-layer interface is required for VLAN"))
			return
		}
		iface["infix-interfaces:vlan"] = map[string]any{
			"id":             vid,
			"lower-layer-if": lowerIf,
		}
	case "veth":
		peer := strings.TrimSpace(r.FormValue("peer"))
		if peer == "" {
			renderSaveError(w, fmt.Errorf("peer name is required for veth"))
			return
		}
		if peer == name {
			renderSaveError(w, fmt.Errorf("peer name must differ from interface name"))
			return
		}
		iface["infix-interfaces:veth"] = map[string]any{"peer": peer}
		peerIface := map[string]any{
			"name":    peer,
			"type":    ifType,
			"enabled": true,
			"infix-interfaces:veth": map[string]any{"peer": name},
		}
		// One PATCH on the parent collection inserts both ends atomically.
		// The veth/peer `must` constraint only fires at commit, so the
		// half-pair window from two sequential PUTs is avoided.
		body := map[string]any{
			"ietf-interfaces:interfaces": map[string]any{
				"interface": []map[string]any{iface, peerIface},
			},
		}
		if err := h.RC.Patch(r.Context(), ifaceCandPath, body); err != nil {
			log.Printf("configure interfaces create veth pair %s/%s: %v", name, peer, err)
			renderSaveError(w, err)
			return
		}
		renderSavedRedirect(w, fmt.Sprintf("veth pair %s/%s created", name, peer), "/configure/interfaces")
		return
	case "gre", "gretap":
		local := strings.TrimSpace(r.FormValue("local"))
		remote := strings.TrimSpace(r.FormValue("remote"))
		if local == "" || remote == "" {
			renderSaveError(w, fmt.Errorf("local and remote IP addresses are required"))
			return
		}
		// Both gre and gretap augment under the same `infix-interfaces:gre`
		// container — the YANG `when` clause discriminates on if:type.
		gre := map[string]any{"local": local, "remote": remote}
		if r.FormValue("pmtu-discovery") == "false" {
			gre["pmtu-discovery"] = false
		}
		iface["infix-interfaces:gre"] = gre
	case "vxlan":
		local := strings.TrimSpace(r.FormValue("local"))
		remote := strings.TrimSpace(r.FormValue("remote"))
		vniStr := strings.TrimSpace(r.FormValue("vni"))
		if local == "" || remote == "" || vniStr == "" {
			renderSaveError(w, fmt.Errorf("local, remote and VNI are required"))
			return
		}
		vni, err := strconv.Atoi(vniStr)
		if err != nil || vni < 0 || vni > 16777215 {
			renderSaveError(w, fmt.Errorf("VNI must be 0–16777215"))
			return
		}
		vxlan := map[string]any{"local": local, "remote": remote, "vni": vni}
		if portStr := strings.TrimSpace(r.FormValue("remote-port")); portStr != "" {
			port, err := strconv.Atoi(portStr)
			if err != nil || port < 0 || port > 65535 {
				renderSaveError(w, fmt.Errorf("UDP port must be 0–65535"))
				return
			}
			vxlan["remote-port"] = port
		}
		iface["infix-interfaces:vxlan"] = vxlan
	case "wireguard":
		keyName := strings.TrimSpace(r.FormValue("private-key"))
		if keyName == "" {
			renderSaveError(w, fmt.Errorf("a keystore private-key reference is required"))
			return
		}
		wg := map[string]any{"private-key": keyName}
		if portStr := strings.TrimSpace(r.FormValue("listen-port")); portStr != "" {
			port, err := strconv.Atoi(portStr)
			if err != nil || port < 0 || port > 65535 {
				renderSaveError(w, fmt.Errorf("listen port must be 0–65535"))
				return
			}
			wg["listen-port"] = port
		}
		iface["infix-interfaces:wireguard"] = wg
	case "wifi":
		radio := strings.TrimSpace(r.FormValue("radio"))
		if radio == "" {
			renderSaveError(w, fmt.Errorf("a WiFi radio reference is required"))
			return
		}
		mode := r.FormValue("wifi-mode")
		ssid := strings.TrimSpace(r.FormValue("ssid"))
		if ssid == "" {
			renderSaveError(w, fmt.Errorf("SSID is required"))
			return
		}
		secMode := r.FormValue("security-mode")
		secret := strings.TrimSpace(r.FormValue("secret"))
		wifi := map[string]any{"radio": radio}
		switch mode {
		case "access-point":
			ap := map[string]any{"ssid": ssid}
			if r.FormValue("hidden") == "true" {
				ap["hidden"] = true
			}
			if secMode == "" {
				secMode = "wpa2-wpa3-personal"
			}
			security := map[string]any{"mode": secMode}
			if secMode != "open" {
				if secret == "" {
					renderSaveError(w, fmt.Errorf("PSK is required for security mode %q", secMode))
					return
				}
				security["secret"] = secret
			}
			ap["security"] = security
			wifi["access-point"] = ap
		case "station":
			sta := map[string]any{"ssid": ssid}
			if secMode == "" {
				secMode = "auto"
			}
			security := map[string]any{"mode": secMode}
			if secMode != "disabled" {
				if secret == "" {
					renderSaveError(w, fmt.Errorf("PSK is required for security mode %q", secMode))
					return
				}
				security["secret"] = secret
			}
			sta["security"] = security
			wifi["station"] = sta
		default:
			renderSaveError(w, fmt.Errorf("WiFi mode must be 'station' or 'access-point'"))
			return
		}
		iface["infix-interfaces:wifi"] = wifi
	}

	body := map[string]any{"ietf-interfaces:interface": []map[string]any{iface}}
	if err := h.RC.Put(r.Context(), ifacePath(name), body); err != nil {
		log.Printf("configure interfaces create %s: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, fmt.Sprintf("%s created", name), "/configure/interfaces")
}

// ─── Add Interface wizard — inline keystore key creation ────────────────────

// keystoreCandPath is the candidate-datastore base for keystore writes.
const keystoreCandPath = candidatePath + "/ietf-keystore:keystore"

// WizardCreateSymKey creates a passphrase-format symmetric key in the
// candidate keystore and returns the refreshed WiFi PSK picker as an
// HTML fragment so HTMX can swap it in place. Used by the inline "+ New"
// affordance in the Add Interface modal's WiFi fieldset.
// POST /configure/interfaces/wizard/sym-key
func (h *ConfigureInterfacesHandler) WizardCreateSymKey(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("key-name"))
	value := r.FormValue("key-value")
	if name == "" {
		renderSaveError(w, fmt.Errorf("key name is required"))
		return
	}
	if value == "" {
		renderSaveError(w, fmt.Errorf("passphrase is required"))
		return
	}
	body := map[string]any{
		"ietf-keystore:symmetric-key": []map[string]any{{
			"name":                    name,
			"key-format":              "infix-crypto-types:passphrase-key-format",
			"cleartext-symmetric-key": base64.StdEncoding.EncodeToString([]byte(value)),
		}},
	}
	path := keystoreCandPath + "/symmetric-keys/symmetric-key=" + url.PathEscape(name)
	if err := h.RC.Put(r.Context(), path, body); err != nil {
		log.Printf("wizard create sym key %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	h.renderPSKPicker(w, r, name)
}

// WizardCreateAsymKey creates an asymmetric key from a PEM private-key
// blob pasted by the user and returns the refreshed WireGuard private-
// key picker. Mirrors AddAsymKey in configure_keystore.go but returns a
// fragment instead of redirecting.
// POST /configure/interfaces/wizard/asym-key
func (h *ConfigureInterfacesHandler) WizardCreateAsymKey(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("key-name"))
	privPEM := r.FormValue("key-value")
	if name == "" {
		renderSaveError(w, fmt.Errorf("key name is required"))
		return
	}
	if privPEM == "" {
		renderSaveError(w, fmt.Errorf("private key (PEM) is required"))
		return
	}
	if err := h.putAsymKeyFromPEM(r.Context(), name, privPEM); err != nil {
		log.Printf("wizard create asym key %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	h.renderWGKeyPicker(w, r, name)
}

// WizardGenerateWGKey freshly generates a Curve25519 (X25519) keypair
// using the `wg genkey` tool already shipped on the target, wraps the
// 32-byte private key in PKCS8 DER via Go's stdlib (crypto/ecdh +
// x509.MarshalPKCS8PrivateKey), and stores it in the candidate keystore.
// Returns the refreshed WireGuard private-key picker.
// POST /configure/interfaces/wizard/wg-genkey
func (h *ConfigureInterfacesHandler) WizardGenerateWGKey(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("key-name"))
	if name == "" {
		renderSaveError(w, fmt.Errorf("key name is required"))
		return
	}
	privPEM, err := generateX25519PEM()
	if err != nil {
		log.Printf("wizard wg-genkey: %v", err)
		renderSaveError(w, fmt.Errorf("generate X25519 key: %v", err))
		return
	}
	if err := h.putAsymKeyFromPEM(r.Context(), name, privPEM); err != nil {
		log.Printf("wizard wg-genkey put %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	h.renderWGKeyPicker(w, r, name)
}

// putAsymKeyFromPEM is the shared keystore-PUT path used by both the
// paste-PEM and generate flows. Decodes the PEM block, derives the
// public key when possible, and writes the entry to the candidate
// keystore.
func (h *ConfigureInterfacesHandler) putAsymKeyFromPEM(ctx context.Context, name, privPEM string) error {
	block, _ := pem.Decode([]byte(privPEM))
	if block == nil {
		return fmt.Errorf("invalid private key PEM: no PEM block found")
	}
	keyBody := map[string]any{
		"name":                  name,
		"private-key-format":    pemTypeToKeyFormat(block.Type),
		"cleartext-private-key": base64.StdEncoding.EncodeToString(block.Bytes),
	}
	if pubPEM := derivePublicKeyFromDER(block.Bytes, block.Type); pubPEM != "" {
		if err := applyPublicKey(keyBody, pubPEM); err != nil {
			return err
		}
	}
	body := map[string]any{"ietf-keystore:asymmetric-key": []map[string]any{keyBody}}
	path := keystoreCandPath + "/asymmetric-keys/asymmetric-key=" + url.PathEscape(name)
	return h.RC.Put(ctx, path, body)
}

// generateX25519PEM runs `wg genkey` to source the 32 raw key bytes
// (random + Curve25519-clamped by wg's standard implementation) and
// wraps them in PKCS8 PEM via crypto/ecdh + x509.MarshalPKCS8PrivateKey.
// Returns the PEM-encoded private key.
func generateX25519PEM() (string, error) {
	out, err := exec.Command("wg", "genkey").Output()
	if err != nil {
		return "", fmt.Errorf("wg genkey: %w", err)
	}
	priv64 := strings.TrimSpace(string(out))
	privRaw, err := base64.StdEncoding.DecodeString(priv64)
	if err != nil {
		return "", fmt.Errorf("decode wg key output: %w", err)
	}
	if len(privRaw) != 32 {
		return "", fmt.Errorf("wg genkey returned %d bytes, expected 32", len(privRaw))
	}
	ecdhPriv, err := ecdh.X25519().NewPrivateKey(privRaw)
	if err != nil {
		return "", fmt.Errorf("ecdh new key: %w", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(ecdhPriv)
	if err != nil {
		return "", fmt.Errorf("marshal pkcs8: %w", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), nil
}

func (h *ConfigureInterfacesHandler) renderPSKPicker(w http.ResponseWriter, r *http.Request, selected string) {
	ks := h.fetchKeystore(r.Context())
	var entries []symKeyEntry
	for _, k := range ks.Keystore.SymmetricKeys.SymmetricKey {
		entries = append(entries, symKeyEntry{Name: k.Name, Value: decodeSymmetricValue(k)})
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	if !containsSymKey(entries, selected) {
		// Keystore fetch failed or candidate hasn't flushed yet — make
		// the new key visible anyway so the picker remains usable.
		entries = append([]symKeyEntry{{Name: selected}}, entries...)
	}
	w.Header().Set("HX-Trigger", `{"wifiPskCreated":true}`)
	if err := h.Template.ExecuteTemplate(w, "wizard-psk-picker.html", map[string]any{
		"Keys":     entries,
		"Selected": selected,
		"ID":       r.FormValue("picker-id"),
	}); err != nil {
		log.Printf("psk picker fragment: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (h *ConfigureInterfacesHandler) renderWGKeyPicker(w http.ResponseWriter, r *http.Request, selected string) {
	ks := h.fetchKeystore(r.Context())
	var names []string
	for _, k := range ks.Keystore.AsymmetricKeys.AsymmetricKey {
		names = append(names, k.Name)
	}
	sort.Strings(names)
	if !containsString(names, selected) {
		names = append([]string{selected}, names...)
	}
	w.Header().Set("HX-Trigger", `{"wgKeyCreated":true}`)
	if err := h.Template.ExecuteTemplate(w, "wizard-wgkey-picker.html", map[string]any{
		"Keys":     names,
		"Selected": selected,
	}); err != nil {
		log.Printf("wg picker fragment: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// fetchKeystore tries the candidate datastore first (so post-PUT refreshes
// see the just-created key) and falls back to /data/ on 404.
func (h *ConfigureInterfacesHandler) fetchKeystore(ctx context.Context) keystoreWrapper {
	var ks keystoreWrapper
	if err := h.RC.Get(ctx, keystoreCandPath, &ks); err != nil {
		if restconf.IsNotFound(err) {
			if fallErr := h.RC.Get(ctx, "/data/ietf-keystore:keystore", &ks); fallErr != nil {
				log.Printf("wizard keystore refresh fallback: %v", fallErr)
			}
		} else {
			log.Printf("wizard keystore refresh: %v", err)
		}
	}
	return ks
}

func containsSymKey(entries []symKeyEntry, want string) bool {
	for _, e := range entries {
		if e.Name == want {
			return true
		}
	}
	return false
}

func containsString(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// WizardCreateRadio adds (or replaces) a WiFi radio hardware component
// in the candidate ietf-hardware and returns the refreshed Radio picker
// for the wizard's WiFi fieldset. Mirrors the "+ New" pattern of the
// keystore-creation handlers.
// POST /configure/interfaces/wizard/radio
func (h *ConfigureInterfacesHandler) WizardCreateRadio(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("radio-name"))
	country := strings.TrimSpace(r.FormValue("country-code"))
	band := strings.TrimSpace(r.FormValue("band"))
	channel := strings.TrimSpace(r.FormValue("channel"))
	if name == "" {
		renderSaveError(w, fmt.Errorf("radio name is required"))
		return
	}
	if country == "" {
		renderSaveError(w, fmt.Errorf("country code is required"))
		return
	}
	radio := map[string]any{"country-code": country}
	if band != "" {
		radio["band"] = band
	}
	if channel == "" {
		channel = "auto"
	}
	if channel == "auto" {
		radio["channel"] = "auto"
	} else {
		n, err := strconv.Atoi(channel)
		if err != nil || n < 1 || n > 196 {
			renderSaveError(w, fmt.Errorf("channel must be 'auto' or a number 1–196"))
			return
		}
		radio["channel"] = n
	}
	comp := map[string]any{
		"name":                      name,
		"class":                     "infix-hardware:wifi",
		"infix-hardware:wifi-radio": radio,
	}
	body := map[string]any{"ietf-hardware:component": []map[string]any{comp}}
	path := candidatePath + "/ietf-hardware:hardware/component=" + url.PathEscape(name)
	if err := h.RC.Put(r.Context(), path, body); err != nil {
		log.Printf("wizard create radio %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	h.renderRadioPicker(w, r, name)
}

func (h *ConfigureInterfacesHandler) renderRadioPicker(w http.ResponseWriter, r *http.Request, selected string) {
	var hwCand hardwareWrapper
	if err := h.RC.Get(r.Context(), candidatePath+"/ietf-hardware:hardware", &hwCand); err != nil {
		log.Printf("wizard radio refresh: %v", err)
	}
	radios := buildWifiRadioOptions(hwCand.Hardware.Component)
	if !containsRadioName(radios, selected) {
		// Race / fetch failure — surface the new radio anyway.
		radios = append([]wifiRadioOption{{Name: selected, Label: selected}}, radios...)
	}
	w.Header().Set("HX-Trigger", `{"wifiRadioCreated":true}`)
	data := map[string]any{
		"Radios":   radios,
		"Selected": selected,
		"ID":       r.FormValue("picker-id"),
	}
	if err := h.Template.ExecuteTemplate(w, "wizard-radio-picker.html", data); err != nil {
		log.Printf("radio picker fragment: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func containsRadioName(rs []wifiRadioOption, want string) bool {
	for _, r := range rs {
		if r.Name == want {
			return true
		}
	}
	return false
}

// DeleteInterface removes an interface from running config. For virtual
// interfaces (bridge, lag, vlan, dummy, …) confd unbinds and tears down the
// netdev; for physical interfaces it just sets them administratively DOWN.
// Loopback is rejected at this layer — the UI guards it by TypeSlug too
// (configure-interfaces.html), but a direct DELETE shouldn't slip through.
// DELETE /configure/interfaces/{name}
func (h *ConfigureInterfacesHandler) DeleteInterface(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "lo" || name == "loopback" {
		renderSaveError(w, fmt.Errorf("loopback interface cannot be deleted"))
		return
	}
	if err := h.RC.Delete(r.Context(), ifacePath(name)); err != nil && !restconf.IsNotFound(err) {
		log.Printf("configure interfaces %s delete: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, name+" deleted", "/configure/interfaces")
}

// SaveGeneral saves description, enabled, and optional custom MAC for any
// interface. Empty MAC clears the override (DELETE on custom-phys-address);
// non-empty installs it as a static override.
// POST /configure/interfaces/{name}
func (h *ConfigureInterfacesHandler) SaveGeneral(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	enabled := r.FormValue("enabled") != "false"
	body := map[string]any{
		"ietf-interfaces:interface": map[string]any{
			"name":        name,
			"enabled":     enabled,
			"description": strings.TrimSpace(r.FormValue("description")),
		},
	}
	if err := h.RC.Patch(r.Context(), ifacePath(name), body); err != nil {
		log.Printf("configure interfaces %s general: %v", name, err)
		renderSaveError(w, err)
		return
	}

	macPath := ifacePath(name) + "/infix-interfaces:custom-phys-address"
	if mac := strings.TrimSpace(r.FormValue("mac")); mac != "" {
		macBody := map[string]any{
			"infix-interfaces:custom-phys-address": map[string]any{"static": mac},
		}
		if err := h.RC.Put(r.Context(), macPath, macBody); err != nil {
			log.Printf("configure interfaces %s mac: %v", name, err)
			renderSaveError(w, err)
			return
		}
	} else {
		if err := h.RC.Delete(r.Context(), macPath); err != nil && !restconf.IsNotFound(err) {
			log.Printf("configure interfaces %s mac clear: %v", name, err)
			renderSaveError(w, err)
			return
		}
	}

	renderSaved(w, "Saved")
}

// AddIPv4 adds an IPv4 address to an interface.
// POST /configure/interfaces/{name}/ipv4
func (h *ConfigureInterfacesHandler) AddIPv4(w http.ResponseWriter, r *http.Request) {
	h.addAddr(w, r, "ipv4", "IPv4", "iface-ipv4-block")
}

// DeleteIPv4 removes an IPv4 address from an interface.
// DELETE /configure/interfaces/{name}/ipv4/{ip}
func (h *ConfigureInterfacesHandler) DeleteIPv4(w http.ResponseWriter, r *http.Request) {
	h.deleteAddr(w, r, "ipv4", "IPv4", "iface-ipv4-block")
}

// AddIPv6 adds an IPv6 address to an interface.
// POST /configure/interfaces/{name}/ipv6
func (h *ConfigureInterfacesHandler) AddIPv6(w http.ResponseWriter, r *http.Request) {
	h.addAddr(w, r, "ipv6", "IPv6", "iface-ipv6-block")
}

// DeleteIPv6 removes an IPv6 address from an interface.
// DELETE /configure/interfaces/{name}/ipv6/{ip}
func (h *ConfigureInterfacesHandler) DeleteIPv6(w http.ResponseWriter, r *http.Request) {
	h.deleteAddr(w, r, "ipv6", "IPv6", "iface-ipv6-block")
}

// SaveBridgePort assigns or updates an interface's bridge membership.
// PUT-replaces the bridge-port augment, so the form must submit every
// field it wants preserved. Changing the `bridge` field from one name
// to another effectively moves the port — the PUT is atomic.
// POST /configure/interfaces/{name}/bridge-port
// SaveEthernet writes ethernet settings via a sequence of PATCH and DELETE
// ops rather than a single PUT.  A PUT that includes every leaf in the
// ethernet container silently drops the duplex leaf on rousette/confd
// (verified with curl); the per-leaf approach matches what manual PATCH
// curls land reliably.  Tri-state fields (duplex, mdi-x) DELETE on "Auto"
// so the candidate ends with an absent leaf, which is how the YANG model
// expresses the default.  data-missing on DELETE is swallowed for
// idempotency.
// POST /configure/interfaces/{name}/ethernet
func (h *ConfigureInterfacesHandler) SaveEthernet(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	name := r.PathValue("name")
	base := ifacePath(name) + "/ieee802-ethernet-interface:ethernet"

	autoneg := r.FormValue("autoneg") == "on"
	var adv []string
	for _, v := range r.Form["advertised"] {
		if v != "" {
			adv = append(adv, v)
		}
	}

	// auto-negotiation: PUT the whole container so an empty advertised list
	// actually clears stale entries.  DELETE on an unqualified leaf-list
	// path fails ("requires exactly one key") under RFC 8040, so omitting
	// the leaf-list from a container PUT is the cleanest clear.  Safe here
	// because the container only carries enable + advertised-pmd-types
	// (negotiation-status is deviate-not-supported in Infix).
	an := map[string]any{"enable": autoneg}
	if len(adv) > 0 {
		an["infix-ethernet-interface:advertised-pmd-types"] = adv
	}
	body := map[string]any{"ieee802-ethernet-interface:auto-negotiation": an}
	if err := h.RC.Put(ctx, base+"/auto-negotiation", body); err != nil {
		log.Printf("configure interfaces %s ethernet autoneg: %v", name, err)
		renderSaveError(w, err)
		return
	}

	// duplex: PATCH if user picked full/half, DELETE on Auto
	if d := r.FormValue("duplex"); d != "" {
		body := map[string]any{
			"ieee802-ethernet-interface:ethernet": map[string]any{"duplex": d},
		}
		if err := h.RC.Patch(ctx, base, body); err != nil {
			log.Printf("configure interfaces %s ethernet duplex: %v", name, err)
			renderSaveError(w, err)
			return
		}
	} else if err := h.RC.Delete(ctx, base+"/duplex"); err != nil && !restconf.IsDataMissing(err) {
		log.Printf("configure interfaces %s ethernet duplex delete: %v", name, err)
		renderSaveError(w, err)
		return
	}

	// mdi-x: only valid when autoneg is off (YANG when).  On autoneg=true
	// or user picks Auto-MDIX, DELETE so the leaf stays absent.
	mdix := r.FormValue("mdix")
	if !autoneg && (mdix == "true" || mdix == "false") {
		body := map[string]any{
			"ieee802-ethernet-interface:ethernet": map[string]any{
				"infix-ethernet-interface:mdi-x": mdix == "true",
			},
		}
		if err := h.RC.Patch(ctx, base, body); err != nil {
			log.Printf("configure interfaces %s ethernet mdi-x: %v", name, err)
			renderSaveError(w, err)
			return
		}
	} else if err := h.RC.Delete(ctx, base+"/infix-ethernet-interface:mdi-x"); err != nil && !restconf.IsDataMissing(err) {
		log.Printf("configure interfaces %s ethernet mdi-x delete: %v", name, err)
		renderSaveError(w, err)
		return
	}

	renderSaved(w, "Ethernet saved")
}

// ResetEthernetAdvertised clears the advertised-pmd-types leaf-list while
// preserving auto-negotiation/enable.  Routed off the per-row reset
// button because the generic /configure/leaf path can't DELETE a leaf-list
// without per-entry key predicates.
// DELETE /configure/interfaces/{name}/ethernet/advertised
func (h *ConfigureInterfacesHandler) ResetEthernetAdvertised(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	base := ifacePath(name) + "/ieee802-ethernet-interface:ethernet"

	var resp struct {
		AN struct {
			Enable *bool `json:"enable"`
		} `json:"ieee802-ethernet-interface:auto-negotiation"`
	}
	enable := true // YANG default
	if err := h.RC.Get(r.Context(), base+"/auto-negotiation", &resp); err == nil {
		if resp.AN.Enable != nil {
			enable = *resp.AN.Enable
		}
	} else if !restconf.IsNotFound(err) {
		log.Printf("configure interfaces %s reset advertised get: %v", name, err)
		renderSaveError(w, err)
		return
	}

	body := map[string]any{
		"ieee802-ethernet-interface:auto-negotiation": map[string]any{"enable": enable},
	}
	if err := h.RC.Put(r.Context(), base+"/auto-negotiation", body); err != nil {
		log.Printf("configure interfaces %s reset advertised put: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSaved(w, "Reset to default")
}

func (h *ConfigureInterfacesHandler) SaveBridgePort(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	bridge := strings.TrimSpace(r.FormValue("bridge"))
	if bridge == "" {
		renderSaveError(w, fmt.Errorf("bridge name is required"))
		return
	}
	bp := buildBridgePortBody(r, bridge)
	body := map[string]any{"infix-interfaces:bridge-port": bp}
	if err := h.RC.Put(r.Context(), ifacePath(name)+"/infix-interfaces:bridge-port", body); err != nil {
		log.Printf("configure interfaces %s bridge-port: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSaved(w, "Bridge port saved")
}

// buildBridgePortBody assembles the bridge-port augment from form
// values for both Save and Move. Bridge is the caller-resolved name
// (the form's "bridge" field for Save, the move destination for Move).
func buildBridgePortBody(r *http.Request, bridge string) map[string]any {
	bp := map[string]any{"bridge": bridge}
	if pvid := r.FormValue("pvid"); pvid != "" {
		if v, err := strconv.Atoi(pvid); err == nil && v > 0 {
			bp["pvid"] = v
		}
	}
	flood := map[string]any{}
	for _, k := range []string{"broadcast", "unicast", "multicast"} {
		flood[k] = r.FormValue("flood-"+k) == "on"
	}
	bp["flood"] = flood
	mc := map[string]any{}
	if mr := r.FormValue("mc-router"); mr != "" {
		mc["router"] = mr
	}
	// Fast-leave is the access-port knob: cut multicast groups the
	// moment we see an IGMP leave (don't wait for the group-specific
	// query timeout). Submit only when the form carried the checkbox.
	if _, ok := r.Form["mc-fast-leave-present"]; ok {
		mc["fast-leave"] = r.FormValue("mc-fast-leave") == "on"
	}
	if len(mc) > 0 {
		bp["multicast"] = mc
	}
	return bp
}

// DeleteBridgePort detaches an interface from its bridge.
// DELETE /configure/interfaces/{name}/bridge-port
func (h *ConfigureInterfacesHandler) DeleteBridgePort(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.removeInterfaceAugment(r.Context(), name, "infix-interfaces:bridge-port"); err != nil {
		log.Printf("configure interfaces %s bridge-port delete: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Removed from bridge", "/configure/interfaces")
}

// removeInterfaceAugment drops a top-level augment (bridge-port,
// lag-port, …) from an interface entry via GET-modify-PUT on the
// candidate. RESTCONF DELETE on a non-presence container returns 502
// from rousette (the augment has no presence resource to remove), so
// we have to rewrite the parent instead — same workaround as
// yang-tree's DeleteContainer, scoped to interfaces here.
func (h *ConfigureInterfacesHandler) removeInterfaceAugment(ctx context.Context, name, augKey string) error {
	path := ifacePath(name)
	raw, err := h.RC.GetRaw(ctx, path)
	if err != nil {
		return fmt.Errorf("fetch interface %s: %w", name, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("decode interface %s: %w", name, err)
	}
	iface, err := unwrapSingleInterface(doc)
	if err != nil {
		return err
	}
	if _, ok := iface[augKey]; !ok {
		return fmt.Errorf("%s not present on interface %s", augKey, name)
	}
	delete(iface, augKey)
	body := map[string]any{"ietf-interfaces:interface": []map[string]any{iface}}
	return h.RC.Put(ctx, path, body)
}

// unwrapSingleInterface peels the RESTCONF envelope for a list-instance
// GET on /ietf-interfaces:interfaces/interface=<name>. The response is
// always {"ietf-interfaces:interfaces":{"interface":[{...}]}} with one
// entry; we return that entry's object.
func unwrapSingleInterface(doc map[string]any) (map[string]any, error) {
	wrap, ok := doc["ietf-interfaces:interfaces"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("response missing ietf-interfaces:interfaces wrapper")
	}
	list, ok := wrap["interface"].([]any)
	if !ok || len(list) == 0 {
		return nil, fmt.Errorf("response has no interface entry")
	}
	iface, ok := list[0].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("interface entry not an object")
	}
	return iface, nil
}

// SaveBridge saves bridge type and member ports in one round trip from the
// unified "Bridge Settings" form.  STP/multicast keep their own foldout
// forms; this handler covers what used to be two side-by-side forms.
// POST /configure/interfaces/{name}/bridge
func (h *ConfigureInterfacesHandler) SaveBridge(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	bridge := map[string]any{}

	stp := map[string]any{}
	if v := r.FormValue("stp-force-protocol"); v != "" {
		stp["force-protocol"] = v
	}
	for _, pair := range []struct{ form, yang string }{
		{"stp-hello-time", "hello-time"},
		{"stp-forward-delay", "forward-delay"},
		{"stp-max-age", "max-age"},
		{"stp-transmit-hold-count", "transmit-hold-count"},
		{"stp-max-hops", "max-hops"},
	} {
		if v, err := strconv.Atoi(r.FormValue(pair.form)); err == nil {
			stp[pair.yang] = v
		}
	}
	if len(stp) > 0 {
		bridge["stp"] = stp
	}

	if len(bridge) > 0 {
		body := map[string]any{"infix-interfaces:bridge": bridge}
		if err := h.RC.Patch(r.Context(), ifacePath(name)+"/infix-interfaces:bridge", body); err != nil {
			log.Printf("configure interfaces %s bridge: %v", name, err)
			renderSaveError(w, err)
			return
		}
	}

	// The bridge type choice is expressed via the vlans presence container:
	// 802.1Q = vlans container present; 802.1D = vlans container absent.
	vlansPath := ifacePath(name) + "/infix-interfaces:bridge/vlans"
	if r.FormValue("bridge-type") == "ieee8021q" {
		body := map[string]any{"vlans": map[string]any{}}
		if err := h.RC.Put(r.Context(), vlansPath, body); err != nil {
			log.Printf("configure interfaces %s bridge type 8021q: %v", name, err)
			renderSaveError(w, err)
			return
		}
	} else {
		if err := h.RC.Delete(r.Context(), vlansPath); err != nil {
			// 404 is fine — vlans already absent (802.1D)
			log.Printf("configure interfaces %s bridge type 8021d (delete vlans): %v", name, err)
		}
	}

	if err := h.applyMembersDiff(r, name, "bridge",
		func(iface ifaceJSON, master string) bool {
			return iface.BridgePort != nil && iface.BridgePort.Bridge == master
		}); err != nil {
		renderSaveError(w, err)
		return
	}

	renderSaved(w, "Bridge saved")
}

// AddVLAN creates a new VLAN on an ieee8021q bridge.
// POST /configure/interfaces/{name}/bridge/vlans
func (h *ConfigureInterfacesHandler) AddVLAN(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	vid, err := strconv.Atoi(r.FormValue("vid"))
	if err != nil || vid < 1 || vid > 4094 {
		renderSaveError(w, fmt.Errorf("VID must be 1–4094"))
		return
	}
	vlan := map[string]any{"vid": vid}
	if untagged := r.Form["untagged"]; len(untagged) > 0 {
		vlan["untagged"] = untagged
	}
	if tagged := r.Form["tagged"]; len(tagged) > 0 {
		vlan["tagged"] = tagged
	}
	body := map[string]any{"infix-interfaces:vlan": []map[string]any{vlan}}
	path := ifacePath(name) + "/infix-interfaces:bridge/vlans/vlan=" + strconv.Itoa(vid)
	if err := h.RC.Put(r.Context(), path, body); err != nil {
		log.Printf("configure interfaces %s vlan add %d: %v", name, vid, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "VLAN added", "/configure/interfaces")
}

// SaveVLAN updates the untagged/tagged port sets for an existing VLAN.
// POST /configure/interfaces/{name}/bridge/vlans/{vid}
func (h *ConfigureInterfacesHandler) SaveVLAN(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	vidStr := r.PathValue("vid")
	vid, err := strconv.Atoi(vidStr)
	if err != nil {
		http.Error(w, "invalid vid", http.StatusBadRequest)
		return
	}
	untagged := r.Form["untagged"]
	if untagged == nil {
		untagged = []string{}
	}
	tagged := r.Form["tagged"]
	if tagged == nil {
		tagged = []string{}
	}
	vlan := map[string]any{
		"vid":      vid,
		"untagged": untagged,
		"tagged":   tagged,
	}
	body := map[string]any{"infix-interfaces:vlan": []map[string]any{vlan}}
	path := ifacePath(name) + "/infix-interfaces:bridge/vlans/vlan=" + vidStr
	if err := h.RC.Put(r.Context(), path, body); err != nil {
		log.Printf("configure interfaces %s vlan save %d: %v", name, vid, err)
		renderSaveError(w, err)
		return
	}
	renderSaved(w, "VLAN saved")
}

// DeleteVLAN removes a VLAN from an ieee8021q bridge.
// DELETE /configure/interfaces/{name}/bridge/vlans/{vid}
func (h *ConfigureInterfacesHandler) DeleteVLAN(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	vidStr := r.PathValue("vid")
	path := ifacePath(name) + "/infix-interfaces:bridge/vlans/vlan=" + vidStr
	if err := h.RC.Delete(r.Context(), path); err != nil {
		log.Printf("configure interfaces %s vlan delete %s: %v", name, vidStr, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "VLAN deleted", "/configure/interfaces")
}

// SaveLagPort assigns an interface to a LAG.
// POST /configure/interfaces/{name}/lag-port
func (h *ConfigureInterfacesHandler) SaveLagPort(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	lagName := strings.TrimSpace(r.FormValue("lag"))
	if lagName == "" {
		renderSaveError(w, fmt.Errorf("LAG name is required"))
		return
	}
	body := map[string]any{
		"infix-interfaces:lag-port": map[string]any{"lag": lagName},
	}
	if err := h.RC.Put(r.Context(), ifacePath(name)+"/infix-interfaces:lag-port", body); err != nil {
		log.Printf("configure interfaces %s lag-port: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSaved(w, "LAG port saved")
}

// SaveBridgeSTP PATCHes the bridge STP container. Split out from
// SaveBridge so the STP fold-out can carry its own Save button (the
// matching pattern the bridge-type radios stay outside).
// POST /configure/interfaces/{name}/bridge/stp
func (h *ConfigureInterfacesHandler) SaveBridgeSTP(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	stp := map[string]any{}
	if v := r.FormValue("stp-force-protocol"); v != "" {
		stp["force-protocol"] = v
	}
	for _, pair := range []struct{ form, yang string }{
		{"stp-hello-time", "hello-time"},
		{"stp-forward-delay", "forward-delay"},
		{"stp-max-age", "max-age"},
		{"stp-transmit-hold-count", "transmit-hold-count"},
		{"stp-max-hops", "max-hops"},
	} {
		if v, err := strconv.Atoi(r.FormValue(pair.form)); err == nil {
			stp[pair.yang] = v
		}
	}
	body := map[string]any{"infix-interfaces:stp": stp}
	path := ifacePath(name) + "/infix-interfaces:bridge/stp"
	if err := h.RC.Patch(r.Context(), path, body); err != nil {
		log.Printf("configure interfaces %s bridge stp: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSaved(w, "STP saved")
}

// SaveBridgeMulticast PATCHes the bridge multicast snooping container.
// Snooping toggle + querier mode are first-class fields; query-interval
// is the one advanced timing knob exposed for now.
// POST /configure/interfaces/{name}/bridge/multicast
func (h *ConfigureInterfacesHandler) SaveBridgeMulticast(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	mc := map[string]any{
		"snooping": r.FormValue("snooping") == "on",
	}
	if q := r.FormValue("querier"); q != "" {
		mc["querier"] = q
	}
	if v, err := strconv.Atoi(r.FormValue("query-interval")); err == nil && v > 0 {
		mc["query-interval"] = v
	}
	body := map[string]any{"infix-interfaces:multicast": mc}
	path := ifacePath(name) + "/infix-interfaces:bridge/multicast"
	if err := h.RC.Patch(r.Context(), path, body); err != nil {
		log.Printf("configure interfaces %s bridge multicast: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSaved(w, "Multicast saved")
}

// SaveWifi PATCHes the WiFi interface container plus, when the form
// also carries radio fields, the mirrored wifi-radio component — both
// in a single PATCH on the candidate root so the interface SSID/sec
// and the radio's country/band/channel land atomically. The mode
// (station vs access-point) is fixed at wizard-create time and not
// switched here.
// POST /configure/interfaces/{name}/wifi
func (h *ConfigureInterfacesHandler) SaveWifi(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	mode := r.FormValue("mode")
	if mode != "station" && mode != "access-point" {
		renderSaveError(w, fmt.Errorf("mode must be 'station' or 'access-point'"))
		return
	}
	leaf := map[string]any{"ssid": r.FormValue("ssid")}
	if secMode := r.FormValue("sec-mode"); secMode != "" {
		sec := map[string]any{"mode": secMode}
		if secret := r.FormValue("secret"); secret != "" {
			sec["secret"] = secret
		}
		leaf["security"] = sec
	}
	if mode == "access-point" {
		leaf["hidden"] = r.FormValue("hidden") == "on"
	}
	wifi := map[string]any{mode: leaf}
	// Picker change re-binds the wifi/radio leaf so the user can move
	// the interface to a different (already-configured) radio.
	if radioRef := strings.TrimSpace(r.FormValue("radio")); radioRef != "" {
		wifi["radio"] = radioRef
	}
	iface := map[string]any{
		"name":                  name,
		"infix-interfaces:wifi": wifi,
	}
	body := map[string]any{
		"ietf-interfaces:interfaces": map[string]any{
			"interface": []map[string]any{iface},
		},
	}
	// Radio half of the atomic write — only when the form actually
	// carried a country (the wifi-radio container's mandatory leaf).
	// Without it parseWiFiRadio would reject a form whose user only
	// touched the WiFi side and left the radio fields untouched-empty.
	radio := strings.TrimSpace(r.FormValue("radio"))
	if radio != "" && strings.TrimSpace(r.FormValue("country-code")) != "" {
		rc, err := parseWiFiRadio(r)
		if err != nil {
			renderSaveError(w, err)
			return
		}
		body["ietf-hardware:hardware"] = map[string]any{
			"component": []map[string]any{{
				"name":                      radio,
				"class":                     "infix-hardware:wifi",
				"infix-hardware:wifi-radio": rc,
			}},
		}
	}
	if err := h.RC.Patch(r.Context(), candidatePath, body); err != nil {
		log.Printf("configure interfaces %s wifi: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSaved(w, "WiFi saved")
}

// DeleteLagPort detaches an interface from its LAG.
// DELETE /configure/interfaces/{name}/lag-port
func (h *ConfigureInterfacesHandler) DeleteLagPort(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.removeInterfaceAugment(r.Context(), name, "infix-interfaces:lag-port"); err != nil {
		log.Printf("configure interfaces %s lag-port delete: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Removed from LAG", "/configure/interfaces")
}

// SaveLAG saves LAG mode and LACP settings.
// POST /configure/interfaces/{name}/lag
func (h *ConfigureInterfacesHandler) SaveLAG(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	lag := map[string]any{
		"mode": r.FormValue("mode"),
	}
	if hash := r.FormValue("hash"); hash != "" {
		lag["hash"] = hash
	}
	if r.FormValue("mode") == "lacp" {
		lacp := map[string]any{
			"mode": r.FormValue("lacp-mode"),
			"rate": r.FormValue("lacp-rate"),
		}
		if v, err := strconv.Atoi(r.FormValue("lacp-system-priority")); err == nil {
			lacp["system-priority"] = v
		}
		lag["lacp"] = lacp
	}
	body := map[string]any{"infix-interfaces:lag": lag}
	if err := h.RC.Patch(r.Context(), ifacePath(name)+"/infix-interfaces:lag", body); err != nil {
		log.Printf("configure interfaces %s lag: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSaved(w, "LAG saved")
}

// SaveLAGMembers performs a diff-and-write to set the LAG's member ports.
// POST /configure/interfaces/{name}/lag/members
func (h *ConfigureInterfacesHandler) SaveLAGMembers(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	h.saveMembersDiff(w, r, r.PathValue("name"), "lag",
		func(iface ifaceJSON, master string) bool {
			return iface.LagPort != nil && iface.LagPort.LAG == master
		}, "LAG members saved")
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func ifacePath(name string) string {
	return ifaceCandPath + "/interface=" + url.PathEscape(name)
}

// indexWifiRadios picks WiFi radio components out of the hardware
// candidate tree and returns the minimal subset the WiFi interface
// editor mirrors (name, country, band, channel). Components without
// a wifi-radio container are skipped.
func indexWifiRadios(comps []hwComponentJSON) map[string]*ifaceRadioMirror {
	out := make(map[string]*ifaceRadioMirror, len(comps))
	for _, c := range comps {
		if c.WiFiRadio == nil {
			continue
		}
		m := &ifaceRadioMirror{
			Name:        c.Name,
			CountryCode: c.WiFiRadio.CountryCode,
			Band:        c.WiFiRadio.Band,
		}
		if ch, ok := c.WiFiRadio.Channel.(float64); ok && ch > 0 {
			v := int(ch)
			m.Channel = &v
		}
		out[c.Name] = m
	}
	return out
}

// saveMembersDiff syncs bridge/lag membership by diffing submitted form values
// against the current state: add port-kind for new members, remove for ex-members.
// kind is "bridge" or "lag"; it determines the YANG augment path and body key.
func (h *ConfigureInterfacesHandler) saveMembersDiff(w http.ResponseWriter, r *http.Request,
	masterName, kind string, isMember func(ifaceJSON, string) bool, successMsg string) {
	if err := h.applyMembersDiff(r, masterName, kind, isMember); err != nil {
		renderSaveError(w, err)
		return
	}
	renderSaved(w, successMsg)
}

// applyMembersDiff is the no-response-writing core of saveMembersDiff so it
// can be reused by callers that compose multiple save steps (e.g. SaveBridge
// which writes type + members in one form submission).
func (h *ConfigureInterfacesHandler) applyMembersDiff(r *http.Request,
	masterName, kind string, isMember func(ifaceJSON, string) bool) error {

	ifaces, err := h.fetchAllInterfaces(r.Context())
	if err != nil {
		return err
	}

	submitted := make(map[string]bool)
	for _, m := range r.Form["members"] {
		submitted[m] = true
	}

	portKey := "infix-interfaces:" + kind + "-port"
	for _, iface := range ifaces {
		if typeSlug(iface.Type) == kind {
			continue // skip master interface itself
		}
		currentlyMember := isMember(iface, masterName)
		wantMember := submitted[iface.Name]
		portPath := ifacePath(iface.Name) + "/" + portKey

		if wantMember && !currentlyMember {
			body := map[string]any{portKey: map[string]any{kind: masterName}}
			if err := h.RC.Put(r.Context(), portPath, body); err != nil {
				log.Printf("configure interfaces %s members add %s→%s: %v", kind, iface.Name, masterName, err)
				return err
			}
		} else if !wantMember && currentlyMember {
			if err := h.RC.Delete(r.Context(), portPath); err != nil {
				log.Printf("configure interfaces %s members remove %s from %s: %v", kind, iface.Name, masterName, err)
				return err
			}
		}
	}
	return nil
}

// unconfiguredPhysical returns physical Ethernet interfaces present in
// operational but absent from running config — the Add Interface row's
// datalist suggestions.
func unconfiguredPhysical(running, oper []ifaceJSON) []string {
	inRunning := make(map[string]bool, len(running))
	for _, iface := range running {
		inRunning[iface.Name] = true
	}
	var out []string
	for _, iface := range oper {
		if inRunning[iface.Name] {
			continue
		}
		if typeSlug(iface.Type) != ifTypeEthernet {
			continue
		}
		out = append(out, iface.Name)
	}
	sort.Strings(out)
	return out
}

func (h *ConfigureInterfacesHandler) fetchAllInterfaces(ctx context.Context) ([]ifaceJSON, error) {
	var wrap interfacesWrapper
	if err := h.RC.Get(ctx, ifaceCandPath, &wrap); err != nil {
		// Fall back to running only on 404 (candidate has no interfaces configured yet).
		// Any other error (validation failure, server error) is surfaced directly to
		// avoid silently showing stale running data while the candidate is in bad shape.
		if !restconf.IsNotFound(err) {
			return nil, err
		}
		log.Printf("configure interfaces: candidate returned 404, using running datastore")
		if err2 := h.RC.Get(ctx, "/data/ietf-interfaces:interfaces", &wrap); err2 != nil {
			return nil, err2
		}
	}
	return wrap.Interfaces.Interface, nil
}

type membership struct{ kind, master string }

func (h *ConfigureInterfacesHandler) buildRows(ifaces []ifaceJSON, oper []ifaceJSON) []cfgIfaceRow {
	// Index operational so each row can pull supported-pmd-types — that
	// leaf is config false, so the candidate read doesn't carry it.
	operByName := make(map[string]*ifaceJSON, len(oper))
	for i := range oper {
		operByName[oper[i].Name] = &oper[i]
	}
	// Build a set of current bridge/lag members for fast lookup.
	memberOf := make(map[string]membership, len(ifaces))
	for _, iface := range ifaces {
		if iface.BridgePort != nil && iface.BridgePort.Bridge != "" {
			memberOf[iface.Name] = membership{"bridge", iface.BridgePort.Bridge}
		} else if iface.LagPort != nil && iface.LagPort.LAG != "" {
			memberOf[iface.Name] = membership{"lag", iface.LagPort.LAG}
		}
	}

	// Pre-compute bridge member sets and a "bridge name → is 802.1Q?"
	// lookup so each port row can hide PVID when its parent isn't
	// VLAN-filtering (PVID only applies in that mode).
	bridgeMembers := make(map[string][]string)
	bridgeIs8021Q := make(map[string]bool)
	for _, iface := range ifaces {
		if typeSlug(iface.Type) == "bridge" && iface.Bridge != nil && iface.Bridge.VLANs != nil {
			bridgeIs8021Q[iface.Name] = true
		}
		if m, ok := memberOf[iface.Name]; ok && m.kind == "bridge" {
			bridgeMembers[m.master] = append(bridgeMembers[m.master], iface.Name)
		}
	}
	lagMembers := make(map[string][]string)
	for _, iface := range ifaces {
		if m, ok := memberOf[iface.Name]; ok && m.kind == "lag" {
			lagMembers[m.master] = append(lagMembers[m.master], iface.Name)
		}
	}

	rows := make([]cfgIfaceRow, 0, len(ifaces))
	for _, iface := range ifaces {
		slug := typeSlug(iface.Type)
		row := cfgIfaceRow{
			ifaceJSON:    iface,
			TypeSlug:     slug,
			TypeDisplay:  typeDisplay(slug),
			AdminEnabled: iface.Enabled == nil || *iface.Enabled,
			IsBridge:     slug == "bridge",
			IsBridgePort: iface.BridgePort != nil,
			IsLag:        slug == "lag",
			IsLagPort:    iface.LagPort != nil,
			IsVlan:       slug == "vlan",
			IsWifi:       slug == "wifi",
		}
		if iface.WiFi != nil {
			switch {
			case iface.WiFi.AccessPoint != nil:
				row.WifiMode = "access-point"
			case iface.WiFi.Station != nil:
				row.WifiMode = "station"
			}
		}
		row.EthAutoneg = true // YANG default when no candidate value is set
		if iface.Ethernet != nil {
			row.EthDuplex = iface.Ethernet.Duplex
			if iface.Ethernet.AutoNegotiation != nil {
				row.EthAutoneg = iface.Ethernet.AutoNegotiation.Enable
				row.EthAdvertised = iface.Ethernet.AutoNegotiation.AdvertisedPMDs
			}
			if iface.Ethernet.MDIX != nil {
				if *iface.Ethernet.MDIX {
					row.MDIXState = "true"
				} else {
					row.MDIXState = "false"
				}
			}
		}
		if op := operByName[iface.Name]; op != nil && op.Ethernet != nil {
			row.EthSupported = op.Ethernet.SupportedPMDs
		}
		if m, ok := memberOf[iface.Name]; ok {
			row.MemberOf = m.master
		}
		if row.IsBridgePort && iface.BridgePort != nil {
			row.ParentBridgeIs8021Q = bridgeIs8021Q[iface.BridgePort.Bridge]
		}
		row.HasIP = !row.IsBridgePort && !row.IsLagPort
		// The DHCP/DHCPv6 foldouts are always rendered so users can
		// discover the settings form before enabling the client. The
		// foldout body iterates .IPv4.DHCP / .IPv6.DHCPv6 — populate
		// placeholders here so the template can use .IPv4.DHCP.X without
		// a forest of nil guards. Whether DHCP is actually configured is
		// captured separately on .DHCPv4Enabled / .DHCPv6Enabled so the
		// checkbox state and foldout-hidden gate still reflect candidate.
		if row.HasIP {
			if row.IPv4 == nil {
				row.IPv4 = &ipCfg{}
			} else {
				row.DHCPv4Enabled = row.IPv4.DHCP != nil
			}
			if row.IPv4.DHCP == nil {
				row.IPv4.DHCP = &dhcpv4CfgJSON{}
			}
			if row.IPv6 == nil {
				row.IPv6 = &ipCfg{}
			} else {
				row.DHCPv6Enabled = row.IPv6.DHCPv6 != nil
			}
			if row.IPv6.DHCPv6 == nil {
				row.IPv6.DHCPv6 = &dhcpv6CfgJSON{}
			}
		}
		row.AddrSummary = addrSummary(iface)
		row.ConfigTags = configSummary(&row)

		if row.IsBridge {
			members := bridgeMembers[iface.Name]
			sort.Strings(members)
			row.BridgeMembers = members
			row.BridgeMemberSet = make(map[string]bool, len(members))
			for _, m := range members {
				row.BridgeMemberSet[m] = true
			}
			row.PortCandidates = portCandidatesFor(iface.Name, ifaces, memberOf)
			if iface.Bridge != nil && iface.Bridge.VLANs != nil {
				row.BridgeIs8021Q = true
				if q := iface.Bridge.VLANs; q != nil {
					for _, v := range q.VLANs {
						untaggedSet := make(map[string]bool, len(v.Untagged))
						for _, u := range v.Untagged {
							untaggedSet[u] = true
						}
						taggedSet := make(map[string]bool, len(v.Tagged))
						for _, t := range v.Tagged {
							taggedSet[t] = true
						}
						row.VLANRows = append(row.VLANRows, cfgVLANRow{
							VID:         v.VID,
							UntaggedTxt: strings.Join(v.Untagged, ", "),
							TaggedTxt:   strings.Join(v.Tagged, ", "),
							UntaggedSet: untaggedSet,
							TaggedSet:   taggedSet,
						})
					}
				}
			}
		}

		if row.IsLag {
			members := lagMembers[iface.Name]
			sort.Strings(members)
			row.BridgeMembers = members // reuse field — LAG members shown same way
			row.BridgeMemberSet = make(map[string]bool, len(members))
			for _, m := range members {
				row.BridgeMemberSet[m] = true
			}
			row.PortCandidates = portCandidatesFor(iface.Name, ifaces, memberOf)
		}

		rows = append(rows, row)
	}

	sort.Slice(rows, func(i, j int) bool {
		ri, rj := rows[i], rows[j]
		// Sort order: bridge/lag first, then by type, then by name.
		orderI, orderJ := typeOrder(ri.TypeSlug), typeOrder(rj.TypeSlug)
		if orderI != orderJ {
			return orderI < orderJ
		}
		return ri.Name < rj.Name
	})
	return rows
}

// portCandidatesFor returns sorted candidate port names for a given bridge or LAG.
// Included: ports that are free (no master) or already a member of masterName.
// Excluded: bridge, lag, loopback, dummy, wireguard, tunnel types, and ports
// enslaved to a different master.
func portCandidatesFor(masterName string, ifaces []ifaceJSON, memberOf map[string]membership) []string {
	var out []string
	for _, iface := range ifaces {
		switch typeSlug(iface.Type) {
		case "bridge", "lag", "loopback", "dummy", "wireguard", "gre", "gretap", "vxlan":
			continue
		}
		m := memberOf[iface.Name]
		if m.master == "" || m.master == masterName {
			out = append(out, iface.Name)
		}
	}
	sort.Strings(out)
	return out
}

func (h *ConfigureInterfacesHandler) addAddr(w http.ResponseWriter, r *http.Request, family, famCap, frag string) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	ip := strings.TrimSpace(r.FormValue("ip"))
	prefixStr := r.FormValue("prefix-length")
	prefix, err := strconv.Atoi(prefixStr)
	if err != nil || ip == "" {
		renderSaveError(w, fmt.Errorf("valid IP address and prefix length required"))
		return
	}
	// A PUT to a list element needs the entry wrapped in a single-element
	// array, not a bare object — a bare object fails YANG validation (LY_EVALID).
	body := map[string]any{
		"ietf-ip:address": []map[string]any{{
			"ip":            ip,
			"prefix-length": prefix,
		}},
	}
	path := ifacePath(name) + "/ietf-ip:" + family + "/address=" + restconf.EscapeKey(ip)
	if err := h.RC.Put(r.Context(), path, body); err != nil {
		log.Printf("configure interfaces %s add %s addr: %v", name, family, err)
		renderSaveError(w, err)
		return
	}
	// Re-render the IP block in place so the new address appears without
	// collapsing the interface foldout — ready to add another straight away.
	h.renderIPBlock(w, r, name, famCap, frag)
}

func (h *ConfigureInterfacesHandler) deleteAddr(w http.ResponseWriter, r *http.Request, family, famCap, frag string) {
	name := r.PathValue("name")
	ip := r.PathValue("ip")
	path := ifacePath(name) + "/ietf-ip:" + family + "/address=" + restconf.EscapeKey(ip)
	if err := h.RC.Delete(r.Context(), path); err != nil {
		log.Printf("configure interfaces %s delete %s addr %s: %v", name, family, ip, err)
		renderSaveError(w, err)
		return
	}
	h.renderIPBlock(w, r, name, famCap, frag)
}

// SaveIPv4Settings PATCHes the per-interface IPv4 group settings — forwarding
// leaf plus the DHCP-client and link-local autoconf presence containers — in
// a single round trip from the IPv4 settings form. Each presence container is
// PUT (enable) or DELETE (disable) per checkbox state; forwarding is PATCHed.
// POST /configure/interfaces/{name}/ipv4/settings
func (h *ConfigureInterfacesHandler) SaveIPv4Settings(w http.ResponseWriter, r *http.Request) {
	h.saveIPSettings(w, r, "ietf-ip:ipv4", "IPv4", "iface-ipv4-block", map[string]string{
		"dhcp":     "infix-dhcp-client:dhcp",
		"autoconf": "infix-ip:autoconf",
	})
}

// SaveIPv6Settings is the IPv6 counterpart of SaveIPv4Settings. SLAAC lives at
// the standard ietf-ip "autoconf" container; DHCPv6 is the Infix augment.
// POST /configure/interfaces/{name}/ipv6/settings
func (h *ConfigureInterfacesHandler) SaveIPv6Settings(w http.ResponseWriter, r *http.Request) {
	h.saveIPSettings(w, r, "ietf-ip:ipv6", "IPv6", "iface-ipv6-block", map[string]string{
		"dhcp":  "infix-dhcpv6-client:dhcp",
		"slaac": "autoconf",
	})
}

// saveIPSettings is the shared body of SaveIPv4Settings / SaveIPv6Settings.
// presenceMap maps form-field names (e.g. "dhcp") to their YANG presence
// container key (e.g. "infix-dhcp-client:dhcp"); each one is PUT when checked
// and DELETEd otherwise. Forwarding is always PATCHed.
func (h *ConfigureInterfacesHandler) saveIPSettings(w http.ResponseWriter, r *http.Request, container, family, fragName string, presenceMap map[string]string) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	base := ifacePath(name) + "/" + container

	forwarding := r.FormValue("forwarding") == "true"
	// PATCH the interface (which always exists) with the family container
	// nested inside, so the container is created on first use.  PATCHing
	// `base` (the ipv4/ipv6 container) directly 400s "Target resource does
	// not exist" on a fresh interface that has no L3 config yet — which is
	// exactly the case when enabling DHCP for the first time.  The list
	// entry must be a single-element array: rousette rejects the bare-object
	// form with LY_EVALID once an augmented container (ietf-ip:ipv4) is
	// nested inside.
	body := map[string]any{
		"ietf-interfaces:interface": []any{
			map[string]any{
				"name":    name,
				container: map[string]any{"forwarding": forwarding},
			},
		},
	}
	if err := h.RC.Patch(r.Context(), ifacePath(name), body); err != nil {
		log.Printf("configure interfaces %s %s settings: forwarding: %v", name, family, err)
		renderSaveError(w, err)
		return
	}

	for field, child := range presenceMap {
		path := base + "/" + child
		if r.FormValue(field) == "true" {
			b := map[string]any{child: map[string]any{}}
			if err := h.RC.Put(r.Context(), path, b); err != nil {
				log.Printf("configure interfaces %s %s settings: enable %s: %v", name, family, field, err)
				renderSaveError(w, err)
				return
			}
		} else {
			if err := h.RC.Delete(r.Context(), path); err != nil && !restconf.IsNotFound(err) {
				log.Printf("configure interfaces %s %s settings: disable %s: %v", name, family, field, err)
				renderSaveError(w, err)
				return
			}
		}
	}

	// Re-render just this interface's IP block from the fresh candidate so
	// confd-inferred values (the DHCP option list, etc.) appear in place,
	// without collapsing the page.  fragName empty → fall back to a toast
	// (e.g. families whose block fragment isn't wired up yet).
	if fragName != "" {
		h.renderIPBlock(w, r, name, family, fragName)
		return
	}
	renderSaved(w, family+" settings saved")
}

// renderIPBlock re-renders one interface's IPv4/IPv6 block fragment from the
// fresh candidate.  Falls back to a plain saved-toast if the interface or
// schema can't be read, so a save is never reported as failed just because
// the in-place refresh couldn't be built.
func (h *ConfigureInterfacesHandler) renderIPBlock(w http.ResponseWriter, r *http.Request, name, family, fragName string) {
	ifaces, err := h.fetchAllInterfaces(r.Context())
	if err != nil {
		renderSaved(w, family+" settings saved")
		return
	}
	rows := h.buildRows(ifaces, nil)
	var row *cfgIfaceRow
	for i := range rows {
		if rows[i].Name == name {
			row = &rows[i]
			break
		}
	}
	if row == nil {
		renderSaved(w, family+" settings saved")
		return
	}
	if mgr := h.Schema.Manager(); mgr != nil {
		ifPath := "/ietf-interfaces:interfaces/interface"
		ip4, ip6 := "/ietf-ip:ipv4", "/ietf-ip:ipv6"
		row.DHCPv4Options = schema.OptionsFor(mgr, ifPath+ip4+"/infix-dhcp-client:dhcp/option/id")
		row.DHCPv6Options = schema.OptionsFor(mgr, ifPath+ip6+"/infix-dhcpv6-client:dhcp/option/id")
		row.Desc = map[string]string{
			"ipv4-address":    schema.DescriptionOf(mgr, ifPath+ip4+"/address/ip"),
			"ipv4-dhcp":       schema.DescriptionOf(mgr, ifPath+ip4+"/infix-dhcp-client:dhcp"),
			"ipv4-autoconf":   schema.DescriptionOf(mgr, ifPath+ip4+"/infix-ip:autoconf"),
			"ipv4-forwarding": schema.DescriptionOf(mgr, ifPath+ip4+"/forwarding"),
			"ipv6-address":    schema.DescriptionOf(mgr, ifPath+ip6+"/address/ip"),
			"ipv6-dhcp":       schema.DescriptionOf(mgr, ifPath+ip6+"/infix-dhcpv6-client:dhcp"),
			"ipv6-slaac":      schema.DescriptionOf(mgr, ifPath+ip6+"/autoconf"),
			"ipv6-forwarding": schema.DescriptionOf(mgr, ifPath+ip6+"/forwarding"),
		}
	}
	row.JustSaved = true // expand the DHCP foldout to reveal inferred options
	// Keep the cfgSaved toast behaviour even though we swap the block.
	w.Header().Set("HX-Trigger", `{"cfgSaved":"`+family+` settings saved"}`)
	if err := h.Template.ExecuteTemplate(w, fragName, row); err != nil {
		log.Printf("configure interfaces %s: render %s: %v", name, fragName, err)
	}
}

func (h *ConfigureInterfacesHandler) SaveIPv4DHCPSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	body := map[string]any{}
	if v := r.FormValue("client-id"); v != "" {
		body["client-id"] = v
	}
	if v := r.FormValue("arping"); v != "" {
		body["arping"] = v == "true"
	}
	if v := r.FormValue("route-preference"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil {
			body["route-preference"] = n
		}
	}
	path := ifacePath(name) + "/ietf-ip:ipv4/infix-dhcp-client:dhcp"
	if err := h.RC.Patch(r.Context(), path, map[string]any{"infix-dhcp-client:dhcp": body}); err != nil {
		log.Printf("configure interfaces dhcpv4 settings %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSaved(w, "DHCP settings saved")
}

func (h *ConfigureInterfacesHandler) addDHCPOption(w http.ResponseWriter, r *http.Request, ipPath, bodyKey string) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	id := strings.TrimSpace(r.FormValue("id"))
	if id == "" {
		renderSaveError(w, fmt.Errorf("option id is required"))
		return
	}
	entry := map[string]any{"id": id}
	if v := strings.TrimSpace(r.FormValue("value")); v != "" {
		entry["value"] = v
	}
	path := ifacePath(name) + "/" + ipPath + "/option=" + url.PathEscape(id)
	if err := h.RC.Put(r.Context(), path, map[string]any{bodyKey: []map[string]any{entry}}); err != nil {
		log.Printf("configure interfaces dhcp option add %q/%q: %v", name, id, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Option added", "/configure/interfaces")
}

func (h *ConfigureInterfacesHandler) deleteDHCPOption(w http.ResponseWriter, r *http.Request, ipPath string) {
	name := r.PathValue("name")
	id := r.PathValue("id")
	path := ifacePath(name) + "/" + ipPath + "/option=" + url.PathEscape(id)
	if err := h.RC.Delete(r.Context(), path); err != nil {
		log.Printf("configure interfaces dhcp option delete %q/%q: %v", name, id, err)
		renderSaveError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *ConfigureInterfacesHandler) AddIPv4DHCPOption(w http.ResponseWriter, r *http.Request) {
	h.addDHCPOption(w, r, "ietf-ip:ipv4/infix-dhcp-client:dhcp", "infix-dhcp-client:option")
}

func (h *ConfigureInterfacesHandler) DeleteIPv4DHCPOption(w http.ResponseWriter, r *http.Request) {
	h.deleteDHCPOption(w, r, "ietf-ip:ipv4/infix-dhcp-client:dhcp")
}

func (h *ConfigureInterfacesHandler) SaveIPv6DHCPSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	body := map[string]any{}
	if v := r.FormValue("duid"); v != "" {
		body["duid"] = v
	}
	if v := r.FormValue("information-only"); v != "" {
		body["information-only"] = v == "true"
	}
	if v := r.FormValue("route-preference"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil {
			body["route-preference"] = n
		}
	}
	path := ifacePath(name) + "/ietf-ip:ipv6/infix-dhcpv6-client:dhcp"
	if err := h.RC.Patch(r.Context(), path, map[string]any{"infix-dhcpv6-client:dhcp": body}); err != nil {
		log.Printf("configure interfaces dhcpv6 settings %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSaved(w, "DHCPv6 settings saved")
}

func (h *ConfigureInterfacesHandler) AddIPv6DHCPOption(w http.ResponseWriter, r *http.Request) {
	h.addDHCPOption(w, r, "ietf-ip:ipv6/infix-dhcpv6-client:dhcp", "infix-dhcpv6-client:option")
}

func (h *ConfigureInterfacesHandler) DeleteIPv6DHCPOption(w http.ResponseWriter, r *http.Request) {
	h.deleteDHCPOption(w, r, "ietf-ip:ipv6/infix-dhcpv6-client:dhcp")
}

func typeSlug(yangType string) string {
	s := schema.StripModulePrefix(yangType)
	// Normalise iana-if-type identities to infix slugs where relevant.
	switch s {
	case "bridge":
		return "bridge"
	case "ieee8023adLag":
		return "lag"
	case "ethernetCsmacd":
		return "ethernet"
	case "l2vlan":
		return "vlan"
	case "softwareLoopback":
		return "loopback"
	}
	return s
}

func typeDisplay(slug string) string {
	switch slug {
	case "bridge":
		return "Bridge"
	case "lag":
		return "LAG"
	case "ethernet":
		return "Ethernet"
	case "vlan":
		return "VLAN"
	case "loopback":
		return "Loopback"
	case "dummy":
		return "Dummy"
	case "wireguard":
		return "WireGuard"
	case "veth":
		return "veth"
	case "gre", "gretap":
		return strings.ToUpper(slug)
	case "vxlan":
		return "VXLAN"
	}
	return slug
}

func typeOrder(slug string) int {
	switch slug {
	case "bridge":
		return 0
	case "lag":
		return 1
	case "ethernet":
		return 2
	case "vlan":
		return 3
	case "loopback":
		return 4
	default:
		return 5
	}
}

// ifaceTypeOption is one option in the Add Interface modal's type pulldown.
type ifaceTypeOption struct {
	Slug  string
	Value string // YANG-qualified identity, e.g. "infix-if-type:bridge"
	Label string
}

// symKeyEntry is one option in the wizard's WiFi PSK picker. Value is
// the decoded cleartext (decoded from the keystore's base64-encoded
// cleartext-symmetric-key) so the Edit affordance can pre-fill the
// passphrase input. Empty when the key isn't printable ASCII (e.g. a
// raw octet-string secret).
type symKeyEntry struct {
	Name  string
	Value string
}

// wifiRadioOption is one entry in the wizard's WiFi radio picker. APReady
// reflects the YANG must-clauses that require band+channel+country-code on
// the radio component before an access-point interface can be attached.
// Country/Band/Channel are surfaced as data-* attributes so the Edit
// affordance can pre-fill the inline form from the currently-selected
// option.
type wifiRadioOption struct {
	Name    string
	Label   string // e.g. "phy0 — 5GHz ch36 SE"
	Country string
	Band    string
	Channel string
	APReady bool
}

// buildIfaceTypeList returns all identities derived from
// infix-if-type:infix-interface-type for the Add Interface modal's
// pulldown. Sorted by typeOrder then alphabetical. Includes types whose
// Create path is not yet implemented (gre, gretap, vxlan, wireguard,
// wifi, …) — the modal's "unsupported" panel handles those by pointing
// the user at the Edit-all YANG tree.
// buildWifiRadioOptions filters detected hardware components down to
// configured WiFi radios (class=wifi with a wifi-radio container present
// in running config) and returns picker entries with a label that hints
// at band/channel/country. APReady reflects the YANG must-clauses on
// access-point — band, channel, and country-code all set, with country
// != "00" (world regulatory domain is rejected for AP mode).
func buildWifiRadioOptions(comps []hwComponentJSON) []wifiRadioOption {
	var out []wifiRadioOption
	for _, c := range comps {
		if shortClass(c.Class) != classWiFi || c.WiFiRadio == nil {
			continue
		}
		ch := wifiChannelString(c.WiFiRadio.Channel)
		opt := wifiRadioOption{
			Name:    c.Name,
			Country: c.WiFiRadio.CountryCode,
			Band:    c.WiFiRadio.Band,
			Channel: ch,
		}
		var bits []string
		if opt.Band != "" {
			bits = append(bits, opt.Band)
		}
		if ch != "" {
			bits = append(bits, "ch"+ch)
		}
		if opt.Country != "" {
			bits = append(bits, opt.Country)
		}
		if len(bits) > 0 {
			opt.Label = c.Name + " — " + strings.Join(bits, " ")
		} else {
			opt.Label = c.Name
		}
		opt.APReady = opt.Band != "" && ch != "" && opt.Country != "" && opt.Country != "00"
		out = append(out, opt)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// descOr returns the YANG description at path, falling back to the
// given literal when the schema lookup returns an empty string (which
// today happens for any leaf reached through an infix-interfaces:*
// augment — goyang doesn't currently traverse the augment into the
// ietf-interfaces:interface Entry tree).
func descOr(mgr *schema.Manager, path, fallback string) string {
	if d := schema.DescriptionOf(mgr, path); d != "" {
		return d
	}
	return fallback
}

func buildIfaceTypeList(mgr *schema.Manager) []ifaceTypeOption {
	names := mgr.IdentitiesOf("infix-interface-type")
	opts := make([]ifaceTypeOption, 0, len(names))
	for _, name := range names {
		opts = append(opts, ifaceTypeOption{
			Slug:  name,
			Value: ifTypePrefix + name,
			Label: typeDisplay(name),
		})
	}
	sort.SliceStable(opts, func(i, j int) bool {
		oa, ob := typeOrder(opts[i].Slug), typeOrder(opts[j].Slug)
		if oa != ob {
			return oa < ob
		}
		return opts[i].Slug < opts[j].Slug
	})
	return opts
}

// buildWizardNames precomputes the next-free name suggestion for every
// type whose name has a natural numeric prefix. The veth peer suggestion
// lives under key "veth-peer" so callers see a single uniform map.
// Ethernet is intentionally absent — its name comes from the datalist
// of unconfigured interfaces, not a numeric prefix.
func buildWizardNames(taken map[string]bool) map[string]string {
	vethA, vethB := nextFreeVethPair(taken)
	return map[string]string{
		"bridge":    nextFreeName("br", taken),
		"lag":       nextFreeName("lag", taken),
		"vlan":      nextFreeName("vlan", taken),
		"dummy":     nextFreeName("dummy", taken),
		"loopback":  nextFreeName("lo", taken),
		"veth":      vethA,
		"veth-peer": vethB,
		"wireguard": nextFreeName("wg", taken),
		"gre":       nextFreeName("gre", taken),
		"gretap":    nextFreeName("gretap", taken),
		"vxlan":     nextFreeName("vxlan", taken),
		"wifi":      nextFreeName("wifi", taken),
	}
}

// nextFreeName returns the smallest N (starting at 0) where <prefix><N>
// is not in `taken`. Used by the wizard to pre-fill the Name field with
// a sensible default.
func nextFreeName(prefix string, taken map[string]bool) string {
	for i := 0; i < 1000; i++ {
		name := fmt.Sprintf("%s%d", prefix, i)
		if !taken[name] {
			return name
		}
	}
	return prefix + "0"
}

// nextFreeVethPair returns (veth<N>a, veth<N>b) for the smallest N where
// neither side is taken. Matches the user's preferred a/b suffix
// convention for VETH pairs.
func nextFreeVethPair(taken map[string]bool) (string, string) {
	for i := 0; i < 1000; i++ {
		a := fmt.Sprintf("veth%da", i)
		b := fmt.Sprintf("veth%db", i)
		if !taken[a] && !taken[b] {
			return a, b
		}
	}
	return "veth0a", "veth0b"
}

func addrSummary(iface ifaceJSON) string {
	var addrs []string
	if iface.IPv4 != nil {
		for _, a := range iface.IPv4.Address {
			addrs = append(addrs, fmt.Sprintf("%s/%d", a.IP, int(a.PrefixLength)))
		}
	}
	if iface.IPv6 != nil {
		for _, a := range iface.IPv6.Address {
			addrs = append(addrs, fmt.Sprintf("%s/%d", a.IP, int(a.PrefixLength)))
		}
	}
	switch len(addrs) {
	case 0:
		return ""
	case 1:
		return addrs[0]
	default:
		return fmt.Sprintf("%d addresses", len(addrs))
	}
}

// configSummary builds the type-aware overview pills for an interface in the
// Configure list. It mirrors the Status > Interfaces "Data" column but is
// slanted to configured intent rather than operational result: each role
// contributes the properties worth seeing without unfolding the row — IP
// acquisition (DHCP, SLAAC, zeroconf) for L3 interfaces, the tag id for VLANs,
// the radio mode for WiFi, and 802.1Q for VLAN-filtering bridges. Static
// addresses are rendered separately via AddrSummary.
func configSummary(row *cfgIfaceRow) []string {
	var tags []string

	if row.IsVlan && row.Vlan != nil {
		tags = append(tags, fmt.Sprintf("vid %d", row.Vlan.ID))
	}
	if row.IsWifi && row.WifiMode != "" {
		tags = append(tags, row.WifiMode)
	}
	if row.IsBridge && row.BridgeIs8021Q {
		tags = append(tags, "802.1Q")
	}
	if row.HasIP {
		if row.DHCPv4Enabled {
			tags = append(tags, "DHCPv4")
		}
		if row.IPv4 != nil && row.IPv4.Autoconf != nil {
			tags = append(tags, "Zeroconf")
		}
		if row.DHCPv6Enabled {
			tags = append(tags, "DHCPv6")
		}
		if row.IPv6 != nil && row.IPv6.SLAACv6 != nil {
			tags = append(tags, "SLAAC")
		}
	}

	return tags
}
