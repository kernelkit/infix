package schema

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"infix/webui/internal/restconf"
)

// A module cut short, next to a submodule that uses a type from it, makes
// goyang dereference nil instead of reporting an error.
func writeBrokenCache(t *testing.T, dir string) {
	t.Helper()
	files := map[string]string{
		"m@2026-01-01.yang": "module m { yang-version 1.1; namespace \"urn:m\"; prefix m; include s; typedef t { ",
		"s@2026-01-01.yang": "submodule s { yang-version 1.1; belongs-to m { prefix m; } leaf x { type t; } }",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0640); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadRejectsBrokenModule(t *testing.T) {
	dir := t.TempDir()
	writeBrokenCache(t, dir)
	if _, err := Load(dir); err == nil {
		t.Fatal("Load accepted a cut-short module")
	}
}

func TestLoadFromCacheDropsBrokenCache(t *testing.T) {
	dir := t.TempDir()
	writeBrokenCache(t, dir)
	if err := os.WriteFile(filepath.Join(dir, ".version"), []byte("v1\n"), 0640); err != nil {
		t.Fatal(err)
	}

	if err := NewCache(nil, dir, "v1").LoadFromCache(); err == nil {
		t.Fatal("broken cache loaded without error")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".yang") {
			t.Errorf("%s kept in a broken cache", e.Name())
		}
	}
	if b, _ := os.ReadFile(filepath.Join(dir, ".version")); string(b) != "v1\n" {
		t.Errorf("stamp = %q", b)
	}
}

// fakeFetcher serves a modules-state listing and the module files from a
// map.  Other Fetcher methods are never called.
type fakeFetcher struct {
	restconf.Fetcher
	modules map[string]string // name@revision -> body
}

func (f fakeFetcher) Get(_ context.Context, path string, target any) error {
	ms := target.(*rfc7895ModulesState)
	for key := range f.modules {
		name, rev, _ := strings.Cut(key, "@")
		ms.ModulesState.Module = append(ms.ModulesState.Module, struct {
			Name      string `json:"name"`
			Revision  string `json:"revision"`
			Submodule []struct {
				Name     string `json:"name"`
				Revision string `json:"revision"`
			} `json:"submodule"`
		}{Name: name, Revision: rev})
	}
	return nil
}

func (f fakeFetcher) GetYANG(_ context.Context, name, revision string) ([]byte, error) {
	return []byte(f.modules[name+"@"+revision]), nil
}

func TestFetchModulesPrunesStaleRevisions(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "example@2025-01-01.yang")
	if err := os.WriteFile(stale, []byte("module example { namespace x; prefix x; }"), 0640); err != nil {
		t.Fatal(err)
	}

	rc := fakeFetcher{modules: map[string]string{
		"example@2026-01-01": "module example { namespace x; prefix x; }",
	}}
	if _, err := FetchModules(context.Background(), rc, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale revision kept: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "example@2026-01-01.yang")); err != nil {
		t.Errorf("current revision missing: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".example") {
			t.Errorf("temporary file left behind: %s", e.Name())
		}
	}
}
