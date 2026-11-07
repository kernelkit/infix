package alarm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRaiseClear(t *testing.T) {
	Root = t.TempDir()

	if err := Raise("external", "backup", "nightly", Major, "backup failed"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(Root, "active", "external", "backup@nightly")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"severity":"major","text":"backup failed"}` {
		t.Fatalf("marker = %s", data)
	}
	if !IsRaised("external", "backup", "nightly") {
		t.Fatal("not raised")
	}
	if err := Clear("external", "backup", "nightly"); err != nil {
		t.Fatal(err)
	}
	if IsRaised("external", "backup", "nightly") {
		t.Fatal("still raised")
	}
	if err := Clear("external", "backup", "nightly"); err != nil {
		t.Fatalf("second clear: %v", err)
	}
}

func TestRaiseRejectsBadNames(t *testing.T) {
	Root = t.TempDir()
	for _, r := range []string{"", "a/b", "a b", "../x"} {
		if err := Raise("external", r, "", Major, "x"); err == nil {
			t.Errorf("resource %q accepted", r)
		}
	}
	if err := Raise("external", "ok", "a/b", Major, "x"); err == nil {
		t.Error("qualifier a/b accepted")
	}
}

func TestSplitName(t *testing.T) {
	for _, tc := range []struct{ name, res, qual string }{
		{"eth0", "eth0", ""},
		{"backup@nightly", "backup", "nightly"},
		{"a@b@c", "a@b", "c"},
	} {
		r, q := SplitName(tc.name)
		if r != tc.res || q != tc.qual {
			t.Errorf("SplitName(%q) = %q, %q", tc.name, r, q)
		}
	}
}
