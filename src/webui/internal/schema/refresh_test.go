package schema

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromCacheDropsOtherImage(t *testing.T) {
	dir := t.TempDir()
	module := filepath.Join(dir, "example@2026-01-01.yang")
	if err := os.WriteFile(module, []byte("module example { namespace x; prefix x; }"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".version"), []byte("v1\n"), 0640); err != nil {
		t.Fatal(err)
	}

	// Same image: the file stays.
	if err := NewCache(nil, dir, "v1").LoadFromCache(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(module); err != nil {
		t.Fatalf("file dropped for the same image: %v", err)
	}

	// New image: the cache is emptied and re-stamped.
	if err := NewCache(nil, dir, "v2").LoadFromCache(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(module); !os.IsNotExist(err) {
		t.Fatalf("file kept across images: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, ".version")); string(b) != "v2\n" {
		t.Errorf("stamp = %q", b)
	}

	// Unknown version: nothing is touched.
	if err := NewCache(nil, dir, "").LoadFromCache(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, ".version")); string(b) != "v2\n" {
		t.Errorf("stamp changed with unknown version: %q", b)
	}
}
