package schema

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/openconfig/goyang/pkg/yang"
)

// Manager holds a fully processed goyang module set and provides schema queries.
// All modules are loaded before Process() is called so that cross-module
// identityref resolution and augments work correctly.
type Manager struct {
	ms *yang.Modules
}

// Load parses all .yang files in yangDir and returns a Manager.
// Errors from Process() that are non-fatal (e.g. unresolved augments for
// modules that were not downloaded) are logged but do not abort loading.
func Load(yangDir string) (*Manager, error) {
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

	return &Manager{ms: ms}, nil
}

// Children returns the direct config-relevant child Nodes of the node at path.
// Use path "/" for the top-level module list.
// choice/case nodes are inlined transparently (their children are promoted).
func (m *Manager) Children(path string) ([]*Node, error) {
	if path == "" || path == "/" {
		return m.topLevelNodes(), nil
	}
	e, err := m.entryAt(path)
	if err != nil {
		return nil, err
	}
	if e.Dir == nil {
		return nil, nil
	}
	return dirToNodes(e.Dir, path), nil
}

// NodeAt returns a Node for the YANG schema node at path (without children).
func (m *Manager) NodeAt(path string) (*Node, error) {
	if path == "" || path == "/" {
		return &Node{Path: "/", Name: "", Kind: "container", Config: true}, nil
	}
	e, err := m.entryAt(path)
	if err != nil {
		return nil, err
	}
	return entryToNode(e, path), nil
}

// IdentitiesOf returns the names of all identities derived (directly or
// transitively) from baseName. baseName may be "name" or "module:name".
// Searches across all loaded modules.
func (m *Manager) IdentitiesOf(baseName string) []string {
	// Strip module prefix if present; search by identity name.
	_, localName := splitPrefix(baseName)

	for _, mod := range m.ms.Modules {
		for _, id := range mod.Identities() {
			if id.Name == localName {
				var names []string
				for _, v := range id.Values {
					names = append(names, v.Name)
				}
				sort.Strings(names)
				return names
			}
		}
	}
	return nil
}

