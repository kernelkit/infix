// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"infix/webui/internal/restconf"
	"infix/webui/internal/schema"
)

const (
	ptpPath        = "/ieee1588-ptp-tt:ptp"
	ptpDefaultDS   = ptpInstance + "/default-ds"
	ptpProfileLeaf = ptpDefaultDS + "/infix-ptp:profile"
)

// ConfigurePTPHandler serves Configure > PTP.
type ConfigurePTPHandler struct {
	Template *template.Template
	RC       restconf.Fetcher
	Schema   *schema.Cache
}

type ptpPortJSON struct {
	Index     int    `json:"port-index"`
	Interface string `json:"underlying-interface"`
}

type ptpInstanceJSON struct {
	Index     int `json:"instance-index"`
	DefaultDS struct {
		Profile          string `json:"infix-ptp:profile"`
		InstanceType     string `json:"instance-type"`
		DomainNumber     *int   `json:"domain-number"`
		Priority1        *int   `json:"priority1"`
		Priority2        *int   `json:"priority2"`
		TimeReceiverOnly bool   `json:"time-receiver-only"`
	} `json:"default-ds"`
	Ports struct {
		Port []ptpPortJSON `json:"port"`
	} `json:"ports"`
}

type ptpConfigWrapper struct {
	PTP struct {
		Instances struct {
			Instance []ptpInstanceJSON `json:"instance"`
		} `json:"instances"`
	} `json:"ieee1588-ptp-tt:ptp"`
}

// ptpPortChoice is an interface the instance can run on.
type ptpPortChoice struct {
	Name    string
	Checked bool
}

type cfgPTPRow struct {
	Index            int
	Profile          string
	InstanceType     string
	Domain           string // empty when left at the model's default
	Priority1        string
	Priority2        string
	TimeReceiverOnly bool
	Interfaces       []string
	Choices          []ptpPortChoice
}

type cfgPTPPageData struct {
	PageData
	Error      string
	Loading    bool
	Instances  []cfgPTPRow
	NextIndex  int
	NewChoices []ptpPortChoice // interfaces offered to a new instance
	Options    map[string][]schema.IdentityOption
	Labels     map[string]map[string]string // field to option value to its description
	Default    map[string]string            // the model's default per field
	Bounds     map[string][2]string
	Desc       map[string]string
}

func fetchPTPConfig(ctx context.Context, rc restconf.Fetcher) ([]ptpInstanceJSON, error) {
	var raw ptpConfigWrapper
	err := rc.Get(ctx, candidatePath+ptpPath, &raw)
	if err != nil && restconf.IsNotFound(err) {
		err = rc.Get(ctx, "/ds/ietf-datastores:running"+ptpPath, &raw)
	}
	if err != nil && !restconf.IsNotFound(err) {
		return nil, err
	}
	instances := raw.PTP.Instances.Instance
	sort.Slice(instances, func(i, j int) bool { return instances[i].Index < instances[j].Index })
	return instances, nil
}

// ptpCapablePorts lists the interfaces that can timestamp PTP messages,
// loopback aside, in natural order.
func ptpCapablePorts(ctx context.Context, rc restconf.Fetcher) []string {
	var raw struct {
		Interfaces struct {
			Interface []struct {
				Name string          `json:"name"`
				Type string          `json:"type"`
				Caps json.RawMessage `json:"infix-interfaces:ptp-capabilities"`
			} `json:"interface"`
		} `json:"ietf-interfaces:interfaces"`
	}
	if err := rc.Get(ctx, "/data/ietf-interfaces:interfaces", &raw); err != nil {
		log.Printf("configure ptp interfaces: %v", err)
		return nil
	}
	var names []string
	for _, iface := range raw.Interfaces.Interface {
		if len(iface.Caps) > 0 && schema.StripModulePrefix(iface.Type) != "loopback" {
			names = append(names, iface.Name)
		}
	}
	sort.Slice(names, func(i, j int) bool { return naturalLess(names[i], names[j]) })
	return names
}

func intText(v *int) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(*v)
}

