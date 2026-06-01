// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/kernelkit/webui/internal/restconf"
	"github.com/kernelkit/webui/internal/schema"
)

const ifaceCandPath = candidatePath + "/ietf-interfaces:interfaces"

// ─── RESTCONF JSON structs (configure-only fields) ───────────────────────────

type bridgeCfgJSON struct {
	IEEEGroupFwd []string           `json:"ieee-group-forward"`
	VLANs        *vlanFilterCfgJSON `json:"vlans"` // non-nil means 802.1Q mode
	STP          bridgeSTPCfgJSON   `json:"stp"`
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
	HasIP           bool // can carry IP addresses
}

type cfgIfacePageData struct {
	PageData
	Loading         bool
	Interfaces      []cfgIfaceRow
	AllNames        []string // every interface name
	BridgeNames     []string // type=bridge only
	LagNames        []string // type=lag only
	Desc            map[string]string
	STPProtoOptions []schema.IdentityOption
	LagModeOptions  []schema.IdentityOption
	LagHashOptions  []schema.IdentityOption
	Error           string
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
		PageData: newPageData(r, "configure-interfaces", "Configure: Interfaces"),
	}

	mgr := h.Schema.Manager()
	data.Loading = mgr == nil
	if mgr != nil {
		ifPath := "/ietf-interfaces:interface"
		bPath := "/infix-interfaces:bridge"
		lPath := "/infix-interfaces:lag"
		ip4 := "/ietf-ip:ipv4"
		ip6 := "/ietf-ip:ipv6"
		data.Desc = map[string]string{
			"description":    schema.DescriptionOf(mgr, ifPath+"/description"),
			"enabled":        schema.DescriptionOf(mgr, ifPath+"/enabled"),
			"bridge-type":    schema.DescriptionOf(mgr, ifPath+bPath+"/vlans"),
			"stp-force":      schema.DescriptionOf(mgr, ifPath+bPath+"/stp/force-protocol"),
			"stp-hello":      schema.DescriptionOf(mgr, ifPath+bPath+"/stp/hello-time"),
			"stp-fwd-delay":  schema.DescriptionOf(mgr, ifPath+bPath+"/stp/forward-delay"),
			"stp-max-age":    schema.DescriptionOf(mgr, ifPath+bPath+"/stp/max-age"),
			"stp-hold-count": schema.DescriptionOf(mgr, ifPath+bPath+"/stp/transmit-hold-count"),
			"stp-max-hops":   schema.DescriptionOf(mgr, ifPath+bPath+"/stp/max-hops"),
			"lag-mode":       schema.DescriptionOf(mgr, ifPath+lPath+"/mode"),
			"lag-hash":       schema.DescriptionOf(mgr, ifPath+lPath+"/hash"),
			"lacp-mode":      schema.DescriptionOf(mgr, ifPath+lPath+"/lacp/mode"),
			"lacp-rate":      schema.DescriptionOf(mgr, ifPath+lPath+"/lacp/rate"),
			"lacp-sysprio":   schema.DescriptionOf(mgr, ifPath+lPath+"/lacp/system-priority"),
			"vlan-id":        schema.DescriptionOf(mgr, ifPath+"/infix-interfaces:vlan/id"),
			"vlan-lower":     schema.DescriptionOf(mgr, ifPath+"/infix-interfaces:vlan/lower-layer-if"),
			"ipv4-address":   schema.DescriptionOf(mgr, ifPath+ip4+"/address/ip"),
			"ipv4-prefix":    schema.DescriptionOf(mgr, ifPath+ip4+"/address/prefix-length"),
			"ipv4-dhcp":      schema.DescriptionOf(mgr, ifPath+ip4+"/infix-dhcp-client:dhcp"),
			"ipv4-autoconf":  schema.DescriptionOf(mgr, ifPath+ip4+"/infix-ip:autoconf"),
			"ipv6-address":   schema.DescriptionOf(mgr, ifPath+ip6+"/address/ip"),
			"ipv6-prefix":    schema.DescriptionOf(mgr, ifPath+ip6+"/address/prefix-length"),
			"ipv6-slaac":     schema.DescriptionOf(mgr, ifPath+ip6+"/autoconf"),
			"ipv6-dhcp":      schema.DescriptionOf(mgr, ifPath+ip6+"/infix-dhcpv6-client:dhcp"),
		}
		data.STPProtoOptions = schema.OptionsFor(mgr, ifPath+bPath+"/stp/force-protocol")
		data.LagModeOptions = schema.OptionsFor(mgr, ifPath+lPath+"/mode")
		data.LagHashOptions = schema.OptionsFor(mgr, ifPath+lPath+"/hash")
	}

	ifaces, err := h.fetchAllInterfaces(r.Context())
	if err != nil {
		log.Printf("configure interfaces: %v", err)
		data.Error = "Could not read interface configuration"
	}

	data.Interfaces = h.buildRows(ifaces)

	for _, iface := range ifaces {
		slug := typeSlug(iface.Type)
		data.AllNames = append(data.AllNames, iface.Name)
		switch slug {
		case "bridge":
			data.BridgeNames = append(data.BridgeNames, iface.Name)
		case "lag":
			data.LagNames = append(data.LagNames, iface.Name)
		}
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
		iface["infix-interfaces:bridge"] = map[string]any{}
	case "lag":
		iface["infix-interfaces:lag"] = map[string]any{"mode": "static"}
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
	}

	body := map[string]any{"ietf-interfaces:interface": []map[string]any{iface}}
	if err := h.RC.Put(r.Context(), ifacePath(name), body); err != nil {
		log.Printf("configure interfaces create %s: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, fmt.Sprintf("%s created", name), "/configure/interfaces")
}

// SaveGeneral saves description and enabled for any interface.
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
	renderSavedRedirect(w, "Saved", "/configure/interfaces")
}

