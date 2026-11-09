// SPDX-License-Identifier: MIT

package handlers

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"infix/webui/internal/restconf"
)

// SaveQoS sets the port's profile reference and egress shaper.  An empty
// profile leaves the port on its own settings or the defaults, an empty
// rate removes the shaper.
// POST /configure/interfaces/{name}/qos
func (h *ConfigureInterfacesHandler) SaveQoS(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	qosPath := ifacePath(name) + "/infix-interfaces:qos"
	ctx := r.Context()

	// The profile leaf and the inline containers are the two cases of a
	// choice; a merge into the candidate keeps both, so the other case
	// is removed first.
	qos := map[string]any{}
	rate := strings.TrimSpace(r.FormValue("rate"))
	if rate != "" {
		bits, err := mbitField(rate, "the egress shaper rate")
		if err != nil {
			renderSaveError(w, err)
			return
		}
		qos["egress-shaper"] = map[string]any{"rate": bits}
	}

	drop := []string{"/profile"}
	if profile := strings.TrimSpace(r.FormValue("profile")); profile != "" {
		qos["profile"] = profile
		drop = []string{"/ingress", "/egress"}
	}
	if rate == "" {
		drop = append(drop, "/egress-shaper")
	}
	for _, node := range drop {
		if err := h.RC.Delete(ctx, qosPath+node); err != nil && !restconf.IsNotFound(err) {
			log.Printf("configure interfaces %s qos delete %s: %v", name, node, err)
			renderSaveError(w, err)
			return
		}
	}

	if len(qos) > 0 {
		body := map[string]any{"infix-interfaces:qos": qos}
		if err := h.RC.Patch(ctx, qosPath, body); err != nil {
			log.Printf("configure interfaces %s qos: %v", name, err)
			renderSaveError(w, err)
			return
		}
	}
	renderSaved(w, "QoS saved")
}

// ─── IEEE 802.1DC shaper and gate sections ──────────────────────────────────

const (
	cbsaNode  = "/ieee802-dot1dc-cbsa-if:cbsa"
	gatesNode = "/ieee802-dot1dc-sched-if:gate-parameter-table"
	setGates  = "ieee802-dot1q-sched:set-gate-states"

	gatesContainer = "ieee802-dot1dc-sched-if:gate-parameter-table"
)

// qosGateFields are the nodes of the gate parameter table the form holds.
var qosGateFields = map[string]bool{
	"gate-enabled":               true,
	"admin-control-list":         true,
	"admin-cycle-time":           true,
	"admin-cycle-time-extension": true,
	"admin-base-time":            true,
	"queue-max-sdu-table":        true,
}

// qosClassesOf lists a port's traffic classes top down, the order the
// forms show.  The count comes from the operational capabilities.
func qosClassesOf(oper *ifaceJSON) []int {
	n := qosNumClasses
	if oper != nil && oper.QoS != nil && oper.QoS.Capabilities != nil &&
		oper.QoS.Capabilities.MaxTrafficClasses > 0 && oper.QoS.Capabilities.MaxTrafficClasses < n {
		n = oper.QoS.Capabilities.MaxTrafficClasses
	}
	classes := make([]int, 0, n)
	for id := n - 1; id >= 0; id-- {
		classes = append(classes, id)
	}
	return classes
}

// qosShaperRow is one class on the port's Credit-Based Shaper table.  On a
// port with its own settings every class has a row, a slope shapes it; on
// a port with a profile only the classes the profile shapes do, plus any
// class the port still holds a slope for.
type qosShaperRow struct {
	ID       int
	Shaped   bool
	IdleMbit string
}

type qosGateEntryRow struct {
	Index      int
	Open       [qosNumClasses]bool // by class
	IntervalUs string
}

// qosGatesRow is the port's gate control list as the form shows it, times
// in microseconds.
type qosGatesRow struct {
	Classes     []int // top down
	Enabled     bool
	CycleUs     string
	BaseSeconds string
	BaseNs      int
	ExtensionUs string
	Entries     []qosGateEntryRow
	MaxSDU      [qosNumClasses]string // by class
	AllOpen     [qosNumClasses]bool   // the gates of a new entry
}

func usText(ns int64) string {
	return strconv.FormatFloat(float64(ns)/1e3, 'f', -1, 64)
}

func mbitText(bits int64) string {
	return strconv.FormatFloat(float64(bits)/1e6, 'f', -1, 64)
}

// mbitField parses a Mbit/s form value into bits per second, as the
// string the uint64 leaves take.
func mbitField(v, what string) (string, error) {
	mbit, err := strconv.ParseFloat(v, 64)
	if err != nil || mbit <= 0 || mbit > 1e6 {
		return "", fmt.Errorf("%s must be a number of Mbit/s above zero", what)
	}
	return strconv.FormatInt(int64(math.Round(mbit*1e6)), 10), nil
}

