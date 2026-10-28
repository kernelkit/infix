// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"

	"infix/webui/internal/restconf"
	"infix/webui/internal/schema"
)

const (
	qosProfilesPath  = "/ietf-system:system/infix-qos:qos"
	qosProfileSchema = qosProfilesPath + "/profile"
	qosAlgoStrict    = "ieee802-dot1q-types:strict-priority"
	qosAlgoETS       = "ieee802-dot1q-types:enhanced-transmission-selection"
	qosAlgoCBS       = "ieee802-dot1q-types:credit-based-shaper"
	qosNumClasses    = 8
)

type ConfigureQoSHandler struct {
	Template *template.Template
	RC       restconf.Fetcher
	Schema   *schema.Cache
}

// RESTCONF JSON for /ietf-system:system/infix-qos:qos.

// qosSystemWrapper is the shape a GET of the qos container returns: the
// datastore wraps it in the system container it augments.
type qosSystemWrapper struct {
	System struct {
		QoS qosSystemJSON `json:"infix-qos:qos"`
	} `json:"ietf-system:system"`
}

type qosSystemJSON struct {
	Profile []qosProfileJSON `json:"profile"`
}

type qosProfileJSON struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Ingress     *qosIngressJSON `json:"ingress,omitempty"`
	Egress      *qosEgressJSON  `json:"egress,omitempty"`
}

type qosIngressJSON struct {
	Trust           string      `json:"trust,omitempty"`
	DefaultPriority *int        `json:"default-priority,omitempty"`
	PCPMap          *qosMapJSON `json:"pcp-map,omitempty"`
	DSCPMap         *qosMapJSON `json:"dscp-map,omitempty"`
}

type qosMapJSON struct {
	Preset string `json:"preset,omitempty"`
	Entry  []any  `json:"entry,omitempty"`
}

type qosEgressJSON struct {
	Table   *qosTableJSON  `json:"traffic-class-table,omitempty"`
	Classes []qosClassJSON `json:"traffic-class,omitempty"`
	Remark  *qosRemarkJSON `json:"remark,omitempty"`
}

type qosTableJSON struct {
	Preset string `json:"preset,omitempty"`
}

type qosClassJSON struct {
	ID        int    `json:"id"`
	Algorithm string `json:"algorithm,omitempty"`
	Bandwidth int    `json:"bandwidth,omitempty"`
}

type qosRemarkJSON struct {
	PCP  string `json:"pcp,omitempty"`
	DSCP string `json:"dscp,omitempty"`
}

// qosClassRow is one line of the transmission selection table, top class first.
type qosClassRow struct {
	ID        int
	Algorithm string
	Bandwidth int
}

// qosProfileRow is a profile as the editor shows it: every leaf filled
// in, with the schema's default where the profile leaves it out.
type qosProfileRow struct {
	qosProfileJSON
	Trust       string
	TableText   string // "ieee-sr, classes 7 and 6 shaped"
	Ports       []string
	Classes     []qosClassRow
	DefaultPri  string
	PCPPreset   string
	DSCPPreset  string
	TablePreset string
	RemarkPCP   string
	RemarkDSCP  string
}

// qosLeafs holds the schema paths of the profile's leaves, and what the
// page reads from them: options, defaults and ranges.
type qosLeafs struct {
	Trust, DefaultPriority, PCPPreset, DSCPPreset, TablePreset, ClassID, Algorithm, Bandwidth, RemarkPCP, RemarkDSCP string
}

func qosProfileLeafs() qosLeafs {
	in := qosProfileSchema + "/ingress"
	eg := qosProfileSchema + "/egress"
	return qosLeafs{
		Trust:           in + "/trust",
		DefaultPriority: in + "/default-priority",
		PCPPreset:       in + "/pcp-map/preset",
		DSCPPreset:      in + "/dscp-map/preset",
		TablePreset:     eg + "/traffic-class-table/preset",
		ClassID:         eg + "/traffic-class/id",
		Algorithm:       eg + "/traffic-class/algorithm",
		Bandwidth:       eg + "/traffic-class/bandwidth",
		RemarkPCP:       eg + "/remark/pcp",
		RemarkDSCP:      eg + "/remark/dscp",
	}
}

type qosDefaults struct {
	Trust, DefaultPriority, PCPPreset, DSCPPreset, TablePreset, Algorithm, RemarkPCP, RemarkDSCP string
	Classes                                                                                      int // the model's traffic class count
}

