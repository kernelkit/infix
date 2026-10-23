package features

import (
	"os"
	"testing"
)

// testdata/sysrepo.startup is trimmed from an x86_64 image with WiFi,
// GPS, containers and FRR built in.
func TestLoadFixture(t *testing.T) {
	s, err := Load("testdata/sysrepo.startup")
	if err != nil {
		t.Fatal(err)
	}
	for name, got := range map[string]bool{
		"wifi": s.WiFi(), "containers": s.Containers(), "gps": s.GPS(), "frr": s.FRR(),
	} {
		if !got {
			t.Errorf("%s: want on", name)
		}
	}
	if s.Has("ietf-interfaces", "wifi") {
		t.Error("feature of another module reported")
	}
	if s.HasModule("infix-modem") {
		t.Error("module not in the list reported installed")
	}
}

// A minimal image: modules present without the optional features, and
// no routing protocol modules at all.
func TestMinimal(t *testing.T) {
	s, err := Parse([]byte(`{"sysrepo:sysrepo-modules":{"module":[
		{"name":"infix-interfaces","revision":"2026-10-11","enabled-feature":["vlan-filtering"]},
		{"name":"infix-hardware","revision":"2026-10-11"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if s.WiFi() || s.Containers() || s.GPS() || s.FRR() {
		t.Fatalf("minimal image: wifi=%v containers=%v gps=%v frr=%v", s.WiFi(), s.Containers(), s.GPS(), s.FRR())
	}
}

func TestMissingFileAndAll(t *testing.T) {
	if _, err := Load("testdata/does-not-exist"); !os.IsNotExist(err) {
		t.Fatalf("want not-exist error, got %v", err)
	}
	if s := All(); !s.WiFi() || !s.FRR() || !s.HasModule("anything") {
		t.Fatal("All must enable everything")
	}
}

func TestBadJSON(t *testing.T) {
	if _, err := Parse([]byte("{")); err == nil {
		t.Fatal("want parse error")
	}
}