// AddIPv4 adds an IPv4 address to an interface.
// POST /configure/interfaces/{name}/ipv4
func (h *ConfigureInterfacesHandler) AddIPv4(w http.ResponseWriter, r *http.Request) {
	h.addAddr(w, r, "ipv4")
}

// DeleteIPv4 removes an IPv4 address from an interface.
// DELETE /configure/interfaces/{name}/ipv4/{ip}
func (h *ConfigureInterfacesHandler) DeleteIPv4(w http.ResponseWriter, r *http.Request) {
	h.deleteAddr(w, r, "ipv4")
}

// AddIPv6 adds an IPv6 address to an interface.
// POST /configure/interfaces/{name}/ipv6
func (h *ConfigureInterfacesHandler) AddIPv6(w http.ResponseWriter, r *http.Request) {
	h.addAddr(w, r, "ipv6")
}

// DeleteIPv6 removes an IPv6 address from an interface.
// DELETE /configure/interfaces/{name}/ipv6/{ip}
func (h *ConfigureInterfacesHandler) DeleteIPv6(w http.ResponseWriter, r *http.Request) {
	h.deleteAddr(w, r, "ipv6")
}

// SaveBridgePort assigns or updates an interface's bridge membership.
// POST /configure/interfaces/{name}/bridge-port
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
	body := map[string]any{
		"infix-interfaces:bridge-port": map[string]any{
			"bridge": bridge,
		},
	}
	if pvid := r.FormValue("pvid"); pvid != "" {
		if v, err := strconv.Atoi(pvid); err == nil && v > 0 {
			body["infix-interfaces:bridge-port"].(map[string]any)["pvid"] = v
		}
	}
	if err := h.RC.Put(r.Context(), ifacePath(name)+"/infix-interfaces:bridge-port", body); err != nil {
		log.Printf("configure interfaces %s bridge-port: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Bridge port saved", "/configure/interfaces")
}

