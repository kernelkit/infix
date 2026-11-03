// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"infix/webui/internal/restconf"
)

// ─── Alarm types ─────────────────────────────────────────────────────────────

// Alarm is one entry of the ietf-alarms alarm list, or of the shelved list.
type Alarm struct {
	Resource    string
	Name        string // Resource as the operator knows it, eth0 for an interface
	Link        string // the resource's own page, when it has one
	TypeID      string // identityref as the device names it, infix-alarms:link-down
	Type        string // the identity without its module, link-down
	Qualifier   string
	Severity    string
	State       string // active, ack, closed, cleared
	Text        string
	TextHTML    template.HTML // Text escaped, with its URLs as links
	Created     string
	LastChanged string
	Changed     string // LastChanged as date and minute, for the table
	Shelf       string
	Cleared     bool
	External    bool // raised by a script or a client, clear-alarm applies
	History     []alarmChange
	// hx-vals of the row's buttons, JSON built here since a template
	// cannot quote a value inside an attribute reliably.
	AckVals   string
	CloseVals string
	ClearVals string
}

type alarmChange struct {
	Time     string
	When     string // Time as date and minute
	Operator string
	State    string
	Text     string
}

type alarmJSON struct {
	Resource    string `json:"resource"`
	TypeID      string `json:"alarm-type-id"`
	Qualifier   string `json:"alarm-type-qualifier"`
	TimeCreated string `json:"time-created"`
	IsCleared   bool   `json:"is-cleared"`
	LastChanged string `json:"last-changed"`
	Severity    string `json:"perceived-severity"`
	Text        string `json:"alarm-text"`
	ShelfName   string `json:"shelf-name"`
	Changes     []struct {
		Time     string `json:"time"`
		Operator string `json:"operator"`
		State    string `json:"state"`
		Text     string `json:"text"`
	} `json:"operator-state-change"`
}

type alarmsWrapper struct {
	Alarms struct {
		List struct {
			Alarm []alarmJSON `json:"alarm"`
		} `json:"alarm-list"`
		Shelved struct {
			Alarm []alarmJSON `json:"shelved-alarm"`
		} `json:"shelved-alarms"`
	} `json:"ietf-alarms:alarms"`
}

// ─── Page data ───────────────────────────────────────────────────────────────

type alarmsPageData struct {
	PageData
	Alarms  []Alarm
	Shelved []Alarm
	Active  int // not cleared
	Open    int // not cleared and not closed, what Close all acts on
	Notice  string
	Error   string
}

// ─── Handler ─────────────────────────────────────────────────────────────────

// AlarmsHandler serves the alarms page and its operator actions.
type AlarmsHandler struct {
	Template *template.Template
	RC       restconf.Fetcher
}

// Overview renders the alarms page (GET /alarms).
func (h *AlarmsHandler) Overview(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "", "")
}