// qosDefaultsOf reads the schema's default for each leaf of the editor.
// Enum and identity defaults are taken from the option lists, which
// carry the value spelled the way the datastore returns it.
func qosDefaultsOf(mgr *schema.Manager, l qosLeafs, opts map[string][]schema.IdentityOption) qosDefaults {
	choice := func(field string) string {
		for _, o := range opts[field] {
			if o.IsDefault {
				return o.Value
			}
		}
		return ""
	}
	prio, _ := mgr.Default(l.DefaultPriority)
	classes := qosNumClasses
	if _, hi := schema.RangeBounds(schemaRange(mgr, l.ClassID)); hi != "" {
		if n, err := strconv.Atoi(hi); err == nil {
			classes = n + 1
		}
	}
	return qosDefaults{
		Classes: classes,
		Trust:   choice("trust"), DefaultPriority: prio, PCPPreset: choice("pcp-preset"),
		DSCPPreset: choice("dscp-preset"), TablePreset: choice("table-preset"), Algorithm: choice("algorithm"),
		RemarkPCP: choice("remark-pcp"), RemarkDSCP: choice("remark-dscp"),
	}
}

// schemaRange is the range of the integer leaf at path, "" when none.
func schemaRange(mgr *schema.Manager, path string) string {
	n, err := mgr.NodeAt(path)
	if err != nil || n == nil || n.Type == nil {
		return ""
	}
	return n.Type.Range
}

type cfgQoSPageData struct {
	PageData
	Error      string
	Loading    bool
	Profiles   []qosProfileRow
	Options    map[string][]schema.IdentityOption // by form field name
	TrustText  map[string]string                  // trust value to its description
	PriorityLo string
	PriorityHi string
	ShareLo    string
	ShareHi    string
	Desc       map[string]string
}

// qosAlgoIs compares two transmission selection identities without
// their module prefix; the daemon accepts both spellings.
func qosAlgoIs(algo, want string) bool {
	return schema.StripModulePrefix(algo) == schema.StripModulePrefix(want)
}

func qosTableText(p *qosProfileJSON, preset string) string {
	var weighted, shaped []string
	if p.Egress != nil {
		for _, c := range p.Egress.Classes {
			switch {
			case qosAlgoIs(c.Algorithm, qosAlgoETS):
				weighted = append(weighted, strconv.Itoa(c.ID))
			case qosAlgoIs(c.Algorithm, qosAlgoCBS):
				shaped = append(shaped, strconv.Itoa(c.ID))
			}
		}
	}
	parts := []string{preset}
	if len(shaped) > 0 {
		parts = append(parts, "classes "+strings.Join(shaped, " and ")+" shaped")
	}
	if len(weighted) > 0 {
		parts = append(parts, "classes "+strings.Join(weighted, " and ")+" weighted")
	}
	if len(shaped) == 0 && len(weighted) == 0 {
		parts = append(parts, "all strict")
	}
	return strings.Join(parts, ", ")
}

func qosProfileRowOf(p qosProfileJSON, ports []string, def qosDefaults) qosProfileRow {
	row := qosProfileRow{
		qosProfileJSON: p, Ports: ports,
		Trust: def.Trust, DefaultPri: def.DefaultPriority, PCPPreset: def.PCPPreset, DSCPPreset: def.DSCPPreset,
		TablePreset: def.TablePreset, RemarkPCP: def.RemarkPCP, RemarkDSCP: def.RemarkDSCP,
	}
	if in := p.Ingress; in != nil {
		if in.Trust != "" {
			row.Trust = in.Trust
		}
		if in.DefaultPriority != nil {
			row.DefaultPri = strconv.Itoa(*in.DefaultPriority)
		}
		if in.PCPMap != nil && in.PCPMap.Preset != "" {
			row.PCPPreset = in.PCPMap.Preset
		}
		if in.DSCPMap != nil && in.DSCPMap.Preset != "" {
			row.DSCPPreset = in.DSCPMap.Preset
		}
	}
	algo := map[int]qosClassJSON{}
	if eg := p.Egress; eg != nil {
		if eg.Table != nil && eg.Table.Preset != "" {
			row.TablePreset = eg.Table.Preset
		}
		for _, c := range eg.Classes {
			algo[c.ID] = c
		}
		if eg.Remark != nil {
			if eg.Remark.PCP != "" {
				row.RemarkPCP = eg.Remark.PCP
			}
			if eg.Remark.DSCP != "" {
				row.RemarkDSCP = eg.Remark.DSCP
			}
		}
	}
	for id := def.Classes - 1; id >= 0; id-- {
		c := qosClassRow{ID: id, Algorithm: def.Algorithm}
		if a, ok := algo[id]; ok {
			if a.Algorithm != "" {
				c.Algorithm = a.Algorithm
			}
			c.Bandwidth = a.Bandwidth
		}
		row.Classes = append(row.Classes, c)
	}
	row.TableText = qosTableText(&p, row.TablePreset)
	return row
}