// DeleteBridgePort removes an interface from its bridge.
// DELETE /configure/interfaces/{name}/bridge-port
func (h *ConfigureInterfacesHandler) DeleteBridgePort(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.RC.Delete(r.Context(), ifacePath(name)+"/infix-interfaces:bridge-port"); err != nil {
		log.Printf("configure interfaces %s bridge-port delete: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Removed from bridge", "/configure/interfaces")
}

// SaveBridgeMembers performs a diff-and-write to set the bridge's member ports.
// POST /configure/interfaces/{name}/bridge/members
func (h *ConfigureInterfacesHandler) SaveBridgeMembers(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	h.saveMembersDiff(w, r, r.PathValue("name"), "bridge",
		func(iface ifaceJSON, master string) bool {
			return iface.BridgePort != nil && iface.BridgePort.Bridge == master
		}, "Bridge members saved")
}

// SaveBridge saves bridge STP settings and bridge type.
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

	renderSavedRedirect(w, "Bridge saved", "/configure/interfaces")
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
	renderSavedRedirect(w, "VLAN saved", "/configure/interfaces")
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
	renderSavedRedirect(w, "LAG port saved", "/configure/interfaces")
}

// DeleteLagPort removes an interface from its LAG.
// DELETE /configure/interfaces/{name}/lag-port
func (h *ConfigureInterfacesHandler) DeleteLagPort(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.RC.Delete(r.Context(), ifacePath(name)+"/infix-interfaces:lag-port"); err != nil {
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
	renderSavedRedirect(w, "LAG saved", "/configure/interfaces")
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

// saveMembersDiff syncs bridge/lag membership by diffing submitted form values
// against the current state: add port-kind for new members, remove for ex-members.
// kind is "bridge" or "lag"; it determines the YANG augment path and body key.
func (h *ConfigureInterfacesHandler) saveMembersDiff(w http.ResponseWriter, r *http.Request,
	masterName, kind string, isMember func(ifaceJSON, string) bool, successMsg string) {

	ifaces, err := h.fetchAllInterfaces(r.Context())
	if err != nil {
		renderSaveError(w, err)
		return
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
				renderSaveError(w, err)
				return
			}
		} else if !wantMember && currentlyMember {
			if err := h.RC.Delete(r.Context(), portPath); err != nil {
				log.Printf("configure interfaces %s members remove %s from %s: %v", kind, iface.Name, masterName, err)
				renderSaveError(w, err)
				return
			}
		}
	}
	renderSavedRedirect(w, successMsg, "/configure/interfaces")
}

func (h *ConfigureInterfacesHandler) fetchAllInterfaces(ctx context.Context) ([]ifaceJSON, error) {
	var wrap interfacesWrapper
	if err := h.RC.Get(ctx, ifaceCandPath, &wrap); err != nil {
		// Fall back to running only on 404 (candidate has no interfaces configured yet).
		// Any other error (validation failure, server error) is surfaced directly to
		// avoid silently showing stale running data while the candidate is in bad shape.
		var rcErr *restconf.Error
		if errors.As(err, &rcErr) && rcErr.StatusCode == http.StatusNotFound {
			log.Printf("configure interfaces: candidate returned 404, using running datastore")
			if err2 := h.RC.Get(ctx, "/data/ietf-interfaces:interfaces", &wrap); err2 != nil {
				return nil, err2
			}
		} else {
			return nil, err
		}
	}
	return wrap.Interfaces.Interface, nil
}

type membership struct{ kind, master string }

