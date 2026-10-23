// SPDX-License-Identifier: MIT

package features

import (
	"os"
	"testing"
)

// testdata/sysrepo.startup is trimmed from an x86_64 image with WiFi,
// GPS, containers and FRR built in.
func TestSysrepoModules(t *testing.T) {
	s, err := LoadSysrepoModules("testdata/sysrepo.startup")
	if err != nil {
		t.Fatal(err)
	}
	for name, on := range s.Enabled() {
		if !on {
			t.Errorf("%s: want on", name)
		}
	}
	if s.Has("ietf-interfaces", "wifi") {
		t.Error("feature of another module reported")
	}
	if s.HasModule("infix-modem") || s.On("modem") {
		t.Error("module not in the list reported installed")
	}
}

// The same modules as ietf-yang-library serves them, with the features
// a minimal image has: no WiFi, no containers, no GPS, no FRR.
func TestYangLibraryMinimal(t *testing.T) {
	s, err := ParseYangLibrary([]byte(`{"ietf-yang-library:yang-library":{"module-set":[{"name":"complete","module":[
		{"name":"infix-interfaces","revision":"2026-10-11","feature":["vlan-filtering"]},
		{"name":"infix-hardware","revision":"2026-10-11"},
		{"name":"ietf-interfaces","revision":"2018-02-20","feature":["if-mib"]}]}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	for name, on := range s.Enabled() {
		if on {
			t.Errorf("%s: want off on a minimal image", name)
		}
	}
	if !s.Has("ietf-interfaces", "if-mib") || !s.HasModule("infix-hardware") {
		t.Error("installed module or enabled feature missed")
	}
}

func TestYangLibraryFull(t *testing.T) {
	s, err := ParseYangLibrary([]byte(`{"ietf-yang-library:yang-library":{"module-set":[{"name":"complete","module":[
		{"name":"infix-interfaces","feature":["wifi","containers"]},
		{"name":"infix-hardware","feature":["gps"]},
		{"name":"ietf-ospf"}]}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	for name, on := range s.Enabled() {
		if !on {
			t.Errorf("%s: want on", name)
		}
	}
}

func TestMissingFileAndAll(t *testing.T) {
	if _, err := LoadSysrepoModules("testdata/does-not-exist"); !os.IsNotExist(err) {
		t.Fatalf("want not-exist error, got %v", err)
	}
	for name, on := range All().Enabled() {
		if !on {
			t.Fatalf("All must enable everything, %s off", name)
		}
	}
}

func TestBadJSON(t *testing.T) {
	if _, err := ParseYangLibrary([]byte("{")); err == nil {
		t.Fatal("want parse error")
	}
	if _, err := ParseSysrepoModules([]byte("[]")); err == nil {
		t.Fatal("want parse error")
	}
}