// ResolveLeafref converts a YANG leafref path expression to an absolute schema
// path that can be fetched via RESTCONF. contextPath is the schema path of the
// leaf that holds the leafref (needed for relative `../` resolution).
// Returns empty string if the path cannot be resolved.
func (m *Manager) ResolveLeafref(leafrefPath, contextPath string) string {
	if strings.HasPrefix(leafrefPath, "/") {
		return normalizeLeafrefPath(leafrefPath)
	}
	// Relative path: resolve `../` against the context, stripping the leaf itself.
	parts := strings.Split(strings.TrimPrefix(contextPath, "/"), "/")
	if len(parts) > 0 {
		parts = parts[:len(parts)-1] // remove the leaf; start from its parent
	}
	for _, seg := range strings.Split(leafrefPath, "/") {
		switch seg {
		case "..":
			if len(parts) > 0 {
				parts = parts[:len(parts)-1]
			}
		case ".", "":
			// skip
		default:
			parts = append(parts, stripPredicate(seg))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "/" + strings.Join(parts, "/")
}

// normalizeLeafrefPath strips XPath predicates from each segment of an
// absolute YANG leafref path so it can be used as a schema lookup path.
func normalizeLeafrefPath(p string) string {
	segs := strings.Split(strings.TrimPrefix(p, "/"), "/")
	for i, s := range segs {
		segs[i] = stripPredicate(s)
	}
	return "/" + strings.Join(segs, "/")
}

// Default returns the YANG default value for the leaf at path, if any.
func (m *Manager) Default(path string) (string, bool) {
	e, err := m.entryAt(path)
	if err != nil {
		return "", false
	}
	return e.SingleDefaultValue()
}

// ModuleName returns the module name for the schema node at path.
func (m *Manager) ModuleName(path string) (string, error) {
	e, err := m.entryAt(path)
	if err != nil {
		return "", err
	}
	return e.InstantiatingModule()
}

// ModuleQualifiedName returns "module:name" for the schema node at path.
// This is the JSON object key used when PUT/PATCHing that node directly via
// RESTCONF (RFC 7951 §4 — namespace-qualified name at module boundaries).
func (m *Manager) ModuleQualifiedName(path string) (string, error) {
	e, err := m.entryAt(path)
	if err != nil {
		return "", err
	}
	modName, err := e.InstantiatingModule()
	if err != nil {
		return "", err
	}
	return modName + ":" + e.Name, nil
}

// internalModules is the deny-list of YANG modules that are infrastructure-only
// and must not appear in the user-facing configure tree.
// Mirrors sr_module_is_internal() from klish-plugin-sysrepo/src/pline.c.
// Note: ietf-netconf-acm is intentionally NOT listed here — Infix exposes NACM
// configuration to users.
var internalModules = map[string]bool{
	// libyang built-ins
	"ietf-yang-metadata": true,
	"yang":               true,
	"ietf-inet-types":    true,
	"ietf-yang-types":    true,
	// YANG library / schema mount
	"ietf-datastores":         true,
	"ietf-yang-schema-mount":  true,
	"ietf-yang-library":       true,
	// NETCONF infrastructure
	"ietf-netconf":                  true,
	"ietf-netconf-with-defaults":    true,
	"ietf-origin":                   true,
	"ietf-netconf-notifications":    true,
	// sysrepo internals
	"sysrepo":            true,
	"sysrepo-monitoring": true,
	"sysrepo-plugind":    true,
	// Infix test/debug subtree and meta-data — not user-facing
	"infix-test": true,
	"infix-meta": true,
	// NETCONF server / transport infrastructure — managed by the system, not users
	"ietf-netconf-server":          true,
	"libnetconf2-netconf-server":   true,
	"ietf-truststore":              true,
	// Notification subscriptions and filters — not user-configurable via WebUI
	"ietf-subscribed-notifications": true,
	// ACL, key-chains, network-instances — not exposed in WebUI yet
	"ietf-access-control-list":  true,
	"ietf-key-chain":            true,
	"ietf-network-instance":     true,
}

// topLevelNodes returns a Node for each config-relevant top-level schema node
// across all loaded modules. Submodules, versioned aliases, internal modules,
// and config:false top-level nodes are all excluded.
func (m *Manager) topLevelNodes() []*Node {
	var nodes []*Node
	for key, mod := range m.ms.Modules {
		if strings.Contains(key, "@") {
			continue // versioned alias (e.g. "ieee802-dot1ab-lldp@2022-03-15") — duplicate
		}
		if mod.BelongsTo != nil {
			continue // submodule — content appears under the parent module
		}
		if internalModules[key] {
			continue
		}
		e := yang.ToEntry(mod)
		if e == nil || e.Dir == nil {
			continue
		}
		for _, child := range sortedEntries(e.Dir) {
			if isNonConfigNode(child) {
				continue
			}
			nodePath := "/" + key + ":" + child.Name
			if isContainerList(child) {
				if lc := listChildOf(child); lc != nil {
					nodes = append(nodes, entryToNode(lc, nodePath+"/"+nodeSegment(lc, key)))
				}
				continue
			}
			nodes = append(nodes, entryToNode(child, nodePath))
		}
	}
	// Sort by node name (not module-qualified path) for a clean alphabetical list.
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })
	return nodes
}

// entryAt resolves a RESTCONF-style path to a goyang Entry.
// Path format: /module:top-node/child[key='val']/grandchild
// Key predicates ([key='val']) are stripped — they carry instance identity,
// not schema identity. choice/case nodes are skipped during traversal.
func (m *Manager) entryAt(path string) (*yang.Entry, error) {
	path = strings.TrimPrefix(path, "/")
	parts := strings.SplitN(path, "/", 2)

	head := stripPredicate(parts[0])
	moduleName, nodeName := splitPrefix(head)

	if moduleName == "" {
		return nil, fmt.Errorf("schema: path must start with module prefix: %s", path)
	}

	mod, ok := m.ms.Modules[moduleName]
	if !ok {
		return nil, fmt.Errorf("schema: module not found: %s", moduleName)
	}

	root := yang.ToEntry(mod)
	if root == nil {
		return nil, fmt.Errorf("schema: no entry for module: %s", moduleName)
	}

	e := findInDir(root.Dir, nodeName)
	if e == nil {
		return nil, fmt.Errorf("schema: %s not found in %s", nodeName, moduleName)
	}

	if len(parts) == 1 {
		return e, nil
	}

	// Traverse remaining path segments, stripping key predicates.
	for _, seg := range strings.Split(parts[1], "/") {
		if seg == "" {
			continue
		}
		_, localName := splitPrefix(stripPredicate(seg))
		child := findInDir(e.Dir, localName)
		if child == nil {
			return nil, fmt.Errorf("schema: %s not found under %s", localName, e.Name)
		}
		e = child
	}
	return e, nil
}

