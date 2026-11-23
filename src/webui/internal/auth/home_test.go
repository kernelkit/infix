// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"errors"
	"testing"
)

func TestParseHomes(t *testing.T) {
	h := ParseHomes("guest=/stats/  bad nopath= rel=stats host=//evil.example/ bs=/\\evil.example/ root=/ op=/op/" +
		" @=/x/ @evil=//evil.example/ @service=/service/ @guest=/panel/")

	if len(h.users) != 2 || h.users["guest"] != "/stats/" || h.users["op"] != "/op/" {
		t.Errorf("users: got %v", h.users)
	}
	want := []groupHome{{"service", "/service/"}, {"guest", "/panel/"}}
	if len(h.groups) != len(want) || h.groups[0] != want[0] || h.groups[1] != want[1] {
		t.Errorf("groups: got %v, want %v", h.groups, want)
	}
}

func TestHomesResolve(t *testing.T) {
	members := map[string][]string{
		"jane":    {"guest"},
		"joe":     {"guest", "service"},
		"olle":    {"operator"},
		"service": {"guest"},
		"op":      {"guest"},
		"admin":   {"admin", "guest"},
	}
	lookups := 0
	lookup := func(_ context.Context, user string) ([]string, error) {
		lookups++
		if user == "broken" {
			return nil, errors.New("no sysrepo")
		}
		return members[user], nil
	}
	h := ParseHomes("op=/op/ @service=/service/ @guest=/panel/")

	for _, tc := range []struct {
		user    string
		admin   bool
		want    string
		lookups int
	}{
		{"jane", false, "/panel/", 1},
		{"joe", false, "/service/", 1},
		{"olle", false, "", 1},
		{"guest", false, "", 1},
		{"service", false, "/panel/", 1},
		{"op", false, "/op/", 0},
		{"op", true, "/op/", 0},
		{"admin", true, "", 0},
	} {
		lookups = 0
		got, err := h.Resolve(context.Background(), tc.user, tc.admin, lookup)
		if err != nil || got != tc.want || lookups != tc.lookups {
			t.Errorf("%s: got %q, %v after %d lookups, want %q after %d",
				tc.user, got, err, lookups, tc.want, tc.lookups)
		}
	}

	if _, err := h.Resolve(context.Background(), "broken", false, lookup); err == nil {
		t.Error("failed group lookup: no error")
	}

	lookups = 0
	got, err := ParseHomes("op=/op/").Resolve(context.Background(), "jane", false, lookup)
	if err != nil || got != "" || lookups != 0 {
		t.Errorf("no group homes: got %q, %v after %d lookups", got, err, lookups)
	}
}
