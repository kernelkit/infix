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
