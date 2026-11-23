// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"log"
	"slices"
	"strings"
)

type groupHome struct {
	group, path string
}

// Homes is WEBUI_HOME: who lands on a page of their own after login
// instead of the dashboard.  nginx serves the page, not this binary.
type Homes struct {
	users  map[string]string
	groups []groupHome
}

// ParseHomes reads WEBUI_HOME, a space separated list of user=path and
// @group=path, the latter for the members of a NACM group.
func ParseHomes(v string) Homes {
	h := Homes{users: make(map[string]string)}
	for _, f := range strings.Fields(v) {
		name, path, ok := strings.Cut(f, "=")
		// A browser takes //host and /\host for another host
		if !ok || name == "" || name == "@" || path == "/" || !strings.HasPrefix(path, "/") ||
			strings.HasPrefix(path, "//") || strings.Contains(path, "\\") {
			log.Printf("WEBUI_HOME: ignoring %q, want user=/path or @group=/path", f)
			continue
		}
		if group, ok := strings.CutPrefix(name, "@"); ok {
			h.groups = append(h.groups, groupHome{group, path})
		} else {
			h.users[name] = path
		}
	}
	return h
}

// Resolve returns the home of user, empty for the dashboard.  The
// groups of the user are only looked up when a group entry can apply.
// It never does for an admin, who could not get back to the regular
// pages to leave the group.
func (h Homes) Resolve(ctx context.Context, user string, admin bool,
	lookup func(context.Context) ([]string, error)) (string, error) {
	if home := h.users[user]; home != "" || admin || len(h.groups) == 0 {
		return home, nil
	}
	groups, err := lookup(ctx)
	if err != nil {
		return "", err
	}
	for _, g := range h.groups {
		if slices.Contains(groups, g.group) {
			return g.path, nil
		}
	}
	return "", nil
}
