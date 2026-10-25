package schema

import "testing"

func TestReflow(t *testing.T) {
	in := "Scripts run once at boot.\n\nScripts are extracted to /etc/rc.d\n   and run as root.\n\nChanges take\neffect at the next boot."
	want := "Scripts run once at boot.\n\nScripts are extracted to /etc/rc.d and run as root.\n\nChanges take effect at the next boot."
	if got := Reflow(in); got != want {
		t.Fatalf("Reflow() = %q, want %q", got, want)
	}

	in = "Options:\n - foo\n - bar\n   continued"
	want = "Options:\n- foo\n- bar continued"
	if got := Reflow(in); got != want {
		t.Fatalf("Reflow() list = %q, want %q", got, want)
	}
}

// An identityref default names the identity without a module prefix, or
// with the module's own prefix, while the options carry the module name.
func TestOptionsForIdentityDefault(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "test-area.yang", `module test-area {
  yang-version 1.1; namespace "urn:test:area"; prefix ta;
  identity kind;
  identity plain { base kind; }
  identity fancy { base kind; }
  leaf bare { type identityref { base kind; } default plain; }
  leaf prefixed { type identityref { base kind; } default ta:fancy; }
}`)
	mgr, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for leaf, want := range map[string]string{"bare": "test-area:plain", "prefixed": "test-area:fancy"} {
		var got []string
		for _, o := range OptionsFor(mgr, "/test-area:"+leaf) {
			if o.IsDefault {
				got = append(got, o.Value)
			}
		}
		if len(got) != 1 || got[0] != want {
			t.Errorf("%s: default %q, want %q", leaf, got, want)
		}
	}
}
