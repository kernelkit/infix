// SPDX-License-Identifier: MIT

package schema

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime/debug"
	"sort"
	"strings"

	"github.com/openconfig/goyang/pkg/yang"
)

// Load parses all .yang files in yangDir and returns a Manager.
// All modules are loaded before Process() is called so that cross-module
// identityref resolution and augments work correctly.
// Errors from Process() that are non-fatal (e.g. unresolved augments for
// modules that were not downloaded) are logged but do not abort loading.
func Load(yangDir string) (mgr *Manager, err error) {
	// goyang dereferences nil on some malformed input, for example a
	// module file cut short, instead of returning an error.  The cache is
	// rebuilt from the device on failure, so turn that into an error.
	defer func() {
		if r := recover(); r != nil {
			mgr, err = nil, fmt.Errorf("schema: parse %s: %v", yangDir, r)
		}
	}()

	ms := yang.NewModules()
	ms.Path = []string{yangDir}

	entries, err := os.ReadDir(yangDir)
	if err != nil {
		return nil, fmt.Errorf("schema: read yang dir: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yang") {
			continue
		}
		if err := ms.Read(filepath.Join(yangDir, e.Name())); err != nil {
			// Non-fatal: goyang may still register the module partially.
			log.Printf("schema: parse (non-fatal) %s: %v", e.Name(), err)
		}
	}

	errs := ms.Process()
	for _, err := range errs {
		// goyang v1.6.3 has known YANG 1.1 gaps (must/when substatements,
		// duplicate augments).  These are non-fatal; log at debug level.
		log.Printf("schema: process (non-fatal): %v", err)
	}

	t := compile(ms)

	// The module set is many times the size of the tree
	debug.FreeOSMemory()

	return &Manager{t: t, features: readFeatures(yangDir)}, nil
}

// compile reduces a processed module set to a tree, after which the
// module set is no longer needed.
func compile(ms *yang.Modules) *tree {
	t := &tree{
		Modules:    make(map[string]*module),
		Identities:   make(map[string][]string),
		IdentityDesc: make(map[string]string),
	}
	derived := make(map[string]map[string]bool)
	done := make(map[*yang.Entry]*entry)

	for key, mod := range ms.Modules {
		if strings.Contains(key, "@") {
			continue // versioned alias (e.g. "ieee802-dot1ab-lldp@2022-03-15") — duplicate
		}
		// A submodule's nodes are under the module it belongs to
		if mod.BelongsTo == nil {
			m := &module{}
			if e := yang.ToEntry(mod); e != nil {
				m.Dir = compileDir(e.Dir, done)
			}
			t.Modules[key] = m
		}

		// Two modules may define an identity of the same name, the
		// lookup is by name alone so it gets what derives from either.
		for _, id := range mod.Identities() {
			if derived[id.Name] == nil {
				derived[id.Name] = make(map[string]bool)
			}
			for _, v := range id.Values {
				derived[id.Name][v.Name] = true
			}
			if id.Description != nil {
				t.IdentityDesc[identityName(id)] = id.Description.Name
			}
		}
	}
	for name, set := range derived {
		if len(set) == 0 {
			continue
		}
		names := make([]string, 0, len(set))
		for n := range set {
			names = append(names, n)
		}
		sort.Strings(names)
		t.Identities[name] = names
	}
	return t
}

