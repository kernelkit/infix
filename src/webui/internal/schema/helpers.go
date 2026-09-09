package schema

import "strings"

// IdentityOption is a schema-resolved identity/enum value for use in select dropdowns.
type IdentityOption struct {
	Value     string // full identity, e.g. "infix-system:clish" — submitted to RESTCONF
	Label     string // display label with module prefix stripped, e.g. "clish"
	IsDefault bool   // true if this matches the leaf's YANG default
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
	opts := make([]IdentityOption, 0, len(values))
	for _, v := range values {
		opts = append(opts, IdentityOption{
			Value:     v,
			Label:     StripModulePrefix(v),
			IsDefault: node.Default != "" && v == node.Default,
		})
	}
	return opts
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
