package schema

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Manager answers schema queries from a compiled tree.
type Manager struct {
	t        *tree
	features map[string]bool // "module:feature" the device enables; nil gates nothing
}

// tree is the schema of a module set, reduced to what the queries need.
type tree struct {
	Modules map[string]*module `json:"modules"`
	// Identities maps an identity name, without module, to the names
	// derived from it.
	Identities map[string][]string `json:"identities"`
	// IdentityDesc maps an identity, "module:name", to its description.
	IdentityDesc map[string]string `json:"identity-desc,omitempty"`
}

type module struct {
	Dir []*entry `json:"dir,omitempty"`
}

// entry is one schema node.  Dir is sorted by name.
type entry struct {
	Name        string    `json:"name"`
	Kind        string    `json:"kind"`
	Module      string    `json:"module,omitempty"` // instantiating module
	Description string    `json:"description,omitempty"`
	Keys        []string  `json:"keys,omitempty"`
	ReadOnly    bool      `json:"read-only,omitempty"` // config false
	NoData      bool      `json:"no-data,omitempty"`   // rpc, notification, anydata, anyxml, deprecated or obsolete
	Mandatory   bool      `json:"mandatory,omitempty"`
	Default     *string   `json:"default,omitempty"`
	Type        *TypeInfo `json:"type,omitempty"`
	When        string    `json:"when,omitempty"`
	Presence    string    `json:"presence,omitempty"`
	// IfFeature holds the if-feature expressions, features as "module:feature"
	IfFeature []string `json:"if-feature,omitempty"`
	// EnumDesc maps an enum value to its description
	EnumDesc map[string]string `json:"enum-desc,omitempty"`
	Dir      []*entry          `json:"dir,omitempty"`
}

func (e *entry) IsChoice() bool    { return e.Kind == "choice" }
func (e *entry) IsCase() bool      { return e.Kind == "case" }
func (e *entry) IsList() bool      { return e.Kind == "list" }
func (e *entry) IsContainer() bool { return e.Kind == "container" }

// Read loads a schema written by Write.
func Read(path string) (*Manager, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	t := &tree{}
	if err := json.Unmarshal(data, t); err != nil {
		return nil, fmt.Errorf("schema: parse %s: %w", path, err)
	}
	return &Manager{t: t}, nil
}

