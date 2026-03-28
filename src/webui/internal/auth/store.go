// SPDX-License-Identifier: MIT

package auth

import (
	"fmt"
	"sync"
	"time"

	"github.com/kernelkit/webui/internal/security"
)

const sessionTimeout = 1 * time.Hour

// sessionEntry is the in-memory state of a single authenticated
// session.  The user's password is kept alongside the token so the
// webui can re-authenticate to RESTCONF on the user's behalf on each
// request; it is never written to disk or surfaced over the wire.
type sessionEntry struct {
	username   string
	password   string
	csrfToken  string
	features   map[string]bool
	lastSeenAt time.Time
}

// SessionStore issues opaque random session tokens backed by an
// in-memory map.  Nothing is persisted: a webui restart drops every
// active session and the UI's 401 handler surfaces a fresh login
// page.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*sessionEntry
}

// NewSessionStore returns a fresh store and starts a janitor
// goroutine that sweeps expired entries once a minute.
func NewSessionStore() *SessionStore {
	s := &SessionStore{
		sessions: make(map[string]*sessionEntry),
	}
	go s.janitor()
	return s
}

// Create issues a session for the given credentials, returning the
// session token (cookie value) and a freshly minted CSRF token.
func (s *SessionStore) Create(username, password string, features map[string]bool) (string, string, error) {
	token, err := security.RandomToken()
	if err != nil {
		return "", "", fmt.Errorf("session token: %w", err)
	}
	csrf, err := security.RandomToken()
	if err != nil {
		return "", "", fmt.Errorf("csrf token: %w", err)
	}

	s.mu.Lock()
	s.sessions[token] = &sessionEntry{
		username:   username,
		password:   password,
		csrfToken:  csrf,
		features:   features,
		lastSeenAt: time.Now(),
	}
	s.mu.Unlock()
	return token, csrf, nil
}

// Lookup returns the credentials associated with a session token, or
// ok=false if the token is unknown or expired.  An expired entry is
// reaped as a side effect.
func (s *SessionStore) Lookup(token string) (username, password, csrf string, features map[string]bool, ok bool) {
	s.mu.RLock()
	e, present := s.sessions[token]
	if present && time.Since(e.lastSeenAt) <= sessionTimeout {
		username, password, csrf, features = e.username, e.password, e.csrfToken, e.features
		s.mu.RUnlock()
		return username, password, csrf, features, true
	}
	s.mu.RUnlock()

	// Slow path: the entry was either missing or expired at read
	// time.  Re-check under the write lock — Refresh may have
	// updated lastSeenAt in the gap — and only delete if still
	// expired.
	if present {
		s.mu.Lock()
		if e, ok := s.sessions[token]; ok && time.Since(e.lastSeenAt) > sessionTimeout {
			delete(s.sessions, token)
		}
		s.mu.Unlock()
	}
	return "", "", "", nil, false
}

// Refresh extends a session's lifetime by resetting its last-seen
// timestamp.  Called by the auth middleware on each user-driven
// request so an active session doesn't expire mid-use.  No-op if the
// token is unknown.
func (s *SessionStore) Refresh(token string) {
	s.mu.Lock()
	if e, ok := s.sessions[token]; ok {
		e.lastSeenAt = time.Now()
	}
	s.mu.Unlock()
}

// Delete revokes a session.  Called by logout and by Lookup on
// expiration.
func (s *SessionStore) Delete(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

func (s *SessionStore) janitor() {
	t := time.NewTicker(1 * time.Minute)
	defer t.Stop()
	for range t.C {
		cutoff := time.Now().Add(-sessionTimeout)
		s.mu.Lock()
		for token, e := range s.sessions {
			if e.lastSeenAt.Before(cutoff) {
				delete(s.sessions, token)
			}
		}
		s.mu.Unlock()
	}
}
