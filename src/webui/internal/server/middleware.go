// SPDX-License-Identifier: MIT

package server

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/kernelkit/webui/internal/auth"
	"github.com/kernelkit/webui/internal/handlers"
	"github.com/kernelkit/webui/internal/restconf"
	"github.com/kernelkit/webui/internal/security"
)

const cookieName = "session"

// authMiddleware checks the session cookie on every request, looks up
// the session, and attaches decrypted credentials to the context.
// Unauthenticated requests are redirected to /login (or get a 401 if
// the request comes from HTMX).
func authMiddleware(store *auth.SessionStore, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		cookie, err := r.Cookie(cookieName)
		if err != nil {
			deny(w, r)
			return
		}

		username, password, csrf, features, ok := store.Lookup(cookie.Value)
		if !ok {
			deny(w, r)
			return
		}

		// Sliding window: re-issue the cookie with a fresh timestamp.
		// Skip for background polling endpoints so they don't keep
		// the session alive indefinitely.
		if !isPollingPath(r.URL.Path) {
			if fresh, err := store.CreateWithCSRF(username, password, csrf, features); err == nil {
				http.SetCookie(w, &http.Cookie{
					Name:     cookieName,
					Value:    fresh,
					Path:     "/",
					HttpOnly: true,
					Secure:   security.IsSecureRequest(r),
					SameSite: http.SameSiteLaxMode,
				})
			}
		}

		ctx := restconf.ContextWithCredentials(r.Context(), restconf.Credentials{
			Username: username,
			Password: password,
		})
		ctx = security.WithToken(ctx, csrf)
		ctx = handlers.ContextWithCapabilities(ctx, handlers.NewCapabilities(features))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func csrfMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := security.TokenFromContext(r.Context())
		token = security.EnsureToken(w, r, token)
		r = r.WithContext(security.WithToken(r.Context(), token))

		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
			next.ServeHTTP(w, r)
			return
		}

		if !sameOrigin(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		if !validCSRF(r, token) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func sameOrigin(r *http.Request) bool {
	host := r.Host
	if xf := r.Header.Get("X-Forwarded-Host"); xf != "" {
		parts := strings.Split(xf, ",")
		host = strings.TrimSpace(parts[0])
	}
	if host == "" {
		return false
	}

	origin := r.Header.Get("Origin")
	if origin != "" {
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		return strings.EqualFold(u.Host, host)
	}

	ref := r.Header.Get("Referer")
	if ref != "" {
		u, err := url.Parse(ref)
		if err != nil {
			return false
		}
		return strings.EqualFold(u.Host, host)
	}

	return true
}

func validCSRF(r *http.Request, token string) bool {
	if token == "" {
		return false
	}
	if hdr := r.Header.Get("X-CSRF-Token"); hdr != "" {
		return subtleConstantTimeEquals(hdr, token)
	}
	if err := r.ParseForm(); err != nil {
		return false
	}
	return subtleConstantTimeEquals(r.FormValue("csrf"), token)
}

func subtleConstantTimeEquals(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := 0; i < len(a); i++ {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'; form-action 'self'")
		if security.IsSecureRequest(r) {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func isPublicPath(path string) bool {
	return path == "/login" || strings.HasPrefix(path, "/assets/")
}

func isPollingPath(path string) bool {
	return path == "/device-status" || strings.HasSuffix(path, "/counters")
}

func deny(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("HX-Request") == "true" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