// fetchQoSProfiles reads the profiles from the candidate datastore,
// falling back to running when the candidate is uninitialised.
func fetchQoSProfiles(ctx context.Context, rc restconf.Fetcher) ([]qosProfileJSON, error) {
	var raw qosSystemWrapper
	err := rc.Get(ctx, candidatePath+qosProfilesPath, &raw)
	if err != nil && restconf.IsNotFound(err) {
		err = rc.Get(ctx, "/ds/ietf-datastores:running"+qosProfilesPath, &raw)
	}
	if err != nil && !restconf.IsNotFound(err) {
		return nil, err
	}
	profiles := raw.System.QoS.Profile
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })
	return profiles, nil
}

// qosProfilePorts maps profile name to the interfaces that reference it.
func qosProfilePorts(ifaces []ifaceJSON) map[string][]string {
	ports := map[string][]string{}
	for _, iface := range ifaces {
		if iface.QoS != nil && iface.QoS.Profile != "" {
			ports[iface.QoS.Profile] = append(ports[iface.QoS.Profile], iface.Name)
		}
	}
	return ports
}

// Overview lists the profiles with the ports on each and an editor per row.
func (h *ConfigureQoSHandler) Overview(w http.ResponseWriter, r *http.Request) {
	data := cfgQoSPageData{PageData: newPageData(w, r, "configure-qos", "QoS")}
	mgr := h.Schema.Manager()
	data.Loading = mgr == nil
	var def qosDefaults
	if mgr != nil {
		l := qosProfileLeafs()
		data.Options = map[string][]schema.IdentityOption{
			"trust":        schema.OptionsFor(mgr, l.Trust),
			"pcp-preset":   schema.OptionsFor(mgr, l.PCPPreset),
			"dscp-preset":  schema.OptionsFor(mgr, l.DSCPPreset),
			"table-preset": schema.OptionsFor(mgr, l.TablePreset),
			"algorithm":    schema.OptionsFor(mgr, l.Algorithm),
			"remark-pcp":   schema.OptionsFor(mgr, l.RemarkPCP),
			"remark-dscp":  schema.OptionsFor(mgr, l.RemarkDSCP),
		}
		def = qosDefaultsOf(mgr, l, data.Options)
		data.TrustText = schema.OptionDescriptions(data.Options["trust"])
		data.PriorityLo, data.PriorityHi = schema.RangeBounds(schemaRange(mgr, l.DefaultPriority))
		data.ShareLo, data.ShareHi = schema.RangeBounds(schemaRange(mgr, l.Bandwidth))
		data.Desc = map[string]string{
			"profile":   schema.DescriptionOf(mgr, qosProfileSchema),
			"trust":     schema.DescriptionOf(mgr, l.Trust),
			"default":   schema.DescriptionOf(mgr, l.DefaultPriority),
			"pcp-map":   schema.DescriptionOf(mgr, qosProfileSchema+"/ingress/pcp-map"),
			"dscp-map":  schema.DescriptionOf(mgr, qosProfileSchema+"/ingress/dscp-map"),
			"table":     schema.DescriptionOf(mgr, qosProfileSchema+"/egress/traffic-class-table"),
			"algorithm": schema.DescriptionOf(mgr, l.Algorithm),
			"bandwidth": schema.DescriptionOf(mgr, l.Bandwidth),
			"remark":    schema.DescriptionOf(mgr, qosProfileSchema+"/egress/remark"),
		}
	}

	var (
		profiles []qosProfileJSON
		ifaces   interfacesWrapper
		perr     error
		wg       sync.WaitGroup
	)
	wg.Add(2)
	go func() { defer wg.Done(); profiles, perr = fetchQoSProfiles(r.Context(), h.RC) }()
	go func() {
		defer wg.Done()
		err := h.RC.Get(r.Context(), candidatePath+"/ietf-interfaces:interfaces", &ifaces)
		if err != nil && restconf.IsNotFound(err) {
			err = h.RC.Get(r.Context(), "/data/ietf-interfaces:interfaces", &ifaces)
		}
		if err != nil && !restconf.IsNotFound(err) {
			log.Printf("configure qos interfaces: %v", err)
		}
	}()
	wg.Wait()
	if perr != nil {
		log.Printf("configure qos: %v", perr)
		data.Error = "Could not read the QoS profiles"
	}
	ports := qosProfilePorts(ifaces.Interfaces.Interface)
	for _, p := range profiles {
		data.Profiles = append(data.Profiles, qosProfileRowOf(p, ports[p.Name], def))
	}

	tmplName := "configure-qos.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func qosProfileCandPath(name string) string {
	return candidatePath + qosProfilesPath + "/profile=" + url.PathEscape(name)
}

