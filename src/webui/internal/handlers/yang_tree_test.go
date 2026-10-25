// SPDX-License-Identifier: MIT

package handlers

import (
	"encoding/json"
	"testing"
)

// RESTCONF reads a ':' in a path segment as a module prefix, so an
// identity key must have it encoded.
func TestBuildKeyPredicateIdentity(t *testing.T) {
	item := map[string]json.RawMessage{
		"type": json.RawMessage(`"infix-routing:ospfv2"`),
		"name": json.RawMessage(`"default"`),
	}
	if got := buildKeyPredicate(item, []string{"type", "name"}); got != "=infix-routing%3Aospfv2,default" {
		t.Errorf("predicate %q", got)
	}
}
