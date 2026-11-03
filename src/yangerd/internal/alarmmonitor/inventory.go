package alarmmonitor

import (
	"fmt"
	"strings"

	"github.com/kernelkit/infix/src/yangerd/internal/alarm"
)

// Module is the YANG module that defines the alarm type identities.
const Module = "infix-alarms"

// alarmType is one entry of the alarm inventory.  Levels is the
// severity-level list, rising, and the default severity of a marker
// without content is the first level.  Resource, when set, turns the
// marker name into the instance-identifier of the alarming resource,
// RFC 8632 prefers that over a bare name when the resource is modeled.
type alarmType struct {
	Levels      []alarm.Severity
	WillClear   bool
	Description string
	Resource    string
}

// inventory is every alarm type the device can raise, keyed by the
// identity name.  An active marker of a type not listed here is ignored
// with a warning, since the identity would not validate.
var inventory = map[string]alarmType{
	"link-down": {
		Levels:      []alarm.Severity{alarm.Major},
		WillClear:   true,
		Description: "An enabled interface has no link.",
		Resource:    "/ietf-interfaces:interfaces/interface[name='%s']",
	},
	"startup-config-failed": {
		Levels:      []alarm.Severity{alarm.Critical},
		WillClear:   false,
		Description: "The startup configuration could not be applied, the device runs its failure configuration.",
	},
	"boot-fallback": {
		Levels:      []alarm.Severity{alarm.Major},
		WillClear:   false,
		Description: "The preferred software slot failed to boot, the device runs the fallback slot.",
	},
	"external": {
		Levels:      []alarm.Severity{alarm.Indeterminate, alarm.Warning, alarm.Minor, alarm.Major, alarm.Critical},
		WillClear:   true,
		Description: "An alarm raised by a script or an external system.",
	},
}

// severities in rising order, as the summary lists them.
var severities = []alarm.Severity{alarm.Indeterminate, alarm.Warning, alarm.Minor, alarm.Major, alarm.Critical}

// rank orders severities for the purge and summary comparisons.
func rank(s alarm.Severity) int {
	for i, v := range severities {
		if v == s {
			return i
		}
	}
	return -1
}

// typeID is the identityref value of an alarm type.
func typeID(typ string) string { return Module + ":" + typ }

// resourceOf is the resource of an alarm of type typ raised under the
// marker name: the instance-identifier for a type that has one, the
// name itself otherwise.
func resourceOf(typ, name string) string {
	if at, ok := inventory[typ]; ok && at.Resource != "" {
		return fmt.Sprintf(at.Resource, name)
	}
	return name
}

// resourceName is the short name of a resource, the last key value of
// an instance-identifier, or the resource itself.
func resourceName(resource string) string {
	if !strings.HasPrefix(resource, "/") {
		return resource
	}
	i := strings.LastIndex(resource, "='")
	if i < 0 {
		return resource
	}
	rest := resource[i+2:]
	if j := strings.IndexByte(rest, '\''); j >= 0 {
		return rest[:j]
	}
	return resource
}
