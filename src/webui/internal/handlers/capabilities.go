// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"log"
	"os"

	"infix/webui/internal/restconf"
)

// docsIndexPath is the entry point of the on-device User's Guide bundled
// by post-build.sh (mkdocs → /var/www/guide), served by nginx at /guide/.
const docsIndexPath = "/var/www/guide/index.html"

// DetectDocs reports whether the User's Guide was bundled into this image
// (the build host had mkdocs), so the UI can show or hide the entry.
func DetectDocs() bool {
	_, err := os.Stat(docsIndexPath)
	return err == nil
}

type feature struct {
	Name    string // key used in Has() and session cookie
	Module  string // YANG module that carries the feature
	Feature string // YANG feature name in the module's feature array
}

// optionalFeatures maps UI capabilities to YANG module features. Extend here.
var optionalFeatures = []feature{
	{Name: "wifi", Module: "infix-interfaces", Feature: "wifi"},
	{Name: "containers", Module: "infix-interfaces", Feature: "containers"},
}

// Capabilities tracks which optional features are present on the device.
// Use Has("feature-name") in templates and Go code.
type Capabilities struct {
	features map[string]bool
}

func NewCapabilities(features map[string]bool) *Capabilities {
	if features == nil {
		features = make(map[string]bool)
	}
	return &Capabilities{features: features}
}

func (c *Capabilities) Has(name string) bool {
	return c != nil && c.features[name]
}

func (c *Capabilities) Features() map[string]bool {
	if c == nil {
		return nil
	}
	return c.features
}

type capsCtxKey struct{}

func ContextWithCapabilities(ctx context.Context, caps *Capabilities) context.Context {
	return context.WithValue(ctx, capsCtxKey{}, caps)
}

func CapabilitiesFromContext(ctx context.Context) *Capabilities {
	caps, _ := ctx.Value(capsCtxKey{}).(*Capabilities)
	if caps == nil {
		return NewCapabilities(nil)
	}
	return caps
}

type yangLibrary struct {
	YangLibrary struct {
		ModuleSet []struct {
			Module []struct {
				Name    string   `json:"name"`
				Feature []string `json:"feature"`
			} `json:"module"`
		} `json:"module-set"`
	} `json:"ietf-yang-library:yang-library"`
}

// webConfig mirrors the slice of infix-services we need to gate the
// external web-app shortcuts (console, netbrowse).
type webConfig struct {
	Web struct {
		Console struct {
			Enabled bool `json:"enabled"`
		} `json:"console"`
		Netbrowse struct {
			Enabled bool `json:"enabled"`
		} `json:"netbrowse"`
	} `json:"infix-services:web"`
}

// DetectWebShortcuts reports whether the web console (ttyd on :7681) and
// the mDNS network browser (netbrowse at network.local) are enabled in
// config, so the UI can show or hide their shortcuts.  One read covers
// both.  Fails closed: a read error hides both rather than offering a
// dead link.
func DetectWebShortcuts(ctx context.Context, rc restconf.Fetcher) (console, netbrowse bool) {
	var cfg webConfig
	if err := rc.Get(ctx, "/data/infix-services:web", &cfg); err != nil {
		log.Printf("web config: %v (console/netbrowse shortcuts hidden)", err)
		return false, false
	}
	return cfg.Web.Console.Enabled, cfg.Web.Netbrowse.Enabled
}

// ApplyWebShortcuts writes the console/netbrowse capability flags into features
// from running-config.  These two are the only config-toggleable shortcuts, so
// keeping the keys in one place lets both login (baking into the session) and
// the per-page-load refresh share them.
func ApplyWebShortcuts(ctx context.Context, rc restconf.Fetcher, features map[string]bool) {
	features["console"], features["netbrowse"] = DetectWebShortcuts(ctx, rc)
}

func DetectCapabilities(ctx context.Context, rc restconf.Fetcher) *Capabilities {
	var lib yangLibrary
	if err := rc.Get(ctx, "/data/ietf-yang-library:yang-library", &lib); err != nil {
		log.Printf("yang-library: %v (ignored, no optional features)", err)
		return NewCapabilities(nil)
	}

	// Build index: module name → set of YANG features advertised.
	modFeatures := make(map[string]map[string]bool)
	for _, ms := range lib.YangLibrary.ModuleSet {
		for _, m := range ms.Module {
			fs := make(map[string]bool, len(m.Feature))
			for _, f := range m.Feature {
				fs[f] = true
			}
			modFeatures[m.Name] = fs
		}
	}

	result := make(map[string]bool, len(optionalFeatures))
	for _, f := range optionalFeatures {
		result[f.Name] = modFeatures[f.Module][f.Feature]
	}

	return NewCapabilities(result)
}