// qosEgressOf returns the egress settings a port runs: its profile's, or
// its own when configured inline.
func qosEgressOf(iface *ifaceJSON, profiles map[string]qosProfileJSON) *qosEgressJSON {
	if iface.QoS == nil {
		return nil
	}
	if iface.QoS.Profile != "" {
		return profiles[iface.QoS.Profile].Egress
	}
	return iface.QoS.Egress
}

// qosShapedClasses is the set of classes an egress serves with the
// credit-based shaper.
func qosShapedClasses(eg *qosEgressJSON) map[int]bool {
	shaped := map[int]bool{}
	if eg != nil {
		for _, c := range eg.Classes {
			if qosAlgoIs(c.Algorithm, qosAlgoCBS) {
				shaped[c.ID] = true
			}
		}
	}
	return shaped
}

func qosShaperRows(iface, oper *ifaceJSON, eg *qosEgressJSON) []qosShaperRow {
	shaped := qosShapedClasses(eg)
	admin := map[int]int64{}
	if iface.CBSA != nil {
		for _, e := range iface.CBSA.Table {
			admin[e.TrafficClass] = int64(e.AdminIdleSlope)
		}
	}
	profile := iface.QoS != nil && iface.QoS.Profile != ""
	var rows []qosShaperRow
	for _, id := range qosClassesOf(oper) {
		_, held := admin[id]
		if profile && !shaped[id] && !held {
			continue
		}
		row := qosShaperRow{ID: id, Shaped: shaped[id]}
		if held {
			row.IdleMbit = mbitText(admin[id])
		}
		rows = append(rows, row)
	}
	return rows
}

func qosGateEntryRows(list *gateListJSON) []qosGateEntryRow {
	if list == nil {
		return nil
	}
	entries := append([]gateEntryJSON(nil), list.Entries...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Index < entries[j].Index })
	var rows []qosGateEntryRow
	for i, e := range entries {
		row := qosGateEntryRow{Index: i, IntervalUs: usText(int64(e.TimeInterval))}
		for c := range row.Open {
			row.Open[c] = e.GateStates&(1<<c) != 0
		}
		rows = append(rows, row)
	}
	return rows
}

func rationalNs(r *rationalJSON) int64 {
	if r == nil || r.Denominator == 0 {
		return 0
	}
	return int64(r.Numerator) * 1e9 / int64(r.Denominator)
}

func qosGatesRowOf(iface, oper *ifaceJSON) qosGatesRow {
	row := qosGatesRow{Classes: qosClassesOf(oper), BaseSeconds: "0"}
	for c := range row.AllOpen {
		row.AllOpen[c] = true
	}
	if g := iface.Gates; g != nil {
		row.Enabled = g.GateEnabled != nil && *g.GateEnabled
		if ns := rationalNs(g.AdminCycleTime); ns > 0 {
			row.CycleUs = usText(ns)
		}
		if g.AdminBaseTime != nil {
			row.BaseSeconds = strconv.FormatInt(int64(g.AdminBaseTime.Seconds), 10)
			row.BaseNs = g.AdminBaseTime.Nanoseconds
		}
		if g.AdminExtension > 0 {
			row.ExtensionUs = usText(int64(g.AdminExtension))
		}
		row.Entries = qosGateEntryRows(g.AdminList)
		for _, m := range g.QueueMaxSDU {
			if m.QueueMaxSDU > 0 && m.TrafficClass < qosNumClasses {
				row.MaxSDU[m.TrafficClass] = strconv.Itoa(m.QueueMaxSDU)
			}
		}
	}
	return row
}

// shapeClasses makes the credit-based shaper the algorithm of exactly the
// classes in slope, on a port with its own settings.  A port with a
// profile is left alone: its profile decides which classes are shaped.
func (h *ConfigureInterfacesHandler) shapeClasses(r *http.Request, slope map[int]bool) error {
	ctx := r.Context()
	qosPath := ifacePath(r.PathValue("name")) + "/infix-interfaces:qos"
	var wrap interfacesWrapper
	if err := h.RC.Get(ctx, qosPath, &wrap); err != nil && !restconf.IsNotFound(err) {
		return err
	}
	var eg *qosEgressJSON
	if ifs := wrap.Interfaces.Interface; len(ifs) > 0 && ifs[0].QoS != nil {
		if ifs[0].QoS.Profile != "" {
			return nil
		}
		eg = ifs[0].QoS.Egress
	}
	shaped := qosShapedClasses(eg)

	var add []map[string]any
	for id := range slope {
		if !shaped[id] {
			add = append(add, map[string]any{"id": id, "algorithm": qosAlgoCBS})
		}
	}
	for id := range shaped {
		if slope[id] {
			continue
		}
		path := qosPath + "/egress/traffic-class=" + strconv.Itoa(id) + "/algorithm"
		if err := h.RC.Delete(ctx, path); err != nil && !restconf.IsNotFound(err) {
			return err
		}
	}
	if len(add) == 0 {
		return nil
	}
	body := map[string]any{"infix-interfaces:qos": map[string]any{
		"egress": map[string]any{"traffic-class": add}}}
	return h.RC.Patch(ctx, qosPath, body)
}