func (h *ConfigureInterfacesHandler) buildRows(ifaces []ifaceJSON) []cfgIfaceRow {
	// Build a set of current bridge/lag members for fast lookup.
	memberOf := make(map[string]membership, len(ifaces))
	for _, iface := range ifaces {
		if iface.BridgePort != nil && iface.BridgePort.Bridge != "" {
			memberOf[iface.Name] = membership{"bridge", iface.BridgePort.Bridge}
		} else if iface.LagPort != nil && iface.LagPort.LAG != "" {
			memberOf[iface.Name] = membership{"lag", iface.LagPort.LAG}
		}
	}

	// Pre-compute bridge member sets.
	bridgeMembers := make(map[string][]string)
	for _, iface := range ifaces {
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
		}
		if m, ok := memberOf[iface.Name]; ok {
			row.MemberOf = m.master
		}
		row.HasIP = !row.IsBridgePort && !row.IsLagPort
		row.AddrSummary = addrSummary(iface)

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

func (h *ConfigureInterfacesHandler) addAddr(w http.ResponseWriter, r *http.Request, family string) {
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
	body := map[string]any{
		"ietf-ip:address": map[string]any{
			"ip":            ip,
			"prefix-length": prefix,
		},
	}
	path := ifacePath(name) + "/ietf-ip:" + family + "/address=" + url.PathEscape(ip)
	if err := h.RC.Put(r.Context(), path, body); err != nil {
		log.Printf("configure interfaces %s add %s addr: %v", name, family, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Address added", "/configure/interfaces")
}

func (h *ConfigureInterfacesHandler) deleteAddr(w http.ResponseWriter, r *http.Request, family string) {
	name := r.PathValue("name")
	ip := r.PathValue("ip")
	path := ifacePath(name) + "/ietf-ip:" + family + "/address=" + url.PathEscape(ip)
	if err := h.RC.Delete(r.Context(), path); err != nil {
		log.Printf("configure interfaces %s delete %s addr %s: %v", name, family, ip, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Address removed", "/configure/interfaces")
}

// SaveIPv4DHCP enables or disables the DHCPv4 client presence container.
// POST /configure/interfaces/{name}/ipv4/dhcp
func (h *ConfigureInterfacesHandler) SaveIPv4DHCP(w http.ResponseWriter, r *http.Request) {
	h.togglePresence(w, r,
		ifacePath(r.PathValue("name"))+"/ietf-ip:ipv4/infix-dhcp-client:dhcp",
		"infix-dhcp-client:dhcp",
		"DHCP client")
}

// SaveIPv4Autoconf enables or disables IPv4 link-local autoconfiguration.
// POST /configure/interfaces/{name}/ipv4/autoconf
func (h *ConfigureInterfacesHandler) SaveIPv4Autoconf(w http.ResponseWriter, r *http.Request) {
	h.togglePresence(w, r,
		ifacePath(r.PathValue("name"))+"/ietf-ip:ipv4/infix-ip:autoconf",
		"infix-ip:autoconf",
		"IPv4 link-local")
}

// SaveIPv6SLAAC enables or disables IPv6 SLAAC (autoconf).
// POST /configure/interfaces/{name}/ipv6/autoconf
func (h *ConfigureInterfacesHandler) SaveIPv6SLAAC(w http.ResponseWriter, r *http.Request) {
	h.togglePresence(w, r,
		ifacePath(r.PathValue("name"))+"/ietf-ip:ipv6/autoconf",
		"autoconf",
		"IPv6 SLAAC")
}

// SaveIPv6DHCP enables or disables the DHCPv6 client presence container.
// POST /configure/interfaces/{name}/ipv6/dhcp
func (h *ConfigureInterfacesHandler) SaveIPv6DHCP(w http.ResponseWriter, r *http.Request) {
	h.togglePresence(w, r,
		ifacePath(r.PathValue("name"))+"/ietf-ip:ipv6/infix-dhcpv6-client:dhcp",
		"infix-dhcpv6-client:dhcp",
		"DHCPv6 client")
}

func (h *ConfigureInterfacesHandler) togglePresence(w http.ResponseWriter, r *http.Request, path, bodyKey, label string) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	if r.FormValue("enabled") == "true" {
		body := map[string]any{bodyKey: map[string]any{}}
		if err := h.RC.Put(r.Context(), path, body); err != nil {
			log.Printf("configure interfaces %s enable %s: %v", name, label, err)
			renderSaveError(w, err)
			return
		}
	} else {
		if err := h.RC.Delete(r.Context(), path); err != nil {
			var rcErr *restconf.Error
			if errors.As(err, &rcErr) && rcErr.StatusCode == http.StatusNotFound {
				// already absent — desired state achieved
			} else {
				log.Printf("configure interfaces %s disable %s: %v", name, label, err)
				renderSaveError(w, err)
				return
			}
		}
	}
	renderSavedRedirect(w, label+" updated", "/configure/interfaces")
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
