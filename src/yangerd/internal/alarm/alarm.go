// Package alarm raises and clears alarms by writing marker files under
// /run/alarm.  A file in active/<type>/<name> is a raised alarm and its
// removal clears it; the alarm monitor turns the directory into the
// ietf-alarms tree.  A producer only needs this package, and a shell
// script only needs touch and rm.
package alarm

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// Root is the marker directory, overridable in tests.
var Root = "/run/alarm"

// Severity of an alarm, the ietf-alarms severity enumeration.
type Severity string

const (
	Indeterminate Severity = "indeterminate"
	Warning       Severity = "warning"
	Minor         Severity = "minor"
	Major         Severity = "major"
	Critical      Severity = "critical"
)

// Marker is the content of an active marker.  An empty file is valid
// and means the alarm type's default severity and description.
type Marker struct {
	Severity Severity `json:"severity,omitempty"`
	Text     string   `json:"text,omitempty"`
}

// validName limits a resource and a qualifier to what makes a file name
// that confd can derive from the YANG keys.
var validName = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)

// ValidResource reports whether s can be used as a resource.
func ValidResource(s string) bool { return validName.MatchString(s) }

// ValidQualifier reports whether s can be used as a qualifier.
func ValidQualifier(s string) bool { return s == "" || validName.MatchString(s) }

// Name is the marker file name for a resource and a qualifier.
func Name(resource, qualifier string) string {
	if qualifier == "" {
		return resource
	}
	return resource + "@" + qualifier
}

// SplitName is the inverse of Name.
func SplitName(name string) (resource, qualifier string) {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '@' {
			return name[:i], name[i+1:]
		}
	}
	return name, ""
}

// Path is the active marker of an alarm.
func Path(typ, resource, qualifier string) string {
	return filepath.Join(Root, "active", typ, Name(resource, qualifier))
}

// Raise raises, or updates, an alarm.  The marker is written to a temp
// file and renamed into place so the monitor never reads half a file.
func Raise(typ, resource, qualifier string, severity Severity, text string) error {
	if !ValidResource(resource) || !ValidQualifier(qualifier) {
		return fmt.Errorf("alarm %s: invalid resource %q or qualifier %q", typ, resource, qualifier)
	}
	data, err := json.Marshal(Marker{Severity: severity, Text: text})
	if err != nil {
		return err
	}
	path := Path(typ, resource, qualifier)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Clear clears an alarm.  Clearing one that is not raised is not an error.
func Clear(typ, resource, qualifier string) error {
	err := os.Remove(Path(typ, resource, qualifier))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// IsRaised reports whether the alarm is raised.
func IsRaised(typ, resource, qualifier string) bool {
	_, err := os.Stat(Path(typ, resource, qualifier))
	return err == nil
}