// compileDir compiles the entries of a goyang Dir map, sorted by name.
func compileDir(dir map[string]*yang.Entry, done map[*yang.Entry]*entry) []*entry {
	if len(dir) == 0 {
		return nil
	}
	out := make([]*entry, 0, len(dir))
	for _, e := range dir {
		out = append(out, compileEntry(e, done))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func compileEntry(e *yang.Entry, done map[*yang.Entry]*entry) *entry {
	if c, ok := done[e]; ok {
		return c
	}
	c := &entry{
		Name:        e.Name,
		Kind:        entryKind(e),
		Description: e.Description,
		ReadOnly:    e.Config == yang.TSFalse,
		NoData: e.RPC != nil ||
			e.Kind == yang.NotificationEntry ||
			e.Kind == yang.AnyDataEntry ||
			e.Kind == yang.AnyXMLEntry ||
			isDeprecatedOrObsolete(e),
		Mandatory: e.Mandatory == yang.TSTrue,
		When:      extractWhen(e),
		Presence:  entryPresence(e),
		IfFeature: entryIfFeatures(e),
	}
	done[e] = c

	if mod, err := e.InstantiatingModule(); err == nil {
		c.Module = mod
	}
	if def, ok := e.SingleDefaultValue(); ok {
		c.Default = &def
	}
	if e.Key != "" {
		c.Keys = strings.Fields(e.Key)
	}
	if e.IsLeaf() || e.IsLeafList() {
		c.Type = yangTypeInfo(e)
		c.EnumDesc = enumDescriptions(e)
	}
	c.Dir = compileDir(e.Dir, done)

	return c
}

// entryIfFeatures returns the if-feature expressions on e, each feature
// written as "module:feature".  A feature is named "prefix:name", or
// bare for the entry's own module.
func entryIfFeatures(e *yang.Entry) []string {
	var vals []*yang.Value
	switch n := e.Node.(type) {
	case *yang.Container:
		vals = n.IfFeature
	case *yang.Leaf:
		vals = n.IfFeature
	case *yang.LeafList:
		vals = n.IfFeature
	case *yang.List:
		vals = n.IfFeature
	case *yang.Choice:
		vals = n.IfFeature
	case *yang.Case:
		vals = n.IfFeature
	}
	var exprs []string
	for _, v := range vals {
		tokens := ifFeatureTokens(v.Name)
		for i, t := range tokens {
			switch t {
			case "(", ")", "not", "and", "or":
				continue
			}
			mod, name := "", t
			if c := strings.Index(t, ":"); c >= 0 {
				if fm := yang.FindModuleByPrefix(e.Node, t[:c]); fm != nil {
					mod = fm.Name
				}
				name = t[c+1:]
			} else if root := yang.RootNode(e.Node); root != nil {
				mod = root.Name
				if root.BelongsTo != nil {
					mod = root.BelongsTo.Name
				}
			}
			tokens[i] = mod + ":" + name
		}
		exprs = append(exprs, strings.Join(tokens, " "))
	}
	return exprs
}

// enumDescriptions collects the descriptions of a leaf's enum values.
// Enums sit on the type statement that declares them: the leaf's own,
// or the base of a typedef chain.
func enumDescriptions(e *yang.Entry) map[string]string {
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
	if t == nil {
		return nil
	}
	var desc map[string]string
	for _, en := range t.Enum {
		if en.Description != nil {
			if desc == nil {
				desc = map[string]string{}
			}
			desc[en.Name] = en.Description.Name
		}
	}
	return desc
}

// canonicalizeLeafrefPath rewrites a YANG leafref path into the RESTCONF
// canonical form (RFC 8040 §3.5.3): each segment's prefix alias is
// resolved to its canonical module name, and the "module:" qualifier
// is dropped from segments whose module matches the preceding
// qualified segment. Unknown prefixes are left untouched.
//
// Example: "/if:interfaces/if:interface/if:name" with the `if` prefix
// pointing at `ietf-interfaces` becomes
// "/ietf-interfaces:interfaces/interface/name", which is what
// nodeSegment() produces for ordinary paths and what RESTCONF expects.
func canonicalizeLeafrefPath(node yang.Node, p string) string {
	if node == nil || p == "" {
		return p
	}
	segs := strings.Split(p, "/")
	var lastMod string
	for i, seg := range segs {
		if seg == "" || seg == ".." || seg == "." {
			continue
		}
		bare, pred := seg, ""
		if b := strings.IndexByte(seg, '['); b >= 0 {
			bare, pred = seg[:b], seg[b:]
		}
		colon := strings.IndexByte(bare, ':')
		if colon < 0 {
			// Unprefixed segments inherit the prevailing module.
			continue
		}
		prefix, rest := bare[:colon], bare[colon+1:]
		mod := yang.FindModuleByPrefix(node, prefix)
		if mod == nil {
			continue
		}
		modName := mod.Name
		if mod.BelongsTo != nil {
			modName = mod.BelongsTo.Name
		}
		if modName == lastMod {
			segs[i] = rest + pred
		} else {
			segs[i] = modName + ":" + rest + pred
			lastMod = modName
		}
	}
	return strings.Join(segs, "/")
}

// derefSiblingPattern captures the `deref(.)/../<sibling>` XPath idiom
// used by Infix YANG must statements to constrain a leafref's target
// to objects that carry a specific sibling node (e.g. `bridge-port/
// bridge`'s must says the referenced interface must have a `bridge`
// container — i.e. it has to be a bridge). The capture group is the
// local sibling name; any prefix (`tt:type`) is matched non-capturing
// so callers don't have to strip it.
var derefSiblingPattern = regexp.MustCompile(`deref\s*\(\s*\.\s*\)\s*/\s*\.\.\s*/\s*(?:[a-zA-Z][\w-]*:)?([a-zA-Z][\w-]*)`)

// leafrefSiblingFilter looks at the leaf's must statements (only Leaf
// nodes carry must in goyang's Entry tree) for the deref(.)/../X
// pattern and returns X, or "" when no such constraint exists.
func leafrefSiblingFilter(e *yang.Entry) string {
	leaf, ok := e.Node.(*yang.Leaf)
	if !ok {
		return ""
	}
	for _, m := range leaf.Must {
		if m == nil {
			continue
		}
		if match := derefSiblingPattern.FindStringSubmatch(m.Name); len(match) == 2 {
			return match[1]
		}
	}
	return ""
}

// entryPresence returns the YANG presence statement string for e, or "".
// Presence is stored on the underlying Container Node via reflection because
// yang.Entry does not expose it directly.
func entryPresence(e *yang.Entry) string {
	if e.Node == nil {
		return ""
	}
	v := reflect.ValueOf(e.Node)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	f := v.FieldByName("Presence")
	if !f.IsValid() || f.Kind() != reflect.Ptr || f.IsNil() {
		return ""
	}
	name := f.Elem().FieldByName("Name")
	if !name.IsValid() {
		return ""
	}
	return name.String()
}

// isDeprecatedOrObsolete returns true when the YANG node carries
// "status deprecated" or "status obsolete". goyang does not expose status
// directly on Entry, so we reach through to the underlying Node via reflection.
// All concrete yang node types (Container, Leaf, List, …) have Status *Value.
func isDeprecatedOrObsolete(e *yang.Entry) bool {
	if e.Node == nil {
		return false
	}
	v := reflect.ValueOf(e.Node)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	f := v.FieldByName("Status")
	if !f.IsValid() || f.Kind() != reflect.Ptr || f.IsNil() {
		return false
	}
	name := f.Elem().FieldByName("Name")
	if !name.IsValid() {
		return false
	}
	s := name.String()
	return s == "deprecated" || s == "obsolete"
}

// prefixInXPath matches a "prefix:X" token where X is a letter or underscore,
// the first character of an identifier.  The matched letter is included so
// ReplaceAllStringFunc can reattach it after resolving the prefix.
var prefixInXPath = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9_\-]*:[a-zA-Z_]`)

// extractWhen returns the pre-resolved YANG when expression for e, or "".
// It first checks e itself (when directly on the node), then checks parent
// augments — the Infix convention is to put when on the augment, not on the
// top-level container inside the augment.
func extractWhen(e *yang.Entry) string {
	if xpath, ok := e.GetWhenXPath(); ok && xpath != "" {
		return resolveWhenPrefixes(e.Node, xpath)
	}
	// Check parent's augment list: the augment may carry the when expression
	// even though the individual container inside it does not.
	if e.Parent == nil {
		return ""
	}
	for _, aug := range e.Parent.Augmented {
		if _, found := aug.Dir[e.Name]; !found {
			continue
		}
		if xpath, ok := aug.GetWhenXPath(); ok && xpath != "" {
			return resolveWhenPrefixes(aug.Node, xpath)
		}
	}
	return ""
}

// resolveWhenPrefixes replaces "prefix:x" tokens in an XPath expression with
// the canonical "module:x" form using FindModuleByPrefix on the given node.
// Unknown prefixes (not imported by the node's module) are left unchanged.
func resolveWhenPrefixes(node yang.Node, xpath string) string {
	return prefixInXPath.ReplaceAllStringFunc(xpath, func(m string) string {
		colon := strings.IndexByte(m, ':')
		prefix := m[:colon]
		rest := m[colon+1:] // the single identifier-start character
		mod := yang.FindModuleByPrefix(node, prefix)
		if mod == nil {
			return m
		}
		modName := mod.Name
		if mod.BelongsTo != nil {
			modName = mod.BelongsTo.Name
		}
		return modName + ":" + rest
	})
}

// entryKind maps a goyang Entry to a kind string.
func entryKind(e *yang.Entry) string {
	switch {
	case e.IsLeaf():
		return "leaf"
	case e.IsLeafList():
		return "leaf-list"
	case e.IsList():
		return "list"
	case e.IsContainer():
		return "container"
	case e.IsChoice():
		return "choice"
	case e.IsCase():
		return "case"
	case e.Kind == yang.AnyDataEntry:
		return "anydata"
	case e.Kind == yang.AnyXMLEntry:
		return "anyxml"
	case e.RPC != nil:
		return "rpc"
	default:
		return "unknown"
	}
}

// identityName returns an identity as "module:name".
func identityName(id *yang.Identity) string {
	root := yang.RootNode(id)
	if root == nil {
		return id.Name
	}
	modName := root.Name
	if root.Kind() == "submodule" && root.BelongsTo != nil {
		modName = root.BelongsTo.Name
	}
	return modName + ":" + id.Name
}

// yangTypeInfo builds a TypeInfo from a leaf's YangType.
func yangTypeInfo(e *yang.Entry) *TypeInfo {
	t := e.Type
	if t == nil {
		return nil
	}

	info := &TypeInfo{
		Kind: yang.TypeKindToName[t.Kind],
	}

	switch t.Kind {
	case yang.Yenum:
		if t.Enum != nil {
			info.Enums = t.Enum.Names()
			sort.Strings(info.Enums)
		}
	case yang.Yidentityref:
		if t.IdentityBase != nil {
			for _, v := range t.IdentityBase.Values {
				info.Identities = append(info.Identities, identityName(v))
			}
			sort.Strings(info.Identities)
		}
	case yang.Yleafref:
		info.Leafref = canonicalizeLeafrefPath(e.Node, t.Path)
		info.LeafrefSibling = leafrefSiblingFilter(e)
	case yang.Yunion:
		for _, sub := range t.Type {
			if sub.Kind == yang.Yenum && sub.Enum != nil {
				info.Enums = append(info.Enums, sub.Enum.Names()...)
			}
		}
		if len(info.Enums) > 0 {
			sort.Strings(info.Enums)
		}
	}

	if len(t.Pattern) > 0 {
		info.Pattern = strings.Join(t.Pattern, "|")
	}

	if len(t.Range) > 0 {
		info.Range = t.Range.String()
	}

	return info
}
