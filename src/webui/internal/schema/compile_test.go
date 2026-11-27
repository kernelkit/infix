// SPDX-License-Identifier: MIT

package schema

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const compileModule = `module test-box {
  yang-version 1.1; namespace "urn:test:box"; prefix tb;
  identity kind;
  identity plain { base kind; }
  container box {
    presence "A box.";
    list item {
      key name;
      leaf name { type string; }
      leaf kind { type identityref { base kind; } default plain; }
      leaf size { type uint8 { range "1..9"; } mandatory true; }
      choice shape {
        case round { leaf radius { type uint8; } }
        case square { leaf side { type uint8; } }
      }
    }
    leaf count { config false; type uint32; }
  }
}`

// A schema read back from file answers as the one compiled from YANG.
func TestWriteRead(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "test-box.yang", compileModule)
	want, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "out", "schema.json")
	if err := want.Write(file); err != nil {
		t.Fatal(err)
	}
	got, err := Read(file)
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"/", "/test-box:box", "/test-box:box/item"} {
		for name, children := range map[string]func(*Manager, string) ([]*Node, error){
			"Children":    (*Manager).Children,
			"ChildrenAll": (*Manager).ChildrenAll,
		} {
			w, err := children(want, path)
			if err != nil {
				t.Fatal(err)
			}
			g, err := children(got, path)
			if err != nil {
				t.Fatal(err)
			}
			wj, _ := json.Marshal(w)
			gj, _ := json.Marshal(g)
			if len(w) == 0 || string(wj) != string(gj) {
				t.Errorf("%s(%s) = %s, want %s", name, path, gj, wj)
			}
		}
	}
	if n, err := got.NodeAt("/test-box:box/item/radius"); err != nil || n.Kind != "leaf" {
		t.Errorf("leaf inside a case: %+v, %v", n, err)
	}
	if def, ok := got.Default("/test-box:box/item/kind"); !ok || def != "plain" {
		t.Errorf("default = %q, %v", def, ok)
	}
	if ids := got.IdentitiesOf("test-box:kind"); len(ids) != 1 || ids[0] != "plain" {
		t.Errorf("identities = %q", ids)
	}
}

// With a compiled schema only the features are asked for, no module is fetched.
func TestLoadFileFetchesOnlyFeatures(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "test-box.yang", compileModule)
	mgr, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "schema.json")
	if err := mgr.Write(file); err != nil {
		t.Fatal(err)
	}

	cache := t.TempDir()
	rc := fakeFetcher{modules: map[string]string{"test-box@2025-01-01": compileModule}}
	c := NewCache(rc, cache, "")
	if err := c.LoadFile(file); err != nil {
		t.Fatal(err)
	}
	if c.Manager().features != nil {
		t.Error("features before the device was asked")
	}
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.Manager().features == nil {
		t.Error("no features after Refresh")
	}
	if files, _ := os.ReadDir(cache); len(files) != 0 {
		t.Errorf("%d file(s) fetched to the cache", len(files))
	}
}
