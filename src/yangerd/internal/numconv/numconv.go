// Package numconv converts the loosely typed numbers yangerd gets from
// decoded JSON, D-Bus variants and command output into Go integers.
package numconv

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Int returns v as an int.  ok is false when v is neither a number nor
// a string holding one.  Floats are truncated.
func Int(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int8:
		return int(n), true
	case int16:
		return int(n), true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case uint:
		return int(n), true
	case uint8:
		return int(n), true
	case uint16:
		return int(n), true
	case uint32:
		return int(n), true
	case uint64:
		return int(n), true
	case float32:
		return int(n), true
	case float64:
		return int(n), true
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i), true
		}
		if f, err := n.Float64(); err == nil {
			return int(f), true
		}
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
			return i, true
		}
	}
	return 0, false
}

// IntOrZero is Int for callers where a missing value reads as 0.
func IntOrZero(v any) int {
	n, _ := Int(v)
	return n
}

// Uint64 returns v as a uint64, for counters.  Negative values and
// anything that is not a number are 0.
func Uint64(v any) uint64 {
	switch n := v.(type) {
	case uint:
		return uint64(n)
	case uint8:
		return uint64(n)
	case uint16:
		return uint64(n)
	case uint32:
		return uint64(n)
	case uint64:
		return n
	case json.Number:
		if u, err := strconv.ParseUint(n.String(), 10, 64); err == nil {
			return u
		}
	case string:
		if u, err := strconv.ParseUint(strings.TrimSpace(n), 10, 64); err == nil {
			return u
		}
		return 0
	}
	if i, ok := Int(v); ok && i > 0 {
		return uint64(i)
	}
	return 0
}