// stripPredicate removes a key predicate from a path segment.
// Handles both RESTCONF ("interface=eth0" → "interface") and
// XPath ("interface[name='eth0']" → "interface") forms.
func stripPredicate(seg string) string {
	if i := strings.IndexByte(seg, '['); i >= 0 {
		return seg[:i]
	}
	if i := strings.IndexByte(seg, '='); i >= 0 {
		return seg[:i]
	}
	return seg
}

// findInDir looks up name in a dir map, transparently descending into
// choice/case nodes which are not part of the RESTCONF path.
func findInDir(dir map[string]*yang.Entry, name string) *yang.Entry {
	if dir == nil {
		return nil
	}
	// Direct match — but if it is a choice/case, descend into it because YANG
	// commonly names a case identically to the leaf it contains (e.g. the
	// ietf-system timezone-name case wrapping the timezone-name leaf).
	if e, ok := dir[name]; ok {
		if !e.IsChoice() && !e.IsCase() {
			return e
		}
		if found := findInDir(e.Dir, name); found != nil {
			return found
		}
	}
	// Search inside all choice/case children.
	for _, e := range dir {
		if e.IsChoice() || e.IsCase() {
			if found := findInDir(e.Dir, name); found != nil {
				return found
			}
		}
	}
	return nil
}

// isContainerList returns true for containers that wrap exactly one list and
// nothing else — the classic YANG container/list idiom (e.g. /interfaces wrapping
// /interfaces/interface).  Mirrors klysc_is_container_list() in klish-plugin-sysrepo
// exactly: ALL direct children are inspected, including config:false ones.  Any
// non-list child (a leaf, another container, …) prevents collapsing.  This is why
// "hardware" does not collapse — it has a config:false "last-change" leaf alongside
// "component", so the default case fires and returns false.
//
// Hard exceptions:
//   - "static-routes"  always collapses (ietf-routing, asymmetric naming)
//   - "mdb"            never collapses
//   - "ipv4", "ipv6"   never collapse (would confuse routing subtrees)
func isContainerList(e *yang.Entry) bool {
	if !e.IsContainer() || e.Dir == nil {
		return false
	}
	if e.Name == "mdb" || e.Name == "ipv4" || e.Name == "ipv6" {
		return false
	}
	if e.Name == "static-routes" {
		return true
	}
	listCount := 0
	for _, child := range e.Dir {
		if !child.IsList() {
			return false // any non-list child (even config:false) prevents collapse
		}
		listCount++
	}
	return listCount == 1
}

// listChildOf returns the single list child of a collapsible container (panics if
// called on a non-collapsible container — callers must guard with isContainerList).
func listChildOf(e *yang.Entry) *yang.Entry {
	for _, child := range e.Dir {
		if child.IsList() && !isNonConfigNode(child) {
			return child
		}
	}
	return nil
}

// dirToNodes converts a goyang Dir map to a sorted slice of Nodes.
// choice/case children are inlined (their contents promoted to this level).
// Collapsible container-list wrappers are transparent: the list child is surfaced
// directly under the parent with the full (un-collapsed) RESTCONF path.
// RPC, notification, anydata, anyxml and config:false nodes are excluded.
//
// RESTCONF requires module qualification ("module:name") whenever a node's
// module differs from its parent's — the common case being augmented nodes.
// e.g. infix-lldp augments ieee802-dot1ab-lldp:lldp, so the path is
// /ieee802-dot1ab-lldp:lldp/infix-lldp:enabled, not /…/enabled.
func dirToNodes(dir map[string]*yang.Entry, parentPath string) []*Node {
	parentMod := extractModuleFromPath(parentPath)
	var nodes []*Node
	for _, e := range sortedEntries(dir) {
		if e.IsChoice() || e.IsCase() {
			// Extract when from the choice/case before inlining its children.
			// Cases contributed by augments carry the augment's when (e.g.
			// lag-port only for ethernetCsmacd, bridge-port only for bridge).
			// Propagate that constraint to each promoted child that has no when of
			// its own; if the child already has one, leave it alone.
			caseWhen := extractWhen(e)
			for _, child := range dirToNodes(e.Dir, parentPath) {
				if caseWhen != "" && child.When == "" {
					child.When = caseWhen
				}
				nodes = append(nodes, child)
			}
			continue
		}
		if isNonConfigNode(e) {
			continue
		}
		nodePath := parentPath + "/" + nodeSegment(e, parentMod)
		if isContainerList(e) {
			if lc := listChildOf(e); lc != nil {
				// Container is collapsed; qualify list child against parentMod.
				nodes = append(nodes, entryToNode(lc, nodePath+"/"+nodeSegment(lc, parentMod)))
			}
			continue
		}
		nodes = append(nodes, entryToNode(e, nodePath))
	}
	return nodes
}

