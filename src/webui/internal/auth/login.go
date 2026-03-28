// SPDX-License-Identifier: MIT

package auth

import (
	"errors"
	"html/template"
	"log"
	"net/http"

	"github.com/kernelkit/webui/internal/handlers"
	"github.com/kernelkit/webui/internal/restconf"
	"github.com/kernelkit/webui/internal/security"
)

const cookieName = "session"

// LoginHandler serves the login page and processes login/logout requests.
type LoginHandler struct {
	Store    *SessionStore
	RC       *restconf.Client
	Template *template.Template
}

type loginData struct {
	Error     string
	CsrfToken string
}

// ShowLogin renders the login page (GET /login).
func (h *LoginHandler) ShowLogin(w http.ResponseWriter, r *http.Request) {
	h.renderLogin(w, r, "")
}

// DoLogin validates credentials against RESTCONF and creates a session (POST /login).
func (h *LoginHandler) DoLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderLogin(w, r, "Invalid request.")
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == "" || password == "" {
		h.renderLogin(w, r, "Username and password are required.")
		return
	}

	// Verify credentials by making a RESTCONF call with Basic Auth.
	err := h.RC.CheckAuth(username, password)
	if err != nil {
		log.Printf("login failed for %q: %v", username, err)
		var authErr *restconf.AuthError
		if errors.As(err, &authErr) {
			h.renderLogin(w, r, "Invalid username or password.")
		} else {
			h.renderLogin(w, r, "Unable to reach the device. Please try again later.")
		}
		return
	}

	// Probe optional features once at login and bake into the session.
	ctx := restconf.ContextWithCredentials(r.Context(), restconf.Credentials{
		Username: username,
		Password: password,
	})
	caps := handlers.DetectCapabilities(ctx, h.RC)

	token, csrfToken, err := h.Store.Create(username, password, caps.Features())
	if err != nil {
		log.Printf("session create error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   security.IsSecureRequest(r),
		SameSite: http.SameSiteLaxMode,
	})
	security.EnsureToken(w, r, csrfToken)

	fullRedirect(w, r, "/")
}

// DoLogout destroys the session and redirects to the login page (POST /logout).
func (h *LoginHandler) DoLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		h.Store.Delete(c.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   security.IsSecureRequest(r),
		SameSite: http.SameSiteLaxMode,
	})
	security.ClearToken(w, r)

	fullRedirect(w, r, "/login")
}

// fullRedirect forces a full page navigation.  When the request comes
// from htmx (boosted form) we use HX-Redirect so the browser does a
// real page load instead of an AJAX swap — this is essential for the
// login/logout transition where the page layout changes completely.
func fullRedirect(w http.ResponseWriter, r *http.Request, url string) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", url)
		return
	}
	http.Redirect(w, r, url, http.StatusSeeOther)
}

func (h *LoginHandler) renderLogin(w http.ResponseWriter, r *http.Request, errMsg string) {
	data := loginData{
		Error:     errMsg,
		CsrfToken: security.TokenFromContext(r.Context()),
	}
	if err := h.Template.ExecuteTemplate(w, "login.html", data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
