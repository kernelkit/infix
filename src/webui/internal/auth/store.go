// SPDX-License-Identifier: MIT

package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const sessionTimeout = 1 * time.Hour

type tokenPayload struct {
	Username  string          `json:"u"`
	Password  string          `json:"p"`
	CsrfToken string          `json:"c"`
	CreatedAt int64           `json:"t"`
	Features  map[string]bool `json:"f,omitempty"`
}

// SessionStore issues and validates stateless encrypted tokens.
// The cookie value is a base64url-encoded AES-256-GCM sealed blob
// containing the user's credentials and a creation timestamp.
// No server-side session map is needed — only the AES key must
// persist across restarts.
type SessionStore struct {
	aead cipher.AEAD
}

// NewSessionStore creates a store.  If keyFile is non-empty, the AES
// key is read from that path (or generated and written there on first
// run).  If keyFile is empty, a random ephemeral key is used.
func NewSessionStore(keyFile string) (*SessionStore, error) {
	key, err := loadOrCreateKey(keyFile)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return &SessionStore{aead: aead}, nil
}

// Create returns an encrypted token carrying the user's credentials and capabilities.
func (s *SessionStore) Create(username, password string, features map[string]bool) (string, string, error) {
	csrf := randomToken()
	token, err := s.CreateWithCSRF(username, password, csrf, features)
	return token, csrf, err
}

// CreateWithCSRF returns an encrypted token carrying the user's credentials,
// capabilities, and a bound CSRF token.
func (s *SessionStore) CreateWithCSRF(username, password, csrf string, features map[string]bool) (string, error) {
	payload, err := json.Marshal(tokenPayload{
		Username:  username,
		Password:  password,
		CsrfToken: csrf,
		CreatedAt: time.Now().Unix(),
		Features:  features,
	})
	if err != nil {
		return "", err
	}

	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	sealed := s.aead.Seal(nonce, nonce, payload, nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

// Lookup decrypts a token and returns the credentials and capabilities if valid.
func (s *SessionStore) Lookup(token string) (username, password, csrf string, features map[string]bool, ok bool) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", "", "", nil, false
	}

	ns := s.aead.NonceSize()
	if len(raw) < ns {
		return "", "", "", nil, false
	}

	plaintext, err := s.aead.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", "", "", nil, false
	}

	var p tokenPayload
	if err := json.Unmarshal(plaintext, &p); err != nil {
		return "", "", "", nil, false
	}

	if time.Since(time.Unix(p.CreatedAt, 0)) > sessionTimeout {
		return "", "", "", nil, false
	}

	return p.Username, p.Password, p.CsrfToken, p.Features, true
}

// Delete is a no-op for stateless tokens (the cookie is cleared by
// the caller), but kept to satisfy the existing logout flow.
func (s *SessionStore) Delete(token string) {}

// loadOrCreateKey returns a 32-byte AES key.  When path is non-empty
// the key is persisted so sessions survive restarts.
func loadOrCreateKey(path string) ([32]byte, error) {
	var key [32]byte

	if path != "" {
		data, err := os.ReadFile(path)
		if err == nil && len(data) == 32 {
			copy(key[:], data)
			return key, nil
		}
	}

	if _, err := io.ReadFull(rand.Reader, key[:]); err != nil {
		return key, fmt.Errorf("generate session key: %w", err)
	}

	if path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return key, fmt.Errorf("create key directory: %w", err)
		}
		if err := os.WriteFile(path, key[:], 0600); err != nil {
			return key, fmt.Errorf("write session key: %w", err)
		}
	}

	return key, nil
}

func randomToken() string {
	var b [32]byte
	if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}
