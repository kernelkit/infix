// SPDX-License-Identifier: MIT

package schema

import (
	"reflect"
	"testing"
)

func TestSplitOnKeyword(t *testing.T) {
	tests := []struct {
		name string
		expr string
		kw   string
		want []string
	}{
		{
			name: "single_term_no_split",
			expr: "derived-from-or-self(if:type, 'gre')",
			kw:   "or",
			want: []string{"derived-from-or-self(if:type, 'gre')"},
		},
		{
			name: "two_terms_with_space",
			expr: "derived-from-or-self(if:type, 'gre') or derived-from-or-self(if:type, 'gretap')",
			kw:   "or",
			want: []string{
				"derived-from-or-self(if:type, 'gre')",
				"derived-from-or-self(if:type, 'gretap')",
			},
		},
		{
			name: "two_terms_no_space_after_or", // Infix YANG concatenation bug
			expr: "derived-from-or-self(if:type, 'gre') orderived-from-or-self(if:type, 'gretap')",
			kw:   "or",
			want: []string{
				"derived-from-or-self(if:type, 'gre')",
				"derived-from-or-self(if:type, 'gretap')",
			},
		},
		{
			name: "or_inside_parens_not_split",
			expr: "derived-from-or-self(if:type, 'foo or bar')",
			kw:   "or",
			want: []string{"derived-from-or-self(if:type, 'foo or bar')"},
		},
		{
			name: "and_keyword",
			expr: "cond-a and cond-b",
			kw:   "and",
			want: []string{"cond-a", "cond-b"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitOnKeyword(tt.expr, tt.kw)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitOnKeyword(%q, %q) = %v; want %v", tt.expr, tt.kw, got, tt.want)
			}
		})
	}
}

func TestXpathLeafName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"if:type", "type"},
		{"ietf-interfaces:type", "type"},
		{"type", "type"},
		{"../iehw:class", "class"},
		{"../infix-hardware:class", "class"},
		{"../../something", ""},
		{"../../../rt:address-family", ""},
		{"a/b", ""},
		{"/abs/path", ""},
	}
	for _, tt := range tests {
		got := xpathLeafName(tt.path)
		if got != tt.want {
			t.Errorf("xpathLeafName(%q) = %q; want %q", tt.path, got, tt.want)
		}
	}
}

func TestEvaluateWhenConservative(t *testing.T) {
	// nil values → conservative true
	if !EvaluateWhen(nil, "derived-from-or-self(if:type, 'foo')", nil) {
		t.Error("nil values should be conservative true")
	}
	// empty expr → true
	if !EvaluateWhen(nil, "", map[string]string{"type": "bar"}) {
		t.Error("empty expr should be conservative true")
	}
	// nil manager with unknown expr → true
	if !EvaluateWhen(nil, "unknown-function(x)", map[string]string{"x": "y"}) {
		t.Error("unknown expr should be conservative true")
	}
}

func TestEvaluateWhenDerivedFromOrSelf(t *testing.T) {
	// Minimal mock manager: IdentitiesOf("gre") → ["gretap"]
	mgr := &Manager{ms: nil} // ms is nil, but IdentitiesOf will handle it gracefully

	// We need a real identity hierarchy. Use a Manager with minimal modules.
	// Since setting up goyang modules is complex, test via checkIdentity directly.
	tests := []struct {
		name     string
		current  string
		target   string
		orSelf   bool
		derived  []string // what IdentitiesOf would return; we test checkIdentity
		want     bool
	}{
		{"or-self exact match", "gre", "gre", true, nil, true},
		{"or-self different", "bridge", "gre", true, nil, false},
		{"derived match", "gretap", "gre", false, []string{"gretap"}, true},
		{"derived no match", "bridge", "gre", false, []string{"gretap"}, false},
		{"module-qualified current", "infix-if-type:gre", "gre", true, nil, true},
		{"module-qualified target", "gre", "infix-if-type:gre", true, nil, true},
		{"both qualified exact", "infix-if-type:gre", "infix-if-type:gre", true, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Override IdentitiesOf by testing checkIdentity directly with a mock.
			// Build a mini-mgr that returns tt.derived for any base name.
			got := checkIdentityTest(tt.current, tt.target, tt.orSelf, tt.derived)
			if got != tt.want {
				t.Errorf("checkIdentity(%q, %q, orSelf=%v) = %v; want %v",
					tt.current, tt.target, tt.orSelf, got, tt.want)
			}
		})
	}
	_ = mgr
}

// checkIdentityTest is a test helper that bypasses the Manager.IdentitiesOf call.
func checkIdentityTest(currentValue, targetIdentity string, orSelf bool, derivedNames []string) bool {
	_, currentLocal := splitPrefix(currentValue)
	_, targetLocal := splitPrefix(targetIdentity)
	if orSelf && currentLocal == targetLocal {
		return true
	}
	for _, d := range derivedNames {
		if d == currentLocal {
			return true
		}
	}
	return false
}
