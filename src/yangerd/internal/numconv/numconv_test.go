package numconv

import (
	"encoding/json"
	"testing"
)

func TestInt(t *testing.T) {
	tests := []struct {
		in   any
		want int
		ok   bool
	}{
		{42, 42, true},
		{int32(-7), -7, true},
		{uint16(16), 16, true},
		{float64(99.9), 99, true},
		{json.Number("12"), 12, true},
		{json.Number("1.5"), 1, true},
		{" 42 ", 42, true},
		{"nope", 0, false},
		{nil, 0, false},
		{true, 0, false},
	}
	for _, tc := range tests {
		got, ok := Int(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("Int(%#v) = %d, %v; want %d, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestUint64(t *testing.T) {
	tests := []struct {
		in   any
		want uint64
	}{
		{uint8(8), 8},
		{uint64(1 << 60), 1 << 60},
		{42, 42},
		{-1, 0},
		{int64(-9), 0},
		{float64(99.9), 99},
		{float64(-0.1), 0},
		{"42", 42},
		{"18446744073709551615", 18446744073709551615},
		{json.Number("18446744073709551615"), 18446744073709551615},
		{"nope", 0},
	}
	for _, tc := range tests {
		if got := Uint64(tc.in); got != tc.want {
			t.Errorf("Uint64(%#v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
