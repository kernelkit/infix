// SPDX-License-Identifier: MIT

package schema

import (
	"os"
	"path/filepath"
	"testing"
)

const featureYang = `
module test-feat {
  yang-version 1.1;
  namespace "urn:test:feat";
  prefix tf;

  feature extra;

  container box {
    leaf always { type string; }
    leaf gated { if-feature "extra"; type string; }
    container more { if-feature "tf:extra"; leaf x { type string; } }
  }
}
`

func loadFeatureModule(t *testing.T, features string) *Manager {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "test-feat.yang"), []byte(featureYang), 0644); err != nil {
		t.Fatal(err)
	}
	if features != "" {
		if err := os.WriteFile(filepath.Join(dir, featuresFile), []byte(features), 0644); err != nil {
			t.Fatal(err)
		}
	}
	mgr, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return mgr
}

func childNames(t *testing.T, mgr *Manager, path string) []string {
	t.Helper()
	nodes, err := mgr.Children(path)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, n := range nodes {
		names = append(names, n.Name)
	}
	return names
}

func TestFeatureGating(t *testing.T) {
	cases := []struct {
		features string
		want     []string
	}{
		{"", []string{"always", "gated", "more"}},
		{`{}`, []string{"always"}},
		{`{"test-feat:extra": true}`, []string{"always", "gated", "more"}},
	}
	for _, c := range cases {
		got := childNames(t, loadFeatureModule(t, c.features), "/test-feat:box")
		if len(got) != len(c.want) {
			t.Errorf("features %q: got %v want %v", c.features, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("features %q: got %v want %v", c.features, got, c.want)
				break
			}
		}
	}
}

// The if-feature expressions survive in a compiled schema.
func TestFeatureGatingCompiled(t *testing.T) {
	file := filepath.Join(t.TempDir(), "schema.json")
	if err := loadFeatureModule(t, "").Write(file); err != nil {
		t.Fatal(err)
	}
	mgr, err := Read(file)
	if err != nil {
		t.Fatal(err)
	}
	if got := childNames(t, mgr, "/test-feat:box"); len(got) != 3 {
		t.Errorf("no features: got %v", got)
	}
	got := childNames(t, mgr.WithFeatures(map[string]bool{}), "/test-feat:box")
	if len(got) != 1 || got[0] != "always" {
		t.Errorf("none enabled: got %v", got)
	}
	if got := childNames(t, mgr.WithFeatures(map[string]bool{"test-feat:extra": true}), "/test-feat:box"); len(got) != 3 {
		t.Errorf("extra enabled: got %v", got)
	}
}

func TestFeatureSet(t *testing.T) {
	set := FeatureSet([]ModuleInfo{{Name: "a", Features: []string{"x", "y"}}, {Name: "b"}})
	if len(set) != 2 || !set["a:x"] || !set["a:y"] {
		t.Errorf("got %v", set)
	}
}

func TestIfFeatureExpr(t *testing.T) {
	on := func(f string) bool { return f == "a" || f == "b" }
	cases := []struct {
		expr string
		want bool
	}{
		{"a", true}, {"x", false}, {"not x", true}, {"not a", false},
		{"a and b", true}, {"a and x", false}, {"x or b", true}, {"x or y", false},
		{"drop-eligible or drop-precedence", false},
		{"x or (a and b)", true}, {"not (a and b)", false}, {"a and not x or y", true},
	}
	for _, c := range cases {
		if got := ifFeatureExpr(c.expr, on); got != c.want {
			t.Errorf("%q: got %v want %v", c.expr, got, c.want)
		}
	}
}