func (h *AlarmsHandler) render(w http.ResponseWriter, r *http.Request, notice, errMsg string) {
	data := alarmsPageData{
		PageData: newPageData(w, r, "alarms", "Alarms"),
		Notice:   notice,
		Error:    errMsg,
	}

	ctx := context.WithoutCancel(r.Context())
	list, shelved, err := fetchAlarms(ctx, h.RC)
	if err != nil {
		log.Printf("restconf alarms: %v", err)
		data.Error = "Failed to fetch alarms"
	}
	data.Alarms, data.Shelved = list, shelved
	for _, a := range list {
		if !a.Cleared {
			data.Active++
			if a.State != "closed" {
				data.Open++
			}
		}
	}

	tmplName := "alarms.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// SetState acknowledges or closes an alarm (POST /alarms/state), the
// alarm named by the resource, alarm-type-id and qualifier form fields.
func (h *AlarmsHandler) SetState(w http.ResponseWriter, r *http.Request) {
	state := r.FormValue("state")
	if state != "ack" && state != "closed" {
		h.render(w, r, "", "Unknown operator state")
		return
	}
	path := alarmPath(r.FormValue("resource"), r.FormValue("type"), r.FormValue("qualifier")) + "/set-operator-state"
	input := map[string]any{"state": state}
	if text := strings.TrimSpace(r.FormValue("text")); text != "" {
		input["text"] = text
	}
	err := h.RC.PostJSON(r.Context(), path, map[string]any{"ietf-alarms:input": input})
	if err != nil {
		log.Printf("set-operator-state: %v", err)
		h.render(w, r, "", "Failed to update the alarm: "+err.Error())
		return
	}
	verb := "acknowledged"
	if state == "closed" {
		verb = "closed"
	}
	h.render(w, r, "Alarm on "+resourceName(r.FormValue("resource"))+" "+verb, "")
}

// CloseAll closes every alarm that is not cleared, in one action on
// the alarm list (POST /alarms/close-all).  The device leaves the
// shelved alarms and the ones already closed alone.
func (h *AlarmsHandler) CloseAll(w http.ResponseWriter, r *http.Request) {
	var reply struct {
		Output struct {
			Changed int `json:"changed-alarms"`
		} `json:"infix-alarms:output"`
	}
	input := map[string]any{"infix-alarms:input": map[string]any{
		"alarm-clearance-status": "not-cleared", "state": "closed"}}
	err := h.RC.CallRPC(r.Context(), "/data/ietf-alarms:alarms/alarm-list/infix-alarms:set-operator-state", input, &reply)
	if err != nil {
		log.Printf("set-operator-state: %v", err)
		h.render(w, r, "", "Failed to close the alarms: "+err.Error())
		return
	}
	h.render(w, r, fmt.Sprintf("Closed %d alarm(s)", reply.Output.Changed), "")
}

// Clear clears an external alarm (POST /alarms/clear).
func (h *AlarmsHandler) Clear(w http.ResponseWriter, r *http.Request) {
	input := map[string]any{"resource": r.FormValue("resource")}
	if q := r.FormValue("qualifier"); q != "" {
		input["alarm-type-qualifier"] = q
	}
	err := h.RC.PostJSON(r.Context(), "/operations/infix-alarms:clear-alarm", map[string]any{"infix-alarms:input": input})
	if err != nil {
		log.Printf("clear-alarm: %v", err)
		h.render(w, r, "", "Failed to clear the alarm: "+err.Error())
		return
	}
	h.render(w, r, "Alarm on "+r.FormValue("resource")+" cleared", "")
}

// Purge removes the cleared alarms from the list (POST /alarms/purge).
func (h *AlarmsHandler) Purge(w http.ResponseWriter, r *http.Request) {
	input := map[string]any{"alarm-clearance-status": "cleared"}
	err := h.RC.PostJSON(r.Context(), "/data/ietf-alarms:alarms/alarm-list/purge-alarms",
		map[string]any{"ietf-alarms:input": input})
	if err != nil {
		log.Printf("purge-alarms: %v", err)
		h.render(w, r, "", "Failed to purge: "+err.Error())
		return
	}
	h.render(w, r, "Cleared alarms purged", "")
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// fetchAlarms reads the alarm list and the shelved list, active first
// and newest first within each.  A device without the alarm model gives
// two empty lists, not an error.
func fetchAlarms(ctx context.Context, rc restconf.Fetcher) (list, shelved []Alarm, err error) {
	var raw alarmsWrapper
	if err := rc.Get(ctx, "/data/ietf-alarms:alarms", &raw); err != nil && !restconf.IsNotFound(err) {
		return nil, nil, err
	}
	for _, a := range raw.Alarms.List.Alarm {
		list = append(list, alarmFromJSON(a))
	}
	for _, a := range raw.Alarms.Shelved.Alarm {
		shelved = append(shelved, alarmFromJSON(a))
	}
	sortAlarms(list)
	sortAlarms(shelved)
	return list, shelved, nil
}

func alarmFromJSON(a alarmJSON) Alarm {
	typ := a.TypeID
	if i := strings.IndexByte(typ, ':'); i >= 0 {
		typ = typ[i+1:]
	}
	alarm := Alarm{
		Resource:    a.Resource,
		Name:        resourceName(a.Resource),
		Link:        resourceLink(a.Resource),
		TypeID:      a.TypeID,
		Type:        typ,
		Qualifier:   a.Qualifier,
		Severity:    a.Severity,
		Text:        a.Text,
		TextHTML:    linkify(a.Text),
		Created:     a.TimeCreated,
		LastChanged: a.LastChanged,
		Changed:     shortStamp(a.LastChanged),
		Shelf:       a.ShelfName,
		Cleared:     a.IsCleared,
		External:    typ == "external",
	}
	alarm.AckVals = jsonVals(map[string]string{"state": "ack", "resource": a.Resource, "type": a.TypeID, "qualifier": a.Qualifier})
	alarm.CloseVals = jsonVals(map[string]string{"state": "closed", "resource": a.Resource, "type": a.TypeID, "qualifier": a.Qualifier})
	alarm.ClearVals = jsonVals(map[string]string{"resource": a.Resource, "qualifier": a.Qualifier})
	for _, c := range a.Changes {
		alarm.History = append(alarm.History, alarmChange{
			Time: c.Time, When: shortStamp(c.Time), Operator: c.Operator, State: c.State, Text: c.Text,
		})
	}
	switch {
	case a.IsCleared:
		alarm.State = "cleared"
	case len(a.Changes) > 0 && (a.Changes[len(a.Changes)-1].State == "ack" || a.Changes[len(a.Changes)-1].State == "closed"):
		alarm.State = a.Changes[len(a.Changes)-1].State
	default:
		alarm.State = "active"
	}
	return alarm
}

// resourceName is the last key value of an instance-identifier, the
// resource itself otherwise.
func resourceName(resource string) string {
	if !strings.HasPrefix(resource, "/") {
		return resource
	}
	i := strings.LastIndex(resource, "='")
	if i < 0 {
		return resource
	}
	rest := resource[i+2:]
	if j := strings.IndexByte(rest, '\''); j >= 0 {
		return rest[:j]
	}
	return resource
}

// resourceLink is the status page of an alarm's resource, only the
// interfaces have one.
func resourceLink(resource string) string {
	if strings.HasPrefix(resource, "/ietf-interfaces:interfaces/interface[") {
		return "/interfaces/" + url.PathEscape(resourceName(resource))
	}
	return ""
}

func sortAlarms(alarms []Alarm) {
	sort.SliceStable(alarms, func(i, j int) bool {
		if alarms[i].Cleared != alarms[j].Cleared {
			return !alarms[i].Cleared
		}
		return alarms[i].LastChanged > alarms[j].LastChanged
	})
}

func jsonVals(m map[string]string) string {
	b, _ := json.Marshal(m)
	return string(b)
}

// shortStamp is an RFC 3339 stamp as date and minute, 2026-11-03 22:09.
func shortStamp(ts string) string {
	if len(ts) < 16 {
		return ts
	}
	return strings.Replace(ts[:16], "T", " ", 1)
}

var urlRe = regexp.MustCompile(`https?://[^\s<>"']+`)

// linkify escapes text for HTML and turns each URL in it into a link
// that opens in a new tab.  Trailing punctuation stays outside the link.
func linkify(text string) template.HTML {
	var sb strings.Builder
	last := 0
	for _, m := range urlRe.FindAllStringIndex(text, -1) {
		start, end := m[0], m[1]
		for end > start && strings.ContainsRune(".,;:)", rune(text[end-1])) {
			end--
		}
		sb.WriteString(template.HTMLEscapeString(text[last:start]))
		u := template.HTMLEscapeString(text[start:end])
		sb.WriteString(`<a href="` + u + `" target="_blank" rel="noopener noreferrer">` + u + `</a>`)
		last = end
	}
	sb.WriteString(template.HTMLEscapeString(text[last:]))
	return template.HTML(sb.String())
}

// alarmPath is the RESTCONF path of one alarm list entry.  The keys
// are one path segment, each value percent-encoded, RFC 8040 3.5.3.
func alarmPath(resource, typeID, qualifier string) string {
	return "/data/ietf-alarms:alarms/alarm-list/alarm=" +
		keyEscape(resource) + "," + keyEscape(typeID) + "," + keyEscape(qualifier)
}

// keyEscape encodes a list key value for a RESTCONF path.  PathEscape
// leaves ':', ',' and '=' alone, but they delimit module, keys and
// values there, and rousette rejects them unencoded.
var keyEscaper = strings.NewReplacer(":", "%3A", ",", "%2C", "=", "%3D")

func keyEscape(v string) string {
	return keyEscaper.Replace(url.PathEscape(v))
}

// alarmHealthItems is what the dashboard Health card shows for the
// alarms that are neither cleared nor closed by an operator: the most
// severe first, at most five.
func alarmHealthItems(alarms []Alarm) []healthItem {
	rank := map[string]int{"critical": 0, "major": 1, "minor": 2, "warning": 3, "indeterminate": 4}
	var active []Alarm
	for _, a := range alarms {
		if !a.Cleared && a.State != "closed" {
			active = append(active, a)
		}
	}
	sort.SliceStable(active, func(i, j int) bool { return rank[active[i].Severity] < rank[active[j].Severity] })

	var items []healthItem
	for i, a := range active {
		if i == 5 {
			items = append(items, healthItem{Class: "status-warn", Text: "and more, see Alarms", Link: "/alarms"})
			break
		}
		class := "status-warn"
		if a.Severity == "critical" || a.Severity == "major" {
			class = "status-down"
		}
		items = append(items, healthItem{Class: class, Text: resourceName(a.Resource) + ": " + a.Text, Link: "/alarms", Title: a.Type + ", " + a.Severity})
	}
	return items
}
