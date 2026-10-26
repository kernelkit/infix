// SPDX-License-Identifier: MIT

package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"infix/webui/internal/schema"
)

// A protocol list keyed on an identity, and a module augmenting it the
// way ietf-ospf augments ietf-routing.
const treeBaseYang = `
module test-base {
  yang-version 1.1;
  namespace "urn:test:base";
  prefix tb;

  container top {
    list proto {
      key "type name";
      leaf type { type string; }
      leaf name { type string; }
    }
  }
}
`

const treeAugYang = `
module test-aug {
  yang-version 1.1;
  namespace "urn:test:aug";
  prefix ta;

  import test-base { prefix tb; }

  augment "/tb:top/tb:proto" {
    container sub {
      container areas {
        list area {
          key id;
          leaf id { type string; }
        }
      }
    }
  }
}
`

func TestBuildRootedPatchAugment(t *testing.T) {
	dir := t.TempDir()
	for name, text := range map[string]string{"test-base.yang": treeBaseYang, "test-aug.yang": treeAugYang} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mgr, err := schema.Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	path := "/test-base:top/proto=infix-routing%3Aospfv2,default/test-aug:sub/areas/area"
	node, err := mgr.NodeAt(path)
	if err != nil {
		t.Fatal(err)
	}
	top, body := buildRootedPatch(mgr, path, node, map[string]any{"id": "0.0.0.0"})
	got, _ := json.Marshal(body)

	want := `{"test-base:top":{"proto":[{"name":"default","test-aug:sub":{"areas":{"area":[{"id":"0.0.0.0"}]}},"type":"infix-routing:ospfv2"}]}}`
	if top != "/test-base:top" || string(got) != want {
		t.Errorf("PATCH %s\n got %s\nwant %s", top, got, want)
	}
}

// RESTCONF reads a ':' in a path segment as a module prefix, so an
// identity key must have it encoded.
func TestBuildKeyPredicateIdentity(t *testing.T) {
	item := map[string]json.RawMessage{
		"type": json.RawMessage(`"infix-routing:ospfv2"`),
		"name": json.RawMessage(`"default"`),
	}
	if got := buildKeyPredicate(item, []string{"type", "name"}); got != "=infix-routing%3Aospfv2,default" {
		t.Errorf("predicate %q", got)
	}
}
