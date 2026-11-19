package schema

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"infix/webui/internal/restconf"
)

// ModuleInfo identifies a YANG module by name and revision, with the
// features the device enables in it.
type ModuleInfo struct {
	Name     string
	Revision string
	Features []string
}

// FeatureSet lists the enabled features as "module:feature".
func FeatureSet(mods []ModuleInfo) map[string]bool {
	set := map[string]bool{}
	for _, m := range mods {
		for _, f := range m.Features {
			set[m.Name+":"+f] = true
		}
	}
	return set
}

func (m ModuleInfo) filename() string {
	if m.Revision == "" {
		return m.Name + ".yang"
	}
	return m.Name + "@" + m.Revision + ".yang"
}

// rfc7895ModulesState is the RFC 7895 /modules-state response structure.
type rfc7895ModulesState struct {
	ModulesState struct {
		Module []struct {
			Name      string   `json:"name"`
			Revision  string   `json:"revision"`
			Feature   []string `json:"feature"`
			Submodule []struct {
				Name     string `json:"name"`
				Revision string `json:"revision"`
			} `json:"submodule"`
		} `json:"module"`
	} `json:"ietf-yang-library:modules-state"`
}

// rfc8525YangLibrary is the RFC 8525 /yang-library response structure (fallback).
type rfc8525YangLibrary struct {
	YangLibrary struct {
		ModuleSet []struct {
			Module []struct {
				Name     string   `json:"name"`
				Revision string   `json:"revision"`
				Feature  []string `json:"feature"`
			} `json:"module"`
		} `json:"module-set"`
	} `json:"ietf-yang-library:yang-library"`
}

// FetchModules downloads any YANG files not already cached in cacheDir.
// It first tries the RFC 7895 modules-state endpoint, then falls back to
// the RFC 8525 yang-library endpoint (same as capabilities.go).
// Each module and its submodules are downloaded from /yang/{name}@{rev}.yang.
func FetchModules(ctx context.Context, rc restconf.Fetcher, cacheDir string) ([]ModuleInfo, error) {
	if err := os.MkdirAll(cacheDir, 0750); err != nil {
		return nil, fmt.Errorf("schema: create cache dir: %w", err)
	}

	modules, err := listModules(ctx, rc)
	if err != nil {
		return nil, err
	}

	var downloaded []ModuleInfo
	for _, m := range modules {
		if err := downloadIfMissing(ctx, rc, cacheDir, m); err != nil {
			log.Printf("schema: skip %s: %v", m.filename(), err)
			continue
		}
		downloaded = append(downloaded, m)
	}
	prune(cacheDir, modules)
	if err := writeFeatures(cacheDir, FeatureSet(modules)); err != nil {
		return nil, err
	}
	return downloaded, nil
}

// prune removes cached YANG files the device no longer lists, such as an
// earlier revision of a module, so that two revisions never load together.
func prune(cacheDir string, modules []ModuleInfo) {
	keep := make(map[string]bool, len(modules))
	for _, m := range modules {
		keep[m.filename()] = true
	}
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".yang") || keep[name] {
			continue
		}
		if err := os.Remove(filepath.Join(cacheDir, name)); err == nil {
			log.Printf("schema: dropped %s, no longer on the device", name)
		}
	}
}

// listModules queries the device for the list of implemented YANG modules.
func listModules(ctx context.Context, rc restconf.Fetcher) ([]ModuleInfo, error) {
	// Try RFC 7895 modules-state first.
	var ms rfc7895ModulesState
	if err := rc.Get(ctx, "/data/ietf-yang-library:modules-state", &ms); err == nil {
		var mods []ModuleInfo
		for _, m := range ms.ModulesState.Module {
			mods = append(mods, ModuleInfo{Name: m.Name, Revision: m.Revision, Features: m.Feature})
			for _, sub := range m.Submodule {
				mods = append(mods, ModuleInfo{Name: sub.Name, Revision: sub.Revision})
			}
		}
		if len(mods) > 0 {
			return mods, nil
		}
	}

	// Fall back to RFC 8525 yang-library.
	var yl rfc8525YangLibrary
	if err := rc.Get(ctx, "/data/ietf-yang-library:yang-library", &yl); err != nil {
		return nil, fmt.Errorf("schema: list modules: %w", err)
	}
	var mods []ModuleInfo
	for _, ms := range yl.YangLibrary.ModuleSet {
		for _, m := range ms.Module {
			mods = append(mods, ModuleInfo{Name: m.Name, Revision: m.Revision, Features: m.Feature})
		}
	}
	return mods, nil
}

// downloadIfMissing fetches a single YANG file from the device if not cached.
func downloadIfMissing(ctx context.Context, rc restconf.Fetcher, cacheDir string, m ModuleInfo) error {
	dest := filepath.Join(cacheDir, m.filename())
	if _, err := os.Stat(dest); err == nil {
		return nil // already cached
	}

	data, err := rc.GetYANG(ctx, m.Name, m.Revision)
	if err != nil {
		return fmt.Errorf("GET /yang/%s: %w", m.filename(), err)
	}

	// Write through a temporary name so that a restart in the middle of
	// the download never leaves a cut-short module behind.
	tmp, err := os.CreateTemp(cacheDir, "."+m.Name+".*")
	if err != nil {
		return fmt.Errorf("write %s: %w", dest, err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return fmt.Errorf("write %s: %w", dest, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("write %s: %w", dest, err)
	}
	if err := os.Chmod(tmp.Name(), 0640); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("write %s: %w", dest, err)
	}
	if err := os.Rename(tmp.Name(), dest); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("write %s: %w", dest, err)
	}
	log.Printf("schema: cached %s", m.filename())
	return nil
}

// The enabled features sit next to the cached modules, so a load from
// the cache hides the same nodes as a load after a refresh.
const featuresFile = "features.json"

func writeFeatures(cacheDir string, set map[string]bool) error {
	b, err := json.Marshal(set)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(cacheDir, featuresFile), b, 0640)
}

// readFeatures returns nil when no feature list was cached, which gates
// nothing.
func readFeatures(cacheDir string) map[string]bool {
	b, err := os.ReadFile(filepath.Join(cacheDir, featuresFile))
	if err != nil {
		return nil
	}
	var set map[string]bool
	if json.Unmarshal(b, &set) != nil {
		return nil
	}
	return set
}