// ResetCBS removes one class's idle slope, and on a port with its own
// settings the class's shaping with it, as clearing its field does.
// DELETE /configure/interfaces/{name}/cbs/{class}
func (h *ConfigureInterfacesHandler) ResetCBS(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("class"))
	if err != nil || id < 0 || id >= qosNumClasses {
		http.Error(w, "bad traffic class", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	path := ifacePath(r.PathValue("name")) + cbsaNode
	var wrap interfacesWrapper
	if err := h.RC.Get(ctx, path, &wrap); err != nil && !restconf.IsNotFound(err) {
		log.Printf("configure interfaces %s cbs reset: %v", r.PathValue("name"), err)
		renderSaveError(w, err)
		return
	}
	slope := map[int]bool{}
	if ifs := wrap.Interfaces.Interface; len(ifs) > 0 && ifs[0].CBSA != nil {
		for _, e := range ifs[0].CBSA.Table {
			if e.TrafficClass != id {
				slope[e.TrafficClass] = true
			}
		}
	}
	err = h.RC.Delete(ctx, path+"/cbsa-parameter-table="+strconv.Itoa(id))
	if err != nil && !restconf.IsNotFound(err) && !restconf.IsDataMissing(err) {
		log.Printf("configure interfaces %s cbs reset: %v", r.PathValue("name"), err)
		renderSaveError(w, err)
		return
	}
	if err := h.shapeClasses(r, slope); err != nil {
		log.Printf("configure interfaces %s cbs reset: %v", r.PathValue("name"), err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Reset to default", "/configure/interfaces")
}

// ResetMaxSDU removes the per-class max SDU limits of the gate list.
// DELETE /configure/interfaces/{name}/gates/max-sdu
func (h *ConfigureInterfacesHandler) ResetMaxSDU(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	p := restconf.NewYangPatch(candidatePath)
	for id := 0; id < qosNumClasses; id++ {
		p.Remove(ifaceTarget(name) + gatesNode + "/queue-max-sdu-table=" + strconv.Itoa(id))
	}
	if err := h.RC.YangPatch(r.Context(), p); err != nil {
		log.Printf("configure interfaces %s max-sdu reset: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Reset to default", "/configure/interfaces")
}

// replaceContainer writes a whole container under the interface, or
// removes it when body is nil.
func (h *ConfigureInterfacesHandler) replaceContainer(w http.ResponseWriter, r *http.Request, node string, body map[string]any, saved string) {
	name := r.PathValue("name")
	path := ifacePath(name) + node
	var err error
	if body == nil {
		err = h.RC.Delete(r.Context(), path)
		if restconf.IsNotFound(err) {
			err = nil
		}
	} else {
		err = h.RC.Put(r.Context(), path, body)
	}
	if err != nil {
		log.Printf("configure interfaces %s %s: %v", name, node, err)
		renderSaveError(w, err)
		return
	}
	renderSaved(w, saved)
}

// SaveCBS replaces the port's shaper table with the idle slopes given, in
// Mbit/s per class; an empty table removes it.  On a port with its own
// settings a slope also shapes its class and a cleared one stops it; with
// a profile the profile decides which classes are shaped.
// POST /configure/interfaces/{name}/cbs
func (h *ConfigureInterfacesHandler) SaveCBS(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var table []map[string]any
	slope := map[int]bool{}
	for id := 0; id < qosNumClasses; id++ {
		v := strings.TrimSpace(r.FormValue("idle_" + strconv.Itoa(id)))
		if v == "" {
			continue
		}
		bits, err := mbitField(v, fmt.Sprintf("class %d: the idle slope", id))
		if err != nil {
			renderSaveError(w, err)
			return
		}
		table = append(table, map[string]any{"traffic-class": id, "admin-idle-slope": bits})
		slope[id] = true
	}
	if err := h.shapeClasses(r, slope); err != nil {
		log.Printf("configure interfaces %s shaped classes: %v", r.PathValue("name"), err)
		renderSaveError(w, err)
		return
	}
	var body map[string]any
	if len(table) > 0 {
		body = map[string]any{"ieee802-dot1dc-cbsa-if:cbsa": map[string]any{"cbsa-parameter-table": table}}
	}
	h.replaceContainer(w, r, cbsaNode, body, "Shaper saved")
}

// usField parses a microsecond field into nanoseconds.
func usField(r *http.Request, field, what string) (int64, bool, error) {
	v := strings.TrimSpace(r.FormValue(field))
	if v == "" {
		return 0, false, nil
	}
	us, err := strconv.ParseFloat(v, 64)
	if err != nil || us < 0 || us > 4e6 {
		return 0, false, fmt.Errorf("%s must be a number of microseconds", what)
	}
	return int64(math.Round(us * 1e3)), true, nil
}

// qosGatesFromForm builds the gate-parameter-table container from the
// form.  It returns nil when the form holds nothing to store.
func qosGatesFromForm(r *http.Request) (map[string]any, error) {
	table := map[string]any{}

	var entries []map[string]any
	var indexes []int
	for key := range r.Form {
		var n int
		if _, err := fmt.Sscanf(key, "entry_interval_%d", &n); err == nil {
			indexes = append(indexes, n)
		}
	}
	sort.Ints(indexes)
	for _, n := range indexes {
		ns, ok, err := usField(r, fmt.Sprintf("entry_interval_%d", n), fmt.Sprintf("entry %d: the interval", n))
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if ns == 0 {
			return nil, fmt.Errorf("entry %d: the interval must be above zero", n)
		}
		mask := 0
		for _, c := range r.Form[fmt.Sprintf("entry_open_%d", n)] {
			if bit, err := strconv.Atoi(c); err == nil && bit >= 0 && bit < qosNumClasses {
				mask |= 1 << bit
			}
		}
		entries = append(entries, map[string]any{
			"index":               len(entries),
			"operation-name":      setGates,
			"gate-states-value":   mask,
			"time-interval-value": ns,
		})
	}
	if len(entries) > 0 {
		table["admin-control-list"] = map[string]any{"gate-control-entry": entries}
	}

	if ns, ok, err := usField(r, "cycle", "the cycle"); err != nil {
		return nil, err
	} else if ok && ns > 0 {
		g := gcd(ns, 1e9)
		table["admin-cycle-time"] = map[string]any{"numerator": ns / g, "denominator": int64(1e9) / g}
	}
	if ns, ok, err := usField(r, "extension", "the cycle time extension"); err != nil {
		return nil, err
	} else if ok && ns > 0 {
		table["admin-cycle-time-extension"] = ns
	}

	sec, err := uintField(r, "base_s", 48)
	if err != nil {
		return nil, fmt.Errorf("the base time seconds must be a whole number")
	}
	nsec, err := uintField(r, "base_ns", 32)
	if err != nil || nsec >= 1e9 {
		return nil, fmt.Errorf("the base time nanoseconds must be below one second")
	}
	if sec > 0 || nsec > 0 {
		table["admin-base-time"] = map[string]any{"seconds": strconv.FormatUint(sec, 10), "nanoseconds": nsec}
	}

	var sdu []map[string]any
	for c := 0; c < qosNumClasses; c++ {
		v := strings.TrimSpace(r.FormValue("maxsdu_" + strconv.Itoa(c)))
		if v == "" {
			continue
		}
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("class %d: max SDU must be a number of bytes above zero", c)
		}
		sdu = append(sdu, map[string]any{"traffic-class": c, "queue-max-sdu": n})
	}
	if len(sdu) > 0 {
		table["queue-max-sdu-table"] = sdu
	}

	enabled := r.FormValue("gate_enabled") != ""
	if enabled && len(entries) == 0 {
		return nil, fmt.Errorf("add at least one entry before enabling the list")
	}
	if !enabled && len(table) == 0 {
		return nil, nil
	}
	table["gate-enabled"] = enabled
	return table, nil
}

// uintField reads a whole number from the form, zero when it is empty.
func uintField(r *http.Request, field string, bits int) (uint64, error) {
	v := strings.TrimSpace(r.FormValue(field))
	if v == "" {
		return 0, nil
	}
	return strconv.ParseUint(v, 10, bits)
}

func gcd(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// SaveGates replaces the port's gate control list; a form with nothing in
// it removes the table.
// POST /configure/interfaces/{name}/gates
func (h *ConfigureInterfacesHandler) SaveGates(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	table, err := qosGatesFromForm(r)
	if err != nil {
		renderSaveError(w, err)
		return
	}
	var current map[string]map[string]any
	path := ifacePath(r.PathValue("name")) + gatesNode
	if err := h.RC.Get(r.Context(), path, &current); err != nil && !restconf.IsNotFound(err) {
		log.Printf("configure interfaces %s read gates: %v", r.PathValue("name"), err)
		renderSaveError(w, err)
		return
	}
	if table == nil {
		table = map[string]any{}
	}
	keepUnknown(table, current[gatesContainer], qosGateFields)
	var body map[string]any
	if len(table) > 0 {
		body = map[string]any{gatesContainer: table}
	}
	h.replaceContainer(w, r, gatesNode, body, "Schedule saved")
}
