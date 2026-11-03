package alarmmonitor

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/kernelkit/infix/src/yangerd/internal/alarm"
)

// Config is what confd renders from the running /ietf-alarms:alarms
// configuration: the shelves and the severity assignments.  The file
// is the JSON encoding of the container itself, so there is no second
// schema to keep in step with the model.
type Config struct {
	Shelves  []shelf
	Profiles []profile
}

type shelf struct {
	Name      string
	Resources []*regexp.Regexp
	Types     []typeMatch
}

type typeMatch struct {
	ID        string
	Qualifier *regexp.Regexp
}

type profile struct {
	ID        string
	Qualifier *regexp.Regexp
	Resource  *regexp.Regexp
	Levels    []alarm.Severity
}

type rawConfig struct {
	Alarms struct {
		Control struct {
			Shelving struct {
				Shelf []struct {
					Name      string   `json:"name"`
					Resource  []string `json:"resource"`
					AlarmType []struct {
						ID        string `json:"alarm-type-id"`
						Qualifier string `json:"alarm-type-qualifier-match"`
					} `json:"alarm-type"`
				} `json:"shelf"`
			} `json:"alarm-shelving"`
		} `json:"control"`
		Profile []struct {
			ID        string `json:"alarm-type-id"`
			Qualifier string `json:"alarm-type-qualifier-match"`
			Resource  string `json:"resource"`
			Severity  struct {
				Levels []string `json:"severity-level"`
			} `json:"alarm-severity-assignment-profile"`
		} `json:"alarm-profile"`
	} `json:"ietf-alarms:alarms"`
}

// loadConfig reads the rendered configuration.  No file means no
// shelves and no assignments, which is also what an empty container
// means.
func loadConfig(path string) (Config, error) {
	var cfg Config

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}

	var raw rawConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}

	for _, s := range raw.Alarms.Control.Shelving.Shelf {
		sh := shelf{Name: s.Name}
		for _, r := range s.Resource {
			re, err := xsdRegexp(r)
			if err != nil {
				return cfg, fmt.Errorf("shelf %s: resource %q: %w", s.Name, r, err)
			}
			sh.Resources = append(sh.Resources, re)
		}
		for _, t := range s.AlarmType {
			re, err := xsdRegexp(t.Qualifier)
			if err != nil {
				return cfg, fmt.Errorf("shelf %s: qualifier %q: %w", s.Name, t.Qualifier, err)
			}
			sh.Types = append(sh.Types, typeMatch{ID: t.ID, Qualifier: re})
		}
		cfg.Shelves = append(cfg.Shelves, sh)
	}

	for _, p := range raw.Alarms.Profile {
		qual, err := xsdRegexp(p.Qualifier)
		if err != nil {
			return cfg, fmt.Errorf("profile %s: qualifier %q: %w", p.ID, p.Qualifier, err)
		}
		res, err := xsdRegexp(p.Resource)
		if err != nil {
			return cfg, fmt.Errorf("profile %s: resource %q: %w", p.ID, p.Resource, err)
		}
		pr := profile{ID: p.ID, Qualifier: qual, Resource: res}
		for _, l := range p.Severity.Levels {
			pr.Levels = append(pr.Levels, alarm.Severity(l))
		}
		cfg.Profiles = append(cfg.Profiles, pr)
	}

	return cfg, nil
}

// xsdRegexp compiles an XML Schema regular expression, which matches
// the whole string, as a Go one.
func xsdRegexp(s string) (*regexp.Regexp, error) {
	return regexp.Compile("^(?:" + s + ")$")
}

// matchType reports whether id is the alarm type, or the module
// prefixed form of it.  Derived identities are not used by this module.
func matchType(id, typ string) bool {
	return id == typeID(typ) || strings.TrimPrefix(id, Module+":") == typ
}

// shelfFor returns the first shelf whose criteria all match, or "".
func (c *Config) shelfFor(typ, resource, qualifier string) string {
	full := resourceOf(typ, resource)
	for _, s := range c.Shelves {
		if len(s.Resources) > 0 && !anyMatch(s.Resources, resource) && !anyMatch(s.Resources, full) {
			continue
		}
		if len(s.Types) > 0 {
			hit := false
			for _, t := range s.Types {
				if matchType(t.ID, typ) && t.Qualifier.MatchString(qualifier) {
					hit = true
					break
				}
			}
			if !hit {
				continue
			}
		}
		return s.Name
	}
	return ""
}

// assign maps the severity a producer reported to the one the profile
// assigns, by position in the type's severity-level list, or returns it
// unchanged when no profile matches.
func (c *Config) assign(typ, resource, qualifier string, at alarmType, sev alarm.Severity) alarm.Severity {
	full := resourceOf(typ, resource)
	for _, p := range c.Profiles {
		if !matchType(p.ID, typ) || !p.Qualifier.MatchString(qualifier) {
			continue
		}
		if !p.Resource.MatchString(resource) && !p.Resource.MatchString(full) {
			continue
		}
		if len(p.Levels) == 0 {
			return sev
		}
		idx := len(p.Levels) - 1
		for i, l := range at.Levels {
			if l == sev && i < len(p.Levels) {
				idx = i
				break
			}
		}
		return p.Levels[idx]
	}
	return sev
}

// anyMatch is whether one of the patterns matches s.  A shelf names a
// resource the way the operator sees it, so the caller tries both the
// marker name and the instance-identifier.
func anyMatch(res []*regexp.Regexp, s string) bool {
	for _, re := range res {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}
