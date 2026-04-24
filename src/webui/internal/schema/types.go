package schema

// Node is a JSON-serialisable representation of a single YANG schema node.
// Children are omitted (nil) by default; use Manager.Children to lazy-load them.
type Node struct {
	Path        string    `json:"path"`
	Name        string    `json:"name"`
	Kind        string    `json:"kind"` // container|list|leaf|leaf-list|choice|case|rpc|notification
	Description string    `json:"description,omitempty"`
	Keys        []string  `json:"keys,omitempty"`
	Children    []*Node   `json:"children,omitempty"`
	Config      bool      `json:"config"`
	Mandatory   bool      `json:"mandatory"`
	Default     string    `json:"default,omitempty"`
	Type        *TypeInfo `json:"type,omitempty"`
	// When holds the pre-resolved YANG when expression (prefix aliases replaced
	// by canonical module names). Empty when there is no constraint.
	When string `json:"when,omitempty"`
}

// TypeInfo describes the type of a leaf or leaf-list node.
type TypeInfo struct {
	Kind       string   `json:"kind"` // string|boolean|int8..uint64|enumeration|identityref|leafref|binary|empty|...
	Enums      []string `json:"enums,omitempty"`      // enumeration values
	Identities []string `json:"identities,omitempty"` // identityref derived identity names
	Range      string   `json:"range,omitempty"`
	Pattern    string   `json:"pattern,omitempty"`
	Leafref    string   `json:"leafref,omitempty"` // target path for leafref
}
