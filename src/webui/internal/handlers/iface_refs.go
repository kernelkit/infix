// SPDX-License-Identifier: MIT

package handlers

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"infix/webui/internal/restconf"
	"infix/webui/internal/schema"
)

const ifaceNameRef = "/ietf-interfaces:interfaces/interface/name"

// ifaceRefs collects what has to go when the interface called name is
// deleted: every leafref to it in the candidate, found by walking the
// data next to the schema.
type ifaceRefs struct {
	mgr        *schema.Manager
	name       string
	removes    []string
	dependants []string
}

func (w *ifaceRefs) refersToIface(node *schema.Node) bool {
	return node.Type != nil && node.Type.Leafref == ifaceNameRef
}

// walk visits obj, a container or list entry at path.  entry is the
// nearest enclosing list entry as a patch target and iface the enclosing
// interface by name, both empty at the top.
func (w *ifaceRefs) walk(path, entry, iface string, obj map[string]any) {
	for key, child := range obj {
		p := path + "/" + key
		node, err := w.mgr.NodeAt(p)
		if err != nil {
			continue
		}
		switch node.Kind {
		case "container":
			if m, ok := child.(map[string]any); ok {
				w.walk(p, entry, iface, m)
			}
		case "list":
			items, _ := child.([]any)
			for _, item := range items {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				t := p + "=" + w.keys(node, m)
				if w.keyIsName(p, node, m) {
					w.removes = append(w.removes, t)
					continue
				}
				in := iface
				if p == ifaceRoot+"/interface" {
					if m["name"] == w.name {
						continue
					}
					in, _ = m["name"].(string)
				}
				w.walk(t, t, in, m)
			}
		case "leaf-list":
			if !w.refersToIface(node) {
				continue
			}
			vals, _ := child.([]any)
			for _, v := range vals {
				if v == w.name {
					w.removes = append(w.removes, p+"="+url.PathEscape(w.name))
				}
			}
		case "leaf":
			if w.refersToIface(node) && child == w.name {
				w.leaf(p, entry, iface, node)
			}
		}
	}
}

// keys renders the key predicate of the list entry m.
func (w *ifaceRefs) keys(node *schema.Node, m map[string]any) string {
	vals := make([]string, 0, len(node.Keys))
	for _, k := range node.Keys {
		vals = append(vals, url.PathEscape(fmt.Sprint(m[k])))
	}
	return strings.Join(vals, ",")
}

// keyIsName reports whether a key of the list entry m refers to the interface.
func (w *ifaceRefs) keyIsName(path string, node *schema.Node, m map[string]any) bool {
	for _, k := range node.Keys {
		if m[k] != w.name {
			continue
		}
		if kn, err := w.mgr.NodeAt(path + "/" + k); err == nil && w.refersToIface(kn) {
			return true
		}
	}
	return false
}

// leaf decides what a leaf pointing at the interface takes with it.
func (w *ifaceRefs) leaf(p, entry, iface string, node *schema.Node) {
	switch {
	case !node.Mandatory:
		w.removes = append(w.removes, p)
	case iface == "" || entry != ifaceTarget(iface):
		// A list entry built on the interface goes with it
		if entry == "" {
			w.dependants = append(w.dependants, p)
		} else {
			w.removes = append(w.removes, entry)
		}
	case strings.HasSuffix(p, "/infix-interfaces:veth/peer"):
		// The two ends refer to each other, the pair goes together
		w.removes = append(w.removes, entry)
	case strings.HasSuffix(p, "/infix-interfaces:bridge-port/bridge"),
		strings.HasSuffix(p, "/infix-interfaces:lag-port/lag"):
		// Membership only, the port lives on by itself
		w.removes = append(w.removes, p[:strings.LastIndex(p, "/")])
	default:
		w.dependants = append(w.dependants, iface)
	}
}

// patch returns the removals as one YANG patch, or nil when there are
// none.  A target inside an entry that is removed as a whole is dropped.
func (w *ifaceRefs) patch() *restconf.YangPatch {
	sort.Strings(w.removes)
	var kept []string
next:
	for _, t := range w.removes {
		for _, k := range kept {
			if t == k || strings.HasPrefix(t, k+"/") {
				continue next
			}
		}
		kept = append(kept, t)
	}
	if len(kept) == 0 {
		return nil
	}
	p := restconf.NewYangPatch(candidatePath)
	for _, t := range kept {
		p.Remove(t)
	}
	return p
}

func uniqueSorted(list []string) []string {
	sort.Strings(list)
	var out []string
	for i, s := range list {
		if i == 0 || s != list[i-1] {
			out = append(out, s)
		}
	}
	return out
}
