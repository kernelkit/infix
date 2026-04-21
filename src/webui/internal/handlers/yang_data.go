// SPDX-License-Identifier: MIT

package handlers

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/kernelkit/webui/internal/restconf"
	"github.com/kernelkit/webui/internal/schema"
)

const candidateDS = "/ds/ietf-datastores:candidate"

// DataHandler serves GET /api/data — raw RESTCONF JSON for a path.
// PUT and DELETE are handled by TreeHandler to share template rendering.
type DataHandler struct {
	RC     restconf.Fetcher
	Schema *schema.Cache
}

// Get serves GET /api/data?path=...
// Returns the raw RESTCONF JSON subtree from candidate (falls back to running).
func (h *DataHandler) Get(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}

	data, err := h.RC.GetRaw(r.Context(), candidateDS+path)
	if err != nil {
		data, err = h.RC.GetRaw(r.Context(), "/data"+path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// navigateToNode traverses a RESTCONF JSON response using path segments to
// reach the target node.  The server always wraps responses in the full
// module-root hierarchy ({"module:root": {"list": [{...}]}}), so we walk each
// segment; for list segments with a key predicate (e.g. "interface=eth0") we
// enter the array and take the first (only) element.
// Returns nil when any segment cannot be found.
func navigateToNode(data []byte, path string) json.RawMessage {
	segs := strings.Split(strings.TrimPrefix(path, "/"), "/")
	current := json.RawMessage(data)
	for _, seg := range segs {
		hasPred := strings.ContainsAny(seg, "[=")
		_, localName := splitModPrefix(stripModPredicate(seg))
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(current, &obj); err != nil {
			return nil
		}
		var found json.RawMessage
		for k, v := range obj {
			_, local := splitModPrefix(k)
			if local == localName {
				found = v
				break
			}
		}
		if found == nil {
			return nil
		}
		if hasPred {
			var arr []json.RawMessage
			if err := json.Unmarshal(found, &arr); err != nil || len(arr) == 0 {
				return nil
			}
			// Extract key value from predicate: "name=eth0" → "eth0".
			keyVal := ""
			if i := strings.IndexByte(seg, '='); i >= 0 {
				keyVal = seg[i+1:]
			}
			matched := arr[0] // fallback: first element
			if keyVal != "" {
				for _, elem := range arr {
					var row map[string]json.RawMessage
					if json.Unmarshal(elem, &row) != nil {
						continue
					}
					for _, v := range row {
						var s string
						if json.Unmarshal(v, &s) == nil && s == keyVal {
							matched = elem
							goto nextSeg
						}
					}
				}
			}
		nextSeg:
			found = matched
		}
		current = found
	}
	return current
}

// flattenNodeValues extracts direct scalar leaf values from a JSON object,
// returning a map of bare-name → string.  Nested objects and arrays (which
// represent sub-containers and sub-lists) are silently skipped.
func flattenNodeValues(raw json.RawMessage) map[string]string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	result := make(map[string]string, len(obj))
	for k, v := range obj {
		if len(v) > 0 && v[0] == '[' {
			// YANG empty type is encoded as [null] in JSON (RFC 7951 §6.9).
			// Represent presence as "true"; skip real arrays (sub-lists).
			trimmed := bytes.TrimSpace(v)
			if bytes.Equal(trimmed, []byte("[null]")) {
				_, local := splitModPrefix(k)
				result[local] = "true"
			}
			continue
		}
		if len(v) > 0 && v[0] == '{' {
			continue // sub-container — not a direct leaf
		}
		_, local := splitModPrefix(k)
		result[local] = extractScalar(v)
	}
	return result
}

// stripModPredicate removes both a module prefix and a RESTCONF key predicate
// from a path segment, e.g. "ietf-interfaces:interface=eth0" → "interface".
func stripModPredicate(seg string) string {
	if i := strings.IndexByte(seg, '['); i >= 0 {
		seg = seg[:i]
	}
	if i := strings.IndexByte(seg, '='); i >= 0 {
		seg = seg[:i]
	}
	return seg
}

// extractLeafValue unwraps the single-key RESTCONF JSON envelope that wraps a
// leaf value: {"module:name": <value>} → string representation of <value>.
// Recursively unwraps single-key nested objects so that a response like
// {"module:parent": {"certificate": "gencert"}} → "gencert".
func extractLeafValue(data []byte) string {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return ""
	}
	for _, raw := range m {
		return extractScalar(raw)
	}
	return ""
}

// extractScalar converts a JSON value to a display string.
// Single-key objects are recursively unwrapped (RESTCONF sometimes wraps leaf
// values in a choice/case or container envelope).
func extractScalar(raw json.RawMessage) string {
	// Plain string.
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	// Bool or number (unquoted JSON token).
	v := string(raw)
	if v == "true" || v == "false" || (len(v) > 0 && (v[0] == '-' || (v[0] >= '0' && v[0] <= '9'))) {
		return v
	}
	// Single-key object: unwrap one level and recurse.
	var nested map[string]json.RawMessage
	if err := json.Unmarshal(raw, &nested); err == nil && len(nested) == 1 {
		for _, inner := range nested {
			return extractScalar(inner)
		}
	}
	// Fallback: return the raw token (strips outer quotes if present).
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		return v[1 : len(v)-1]
	}
	return v
}

// coerceLeafValue converts the raw form string to the JSON type that RESTCONF
// expects for the leaf based on the schema Node type.
func coerceLeafValue(raw string, node *schema.Node) any {
	if node == nil || node.Type == nil {
		return raw
	}
	switch node.Type.Kind {
	case "boolean":
		return raw == "on" || raw == "true"
	case "int8", "int16", "int32", "int64",
		"uint8", "uint16", "uint32", "uint64":
		var n int64
		if _, err := fmt.Sscanf(raw, "%d", &n); err == nil {
			return n
		}
	case "binary":
		// Strip whitespace that textarea input may add (trailing newlines, spaces).
		// If the cleaned value is valid base64, send it as-is; otherwise encode.
		cleaned := strings.Map(func(r rune) rune {
			if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
				return -1
			}
			return r
		}, raw)
		if _, err := base64.StdEncoding.DecodeString(cleaned); err == nil {
			return cleaned
		}
		return base64.StdEncoding.EncodeToString([]byte(raw))
	}
	return raw
}
