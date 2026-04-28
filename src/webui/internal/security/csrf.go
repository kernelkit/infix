// SPDX-License-Identifier: MIT

package security

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"
)

const csrfCookieName = "csrf"

type csrfKey struct{}

// EnsureToken sets a CSRF cookie if missing (or preferred is provided)
// and returns the current token.
func EnsureToken(w http.ResponseWriter, r *http.Request, preferred string) string {
	if token := strings.TrimSpace(preferred); token != "" {
		http.SetCookie(w, &http.Cookie{
			Name:     csrfCookieName,
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			Secure:   IsSecureRequest(r),
			SameSite: http.SameSiteStrictMode,
		})
		return token
	}

	if c, err := r.Cookie(csrfCookieName); err == nil {
		if token := strings.TrimSpace(c.Value); validToken(token) {
			return token
		}
	}

	token := randomToken()
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   IsSecureRequest(r),
		SameSite: http.SameSiteStrictMode,
	})
	return token
}

// WithToken stores the token in the request context.
func WithToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, csrfKey{}, token)
}

// TokenFromContext returns the CSRF token from context, if set.
func TokenFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(csrfKey{}).(string); ok {
		return v
	}
	return ""
}

func randomToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func validToken(token string) bool {
	if token == "" {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(token)
	return err == nil
}

// ClearToken removes the CSRF cookie.
func ClearToken(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   IsSecureRequest(r),
		SameSite: http.SameSiteStrictMode,
	})
}

// IsSecureRequest returns true for TLS or proxy-terminated HTTPS.
func IsSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if xf := r.Header.Get("X-Forwarded-Proto"); xf != "" {
		parts := strings.Split(xf, ",")
		return strings.EqualFold(strings.TrimSpace(parts[0]), "https")
	}
	return false
}
