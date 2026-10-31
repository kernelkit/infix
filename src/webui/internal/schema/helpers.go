package schema

import (
	"strings"

	"github.com/openconfig/goyang/pkg/yang"
)

// IdentityOption is a schema-resolved identity/enum value for use in select dropdowns.
type IdentityOption struct {
	Value       string // full identity, e.g. "infix-system:clish" — submitted to RESTCONF
	Label       string // display label with module prefix stripped, e.g. "clish"
	Description string // the enum's or identity's YANG description
	IsDefault   bool   // true if this matches the leaf's YANG default
}

// StripModulePrefix strips the "module:" prefix from an identity or enum value.
func StripModulePrefix(v string) string {
	if i := strings.LastIndex(v, ":"); i >= 0 {
		return v[i+1:]
	}
	return v
}

// OptionsFor returns IdentityOption entries for the identityref or enumeration
// leaf at path. Returns nil when schema is unavailable or the leaf has no options.
func OptionsFor(mgr *Manager, path string) []IdentityOption {
	if mgr == nil {
		return nil
	}
	node, err := mgr.NodeAt(path)
	if err != nil || node == nil || node.Type == nil {
		return nil
	}
	values := node.Type.Identities
	if len(values) == 0 {
		values = node.Type.Enums
	}
	// An identity default is written with the module's own prefix, or none
	// at all, so only the name tells.
	def := StripModulePrefix(node.Default)
	desc := valueDescriptions(mgr, path)
	opts := make([]IdentityOption, 0, len(values))
	for _, v := range values {
		opts = append(opts, IdentityOption{
			Value:       v,
			Label:       StripModulePrefix(v),
			Description: Reflow(desc[StripModulePrefix(v)]),
			IsDefault:   def != "" && StripModulePrefix(v) == def,
		})
	}
	return opts
}

// OptionDescriptions maps each option's value to its description.
func OptionDescriptions(opts []IdentityOption) map[string]string {
	m := make(map[string]string, len(opts))
	for _, o := range opts {
		m[o.Value] = o.Description
	}
	return m
}

// valueDescriptions collects the descriptions of a leaf's enum values or
// derived identities, by bare name.  Enums sit on the type statement that
// declares them: the leaf's own, or the base of a typedef chain.
func valueDescriptions(mgr *Manager, path string) map[string]string {
	e, err := mgr.entryAt(path)
	if err != nil || e == nil || e.Type == nil {
		return nil
	}
	desc := map[string]string{}
	if e.Type.IdentityBase != nil {
		for _, id := range e.Type.IdentityBase.Values {
			if id.Description != nil {
				desc[id.Name] = id.Description.Name
			}
		}
		return desc
	}
	var t *yang.Type
	switch n := e.Node.(type) {
	case *yang.Leaf:
		t = n.Type
	case *yang.LeafList:
		t = n.Type
	}
	for t != nil && len(t.Enum) == 0 {
		next := (*yang.Type)(nil)
		if t.YangType != nil && t.YangType.Base != t {
			next = t.YangType.Base
		}
		t = next
	}
	if t != nil {
		for _, en := range t.Enum {
			if en.Description != nil {
				desc[en.Name] = en.Description.Name
			}
		}
	}
	return desc
}

// RangeBounds splits a YANG range such as "1..100" into its ends; a
// single value or an open end comes back empty on that side.
func RangeBounds(r string) (lo, hi string) {
	r = strings.TrimSpace(r)
	if i := strings.Index(r, ".."); i >= 0 {
		lo, hi = strings.TrimSpace(r[:i]), strings.TrimSpace(r[i+2:])
	} else if r != "" {
		lo, hi = r, r
	}
	if lo == "min" {
		lo = ""
	}
	if hi == "max" {
		hi = ""
	}
	return lo, hi
}

// DescriptionOf returns the YANG description for the leaf at path, or "".
func DescriptionOf(mgr *Manager, path string) string {
	if mgr == nil {
		return ""
	}
	node, err := mgr.NodeAt(path)
	if err != nil || node == nil {
		return ""
	}
	return Reflow(node.Description)
}

// Reflow joins the hard-wrapped lines of a YANG description into one line
// per paragraph.  Paragraphs are separated by a blank line and lines that
// start a list item (- or *) keep their line break.
func Reflow(desc string) string {
	var out strings.Builder
	var para strings.Builder

	flush := func() {
		if para.Len() > 0 {
			if out.Len() > 0 {
				out.WriteString("\n\n")
			}
			out.WriteString(para.String())
			para.Reset()
		}
	}

	for _, line := range strings.Split(desc, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* "):
			if para.Len() > 0 {
				para.WriteString("\n")
			}
			para.WriteString(line)
		case para.Len() > 0:
			para.WriteString(" ")
			para.WriteString(line)
		default:
			para.WriteString(line)
		}
	}
	flush()

	return out.String()
}