// Write saves the schema to path for Read.
func (m *Manager) Write(path string) error {
	data, err := json.Marshal(m.t)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// WithFeatures returns a Manager for the same schema that hides the nodes
// behind features not in set, "module:feature".
func (m *Manager) WithFeatures(set map[string]bool) *Manager {
	return &Manager{t: m.t, features: set}
}

// enabled reports whether every if-feature on the entry holds.
func (m *Manager) enabled(e *entry) bool {
	if m.features == nil {
		return true
	}
	on := func(f string) bool { return m.features[f] }
	for _, expr := range e.IfFeature {
		if !ifFeatureExpr(expr, on) {
			return false
		}
	}
	return true
}

// ifFeatureExpr evaluates a YANG 1.1 if-feature expression, RFC 7950
// section 7.20.2: "not" binds tightest, then "and", then "or".
func ifFeatureExpr(expr string, on func(string) bool) bool {
	tokens := ifFeatureTokens(expr)
	pos := 0
	peek := func() string {
		if pos < len(tokens) {
			return tokens[pos]
		}
		return ""
	}
	var orExpr func() bool
	atom := func() bool {
		switch t := peek(); t {
		case "(":
			pos++
			v := orExpr()
			if peek() == ")" {
				pos++
			}
			return v
		case "not":
			pos++
			return !atomNot(peek, &pos, on, orExpr)
		case "":
			return false
		default:
			pos++
			return on(t)
		}
	}
	andExpr := func() bool {
		v := atom()
		for peek() == "and" {
			pos++
			v = atom() && v
		}
		return v
	}
	orExpr = func() bool {
		v := andExpr()
		for peek() == "or" {
			pos++
			v = andExpr() || v
		}
		return v
	}
	return orExpr()
}

// atomNot evaluates the operand of a "not": a feature, or a group.
func atomNot(peek func() string, pos *int, on func(string) bool, group func() bool) bool {
	t := peek()
	if t == "(" {
		*pos++
		v := group()
		if peek() == ")" {
			*pos++
		}
		return v
	}
	*pos++
	return on(t)
}

func ifFeatureTokens(expr string) []string {
	expr = strings.ReplaceAll(strings.ReplaceAll(expr, "(", " ( "), ")", " ) ")
	return strings.Fields(expr)
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
	return m.dirToNodes(e.Dir, path), nil
}

// ChildrenAll is like Children but includes config:false nodes, making it
// suitable for the operational/status tree where state data reuses read-write
// schema nodes. Only RPCs, notifications, anydata/anyxml, and deprecated or
// obsolete nodes are excluded.
func (m *Manager) ChildrenAll(path string) ([]*Node, error) {
	if path == "" || path == "/" {
		return m.topLevelNodesAll(), nil
	}
	e, err := m.entryAt(path)
	if err != nil {
		return nil, err
	}
	if e.Dir == nil {
		return nil, nil
	}
	return m.dirToNodesAll(e.Dir, path), nil
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
// Identities of that name are searched for across all loaded modules.
func (m *Manager) IdentitiesOf(baseName string) []string {
	_, localName := splitPrefix(baseName)
	return m.t.Identities[localName]
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
	if e.Default == nil {
		return "", false
	}
	return *e.Default, true
}

// ModuleName returns the module name for the schema node at path.
func (m *Manager) ModuleName(path string) (string, error) {
	e, err := m.entryAt(path)
	if err != nil {
		return "", err
	}
	if e.Module == "" {
		return "", fmt.Errorf("schema: no module for %s", path)
	}
	return e.Module, nil
}

// ModuleQualifiedName returns "module:name" for the schema node at path.
// This is the JSON object key used when PUT/PATCHing that node directly via
// RESTCONF (RFC 7951 §4 — namespace-qualified name at module boundaries).
func (m *Manager) ModuleQualifiedName(path string) (string, error) {
	e, err := m.entryAt(path)
	if err != nil {
		return "", err
	}
	if e.Module == "" {
		return "", fmt.Errorf("schema: no module for %s", path)
	}
	return e.Module + ":" + e.Name, nil
}

// internalOperationalTopNodes lists bare node names that must not appear at
// the top level of the operational/status tree.  These are all transport- or
// server-monitoring nodes of no interest to end users.  They are already
// absent from the configure tree because they are config:false; this list
// applies the same exclusion to topLevelNodesAll.
var internalOperationalTopNodes = map[string]bool{
	"netconf":              true, // NETCONF session/lock monitoring
	"netconf-state":        true, // ietf-netconf-monitoring (RFC 6022)
	"notification":         true, // nc-notifications stream container
	"restconf-state":       true, // ietf-restconf-monitoring (RFC 8527)
	"supported-algorithms": true, // SSH/TLS algorithm capability advertisement
	"system-capabilities":  true, // ietf-system-capabilities (RFC 9196)
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
// across all loaded modules. Internal modules and config:false top-level
// nodes are excluded.
func (m *Manager) topLevelNodes() []*Node {
	var nodes []*Node
	for key, mod := range m.t.Modules {
		if internalModules[key] {
			continue
		}
		for _, child := range mod.Dir {
			if isNonConfigNode(child) || !m.enabled(child) {
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

// entryAt resolves a RESTCONF-style path to its schema entry.
// Path format: /module:top-node/child[key='val']/grandchild
// Key predicates ([key='val']) are stripped — they carry instance identity,
// not schema identity. choice/case nodes are skipped during traversal.
func (m *Manager) entryAt(path string) (*entry, error) {
	path = strings.TrimPrefix(path, "/")
	parts := strings.SplitN(path, "/", 2)

	head := stripPredicate(parts[0])
	moduleName, nodeName := splitPrefix(head)

	if moduleName == "" {
		return nil, fmt.Errorf("schema: path must start with module prefix: %s", path)
	}

	mod, ok := m.t.Modules[moduleName]
	if !ok {
		return nil, fmt.Errorf("schema: module not found: %s", moduleName)
	}

	e := findInDir(mod.Dir, nodeName)
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

// findInDir looks up name in a dir, transparently descending into
// choice/case nodes which are not part of the RESTCONF path.
func findInDir(dir []*entry, name string) *entry {
	// Direct match — but if it is a choice/case, descend into it because YANG
	// commonly names a case identically to the leaf it contains (e.g. the
	// ietf-system timezone-name case wrapping the timezone-name leaf).
	i := sort.Search(len(dir), func(i int) bool { return dir[i].Name >= name })
	if i < len(dir) && dir[i].Name == name {
		e := dir[i]
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
func isContainerList(e *entry) bool {
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
func listChildOf(e *entry) *entry {
	for _, child := range e.Dir {
		if child.IsList() && !isNonConfigNode(child) {
			return child
		}
	}
	return nil
}

// listChildOfAll is like listChildOf but uses the data-node filter (includes
// config:false lists), used by dirToNodesAll / the operational status tree.
func listChildOfAll(e *entry) *entry {
	for _, child := range e.Dir {
		if child.IsList() && !isNonDataNode(child) {
			return child
		}
	}
	return nil
}

// topLevelNodesAll is like topLevelNodes but includes config:false top-level
// nodes, for use by the operational status tree.
func (m *Manager) topLevelNodesAll() []*Node {
	var nodes []*Node
	for key, mod := range m.t.Modules {
		if internalModules[key] {
			continue
		}
		for _, child := range mod.Dir {
			if isNonDataNode(child) || !m.enabled(child) {
				continue
			}
			if internalOperationalTopNodes[child.Name] {
				continue
			}
			nodePath := "/" + key + ":" + child.Name
			if isContainerList(child) {
				if lc := listChildOfAll(child); lc != nil {
					nodes = append(nodes, entryToNode(lc, nodePath+"/"+nodeSegment(lc, key)))
				}
				continue
			}
			nodes = append(nodes, entryToNode(child, nodePath))
		}
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })
	return nodes
}

// dirToNodesAll is like dirToNodes but uses isNonDataNode, including
// config:false leaves and containers in the result.
func (m *Manager) dirToNodesAll(dir []*entry, parentPath string) []*Node {
	parentMod := extractModuleFromPath(parentPath)
	var nodes []*Node
	for _, e := range dir {
		if !m.enabled(e) {
			continue
		}
		if e.IsChoice() || e.IsCase() {
			caseWhen := e.When
			for _, child := range m.dirToNodesAll(e.Dir, parentPath) {
				if caseWhen != "" && child.When == "" {
					child.When = caseWhen
				}
				nodes = append(nodes, child)
			}
			continue
		}
		if isNonDataNode(e) {
			continue
		}
		nodePath := parentPath + "/" + nodeSegment(e, parentMod)
		if isContainerList(e) {
			if lc := listChildOfAll(e); lc != nil {
				nodes = append(nodes, entryToNode(lc, nodePath+"/"+nodeSegment(lc, parentMod)))
			}
			continue
		}
		nodes = append(nodes, entryToNode(e, nodePath))
	}
	return nodes
}

// dirToNodes converts a dir to a slice of Nodes, in the same order.
// choice/case children are inlined (their contents promoted to this level).
// Collapsible container-list wrappers are transparent: the list child is surfaced
// directly under the parent with the full (un-collapsed) RESTCONF path.
// RPC, notification, anydata, anyxml and config:false nodes are excluded.
//
// RESTCONF requires module qualification ("module:name") whenever a node's
// module differs from its parent's — the common case being augmented nodes.
// e.g. infix-lldp augments ieee802-dot1ab-lldp:lldp, so the path is
// /ieee802-dot1ab-lldp:lldp/infix-lldp:enabled, not /…/enabled.
func (m *Manager) dirToNodes(dir []*entry, parentPath string) []*Node {
	parentMod := extractModuleFromPath(parentPath)
	var nodes []*Node
	for _, e := range dir {
		if !m.enabled(e) {
			continue
		}
		if e.IsChoice() || e.IsCase() {
			// Extract when from the choice/case before inlining its children.
			// Cases contributed by augments carry the augment's when (e.g.
			// lag-port only for ethernetCsmacd, bridge-port only for bridge).
			// Propagate that constraint to each promoted child that has no when of
			// its own; if the child already has one, leave it alone.
			caseWhen := e.When
			for _, child := range m.dirToNodes(e.Dir, parentPath) {
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
func nodeSegment(e *entry, parentMod string) string {
	if e.Module == "" || e.Module == parentMod {
		return e.Name
	}
	return e.Module + ":" + e.Name
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
// config:false is inherited from parent to children, so a single check at
// each level is sufficient to prune entire read-only subtrees.
func isNonConfigNode(e *entry) bool {
	return e.NoData || e.ReadOnly
}

// isNonDataNode is like isNonConfigNode but keeps config:false nodes.
// Used by ChildrenAll / the operational status tree which shows all data —
// operational state reuses read-write schema nodes alongside state-only leaves.
func isNonDataNode(e *entry) bool {
	return e.NoData
}

// entryToNode converts an entry to a schema Node (no children populated).
func entryToNode(e *entry, path string) *Node {
	n := &Node{
		Path:        path,
		Name:        e.Name,
		Kind:        e.Kind,
		Description: e.Description,
		Keys:        e.Keys,
		Config:      !e.ReadOnly,
		Mandatory:   e.Mandatory,
		Type:        e.Type,
		When:        e.When,
		Presence:    e.Presence,
	}
	if e.Default != nil {
		n.Default = *e.Default
	}
	return n
}

// splitPrefix splits "module:name" into ("module", "name").
// If there is no prefix, it returns ("", name).
func splitPrefix(s string) (prefix, name string) {
	if i := strings.Index(s, ":"); i >= 0 {
		return s[:i], s[i+1:]
	}
	return "", s
}
