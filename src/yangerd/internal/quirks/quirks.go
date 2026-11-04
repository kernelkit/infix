// Package quirks reads the interface quirks: what a board or a driver
// cannot do, or does differently, kept out of the code in the quirks
// files of the system and of the product.
package quirks

import (
	"encoding/json"
	"os"
)

// Files are the quirks of the system, then those of the product, which
// override them entry by entry.
var Files = []string{"/etc/interface-quirks.json", "/etc/product/interface-quirks.json"}

// Quirks maps an interface name, or a pattern such as "@driver=<name>",
// to its quirks.
type Quirks map[string]map[string]json.RawMessage

// Load reads the files given, later ones overriding earlier ones entry
// by entry.  A file that is missing or broken is skipped.
func Load(files ...string) Quirks {
	q := Quirks{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var part Quirks
		if json.Unmarshal(data, &part) != nil {
			continue
		}
		for k, v := range part {
			q[k] = v
		}
	}
	return q
}

// Entry is the quirks of one interface: its own entry, and behind it
// the entry of its driver.
type Entry struct {
	own, driver map[string]json.RawMessage
}

// For is the quirks of the interface named, bound to the driver given.
func (q Quirks) For(ifname, driver string) Entry {
	e := Entry{own: q[ifname]}
	if driver != "" {
		e.driver = q["@driver="+driver]
	}
	return e
}

// Get is the raw value of a quirk, nil when unset.
func (e Entry) Get(name string) json.RawMessage {
	if v := e.own[name]; v != nil {
		return v
	}
	return e.driver[name]
}

// Strings is a list quirk, nil when unset.
func (e Entry) Strings(name string) []string {
	var list []string
	json.Unmarshal(e.Get(name), &list)
	return list
}

// Int is a number quirk, 0 when unset.
func (e Entry) Int(name string) int64 {
	var n int64
	json.Unmarshal(e.Get(name), &n)
	return n
}
