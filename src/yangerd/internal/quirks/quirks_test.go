package quirks

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadAndGet(t *testing.T) {
	dir := t.TempDir()
	system := filepath.Join(dir, "system.json")
	product := filepath.Join(dir, "product.json")
	os.WriteFile(system, []byte(`{
		"@driver=sparx5-switch": {"qos-offload": ["classification"], "sched-supported-list-max": 10000},
		"@driver=mv88e6085": {"qos-pcp-encoded": true}
	}`), 0o644)
	os.WriteFile(product, []byte(`{
		"@driver=sparx5-switch": {"qos-offload": ["classification", "time-aware-shaper"], "sched-supported-list-max": 3000},
		"e1": {"qos-offload": []}
	}`), 0o644)

	q := Load(system, product, filepath.Join(dir, "missing.json"))

	e2 := q.For("e2", "sparx5-switch")
	if got := e2.Strings("qos-offload"); !reflect.DeepEqual(got, []string{"classification", "time-aware-shaper"}) {
		t.Fatalf("product entry should override the system one, got %v", got)
	}
	if got := e2.Int("sched-supported-list-max"); got != 3000 {
		t.Fatalf("want 3000, got %d", got)
	}
	if got := q.For("e1", "sparx5-switch").Strings("qos-offload"); got == nil || len(got) != 0 {
		t.Fatalf("interface entry should win over the driver, got %v", got)
	}
	if got := q.For("e3", "mv88e6085").Strings("qos-offload"); got != nil {
		t.Fatalf("unset list quirk should be nil, got %v", got)
	}
	if got := q.For("e3", "").Get("qos-pcp-encoded"); got != nil {
		t.Fatalf("no driver should match no driver entry, got %s", got)
	}
}
