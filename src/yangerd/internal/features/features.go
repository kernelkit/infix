// Package features tells which optional parts of the YANG model this
// image carries, from sysrepo's own module list.  confd installs the
// modules and enables their features at build time, gated on the same
// Buildroot options that build the daemons, so the list is what
// ietf-yang-library reports to clients: the one source of truth for
// what is built in.  Read once at start, no sysrepo library, no fork.
package features

import (
	"encoding/json"
	"fmt"
	"os"
)

// ModulesPath is sysrepo's internal module list, JSON on disk.
const ModulesPath = "/etc/sysrepo/data/sysrepo.startup"

// Set is the installed modules and their enabled features.
type Set struct {
	modules map[string]map[string]bool
}

type moduleList struct {
	Modules struct {
		Module []struct {
			Name     string   `json:"name"`
			Features []string `json:"enabled-feature"`
		} `json:"module"`
	} `json:"sysrepo:sysrepo-modules"`
}

// Load reads the module list at path.
func Load(path string) (*Set, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Parse decodes a module list.
func Parse(data []byte) (*Set, error) {
	var list moduleList
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("sysrepo module list: %w", err)
	}
	s := &Set{modules: make(map[string]map[string]bool, len(list.Modules.Module))}
	for _, m := range list.Modules.Module {
		fs := make(map[string]bool, len(m.Features))
		for _, f := range m.Features {
			fs[f] = true
		}
		s.modules[m.Name] = fs
	}
	return s, nil
}

// All is what to assume when the module list cannot be read: every
// monitor runs and copes with an absent daemon itself.
func All() *Set { return &Set{} }

// HasModule reports whether the module is installed.
func (s *Set) HasModule(name string) bool {
	if s.modules == nil {
		return true
	}
	_, ok := s.modules[name]
	return ok
}

// Has reports whether the module is installed with the feature enabled.
func (s *Set) Has(module, feature string) bool {
	if s.modules == nil {
		return true
	}
	return s.modules[module][feature]
}

// The optional parts of yangerd and what in the model switches them on.

// WiFi: radios, access points, stations and mesh.
func (s *Set) WiFi() bool { return s.Has("infix-interfaces", "wifi") }

// Containers: podman containers and their networks.
func (s *Set) Containers() bool { return s.Has("infix-interfaces", "containers") }

// GPS: receivers behind gpsd.
func (s *Set) GPS() bool { return s.Has("infix-hardware", "gps") }

// FRR: routes from zebra and the OSPF, RIP and BFD control planes.
// The routing protocol modules are installed only when FRR is built.
func (s *Set) FRR() bool { return s.HasModule("ietf-ospf") }
