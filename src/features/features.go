// SPDX-License-Identifier: MIT

// Package features tells which optional parts of Infix a device carries,
// from the YANG modules it has installed and the features it has enabled.
// confd installs the optional modules and enables their features at build
// time, gated on the same Buildroot options that build the daemons, so the
// model is the one source of truth for what is built in.
//
// A client gets the list over RESTCONF or NETCONF as ietf-yang-library
// (ParseYangLibrary).  A daemon on the device reads the file the library
// is served from, sysrepo's module list (LoadSysrepoModules).  Both give
// the same Set.
package features

import (
	"encoding/json"
	"fmt"
	"os"
)

// SysrepoModulesPath is sysrepo's internal module list on a device.
const SysrepoModulesPath = "/etc/sysrepo/data/sysrepo.startup"

// Optional is one optional part of Infix and what in the model switches
// it on: a feature of a module, or with Feature empty the module itself.
type Optional struct {
	Name    string
	Module  string
	Feature string
}

// Names of the optional parts, the keys of Set.Enabled.
const (
	WiFi       = "wifi"
	Containers = "containers"
	GPS        = "gps"
	Modem      = "modem"
	Routing    = "routing"
	QoS        = "qos"
	TSN        = "tsn"
	RateLimit  = "rate-limit"
)

// Optionals is the table of optional parts.  Extend here, both the
// device daemons and the WebUI follow it.
var Optionals = []Optional{
	{Name: WiFi, Module: "infix-interfaces", Feature: "wifi"},
	{Name: Containers, Module: "infix-interfaces", Feature: "containers"},
	{Name: GPS, Module: "infix-hardware", Feature: "gps"},
	{Name: Modem, Module: "infix-hardware", Feature: "modem"},
	// The routing protocol modules are installed only with FRR.
	{Name: Routing, Module: "ietf-ospf"},
	{Name: QoS, Module: "infix-qos"},
	{Name: TSN, Module: "infix-tsn"},
	{Name: RateLimit, Module: "infix-interfaces", Feature: "rate-limit"},
}

// Set is the installed modules and their enabled features.
type Set struct {
	modules map[string]map[string]bool
}

type yangLibrary struct {
	Library struct {
		ModuleSet []struct {
			Module []struct {
				Name    string   `json:"name"`
				Feature []string `json:"feature"`
			} `json:"module"`
		} `json:"module-set"`
	} `json:"ietf-yang-library:yang-library"`
}

type sysrepoModules struct {
	Modules struct {
		Module []struct {
			Name    string   `json:"name"`
			Feature []string `json:"enabled-feature"`
		} `json:"module"`
	} `json:"sysrepo:sysrepo-modules"`
}

// ParseYangLibrary decodes ietf-yang-library as a GET of
// /restconf/data/ietf-yang-library:yang-library returns it.
func ParseYangLibrary(data []byte) (*Set, error) {
	var lib yangLibrary
	if err := json.Unmarshal(data, &lib); err != nil {
		return nil, fmt.Errorf("yang-library: %w", err)
	}
	s := newSet()
	for _, ms := range lib.Library.ModuleSet {
		for _, m := range ms.Module {
			s.add(m.Name, m.Feature)
		}
	}
	return s, nil
}

// ParseSysrepoModules decodes sysrepo's module list.
func ParseSysrepoModules(data []byte) (*Set, error) {
	var list sysrepoModules
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("sysrepo module list: %w", err)
	}
	s := newSet()
	for _, m := range list.Modules.Module {
		s.add(m.Name, m.Feature)
	}
	return s, nil
}

// LoadSysrepoModules reads sysrepo's module list from path.
func LoadSysrepoModules(path string) (*Set, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseSysrepoModules(data)
}

// All is the Set to assume when the model cannot be read: everything on.
func All() *Set { return &Set{} }

func newSet() *Set { return &Set{modules: make(map[string]map[string]bool)} }

func (s *Set) add(module string, features []string) {
	fs := make(map[string]bool, len(features))
	for _, f := range features {
		fs[f] = true
	}
	s.modules[module] = fs
}

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

// On reports whether the optional part named name is present.
func (s *Set) On(name string) bool {
	for _, o := range Optionals {
		if o.Name != name {
			continue
		}
		if o.Feature == "" {
			return s.HasModule(o.Module)
		}
		return s.Has(o.Module, o.Feature)
	}
	return false
}

// Enabled is every optional part by name, present or not.
func (s *Set) Enabled() map[string]bool {
	out := make(map[string]bool, len(Optionals))
	for _, o := range Optionals {
		out[o.Name] = s.On(o.Name)
	}
	return out
}
