// SPDX-License-Identifier: MIT

package auth

import (
	"testing"
	"time"
)

func TestSessionLifetime(t *testing.T) {
	s := &SessionStore{sessions: make(map[string]*sessionEntry)}
	token, _, err := s.Create("admin", "admin", nil)
	if err != nil {
		t.Fatal(err)
	}
	s.SetTimeout(token, 0)

	// Auto-logout "Off" and in use until a minute ago, but logged in a day ago
	e := s.sessions[token]
	e.createdAt = time.Now().Add(-sessionLifetime - time.Minute)
	e.lastSeenAt = time.Now().Add(-time.Minute)
	if _, _, _, _, ok := s.Lookup(token); ok {
		t.Error("session outlived its lifetime")
	}

	token, _, _ = s.Create("admin", "admin", nil)
	s.SetTimeout(token, 0)
	s.sessions[token].lastSeenAt = time.Now().Add(-12 * time.Hour)
	if _, _, _, _, ok := s.Lookup(token); !ok {
		t.Error("idle session with Auto-logout off expired before its lifetime")
	}
}