// qosProfileFromForm builds one profile entry from the editor's fields.
// Values are passed through as the selects and inputs give them; the
// schema bounds the inputs and the commit validates the rest.
func qosProfileFromForm(r *http.Request, name string) (qosProfileJSON, error) {
	p := qosProfileJSON{Name: name, Description: strings.TrimSpace(r.FormValue("description"))}
	in := &qosIngressJSON{Trust: r.FormValue("trust")}
	if v := r.FormValue("default-priority"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return p, fmt.Errorf("default priority must be a number")
		}
		in.DefaultPriority = &n
	}
	if v := r.FormValue("pcp-preset"); v != "" {
		in.PCPMap = &qosMapJSON{Preset: v}
	}
	if v := r.FormValue("dscp-preset"); v != "" {
		in.DSCPMap = &qosMapJSON{Preset: v}
	}
	p.Ingress = in

	eg := &qosEgressJSON{}
	if v := r.FormValue("table-preset"); v != "" {
		eg.Table = &qosTableJSON{Preset: v}
	}
	for id := 0; id < qosNumClasses; id++ {
		algo := r.FormValue(fmt.Sprintf("algorithm-%d", id))
		if algo == "" || qosAlgoIs(algo, qosAlgoStrict) {
			continue
		}
		c := qosClassJSON{ID: id, Algorithm: algo}
		if qosAlgoIs(algo, qosAlgoETS) {
			n, err := strconv.Atoi(r.FormValue(fmt.Sprintf("bandwidth-%d", id)))
			if err != nil {
				return p, fmt.Errorf("class %d: a weighted class needs a share", id)
			}
			c.Bandwidth = n
		}
		eg.Classes = append(eg.Classes, c)
	}
	eg.Remark = &qosRemarkJSON{PCP: r.FormValue("remark-pcp"), DSCP: r.FormValue("remark-dscp")}
	p.Egress = eg
	return p, nil
}

// AddProfile creates an empty profile; its settings are edited in place.
// POST /configure/qos/profiles
func (h *ConfigureQoSHandler) AddProfile(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		renderSaveError(w, fmt.Errorf("profile name is required"))
		return
	}
	body := map[string]any{
		"ietf-system:system": map[string]any{
			"infix-qos:qos": map[string]any{"profile": []any{map[string]any{"name": name}}},
		},
	}
	if err := h.RC.Patch(r.Context(), candidatePath+"/ietf-system:system", body); err != nil {
		log.Printf("configure qos add profile %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Profile added", "/configure/qos")
}

// SaveProfile replaces one profile with the editor's values.
// POST /configure/qos/profiles/{name}
func (h *ConfigureQoSHandler) SaveProfile(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	p, err := qosProfileFromForm(r, name)
	if err != nil {
		renderSaveError(w, err)
		return
	}
	body := map[string]any{"infix-qos:profile": []any{p}}
	if err := h.RC.Put(r.Context(), qosProfileCandPath(name), body); err != nil {
		log.Printf("configure qos save profile %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSaved(w, "Profile saved")
}

// DeleteProfile removes a profile.  A profile ports still reference is
// refused by the device, and the error says so.
// DELETE /configure/qos/profiles/{name}
func (h *ConfigureQoSHandler) DeleteProfile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	// The leafref is checked at commit; refusing here names the ports.
	var ifaces interfacesWrapper
	if err := h.RC.Get(r.Context(), candidatePath+"/ietf-interfaces:interfaces", &ifaces); err != nil && !restconf.IsNotFound(err) {
		log.Printf("configure qos delete profile %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	if ports := qosProfilePorts(ifaces.Interfaces.Interface)[name]; len(ports) > 0 {
		renderSaveError(w, fmt.Errorf("profile %s is used by %s, take the ports off it first", name, strings.Join(ports, ", ")))
		return
	}
	if err := h.RC.Delete(r.Context(), qosProfileCandPath(name)); err != nil && !restconf.IsNotFound(err) {
		log.Printf("configure qos delete profile %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Profile deleted", "/configure/qos")
}
