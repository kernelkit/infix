// SPDX-License-Identifier: MIT

package schema

import "strings"

// EvaluateWhen evaluates a pre-resolved YANG when XPath expression against
// the flat key→value map of the context node's parent data.
//
// Handles the practical subset used in Infix YANG models:
//   - derived-from-or-self(path, 'module:identity')
//   - derived-from(path, 'module:identity')
//   - combinations joined with 'or' / 'and'
//
// Conservative: any unrecognised expression or absent data returns true (show).
func EvaluateWhen(mgr *Manager, expr string, values map[string]string) bool {
	if expr == "" || values == nil {
		return true
	}
	return evalOr(mgr, strings.TrimSpace(expr), values)
}

func evalOr(mgr *Manager, expr string, values map[string]string) bool {
	parts := splitOnKeyword(expr, "or")
	for _, p := range parts {
		if evalAnd(mgr, strings.TrimSpace(p), values) {
			return true
		}
	}
	return false
}

func evalAnd(mgr *Manager, expr string, values map[string]string) bool {
	parts := splitOnKeyword(expr, "and")
	for _, p := range parts {
		if !evalAtom(mgr, strings.TrimSpace(p), values) {
			return false
		}
	}
	return true
}

func evalAtom(mgr *Manager, expr string, values map[string]string) bool {
	if expr == "" {
		return true // empty: conservative
	}
	if strings.HasPrefix(expr, "derived-from-or-self(") && strings.HasSuffix(expr, ")") {
		inner := expr[len("derived-from-or-self(") : len(expr)-1]
		return evalDerivedFrom(mgr, inner, values, true)
	}
	if strings.HasPrefix(expr, "derived-from(") && strings.HasSuffix(expr, ")") {
		inner := expr[len("derived-from(") : len(expr)-1]
		return evalDerivedFrom(mgr, inner, values, false)
	}
	return true // unknown expression: conservative show
}

func evalDerivedFrom(mgr *Manager, inner string, values map[string]string, orSelf bool) bool {
	comma := strings.Index(inner, ",")
	if comma < 0 {
		return true // malformed: conservative
	}
	xpathPath := strings.TrimSpace(inner[:comma])
	identity := strings.Trim(strings.TrimSpace(inner[comma+1:]), "'\"")

	leafName := xpathLeafName(xpathPath)
	if leafName == "" {
		return true // unresolvable path: conservative
	}

	current := values[leafName]
	if current == "" {
		return true // no data: conservative show
	}
	return checkIdentity(mgr, current, identity, orSelf)
}

// xpathLeafName extracts the bare leaf name from a simple XPath step.
//   - "module:name" or "name"        → "name"
//   - "../module:name"               → "name"  (single parent step)
//   - "../../../…" or path with "/"  → ""  (multi-level: conservative)
func xpathLeafName(path string) string {
	if strings.HasPrefix(path, "../") {
		path = path[3:]
		// After one parent step, path must be a bare leaf name.
		if strings.Contains(path, "/") || strings.HasPrefix(path, "..") {
			return ""
		}
	} else if strings.Contains(path, "/") {
		return "" // forward traversal: conservative
	}
	_, local := splitPrefix(strings.TrimSpace(path))
	return local
}

// checkIdentity reports whether currentValue is equal to or derived from
// targetIdentity according to the YANG identity hierarchy.
func checkIdentity(mgr *Manager, currentValue, targetIdentity string, orSelf bool) bool {
	_, currentLocal := splitPrefix(currentValue)
	_, targetLocal := splitPrefix(targetIdentity)

	if orSelf && currentLocal == targetLocal {
		return true
	}
	for _, d := range mgr.IdentitiesOf(targetLocal) {
		if d == currentLocal {
			return true
		}
	}
	return false
}

// splitOnKeyword splits expr on the XPath keyword kw at parenthesis depth 0,
// but NOT when kw is part of a compound name (e.g. "derived-from-or-self"
// contains "or"; "derived-from-and-…" would contain "and").
// A keyword match is suppressed when immediately preceded by a hyphen.
//
// The Infix YANG files contain a concatenation bug where the 'or' keyword is
// immediately followed by the next function name without a separating space
// (e.g. "…') orderived-from-or-self(…)").  This splitter handles that case
// by treating any 'or'/'and' that is not preceded by '-' as a keyword.
func splitOnKeyword(expr, kw string) []string {
	n := len(kw)
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(expr); i++ {
		switch expr[i] {
		case '(':
			depth++
		case ')':
			depth--
		}
		if depth != 0 || i+n > len(expr) || expr[i:i+n] != kw {
			continue
		}
		// Suppress if immediately preceded by '-' (part of compound name).
		if i > 0 && expr[i-1] == '-' {
			continue
		}
		// Found keyword: save part, trim trailing space before keyword.
		end := i
		if end > start && expr[end-1] == ' ' {
			end--
		}
		parts = append(parts, strings.TrimSpace(expr[start:end]))
		start = i + n
		if start < len(expr) && expr[start] == ' ' {
			start++
		}
		i = start - 1 // loop will increment
	}
	parts = append(parts, strings.TrimSpace(expr[start:]))
	return parts
}
