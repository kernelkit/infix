package alarmmonitor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/alarm"
	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

type alarmsDoc struct {
	Alarms alarmsBody
}

type alarmsBody struct {
	Inventory struct {
		Type []inventoryJSON `json:"alarm-type"`
	} `json:"alarm-inventory"`
	Summary struct {
		Summary       []summaryJSON `json:"alarm-summary"`
		ShelvesActive []any         `json:"shelves-active"`
	} `json:"summary"`
	List struct {
		Number      int         `json:"number-of-alarms"`
		LastChanged string      `json:"last-changed"`
		Alarm       []alarmJSON `json:"alarm"`
	} `json:"alarm-list"`
	Shelved struct {
		Number int         `json:"number-of-shelved-alarms"`
		Alarm  []alarmJSON `json:"shelved-alarm"`
	} `json:"shelved-alarms"`
}

func newTestMonitor(t *testing.T) (*Monitor, *tree.Tree) {
	t.Helper()
	root := t.TempDir()
	alarm.Root = root
	for _, d := range []string{"active", "entry", "state"} {
		os.MkdirAll(filepath.Join(root, d), 0755)
	}
	tr := tree.New()
	m := New(tr, nil)
	m.ledDir = filepath.Join(root, "led")
	os.MkdirAll(m.ledDir, 0755)
	clock := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return clock }
	return m, tr
}

