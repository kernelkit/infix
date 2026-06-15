package schema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testTargetYang = `
module test-target {
  yang-version 1.1;
  namespace "urn:test:target";
  prefix tt;

  container interfaces {
    list interface {
      key name;
      leaf name { type string; }
      leaf type { type string; }
    }
  }
}
`

const testReferYang = `
module test-refer {
  yang-version 1.1;
  namespace "urn:test:refer";
  prefix tr;

  import test-target { prefix tt; }

  container config {
    leaf port {
      type leafref {
        path "/tt:interfaces/tt:interface/tt:name";
      }
      must "deref(.)/../tt:type" {
        error-message "target must have type";
      }
    }
  }
}
`

func TestLeafrefCanonicalize(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "test-target.yang", testTargetYang)
	mustWrite(t, dir, "test-refer.yang", testReferYang)

	mgr, err := Load(dir)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	n, err := mgr.NodeAt("/test-refer:config/port")
	if err != nil {
		t.Fatalf("NodeAt: %v", err)
	}
	if n == nil || n.Type == nil {
		t.Fatalf("missing node or type: %+v", n)
	}
	t.Logf("Leafref path = %q", n.Type.Leafref)
	t.Logf("LeafrefSibling = %q", n.Type.LeafrefSibling)

	if strings.Contains(n.Type.Leafref, "tt:") {
		t.Errorf("Leafref still has unresolved prefix: %q", n.Type.Leafref)
	}
	want := "/test-target:interfaces/interface/name"
	if n.Type.Leafref != want {
		t.Errorf("Leafref = %q, want %q (RESTCONF canonical, no repeated module prefixes)", n.Type.Leafref, want)
	}
	if n.Type.LeafrefSibling != "type" {
		t.Errorf("LeafrefSibling = %q, want \"type\"", n.Type.LeafrefSibling)
	}
}

func mustWrite(t *testing.T, dir, name, content string) {
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// TestLeafrefParentIsList verifies that NodeAt reports the correct Kind
// for each segment of a canonical leafref path so fetchLeafrefValues can
// walk up past the list to its container parent (RESTCONF refuses bare
// list paths). Mirrors the bridge-port/bridge case where the path
// ends in /interfaces/interface/name and we have to GET /interfaces.
func TestLeafrefParentIsList(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "test-target.yang", testTargetYang)
	mgr, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	cases := []struct{ path, kind string }{
		{"/test-target:interfaces", "container"},
		{"/test-target:interfaces/interface", "list"},
		{"/test-target:interfaces/interface/name", "leaf"},
	}
	for _, tc := range cases {
		n, err := mgr.NodeAt(tc.path)
		if err != nil || n == nil {
			t.Fatalf("NodeAt(%q): %v", tc.path, err)
		}
		if n.Kind != tc.kind {
			t.Errorf("NodeAt(%q).Kind = %q, want %q", tc.path, n.Kind, tc.kind)
		}
	}
}

const testBridgeYang = `
module test-br {
  yang-version 1.1;
  namespace "urn:test:br";
  prefix tb;

  import test-target { prefix tt; }

  grouping br-port {
    leaf bridge {
      type leafref {
        path "/tt:interfaces/tt:interface/tt:name";
      }
      must "deref(.)/../bridge and not(. = ../../tt:name)";
    }
  }

  augment "/tt:interfaces/tt:interface" {
    container bridge-port {
      uses br-port;
    }
  }
}
`

func TestLeafrefBridgeAugment(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "test-target.yang", testTargetYang)
	mustWrite(t, dir, "test-br.yang", testBridgeYang)
	mgr, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	n, err := mgr.NodeAt("/test-target:interfaces/interface/test-br:bridge-port/bridge")
	if err != nil {
		t.Logf("first NodeAt failed: %v", err)
		n, err = mgr.NodeAt("/test-target:interfaces/test-target:interface/test-br:bridge-port/test-br:bridge")
		if err != nil {
			t.Fatalf("NodeAt: %v", err)
		}
	}
	if n == nil || n.Type == nil {
		t.Fatalf("missing: %+v", n)
	}
	t.Logf("Leafref = %q", n.Type.Leafref)
	t.Logf("LeafrefSibling = %q", n.Type.LeafrefSibling)
	if n.Type.LeafrefSibling != "bridge" {
		t.Errorf("LeafrefSibling = %q, want \"bridge\"", n.Type.LeafrefSibling)
	}
	want := "/test-target:interfaces/interface/name"
	if n.Type.Leafref != want {
		t.Errorf("Leafref = %q, want %q", n.Type.Leafref, want)
	}
}