func ptpRowOf(in ptpInstanceJSON, ports []string) cfgPTPRow {
	d := in.DefaultDS
	row := cfgPTPRow{
		Index:            in.Index,
		Profile:          d.Profile,
		InstanceType:     d.InstanceType,
		Domain:           intText(d.DomainNumber),
		Priority1:        intText(d.Priority1),
		Priority2:        intText(d.Priority2),
		TimeReceiverOnly: d.TimeReceiverOnly,
	}
	used := map[string]bool{}
	for _, p := range in.Ports.Port {
		if p.Interface != "" {
			used[p.Interface] = true
			row.Interfaces = append(row.Interfaces, p.Interface)
		}
	}
	sort.Slice(row.Interfaces, func(i, j int) bool { return naturalLess(row.Interfaces[i], row.Interfaces[j]) })
	// A port already on the instance stays in the list, capable or not
	names := slices.Clone(ports)
	for _, name := range row.Interfaces {
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	for _, name := range names {
		row.Choices = append(row.Choices, ptpPortChoice{Name: name, Checked: used[name]})
	}
	return row
}

// Overview lists the PTP instances with an editor per row.
func (h *ConfigurePTPHandler) Overview(w http.ResponseWriter, r *http.Request) {
	data := cfgPTPPageData{PageData: newPageData(w, r, "configure-ptp", "PTP")}
	mgr := h.Schema.Manager()
	data.Loading = mgr == nil
	if mgr != nil {
		leafs := map[string]string{
			"profile":            ptpProfileLeaf,
			"instance-type":      ptpDefaultDS + "/instance-type",
			"domain-number":      ptpDefaultDS + "/domain-number",
			"priority1":          ptpDefaultDS + "/priority1",
			"priority2":          ptpDefaultDS + "/priority2",
			"time-receiver-only": ptpDefaultDS + "/time-receiver-only",
			"ports":              ptpInstance + "/ports/port/underlying-interface",
		}
		data.Options = map[string][]schema.IdentityOption{}
		data.Labels = map[string]map[string]string{}
		data.Default = map[string]string{}
		data.Bounds = map[string][2]string{}
		data.Desc = map[string]string{"instance": schema.DescriptionOf(mgr, ptpInstance)}
		for field, path := range leafs {
			data.Desc[field] = schema.DescriptionOf(mgr, path)
			data.Default[field], _ = mgr.Default(path)
			if opts := schema.OptionsFor(mgr, path); len(opts) > 0 {
				data.Options[field] = opts
				data.Labels[field] = schema.OptionDescriptions(opts)
			}
			lo, hi := schema.RangeBounds(schemaRange(mgr, path))
			data.Bounds[field] = [2]string{lo, hi}
		}
	}

	var (
		instances []ptpInstanceJSON
		err       error
		wg        sync.WaitGroup
	)
	wg.Add(2)
	go func() { defer wg.Done(); instances, err = fetchPTPConfig(r.Context(), h.RC) }()
	var ports []string
	go func() { defer wg.Done(); ports = ptpCapablePorts(r.Context(), h.RC) }()
	wg.Wait()
	if err != nil {
		log.Printf("configure ptp: %v", err)
		data.Error = "Could not read the PTP configuration"
	}
	for _, name := range ports {
		data.NewChoices = append(data.NewChoices, ptpPortChoice{Name: name})
	}
	for _, in := range instances {
		data.Instances = append(data.Instances, ptpRowOf(in, ports))
		if in.Index >= data.NextIndex {
			data.NextIndex = in.Index + 1
		}
	}

	tmplName := "configure-ptp.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func ptpInstanceCandPath(index int) string {
	return candidatePath + ptpInstance + "=" + strconv.Itoa(index)
}

// ptpDefaultDSFromForm takes the fields the form filled in.  An empty
// number is left out, so the leaf keeps what it has; its reset button
// takes it back to the model's default.
func ptpDefaultDSFromForm(r *http.Request) (map[string]any, error) {
	ds := map[string]any{"time-receiver-only": r.FormValue("time-receiver-only") == "on"}
	for _, field := range []string{"profile", "instance-type"} {
		if v := r.FormValue(field); v != "" {
			key := field
			if field == "profile" {
				key = "infix-ptp:profile"
			}
			ds[key] = v
		}
	}
	for _, field := range []string{"domain-number", "priority1", "priority2"} {
		v := strings.TrimSpace(r.FormValue(field))
		if v == "" {
			continue
		}
		n, err := strconv.ParseUint(v, 10, 8)
		if err != nil {
			return nil, fmt.Errorf("%s: %q is not a number the field takes", field, v)
		}
		ds[field] = n
	}
	return ds, nil
}

// ptpPortsFor gives the ports to add so the instance runs on want, and
// the indexes of the ports to remove.  A port that stays keeps its index
// and everything set on it.
func ptpPortsFor(current []ptpPortJSON, want []string) (add []map[string]any, remove []int) {
	taken := map[int]bool{}
	have := map[string]bool{}
	for _, p := range current {
		taken[p.Index] = true
		if slices.Contains(want, p.Interface) {
			have[p.Interface] = true
		} else {
			remove = append(remove, p.Index)
		}
	}
	next := 1
	for _, name := range want {
		if have[name] {
			continue
		}
		for taken[next] {
			next++
		}
		taken[next] = true
		have[name] = true
		add = append(add, map[string]any{"port-index": next, "underlying-interface": name})
	}
	return add, remove
}

func ptpPatch(index int, ds map[string]any, ports []map[string]any) map[string]any {
	instance := map[string]any{"instance-index": index, "default-ds": ds}
	if len(ports) > 0 {
		instance["ports"] = map[string]any{"port": ports}
	}
	return map[string]any{
		"ieee1588-ptp-tt:ptp": map[string]any{
			"instances": map[string]any{"instance": []any{instance}},
		},
	}
}

// save writes the form to instance index: its settings merged in, its
// ports brought in line with the ones ticked.
func (h *ConfigurePTPHandler) save(r *http.Request, index int, current []ptpPortJSON) error {
	ds, err := ptpDefaultDSFromForm(r)
	if err != nil {
		return err
	}
	add, remove := ptpPortsFor(current, r.Form["port"])
	for _, pi := range remove {
		path := ptpInstanceCandPath(index) + "/ports/port=" + strconv.Itoa(pi)
		if err := h.RC.Delete(r.Context(), path); err != nil && !restconf.IsNotFound(err) {
			return err
		}
	}
	return h.RC.Patch(r.Context(), candidatePath+ptpPath, ptpPatch(index, ds, add))
}

// AddInstance creates an instance on the ports chosen.
// POST /configure/ptp/instances
func (h *ConfigurePTPHandler) AddInstance(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	index, err := strconv.Atoi(strings.TrimSpace(r.FormValue("index")))
	if err != nil || index < 0 {
		renderSaveError(w, fmt.Errorf("the instance needs a number, 0 or higher"))
		return
	}
	instances, err := fetchPTPConfig(r.Context(), h.RC)
	if err != nil {
		log.Printf("configure ptp add %d: %v", index, err)
		renderSaveError(w, err)
		return
	}
	if slices.ContainsFunc(instances, func(in ptpInstanceJSON) bool { return in.Index == index }) {
		renderSaveError(w, fmt.Errorf("instance %d exists already", index))
		return
	}
	if err := h.save(r, index, nil); err != nil {
		log.Printf("configure ptp add %d: %v", index, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Instance added", "/configure/ptp")
}

// SaveInstance updates an instance.
// POST /configure/ptp/instances/{index}
func (h *ConfigurePTPHandler) SaveInstance(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	index, err := strconv.Atoi(r.PathValue("index"))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	instances, err := fetchPTPConfig(r.Context(), h.RC)
	if err != nil {
		log.Printf("configure ptp save %d: %v", index, err)
		renderSaveError(w, err)
		return
	}
	var current []ptpPortJSON
	if i := slices.IndexFunc(instances, func(in ptpInstanceJSON) bool { return in.Index == index }); i >= 0 {
		current = instances[i].Ports.Port
	}
	if err := h.save(r, index, current); err != nil {
		log.Printf("configure ptp save %d: %v", index, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Instance saved", "/configure/ptp")
}

// DeleteInstance removes an instance.
// DELETE /configure/ptp/instances/{index}
func (h *ConfigurePTPHandler) DeleteInstance(w http.ResponseWriter, r *http.Request) {
	index, err := strconv.Atoi(r.PathValue("index"))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := h.RC.Delete(r.Context(), ptpInstanceCandPath(index)); err != nil && !restconf.IsNotFound(err) {
		log.Printf("configure ptp delete %d: %v", index, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Instance deleted", "/configure/ptp")
}