func get(t *testing.T, tr *tree.Tree) alarmsDoc {
	t.Helper()
	var doc alarmsDoc
	raw := tr.Get(treeKey)
	if raw == nil {
		t.Fatal("no tree data")
	}
	if err := json.Unmarshal(raw, &doc.Alarms); err != nil {
		t.Fatalf("parse tree: %v\n%s", err, raw)
	}
	return doc
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestEmpty(t *testing.T) {
	m, tr := newTestMonitor(t)
	m.reconcile()
	doc := get(t, tr)
	if doc.Alarms.List.Number != 0 || len(doc.Alarms.List.Alarm) != 0 {
		t.Fatalf("list = %+v", doc.Alarms.List)
	}
	if len(doc.Alarms.Inventory.Type) != len(inventory) {
		t.Fatalf("inventory has %d types, want %d", len(doc.Alarms.Inventory.Type), len(inventory))
	}
	if len(doc.Alarms.Summary.Summary) != 5 {
		t.Fatalf("summary has %d rows", len(doc.Alarms.Summary.Summary))
	}
	if !exists(filepath.Join(m.ledDir, "fault-ok")) {
		t.Fatal("fault-ok not set")
	}
}

func TestRaiseClearPurge(t *testing.T) {
	m, tr := newTestMonitor(t)
	m.reconcile()

	if err := alarm.Raise("external", "backup", "", alarm.Critical, "backup failed"); err != nil {
		t.Fatal(err)
	}
	m.reconcile()

	doc := get(t, tr)
	if doc.Alarms.List.Number != 1 {
		t.Fatalf("number-of-alarms = %d", doc.Alarms.List.Number)
	}
	a := doc.Alarms.List.Alarm[0]
	if a.Resource != "backup" || a.TypeID != "infix-alarms:external" || a.Qualifier != "" ||
		a.Severity != alarm.Critical || a.Text != "backup failed" || a.IsCleared {
		t.Fatalf("alarm = %+v", a)
	}
	if a.TimeCreated == "" || a.LastRaised != a.TimeCreated || a.LastChanged != a.TimeCreated {
		t.Fatalf("stamps = %+v", a)
	}
	if doc.Alarms.Summary.Summary[rank(alarm.Critical)].NotClearedNotClosed != 1 {
		t.Fatalf("summary = %+v", doc.Alarms.Summary.Summary)
	}
	if !exists(filepath.Join(m.ledDir, "fault-crit")) || exists(filepath.Join(m.ledDir, "fault-ok")) {
		t.Fatal("fault LED not critical")
	}

	// Cleared: the entry stays, flagged, and its file carries it.
	alarm.Clear("external", "backup", "")
	m.reconcile()
	doc = get(t, tr)
	a = doc.Alarms.List.Alarm[0]
	if !a.IsCleared || a.LastChanged != "2026-11-03T12:00:00Z" || a.Severity != alarm.Critical {
		t.Fatalf("cleared alarm = %+v", a)
	}
	if doc.Alarms.Summary.Summary[rank(alarm.Critical)].ClearedNotClosed != 1 {
		t.Fatalf("summary = %+v", doc.Alarms.Summary.Summary)
	}
	if !exists(filepath.Join(m.ledDir, "fault-ok")) {
		t.Fatal("fault LED not back to ok")
	}

	// Purged by confd: the entry file goes, so does the alarm.
	os.Remove(filepath.Join(alarm.Root, "entry", "external", "backup"))
	m.reconcile()
	if doc = get(t, tr); doc.Alarms.List.Number != 0 {
		t.Fatalf("after purge: %+v", doc.Alarms.List)
	}
}

func TestReraiseKeepsTimeCreated(t *testing.T) {
	m, tr := newTestMonitor(t)
	alarm.Raise("external", "x", "", alarm.Major, "first")
	m.reconcile()
	created := get(t, tr).Alarms.List.Alarm[0].TimeCreated

	alarm.Clear("external", "x", "")
	m.reconcile()

	alarm.Raise("external", "x", "", alarm.Minor, "again")
	later := time.Now().Add(2 * time.Second)
	os.Chtimes(alarm.Path("external", "x", ""), later, later)
	m.reconcile()

	a := get(t, tr).Alarms.List.Alarm[0]
	if a.IsCleared || a.TimeCreated != created || a.LastRaised == created || a.Severity != alarm.Minor || a.Text != "again" {
		t.Fatalf("re-raised alarm = %+v, created %s", a, created)
	}
}

func TestDefaultsAndUnknownType(t *testing.T) {
	m, tr := newTestMonitor(t)
	os.MkdirAll(filepath.Join(alarm.Root, "active", "link-down"), 0755)
	os.WriteFile(filepath.Join(alarm.Root, "active", "link-down", "eth0"), nil, 0644)
	os.MkdirAll(filepath.Join(alarm.Root, "active", "bogus"), 0755)
	os.WriteFile(filepath.Join(alarm.Root, "active", "bogus", "x"), nil, 0644)
	m.reconcile()

	doc := get(t, tr)
	if doc.Alarms.List.Number != 1 {
		t.Fatalf("list = %+v", doc.Alarms.List)
	}
	a := doc.Alarms.List.Alarm[0]
	if a.TypeID != "infix-alarms:link-down" || a.Severity != alarm.Major || a.Text != inventory["link-down"].Description {
		t.Fatalf("alarm = %+v", a)
	}
}

func TestOperatorState(t *testing.T) {
	m, tr := newTestMonitor(t)
	alarm.Raise("external", "x", "q1", alarm.Major, "text")
	m.reconcile()

	os.MkdirAll(filepath.Join(alarm.Root, "state", "external"), 0755)
	os.WriteFile(filepath.Join(alarm.Root, "state", "external", "x@q1"),
		[]byte(`[{"time":"2027-01-01T13:00:00Z","operator":"admin","state":"ack","text":"on it"},
			 {"time":"2027-01-01T14:00:00Z","operator":"admin","state":"closed"}]`), 0644)
	m.reconcile()

	doc := get(t, tr)
	a := doc.Alarms.List.Alarm[0]
	if a.Resource != "x" || a.Qualifier != "q1" || len(a.Ops) != 2 || a.Ops[0].Text != "on it" || a.Ops[1].State != "closed" {
		t.Fatalf("alarm = %+v", a)
	}
	if doc.Alarms.List.LastChanged != "2027-01-01T14:00:00Z" {
		t.Fatalf("last-changed = %s", doc.Alarms.List.LastChanged)
	}
	if doc.Alarms.Summary.Summary[rank(alarm.Major)].NotClearedClosed != 1 {
		t.Fatalf("summary = %+v", doc.Alarms.Summary.Summary)
	}
	if !exists(filepath.Join(m.ledDir, "fault-ok")) {
		t.Fatal("a closed alarm must not light the fault LED")
	}
}

func TestShelfAndProfile(t *testing.T) {
	m, tr := newTestMonitor(t)
	alarm.Raise("link-down", "eth0", "", alarm.Major, "no carrier")
	alarm.Raise("link-down", "eth1", "", alarm.Major, "no carrier")
	alarm.Raise("external", "x", "", alarm.Critical, "boom")
	os.WriteFile(filepath.Join(alarm.Root, configFile), []byte(`{"ietf-alarms:alarms":{
	  "control":{"alarm-shelving":{"shelf":[
	    {"name":"spare-ports","resource":["eth1"],"alarm-type":[{"alarm-type-id":"infix-alarms:link-down","alarm-type-qualifier-match":""}]}
	  ]}},
	  "alarm-profile":[
	    {"alarm-type-id":"infix-alarms:link-down","alarm-type-qualifier-match":"","resource":".*","description":"ports are warnings here",
	     "alarm-severity-assignment-profile":{"severity-level":["warning"]}}
	  ]}}`), 0644)
	m.reconcile()

	doc := get(t, tr)
	if doc.Alarms.List.Number != 2 || doc.Alarms.Shelved.Number != 1 {
		t.Fatalf("list %d shelved %d", doc.Alarms.List.Number, doc.Alarms.Shelved.Number)
	}
	if s := doc.Alarms.Shelved.Alarm[0]; s.Resource != "/ietf-interfaces:interfaces/interface[name='eth1']" || s.ShelfName != "spare-ports" || s.TimeCreated != "" {
		t.Fatalf("shelved = %+v", s)
	}
	if len(doc.Alarms.Summary.ShelvesActive) != 1 {
		t.Fatal("shelves-active not set")
	}
	for _, a := range doc.Alarms.List.Alarm {
		if resourceName(a.Resource) == "eth0" && a.Severity != alarm.Warning {
			t.Fatalf("profile not applied: %+v", a)
		}
	}
	if !exists(filepath.Join(m.ledDir, "fault-crit")) {
		t.Fatal("external critical alarm should light the fault LED")
	}

	// The shelf is persisted for confd's purge-shelved-alarms.
	data, _ := os.ReadFile(filepath.Join(alarm.Root, "entry", "link-down", "eth1"))
	var mt entry
	json.Unmarshal(data, &mt)
	if mt.Shelf != "spare-ports" {
		t.Fatalf("entry = %s", data)
	}

	// Removing the shelf brings the alarm back.
	os.Remove(filepath.Join(alarm.Root, configFile))
	m.reconcile()
	if doc = get(t, tr); doc.Alarms.List.Number != 3 || doc.Alarms.Shelved.Number != 0 {
		t.Fatalf("list %d shelved %d", doc.Alarms.List.Number, doc.Alarms.Shelved.Number)
	}
}

func TestBadConfigKeepsPrevious(t *testing.T) {
	m, _ := newTestMonitor(t)
	os.WriteFile(filepath.Join(alarm.Root, configFile), []byte(`{"ietf-alarms:alarms":{"control":{"alarm-shelving":{"shelf":[{"name":"all"}]}}}}`), 0644)
	m.reconcile()
	if len(m.cfg.Shelves) != 1 {
		t.Fatal("shelf not loaded")
	}
	os.WriteFile(filepath.Join(alarm.Root, configFile), []byte(`{"ietf-alarms:alarms":{"control":{"alarm-shelving":{"shelf":[{"name":"bad","resource":["("]}]}}}}`), 0644)
	m.reconcile()
	if len(m.cfg.Shelves) != 1 || m.cfg.Shelves[0].Name != "all" {
		t.Fatalf("cfg = %+v", m.cfg)
	}
}

func TestRunReactsToMarkers(t *testing.T) {
	m, tr := newTestMonitor(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- m.Run(ctx) }()

	deadline := time.Now().Add(3 * time.Second)
	for tr.Get(treeKey) == nil && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if tr.Get(treeKey) == nil {
		t.Fatal("no initial rebuild")
	}

	alarm.Raise("external", "live", "", alarm.Major, "x")
	for time.Now().Before(deadline) {
		if get(t, tr).Alarms.List.Number == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if get(t, tr).Alarms.List.Number != 1 {
		t.Fatal("marker not picked up")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return")
	}
}

// A link-down alarm names the interface by its instance-identifier, any
// other type by the marker name, and the short name comes back out.
func TestResourceOf(t *testing.T) {
	iid := "/ietf-interfaces:interfaces/interface[name='eth0']"
	if got := resourceOf("link-down", "eth0"); got != iid {
		t.Fatalf("resourceOf = %q", got)
	}
	if got := resourceOf("external", "backup"); got != "backup" {
		t.Fatalf("resourceOf = %q", got)
	}
	for in, want := range map[string]string{iid: "eth0", "backup": "backup", "/odd": "/odd"} {
		if got := resourceName(in); got != want {
			t.Fatalf("resourceName(%q) = %q", in, got)
		}
	}
}
