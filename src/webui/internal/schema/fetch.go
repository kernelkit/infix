package schema

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/kernelkit/webui/internal/restconf"
)

// ModuleInfo identifies a YANG module by name and revision.
type ModuleInfo struct {
	Name     string
	Revision string
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
			Name       string `json:"name"`
			Revision   string `json:"revision"`
			Submodule  []struct {
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
				Name     string `json:"name"`
				Revision string `json:"revision"`
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
	return downloaded, nil
}

// listModules queries the device for the list of implemented YANG modules.
func listModules(ctx context.Context, rc restconf.Fetcher) ([]ModuleInfo, error) {
	// Try RFC 7895 modules-state first.
	var ms rfc7895ModulesState
	if err := rc.Get(ctx, "/data/ietf-yang-library:modules-state", &ms); err == nil {
		var mods []ModuleInfo
		for _, m := range ms.ModulesState.Module {
			mods = append(mods, ModuleInfo{Name: m.Name, Revision: m.Revision})
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
			mods = append(mods, ModuleInfo{Name: m.Name, Revision: m.Revision})
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

	if err := os.WriteFile(dest, data, 0640); err != nil {
		return fmt.Errorf("write %s: %w", dest, err)
	}
	log.Printf("schema: cached %s", m.filename())
	return nil
}
