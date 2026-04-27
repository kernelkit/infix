// SPDX-License-Identifier: MIT

package handlers

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kernelkit/webui/internal/restconf"
	"github.com/kernelkit/webui/internal/schema"
)

// SchemaHandler serves YANG schema queries as JSON (used by the tree UI and
// for direct API access / testing).
type SchemaHandler struct {
	Cache *schema.Cache
}

// Schema serves GET /api/schema?path=<restconf-path>
// Returns a single Node (without children) as JSON.
func (h *SchemaHandler) Schema(w http.ResponseWriter, r *http.Request) {
	mgr := h.Cache.Manager()
	if mgr == nil {
		http.Error(w, "schema not yet loaded", http.StatusServiceUnavailable)
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}

	node, err := mgr.NodeAt(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(node)
}

// Children serves GET /api/schema/children?path=<restconf-path>
// Returns a JSON array of direct child Nodes.
func (h *SchemaHandler) Children(w http.ResponseWriter, r *http.Request) {
	mgr := h.Cache.Manager()
	if mgr == nil {
		http.Error(w, "schema not yet loaded", http.StatusServiceUnavailable)
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}

	nodes, err := mgr.Children(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(nodes)
}

// ─── Tree UI handler ─────────────────────────────────────────────────────────

// TreeHandler serves the YANG tree navigation UI pages and HTMX fragments.
type TreeHandler struct {
	Cache    *schema.Cache
	RC       restconf.Fetcher
	PageTmpl *template.Template
	FragTmpl *template.Template
	ReadOnly bool // true for the status/operational tree; suppresses all writes
}

// treeNodeData wraps a schema.Node with live presence-state from the datastore.
// Exists is only meaningful when Node.Presence != "".
// TreeBase is the URL prefix for tree HTMX requests ("/configure/tree" or "/status/tree").
type treeNodeData struct {
	*schema.Node
	Exists   bool
	TreeBase string
}

type yangTreePageData struct {
	PageData
	Nodes       []*treeNodeData
	Loading     bool
	InitialPath string // non-empty: auto-load this node in the right pane on page load
	ReadOnly    bool   // true for the operational status tree
	TreeBase    string // URL prefix: "/configure/tree" or "/status/tree"
}

// nodeDetailData is the template data for the yang-node-detail fragment.
type nodeDetailData struct {
	*schema.Node
	CurrentValue  string
	UsingDefault  bool // true when CurrentValue is the YANG default, not an explicit candidate value
	IsBinary      bool // true when the leaf is binary type and CurrentValue holds decoded text
	LeafrefValues []string
	SavedOK       bool
	Error         string
	ReadOnly      bool
}

// leafGroupData is the template data for yang-leaf-group: a container or
// list-instance rendered as an auto-generated "level page".  It shows direct
// leaf children as an inline editable form, and structural children (sub-containers
// and lists) as clickable navigation items below.
type leafGroupData struct {
	Path             string
	ParentPath       string           // set when rendered as an inline sub-container
	Name             string           // display name of the parent node
	Kind             string           // "container" or "list-instance"
	Presence         string           // non-empty for presence containers (YANG presence stmt)
	Exists           bool             // only meaningful when Presence != ""; true = present in datastore
	Leaves           []*leafGroupItem
	InlineLists      []*listTableData  // simple sub-lists shown inline as tables
	InlineContainers []*leafGroupData  // flat sub-containers (all-leaf) shown inline
	SubNodes         []*schema.Node   // complex containers/lists → navigation buttons
	SavedOK          bool
	Error            string
	ReadOnly         bool
}

// listTableColumn is a schema Node with an optional display-name override,
// used so column headers can differ from the YANG leaf name (e.g. "hidden-*"
// presence leaves are shown without the "hidden-" prefix in the heading).
type listTableColumn struct {
	*schema.Node
	DisplayName string
}

// listTableData is the template data for yang-list-table.
type listTableData struct {
	Path        string
	ParentPath  string // set when rendered inline inside a container leaf-group
	Name        string
	Keys        []string
	Columns     []*listTableColumn // display columns (binary excluded to keep table readable)
	FormColumns []*listTableColumn // all leaf columns including binary, used by the add-row form
	Rows        []listTableRow
	Complex     bool // has nested containers/lists; rows navigate to a full detail page
	SavedOK     bool
	Error       string
	ReadOnly    bool
}

// listTableRow holds one instance's display path and column values.
type listTableRow struct {
	InstancePath string
	InstanceName string // key value(s) for display
	Values       map[string]string
}

// listAddData is the template data for yang-list-add: the add-row form.
type listAddData struct {
	Path       string
	ParentPath string // set when opened from an inline list
	Name       string
	Keys       []string
	Columns    []*schema.Node
	Error      string
}

// leafGroupItem holds a single leaf's schema node and its current candidate value.
type leafGroupItem struct {
	*schema.Node
	CurrentValue  string
	UsingDefault  bool
	IsBinary      bool   // leaf has binary type
	HasBinary     bool   // a value is present (even if non-decodable to text, e.g. DER keys)
	RawBase64     string // raw RESTCONF base64 value, populated when HasBinary && !CurrentValue
	LeafrefValues []string
}

// Overview serves GET /configure/tree (or /status/tree for ReadOnly mode).
// When path is set the right pane auto-loads the node on page load.
func (h *TreeHandler) Overview(w http.ResponseWriter, r *http.Request) {
	activePage := "configure-tree"
	title := "Advanced Configuration"
	if h.ReadOnly {
		activePage = "status-tree"
		title = "Advanced Status"
	}
	base := h.treeBase()
	data := yangTreePageData{
		PageData:    newPageData(r, activePage, title),
		InitialPath: r.URL.Query().Get("path"),
		ReadOnly:    h.ReadOnly,
		TreeBase:    base,
	}

	mgr := h.Cache.Manager()
	if mgr == nil {
		data.Loading = true
	} else {
		nodes, err := h.childrenFunc(mgr)("/")
		if err == nil {
			treeNodes := make([]*treeNodeData, len(nodes))
			for i, n := range nodes {
				treeNodes[i] = &treeNodeData{Node: n, Exists: true, TreeBase: base}
			}
			data.Nodes = treeNodes
		}
	}

	if r.Header.Get("HX-Request") == "true" {
		h.PageTmpl.ExecuteTemplate(w, "content", data)
	} else {
		h.PageTmpl.ExecuteTemplate(w, "yang-tree.html", data)
	}
}

// TreeChildren serves GET /configure/tree/children?path=...
// For plain list paths (no key predicate) it returns the actual instances from
// the candidate datastore. For everything else it returns schema children.
func (h *TreeHandler) TreeChildren(w http.ResponseWriter, r *http.Request) {
	mgr := h.Cache.Manager()
	if mgr == nil {
		http.Error(w, "schema not yet loaded", http.StatusServiceUnavailable)
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}

	// For a plain list path (no key predicate in the *last* segment), show data
	// instances rather than the schema template.  Check only the last segment so
	// that paths like /…/interface=br0/ietf-ip:ipv4/address are not excluded.
	lastSeg := path
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		lastSeg = path[i+1:]
	}
	base := h.treeBase()
	if !strings.ContainsAny(lastSeg, "[=") {
		if node, err := mgr.NodeAt(path); err == nil && node.Kind == "list" {
			instances := h.fetchListInstances(r, path, node)
			treeNodes := make([]*treeNodeData, len(instances))
			for i, n := range instances {
				treeNodes[i] = &treeNodeData{Node: n, Exists: true, TreeBase: base}
			}
			h.FragTmpl.ExecuteTemplate(w, "yang-tree-nodes", treeNodes)
			return
		}
	}

	childFn := h.childrenFunc(mgr)
	nodes, err := childFn(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// For list instances, fetch live data so when conditions can be evaluated.
	// For bare schema paths (no instance key) values stays nil → conservative.
	var values map[string]string
	if strings.ContainsAny(lastSeg, "[=") {
		values = h.fetchNodeValues(r, path)
	}

	// Filter out nodes that are inlined in the detail pane (simple lists and
	// flat containers) so they don't clutter the tree.  Also hide nodes whose
	// when condition is not satisfied by the current instance data.
	var visible []*schema.Node
	for _, n := range nodes {
		if n.When != "" && !schema.EvaluateWhen(mgr, n.When, values) {
			continue
		}
		if n.Kind == "list" || n.Kind == "container" {
			kids, _ := childFn(n.Path)
			if isSimpleList(kids) {
				continue
			}
		}
		visible = append(visible, n)
	}
	h.FragTmpl.ExecuteTemplate(w, "yang-tree-nodes", h.resolvePresenceState(r, path, visible))
}

// resolvePresenceState wraps schema nodes in treeNodeData.
// Existence is not checked here — presence toggle lives in the right-pane card header
// and is only resolved lazily when a node is selected (see TreeNode/checkPresenceExists).
func (h *TreeHandler) resolvePresenceState(r *http.Request, parentPath string, nodes []*schema.Node) []*treeNodeData {
	base := h.treeBase()
	result := make([]*treeNodeData, len(nodes))
	for i, n := range nodes {
		result[i] = &treeNodeData{Node: n, TreeBase: base}
	}
	return result
}

// treeBase returns the URL prefix for this handler's tree routes.
func (h *TreeHandler) treeBase() string {
	if h.ReadOnly {
		return "/status/tree"
	}
	return "/configure/tree"
}

func (h *TreeHandler) childrenFunc(mgr *schema.Manager) func(string) ([]*schema.Node, error) {
	if h.ReadOnly {
		return mgr.ChildrenAll
	}
	return mgr.Children
}

// fetchData fetches from the candidate datastore in config mode, or from /data
// (operational) in ReadOnly mode. In config mode it falls back to /data on error.
func (h *TreeHandler) fetchData(r *http.Request, path string) ([]byte, error) {
	if h.ReadOnly {
		return h.RC.GetRaw(r.Context(), "/data"+path)
	}
	data, err := h.RC.GetRaw(r.Context(), candidateDS+path)
	if err != nil {
		return h.RC.GetRaw(r.Context(), "/data"+path)
	}
	return data, nil
}

// checkPresenceExists reports whether the YANG presence container at path
// currently exists in the candidate (or running) datastore.
func (h *TreeHandler) checkPresenceExists(r *http.Request, path string) bool {
	_, err := h.fetchData(r, path)
	return err == nil
}

// fetchListInstances queries the candidate (fallback: running) for a list
// node and returns one schema.Node per instance, using the key values to
// build RESTCONF key predicates for the path.
//
// rousette always wraps GET responses in the full module-root hierarchy, so
// we can GET the parent path and then use navigateToNode with the full list
// path to reach the list array directly.
func (h *TreeHandler) fetchListInstances(r *http.Request, path string, listNode *schema.Node) []*schema.Node {
	segs := strings.Split(strings.TrimPrefix(path, "/"), "/")
	parentPath := "/" + strings.Join(segs[:len(segs)-1], "/")
	if len(segs) < 2 {
		parentPath = path
	}

	data, err := h.fetchData(r, parentPath)
	if err != nil {
		log.Printf("yang-tree: list GET %s: %v", parentPath, err)
		return nil
	}

	// Navigate directly to the list array using the full path.
	rawItems := navigateToNode(data, path)
	if rawItems == nil {
		log.Printf("yang-tree: list %q not found in response for %s", listNode.Name, path)
		return nil
	}

	var items []map[string]json.RawMessage
	if err := json.Unmarshal(rawItems, &items); err != nil {
		log.Printf("yang-tree: list items unmarshal %s: %v", path, err)
		return nil
	}

	var nodes []*schema.Node
	for _, item := range items {
		// Build RESTCONF key predicate: "=val" or "=val1,val2".
		pred := buildKeyPredicate(item, listNode.Keys)
		instancePath := path + pred
		// Display name is the key value(s) without the leading "=".
		displayName := pred[1:]

		nodes = append(nodes, &schema.Node{
			Path:   instancePath,
			Name:   displayName,
			Kind:   "list-instance",
			Config: listNode.Config,
			Keys:   listNode.Keys,
		})
	}
	// Natural sort: eth2 before eth10.
	sort.Slice(nodes, func(i, j int) bool {
		return naturalLess(nodes[i].Name, nodes[j].Name)
	})
	return nodes
}

// buildKeyPredicate constructs a RESTCONF list-key predicate from a JSON
// object and a list of key names (RFC 8040 §3.5.3).
// Single key:    "=eth0"
// Composite key: "=default,ipv4"
func buildKeyPredicate(item map[string]json.RawMessage, keys []string) string {
	var vals []string
	for _, key := range keys {
		raw, ok := item[key]
		if !ok {
			// Try with module prefix (RESTCONF may qualify key names).
			for k, v := range item {
				if k == key || strings.HasSuffix(k, ":"+key) {
					raw = v
					ok = true
					break
				}
			}
		}
		s := "?"
		if ok {
			if json.Unmarshal(raw, &s) != nil {
				s = strings.Trim(string(raw), `"`)
			}
			s = url.PathEscape(s)
		}
		vals = append(vals, s)
	}
	if len(vals) == 0 {
		return "=?"
	}
	return "=" + strings.Join(vals, ",")
}

// TreeNode serves GET /configure/tree/node?path=...
// For containers/list-instances whose children are all leaves it renders an
// inline leaf-group form.  For individual leaves it renders the leaf detail form.
func (h *TreeHandler) TreeNode(w http.ResponseWriter, r *http.Request) {
	mgr := h.Cache.Manager()
	if mgr == nil {
		http.Error(w, "schema not yet loaded", http.StatusServiceUnavailable)
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}
	// parent= is set when navigating from an inline list row so that saving
	// re-renders the parent container page instead of the current one.
	parentPath := r.URL.Query().Get("parent")

	node, err := mgr.NodeAt(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Containers and list-instances always render as a level page — direct leaves
	// as an editable form, structural children as navigation items below.
	// NodeAt strips key predicates so a list instance always comes back as "list";
	// detect an instance by checking ONLY the last path segment for a key predicate,
	// so that nested paths like /…/interface=br0/ietf-ip:ipv4/address are not
	// mistakenly treated as instances due to a predicate in a parent segment.
	lastSeg := path
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		lastSeg = path[i+1:]
	}
	isInstance := node.Kind == "list" && strings.ContainsAny(lastSeg, "[=")

	// Bare list node (no key predicate): show a table of all instances when the
	// list is "simple" (only leaf children — no nested containers or lists).
	if node.Kind == "list" && !isInstance {
		if td := h.buildListTable(r, mgr, path, node); td != nil {
			if parentPath != "" {
				td.ParentPath = parentPath
			}
			h.FragTmpl.ExecuteTemplate(w, "yang-list-table", td)
			return
		}
	}

	if node.Kind == "container" || node.Kind == "list-instance" || isInstance {
		dispKind := node.Kind
		if isInstance {
			dispKind = "list-instance"
		}
		gd := h.buildLeafGroup(r, mgr, path, node.Name, dispKind)
		if gd == nil && node.Presence == "" {
			// Non-presence empty container — fall through to node detail view.
		} else {
			if gd == nil {
				gd = &leafGroupData{Path: path, Name: node.Name, Kind: dispKind}
			}
			if parentPath != "" {
				gd.ParentPath = parentPath
			}
			if node.Presence != "" {
				gd.Presence = node.Presence
				gd.Exists = h.checkPresenceExists(r, path)
			}
			gd.ReadOnly = h.ReadOnly
			h.FragTmpl.ExecuteTemplate(w, "yang-leaf-group", gd)
			return
		}
	}

	data := &nodeDetailData{Node: node, ReadOnly: h.ReadOnly}
	if node.Kind == "leaf" || node.Kind == "leaf-list" {
		item := &leafGroupItem{Node: node}
		resolveLeafItem(item, h.fetchLeafValue(r, path))
		data.CurrentValue = item.CurrentValue
		data.UsingDefault = item.UsingDefault
		data.IsBinary = item.IsBinary
		data.LeafrefValues = h.fetchLeafrefValues(r, mgr, node)
	}

	h.FragTmpl.ExecuteTemplate(w, "yang-node-detail", data)
}

// buildLeafGroup builds a level-page for any container or list-instance.
// It fetches the whole node once (one HTTP round-trip) and extracts leaf
// values from the response, rather than fetching each leaf individually.
// Structural children (sub-containers, lists) are listed as navigation items.
// Returns nil only when the schema has no children (empty container).
func (h *TreeHandler) buildLeafGroup(r *http.Request, mgr *schema.Manager, path, name, kind string) *leafGroupData {
	childFn := h.childrenFunc(mgr)
	children, err := childFn(path)
	if err != nil || len(children) == 0 {
		return nil
	}
	gd := &leafGroupData{Path: path, Name: name, Kind: kind, ReadOnly: h.ReadOnly}
	values := h.fetchNodeValues(r, path)

	// For list instances: enrich the card heading ("interface wan") and build a
	// key-name set so key leaves can be sorted first in the form.
	var keySet map[string]bool
	if kind == "list-instance" {
		schemaPath := stripKeyPredicate(path)
		if listNode, lerr := mgr.NodeAt(schemaPath); lerr == nil && len(listNode.Keys) > 0 {
			keySet = make(map[string]bool, len(listNode.Keys))
			for _, k := range listNode.Keys {
				keySet[k] = true
			}
			_, lastSeg := splitLastSegment(path)
			if i := strings.IndexByte(lastSeg, '='); i >= 0 {
				if keyVal, uerr := url.PathUnescape(lastSeg[i+1:]); uerr == nil && keyVal != "" {
					gd.Name = name + " " + keyVal
				}
			}
		}
	}

	// mgr.Children always returns schema paths (no key predicates).  When path
	// is a list instance (e.g. /…/user=admin), we must rebuild each child's
	// path using the instance path as prefix so that RESTCONF requests and form
	// actions address the correct instance, not a bare list path.
	schemaBase := stripKeyPredicate(path) // path with last key predicate removed

	for _, c := range children {
		// Skip nodes whose when condition is false for the current data values.
		if c.When != "" && !schema.EvaluateWhen(mgr, c.When, values) {
			continue
		}

		childPath := c.Path
		if schemaBase != path && strings.HasPrefix(c.Path, schemaBase+"/") {
			childPath = path + c.Path[len(schemaBase):]
		}

		switch c.Kind {
		case "leaf", "leaf-list":
			item := &leafGroupItem{Node: c}
			val := ""
			if values != nil {
				val = values[c.Name]
			}
			resolveLeafItem(item, val)
			item.LeafrefValues = h.fetchLeafrefValues(r, mgr, c)
			gd.Leaves = append(gd.Leaves, item)
		case "list":
			if td := h.buildListTable(r, mgr, childPath, c); td != nil {
				td.ParentPath = path
				gd.InlineLists = append(gd.InlineLists, td)
			}
		case "container":
			kids2, err2 := childFn(c.Path)
			if err2 == nil && isSimpleList(kids2) {
				sub := h.buildLeafGroup(r, mgr, childPath, c.Name, "container")
				if sub != nil {
					sub.ParentPath = path
					gd.InlineContainers = append(gd.InlineContainers, sub)
					break
				}
			}
			subNode := c
			if childPath != c.Path {
				cn := *c
				cn.Path = childPath
				subNode = &cn
			}
			gd.SubNodes = append(gd.SubNodes, subNode)
		default:
			subNode := c
			if childPath != c.Path {
				cn := *c
				cn.Path = childPath
				subNode = &cn
			}
			gd.SubNodes = append(gd.SubNodes, subNode)
		}
	}

	// Sort key leaves to the top of the form (schema order preserved within groups).
	if len(keySet) > 0 {
		sort.SliceStable(gd.Leaves, func(i, j int) bool {
			return keySet[gd.Leaves[i].Name] && !keySet[gd.Leaves[j].Name]
		})
	}

	return gd
}

// isSimpleList returns true when every direct child of the list is a leaf or
// leaf-list — no nested containers or sub-lists.  Used to decide whether to
// render a bare list node as a data table rather than a tree expansion.
func isSimpleList(children []*schema.Node) bool {
	if len(children) == 0 {
		return false
	}
	for _, c := range children {
		if c.Kind != "leaf" && c.Kind != "leaf-list" {
			return false
		}
	}
	return true
}

// buildListTable builds a listTableData for a bare (no-predicate) list node.
// Simple lists (all-leaf children) show all leaf columns.
// Complex lists (with nested containers/lists) show only leaf columns and set
// Complex=true so the template renders rows as click-through navigation items
// instead of an inline add form.
// Returns nil only if children cannot be resolved.
func (h *TreeHandler) buildListTable(r *http.Request, mgr *schema.Manager, path string, listNode *schema.Node) *listTableData {
	children, err := h.childrenFunc(mgr)(path)
	if err != nil {
		return nil
	}

	simple := isSimpleList(children)

	// Keys first, then up to 4 non-key LEAF columns so the table stays readable.
	// For complex lists we skip containers and sub-lists from the column set.
	// FormColumns tracks all leaf columns (including binary) for use by the add-row
	// form — binary is excluded from Columns only to keep the table display readable.
	keySet := make(map[string]bool, len(listNode.Keys))
	for _, k := range listNode.Keys {
		keySet[k] = true
	}
	var keyNodes, otherNodes, formNodes []*listTableColumn
	for _, c := range children {
		if c.Kind != "leaf" && c.Kind != "leaf-list" {
			continue // skip containers/sub-lists from column display
		}
		col := &listTableColumn{Node: c, DisplayName: c.Name}
		// Strip "hidden-" prefix from the display name (e.g. "hidden-private-key"
		// → "private-key") so the heading names the data concept, not the YANG case.
		if c.Type != nil && c.Type.Kind == "empty" && strings.HasPrefix(c.Name, "hidden-") {
			col.DisplayName = strings.TrimPrefix(c.Name, "hidden-")
		}
		formNodes = append(formNodes, col)
		if keySet[c.Name] {
			keyNodes = append(keyNodes, col)
		} else if c.Type == nil || (c.Type.Kind != "binary") {
			// Skip binary columns — unreadable in a table and blow out column widths.
			otherNodes = append(otherNodes, col)
		}
	}
	const maxOther = 4
	if len(otherNodes) > maxOther {
		otherNodes = otherNodes[:maxOther]
	}
	columns := append(keyNodes, otherNodes...)
	if len(columns) == 0 {
		return nil
	}

	instances := h.fetchListInstances(r, path, listNode)
	td := &listTableData{
		Path:        path,
		Name:        listNode.Name,
		Keys:        listNode.Keys,
		Columns:     columns,
		FormColumns: formNodes,
		Complex:     !simple,
		ReadOnly:    h.ReadOnly,
	}
	for _, inst := range instances {
		rawVals := h.fetchNodeValues(r, inst.Path)
		display := make(map[string]string, len(rawVals))
		for _, col := range columns {
			v := rawVals[col.Name]
			switch {
			case col.Type != nil && col.Type.Kind == "identityref":
				// Strip module prefix for display.
				if i := strings.LastIndexByte(v, ':'); i >= 0 {
					v = v[i+1:]
				}
			case col.Type != nil && col.Type.Kind == "empty":
				// YANG empty type: "true" means present (from [null] in JSON).
				// Hidden-* leaves indicate the value is stored but not exported.
				if v == "true" {
					if strings.HasPrefix(col.Name, "hidden-") {
						v = "Hidden"
					} else {
						v = "✓"
					}
				} else {
					v = ""
				}
			}
			display[col.Name] = v
		}
		td.Rows = append(td.Rows, listTableRow{
			InstancePath: inst.Path,
			InstanceName: inst.Name,
			Values:       display,
		})
	}
	return td
}

// stripKeyPredicate removes the key predicate from the last segment of a path,
// e.g. "/interfaces/interface=eth0" → "/interfaces/interface".
// splitLastSegment splits "/a/b/c=d" into ("/a/b", "c=d").
func splitLastSegment(path string) (parent, last string) {
	i := strings.LastIndexByte(path, '/')
	if i < 0 {
		return "", path
	}
	return path[:i], path[i+1:]
}

// buildRootedPatch returns the top-level module container path and a body
// suitable for PATCH /candidateDS+topPath that nests entry inside the full
// schema path.  Patching at the module root (like configure-system.go does
// for hostname/NTP) gives libyang the complete ancestor-key context it needs
// to validate nested list entries — patching at a sub-path like user=admin
// leaves libyang without parent-list key context.
func buildRootedPatch(mgr *schema.Manager, listPath string, listNode *schema.Node, entry map[string]any) (topPath string, body map[string]any) {
	segs := strings.Split(strings.TrimPrefix(listPath, "/"), "/")
	if len(segs) == 0 {
		qn, _ := mgr.ModuleQualifiedName(listPath)
		return listPath, map[string]any{qn: []map[string]any{entry}}
	}

	topSeg := segs[0] // e.g. "ietf-system:system"
	topModName, _ := splitModPrefix(topSeg)

	// qualName returns the bare name if the node is in the same module as
	// topMod, or "module:name" if it's in a different module.
	qualName := func(nodePath, name string) string {
		mod, err := mgr.ModuleName(nodePath)
		if err == nil && mod != "" && mod != topModName {
			return mod + ":" + name
		}
		return name
	}

	// Start with the innermost: the target list → [entry].
	innerVal := map[string]any{qualName(listPath, listNode.Name): []map[string]any{entry}}

	// Walk upward through intermediate segments, wrapping at each level.
	for i := len(segs) - 2; i >= 1; i-- {
		seg := segs[i]
		segPath := "/" + strings.Join(segs[:i+1], "/")
		eqIdx := strings.IndexByte(seg, '=')
		if eqIdx >= 0 {
			// List instance (e.g. "user=admin"): build a list entry that
			// includes the key value(s) extracted from the path predicate.
			listName := seg[:eqIdx]
			predVals := strings.SplitN(seg[eqIdx+1:], ",", 16)
			schemaPath := stripKeyPredicate(segPath)
			schemaNode, err := mgr.NodeAt(schemaPath)
			listEntry := make(map[string]any)
			if err == nil {
				for j, k := range schemaNode.Keys {
					if j < len(predVals) {
						v, _ := url.PathUnescape(predVals[j])
						listEntry[k] = v
					}
				}
			}
			for k, v := range innerVal {
				listEntry[k] = v
			}
			innerVal = map[string]any{qualName(schemaPath, listName): []map[string]any{listEntry}}
		} else {
			// Container segment: wrap innerVal in the container object.
			innerVal = map[string]any{qualName(segPath, seg): innerVal}
		}
	}

	return "/" + topSeg, map[string]any{topSeg: innerVal}
}

func stripKeyPredicate(path string) string {
	segs := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(segs) == 0 {
		return path
	}
	last := segs[len(segs)-1]
	if i := strings.IndexByte(last, '='); i >= 0 {
		segs[len(segs)-1] = last[:i]
	}
	return "/" + strings.Join(segs, "/")
}

// AddListRowForm serves GET /configure/tree/list-add?path=...&parent=...
// Renders a blank add-row form for the list at path.
// parent is set when the list is embedded inside a container leaf-group.
func (h *TreeHandler) AddListRowForm(w http.ResponseWriter, r *http.Request) {
	mgr := h.Cache.Manager()
	if mgr == nil {
		http.Error(w, "schema not yet loaded", http.StatusServiceUnavailable)
		return
	}
	path := r.URL.Query().Get("path")
	parent := r.URL.Query().Get("parent")
	node, err := mgr.NodeAt(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	children, _ := mgr.Children(path)
	var leafCols []*schema.Node
	for _, c := range children {
		if c.Kind == "leaf" || c.Kind == "leaf-list" {
			leafCols = append(leafCols, c)
		}
	}
	h.FragTmpl.ExecuteTemplate(w, "yang-list-add", &listAddData{
		Path:       path,
		ParentPath: parent,
		Name:       node.Name,
		Keys:       node.Keys,
		Columns:    leafCols,
	})
}

// SaveListRow serves POST /configure/tree/list-row?path=...&parent=...
// Creates a new list instance from posted form values, then re-renders.
// If parent is set, re-renders the parent container leaf-group (inline mode).
func (h *TreeHandler) SaveListRow(w http.ResponseWriter, r *http.Request) {
	mgr := h.Cache.Manager()
	if mgr == nil {
		http.Error(w, "schema not yet loaded", http.StatusServiceUnavailable)
		return
	}
	path := r.URL.Query().Get("path")
	parent := r.URL.Query().Get("parent")
	listNode, err := mgr.NodeAt(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	r.ParseForm()
	children, _ := mgr.Children(path)

	// Build RESTCONF body from all posted leaf values.
	// Use bare field names (no module prefix) inside the entry; cross-module
	// augmented fields still need their module prefix.
	listModName, _ := mgr.ModuleName(path)
	entry := make(map[string]any)
	for _, child := range children {
		if child.Kind != "leaf" && child.Kind != "leaf-list" {
			continue
		}
		raw := r.FormValue(child.Name)
		if raw == "" {
			continue
		}
		fieldModName, _ := mgr.ModuleName(child.Path)
		fieldKey := child.Name
		if fieldModName != listModName {
			fieldKey = fieldModName + ":" + child.Name
		}
		entry[fieldKey] = coerceLeafValue(raw, child)
	}

	// PATCH at the top-level module container (e.g. ietf-system:system) with
	// the full nested structure so libyang has complete ancestor-key context.
	// Patching at a sub-path leaves libyang without parent list-key context
	// and produces "List requires N keys" errors.
	topPath, rootBody := buildRootedPatch(mgr, path, listNode, entry)
	putErr := h.RC.Patch(r.Context(), candidateDS+topPath, rootBody)

	if parent != "" {
		parentNode, pErr := mgr.NodeAt(parent)
		if pErr == nil {
			gd := h.buildLeafGroup(r, mgr, parent, parentNode.Name, parentNode.Kind)
			if gd != nil {
				if putErr != nil {
					gd.Error = putErr.Error()
				} else {
					gd.SavedOK = true
				}
				h.FragTmpl.ExecuteTemplate(w, "yang-leaf-group", gd)
				return
			}
		}
	}

	td := h.buildListTable(r, mgr, path, listNode)
	if td == nil {
		td = &listTableData{Path: path, Name: listNode.Name}
	}
	if putErr != nil {
		td.Error = putErr.Error()
	} else {
		td.SavedOK = true
	}
	h.FragTmpl.ExecuteTemplate(w, "yang-list-table", td)
}

// DeleteListRow serves DELETE /configure/tree/list-row?path=...&parent=...
// Deletes a list instance and re-renders. If parent is set, re-renders the
// parent container leaf-group (inline mode); otherwise re-renders the list table.
func (h *TreeHandler) DeleteListRow(w http.ResponseWriter, r *http.Request) {
	mgr := h.Cache.Manager()
	if mgr == nil {
		http.Error(w, "schema not yet loaded", http.StatusServiceUnavailable)
		return
	}
	path := r.URL.Query().Get("path")
	parent := r.URL.Query().Get("parent")
	listPath := stripKeyPredicate(path)
	listNode, err := mgr.NodeAt(listPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	errMsg := ""
	// Skip direct DELETE when the path contains '@': libyang treats it as a
	// module@revision separator in path predicates and always returns 400.
	useParentPut := strings.ContainsRune(path, '@')
	if !useParentPut {
		if delErr := h.RC.Delete(r.Context(), candidateDS+path); delErr != nil {
			useParentPut = true
		}
	}
	if useParentPut {
		if fbErr := h.deleteViaParentPut(r, mgr, path, listPath, listNode); fbErr != nil {
			errMsg = fbErr.Error()
		}
	}

	// Inline list delete (parent set): the button uses hx-swap="delete" to remove
	// the row from the DOM — no full re-render needed.  Return a minimal signal.
	if parent != "" {
		if errMsg != "" {
			renderSaveError(w, errors.New(errMsg))
		} else {
			renderSaved(w, "Deleted")
		}
		return
	}

	td := h.buildListTable(r, mgr, listPath, listNode)
	if td == nil {
		td = &listTableData{Path: listPath, Name: listNode.Name}
	}
	td.Error = errMsg
	h.FragTmpl.ExecuteTemplate(w, "yang-list-table", td)
}

// deleteViaParentPut removes a list instance without using a DELETE to the
// instance path. It GETs the parent, filters the list, and PUTs the parent
// back — identical in spirit to the curated DeleteKey workaround.
// Used when a direct DELETE fails (e.g. '@' in key value causes libyang to
// treat it as a module@revision separator and return "Syntax error").
func (h *TreeHandler) deleteViaParentPut(r *http.Request, mgr *schema.Manager, instancePath, listPath string, listNode *schema.Node) error {
	// Parent of the list (e.g. user=admin for authorized-key).
	listParentPath, _ := splitLastSegment(listPath)
	if listParentPath == "" {
		return fmt.Errorf("no parent for %s", listPath)
	}

	// GET parent — rousette returns full module-root hierarchy.
	data, err := h.fetchData(r, listParentPath)
	if err != nil {
		return err
	}

	// Navigate to the parent instance.
	parentRaw := navigateToNode(data, listParentPath)
	if parentRaw == nil {
		return nil // already absent
	}
	var parentObj map[string]json.RawMessage
	if err := json.Unmarshal(parentRaw, &parentObj); err != nil {
		return err
	}

	// Find the list array by bare name inside the parent object.
	_, listBareName := splitModPrefix(listPath[strings.LastIndexByte(listPath, '/')+1:])
	var listKey string
	var rawItems json.RawMessage
	for k, v := range parentObj {
		_, local := splitModPrefix(k)
		if local == listBareName {
			listKey, rawItems = k, v
			break
		}
	}
	if rawItems == nil {
		return nil // list absent, nothing to do
	}

	var items []map[string]json.RawMessage
	if err := json.Unmarshal(rawItems, &items); err != nil {
		return err
	}

	// Key values from instance path predicate (after the '=', comma-separated).
	_, instanceSeg := splitLastSegment(instancePath)
	eqIdx := strings.IndexByte(instanceSeg, '=')
	if eqIdx < 0 {
		return fmt.Errorf("no predicate in %s", instancePath)
	}
	delVals := strings.Split(instanceSeg[eqIdx+1:], ",")

	// Filter: keep all entries except the one matching the deletion key.
	var kept []json.RawMessage
	for _, item := range items {
		pred := buildKeyPredicate(item, listNode.Keys)
		itemVals := strings.Split(pred[1:], ",") // strip leading "="
		match := len(delVals) == len(itemVals)
		for i := range delVals {
			if !match {
				break
			}
			d1, _ := url.PathUnescape(delVals[i])
			d2, _ := url.PathUnescape(itemVals[i])
			if d1 != d2 {
				match = false
			}
		}
		if !match {
			b, _ := json.Marshal(item)
			kept = append(kept, b)
		}
	}

	filteredJSON, _ := json.Marshal(kept)
	parentObj[listKey] = json.RawMessage(filteredJSON)

	// Convert parent object back to map[string]any for the PUT body.
	parentAny := make(map[string]any, len(parentObj))
	for k, v := range parentObj {
		var val any
		json.Unmarshal(v, &val) //nolint:errcheck
		parentAny[k] = val
	}

	// Wrap in the module-qualified list-instance envelope expected by RESTCONF.
	_, parentLastSeg := splitLastSegment(listParentPath)
	_, parentBareName := splitModPrefix(stripModPredicate(parentLastSeg))
	parentModName, _ := mgr.ModuleName(listParentPath)
	body := map[string]any{
		parentModName + ":" + parentBareName: []any{parentAny},
	}
	return h.RC.Put(r.Context(), candidateDS+listParentPath, body)
}

// fetchNodeValues fetches a container or list-instance and returns a flat map
// of bare-leaf-name → string for its direct scalar children.
//
// When the path contains characters like '@' that rousette/libyang rejects in
// URL path predicates, a direct GET fails.  We fall back to GETting the parent
// path — rousette always returns the full module-root hierarchy, so
// navigateToNode can still walk down to the target node.
func (h *TreeHandler) fetchNodeValues(r *http.Request, path string) map[string]string {
	// If the last segment contains percent-encoded characters (e.g. %40 for @),
	// Go's HTTP client decodes them before sending and libyang then rejects the
	// path with "Syntax error".  Skip directly to the parent-path fallback.
	_, lastSeg := splitLastSegment(path)

	var data []byte
	var err error
	if !strings.ContainsRune(lastSeg, '%') {
		data, err = h.fetchData(r, path)
	}
	if data == nil {
		parentPath, _ := splitLastSegment(path)
		if parentPath == "" {
			return nil
		}
		data, err = h.fetchData(r, parentPath)
		if err != nil {
			return nil
		}
	}
	raw := navigateToNode(data, path)
	if raw == nil {
		return nil
	}
	return flattenNodeValues(raw)
}

// SaveGroup serves PUT /configure/tree/group?path=...
// Saves every leaf in the group form to the candidate datastore.
// Returns HX-Trigger cfgSaved/cfgError so the form shows inline feedback
// without a full re-render (forms use hx-swap="none").
func (h *TreeHandler) SaveGroup(w http.ResponseWriter, r *http.Request) {
	mgr := h.Cache.Manager()
	if mgr == nil {
		http.Error(w, "schema not yet loaded", http.StatusServiceUnavailable)
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}

	node, err := mgr.NodeAt(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	r.ParseForm()

	children, _ := mgr.Children(path)
	var firstErr string

	for _, child := range children {
		if child.Kind != "leaf" && child.Kind != "leaf-list" {
			continue
		}
		rawValue := r.FormValue(child.Name)
		qualName, qErr := mgr.ModuleQualifiedName(child.Path)
		if qErr != nil {
			qualName = child.Name
		}
		body := map[string]any{qualName: coerceLeafValue(rawValue, child)}
		if putErr := h.RC.Put(r.Context(), candidateDS+child.Path, body); putErr != nil && firstErr == "" {
			firstErr = child.Name + ": " + putErr.Error()
		}
	}

	if firstErr != "" {
		w.Header().Set("HX-Trigger", `{"cfgError":"`+firstErr+`"}`)
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("HX-Trigger", `{"cfgSaved":"Saved `+node.Name+` to candidate"}`)
	w.WriteHeader(http.StatusNoContent)
}

// SaveLeaf serves PUT /configure/tree/node?path=...
// Writes the form value to the candidate datastore and re-renders the detail pane.
func (h *TreeHandler) SaveLeaf(w http.ResponseWriter, r *http.Request) {
	mgr := h.Cache.Manager()
	if mgr == nil {
		http.Error(w, "schema not yet loaded", http.StatusServiceUnavailable)
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}

	node, err := mgr.NodeAt(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	rawValue := r.FormValue("value")
	qualName, err := mgr.ModuleQualifiedName(path)
	if err != nil {
		log.Printf("yang: qualified name for %s: %v", path, err)
		qualName = node.Name
	}

	body := map[string]any{qualName: coerceLeafValue(rawValue, node)}
	data := &nodeDetailData{Node: node}

	if putErr := h.RC.Put(r.Context(), candidateDS+path, body); putErr != nil {
		data.Error = putErr.Error()
		data.CurrentValue = rawValue
	} else {
		data.SavedOK = true
		data.CurrentValue = rawValue
	}

	h.FragTmpl.ExecuteTemplate(w, "yang-node-detail", data)
}

// DeleteLeaf serves DELETE /configure/tree/node?path=...
// Removes the node from the candidate datastore; returns 204.
func (h *TreeHandler) DeleteLeaf(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}

	if err := h.RC.Delete(r.Context(), candidateDS+path); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// fetchLeafrefValues resolves a leafref schema path against the candidate
// (fallback: running) datastore and returns the available key/leaf values.
// Returns nil when the leafref cannot be resolved or the datastore is empty.
func (h *TreeHandler) fetchLeafrefValues(r *http.Request, mgr *schema.Manager, node *schema.Node) []string {
	if node.Type == nil || node.Type.Kind != "leafref" || node.Type.Leafref == "" {
		return nil
	}
	absPath := mgr.ResolveLeafref(node.Type.Leafref, node.Path)
	if absPath == "" {
		return nil
	}

	// Split into parent (the list/container) and target leaf name.
	segs := strings.Split(strings.TrimPrefix(absPath, "/"), "/")
	if len(segs) < 2 {
		return nil
	}
	_, targetLeaf := splitModPrefix(segs[len(segs)-1])
	parentPath := "/" + strings.Join(segs[:len(segs)-1], "/")

	data, err := h.fetchData(r, parentPath)
	if err != nil {
		return nil
	}
	return extractFieldValues(data, targetLeaf)
}

// splitModPrefix splits "module:name" into ("module", "name").
// If there is no prefix it returns ("", name).
func splitModPrefix(s string) (string, string) {
	if i := strings.IndexByte(s, ':'); i >= 0 {
		return s[:i], s[i+1:]
	}
	return "", s
}

// extractFieldValues walks a RESTCONF JSON response and collects every string
// value whose key (stripped of module prefix) matches fieldName.
// Handles both single-object and array envelopes.
func extractFieldValues(data []byte, fieldName string) []string {
	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	var vals []string
	collectField(raw, fieldName, &vals)
	sort.Slice(vals, func(i, j int) bool { return naturalLess(vals[i], vals[j]) })
	return vals
}

func collectField(v interface{}, field string, out *[]string) {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, child := range t {
			_, local := splitModPrefix(k)
			if local == field {
				if s, ok := child.(string); ok {
					*out = append(*out, s)
				}
			} else {
				collectField(child, field, out)
			}
		}
	case []interface{}:
		for _, item := range t {
			collectField(item, field, out)
		}
	}
}

// naturalLess compares two strings with numeric-aware ordering so that
// "eth2" < "eth10" (matching faux_str_numcmp in klish-plugin-sysrepo).
func naturalLess(a, b string) bool {
	for len(a) > 0 && len(b) > 0 {
		ra, rb := rune(a[0]), rune(b[0])
		if unicode.IsDigit(ra) && unicode.IsDigit(rb) {
			// Collect digit runs from both strings.
			na, nb := 0, 0
			ia, ib := 0, 0
			for ia < len(a) && unicode.IsDigit(rune(a[ia])) {
				na = na*10 + int(a[ia]-'0')
				ia++
			}
			for ib < len(b) && unicode.IsDigit(rune(b[ib])) {
				nb = nb*10 + int(b[ib]-'0')
				ib++
			}
			if na != nb {
				return na < nb
			}
			a, b = a[ia:], b[ib:]
			continue
		}
		if ra != rb {
			return ra < rb
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}

// fetchLeafValue reads the current leaf value from the candidate datastore
// (falling back to running) and returns a display string.
// For paths through list instances the server wraps the response in the full
// module hierarchy; navigateToNode is used to reach the value in that case,
// with a fallback to the simpler single-key envelope unwrap.
func (h *TreeHandler) fetchLeafValue(r *http.Request, path string) string {
	var data []byte
	var err error
	_, lastSeg := splitLastSegment(path)
	if !strings.ContainsRune(lastSeg, '%') {
		data, err = h.fetchData(r, path)
	}
	if data == nil {
		parentPath, _ := splitLastSegment(path)
		if parentPath == "" {
			return ""
		}
		data, err = h.fetchData(r, parentPath)
		if err != nil {
			return ""
		}
	}
	if raw := navigateToNode(data, path); raw != nil {
		return extractScalar(raw)
	}
	return extractLeafValue(data)
}

// resolveLeafItem populates CurrentValue, UsingDefault, and IsBinary for a leafGroupItem.
// For boolean leaves with no stored value and no YANG default, absent == false.
// For binary leaves the RESTCONF value is base64; we decode it for display as
// plain text.  If decoding fails or the result is not valid UTF-8 the leaf is
// still marked IsBinary=true but CurrentValue is left empty so the template
// can show a "non-text binary data" placeholder instead.
func resolveLeafItem(item *leafGroupItem, val string) {
	node := item.Node
	if node.Type != nil && node.Type.Kind == "binary" {
		item.IsBinary = true
		if val != "" {
			item.HasBinary = true
			if decoded, err := base64.StdEncoding.DecodeString(val); err == nil && utf8.Valid(decoded) {
				item.CurrentValue = string(decoded)
			} else {
				item.RawBase64 = val // non-decodable (DER); show raw base64 for display
			}
		}
		return
	}
	if val == "" && node.Default != "" {
		item.CurrentValue = node.Default
		item.UsingDefault = true
	} else if val == "" && node.Type != nil && node.Type.Kind == "boolean" {
		item.CurrentValue = "false"
		item.UsingDefault = true
	} else {
		item.CurrentValue = val
	}
}

// TogglePresence serves PUT and DELETE /configure/tree/presence?path=...
// PUT creates the presence container with an empty body; DELETE removes it.
// Returns the updated yang-tree-node HTML fragment for HTMX outerHTML swap.
func (h *TreeHandler) TogglePresence(w http.ResponseWriter, r *http.Request) {
	if h.ReadOnly {
		http.Error(w, "read-only tree", http.StatusMethodNotAllowed)
		return
	}
	mgr := h.Cache.Manager()
	if mgr == nil {
		http.Error(w, "schema not yet loaded", http.StatusServiceUnavailable)
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}

	node, err := mgr.NodeAt(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	var exists bool
	if r.Method == http.MethodDelete {
		if delErr := h.RC.Delete(r.Context(), candidateDS+path); delErr != nil {
			http.Error(w, delErr.Error(), http.StatusBadGateway)
			return
		}
		exists = false
	} else {
		qualName, qErr := mgr.ModuleQualifiedName(path)
		if qErr != nil {
			http.Error(w, qErr.Error(), http.StatusBadRequest)
			return
		}
		if putErr := h.RC.Put(r.Context(), candidateDS+path, map[string]any{qualName: map[string]any{}}); putErr != nil {
			http.Error(w, putErr.Error(), http.StatusBadGateway)
			return
		}
		exists = true
	}

	setCfgUnsaved(w)

	// Re-render the right-pane leaf group with the updated presence state.
	gd := h.buildLeafGroup(r, mgr, path, node.Name, "container")
	if gd == nil {
		gd = &leafGroupData{Path: path, Name: node.Name, Kind: "container"}
	}
	gd.Presence = node.Presence
	gd.Exists = exists
	h.FragTmpl.ExecuteTemplate(w, "yang-leaf-group", gd)
}