// nodeSegment returns the path segment for e, qualified as "module:name" when
// e's instantiating module differs from parentMod (RESTCONF RFC 8040 §3.5.3).
func nodeSegment(e *yang.Entry, parentMod string) string {
	mod, err := e.InstantiatingModule()
	if err != nil || mod == parentMod {
		return e.Name
	}
	return mod + ":" + e.Name
}

// extractModuleFromPath returns the module from the rightmost module-qualified
// segment in a RESTCONF path, e.g. "/ieee802-dot1ab-lldp:lldp/port" → "ieee802-dot1ab-lldp".
func extractModuleFromPath(path string) string {
	segs := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i := len(segs) - 1; i >= 0; i-- {
		seg := stripPredicate(segs[i])
		if j := strings.IndexByte(seg, ':'); j > 0 {
			return seg[:j]
		}
	}
	return ""
}

// isNonConfigNode returns true for schema nodes that do not belong in the
// configuration tree: RPCs, notifications, anydata/anyxml, config:false subtrees,
// and nodes with YANG status deprecated or obsolete.
// goyang propagates config:false from parent to children during Process(), so a
// single check at each level is sufficient to prune entire read-only subtrees.
func isNonConfigNode(e *yang.Entry) bool {
	return e.RPC != nil ||
		e.Kind == yang.NotificationEntry ||
		e.Kind == yang.AnyDataEntry ||
		e.Kind == yang.AnyXMLEntry ||
		e.Config == yang.TSFalse ||
		isDeprecatedOrObsolete(e)
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

// entryToNode converts a goyang Entry to a schema Node (no children populated).
func entryToNode(e *yang.Entry, path string) *Node {
	n := &Node{
		Path:        path,
		Name:        e.Name,
		Kind:        entryKind(e),
		Description: e.Description,
		Config:      e.Config != yang.TSFalse,
		Mandatory:   e.Mandatory == yang.TSTrue,
		When:        extractWhen(e),
	}

	if def, ok := e.SingleDefaultValue(); ok {
		n.Default = def
	}

	if strings.Contains(e.Key, " ") || e.Key != "" {
		n.Keys = strings.Fields(e.Key)
	}

	if e.IsLeaf() || e.IsLeafList() {
		n.Type = yangTypeInfo(e)
	}

	return n
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
				name := v.Name
				if root := yang.RootNode(v); root != nil {
					modName := root.Name
					if root.Kind() == "submodule" && root.BelongsTo != nil {
						modName = root.BelongsTo.Name
					}
					name = modName + ":" + v.Name
				}
				info.Identities = append(info.Identities, name)
			}
			sort.Strings(info.Identities)
		}
	case yang.Yleafref:
		info.Leafref = t.Path
	}

	if len(t.Pattern) > 0 {
		info.Pattern = strings.Join(t.Pattern, "|")
	}

	if len(t.Range) > 0 {
		info.Range = t.Range.String()
	}

	return info
}

// splitPrefix splits "module:name" into ("module", "name").
// If there is no prefix, it returns ("", name).
func splitPrefix(s string) (prefix, name string) {
	if i := strings.Index(s, ":"); i >= 0 {
		return s[:i], s[i+1:]
	}
	return "", s
}

// sortedEntries returns the entries in a Dir map sorted by name.
func sortedEntries(dir map[string]*yang.Entry) []*yang.Entry {
	keys := make([]string, 0, len(dir))
	for k := range dir {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	result := make([]*yang.Entry, 0, len(keys))
	for _, k := range keys {
		result = append(result, dir[k])
	}
	return result
}
