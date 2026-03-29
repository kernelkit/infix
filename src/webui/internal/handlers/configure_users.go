// SPDX-License-Identifier: MIT

package handlers

import (
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/kernelkit/webui/internal/restconf"
)

// ─── RESTCONF JSON types ──────────────────────────────────────────────────────

// cfgAuthWrapper reads the authentication container via the full system path,
// which avoids sub-path encoding ambiguities and matches what configure-system uses.
type cfgAuthWrapper struct {
	System struct {
		Auth cfgAuthJSON `json:"authentication"`
	} `json:"ietf-system:system"`
}

type cfgAuthJSON struct {
	Users []cfgUserJSON `json:"user"`
}

type cfgUserJSON struct {
	Name           string       `json:"name"`
	Password       string       `json:"password,omitempty"`
	Shell          string       `json:"infix-system:shell,omitempty"`
	AuthorizedKeys []cfgKeyJSON `json:"authorized-key,omitempty"`
}

type cfgKeyJSON struct {
	Name      string `json:"name"`
	Algorithm string `json:"algorithm"`
	KeyData   []byte `json:"key-data"`
}

// ─── Display helper ───────────────────────────────────────────────────────────

type cfgUserDisplay struct {
	cfgUserJSON
	ShellLabel string
	KeyCount   int
}

func shellLabel(s string) string {
	switch s {
	case "infix-system:clish":
		return "CLI Shell"
	case "infix-system:bash":
		return "Bash"
	case "infix-system:sh":
		return "Sh"
	default:
		return "Disabled"
	}
}

// ─── Template data ────────────────────────────────────────────────────────────

type cfgUsersPageData struct {
	PageData
	Users []cfgUserDisplay
	Error string
}

// ─── Handler ─────────────────────────────────────────────────────────────────

// ConfigureUsersHandler serves the Configure > Users page.
type ConfigureUsersHandler struct {
	Template *template.Template
	RC       restconf.Fetcher
}

const authPath = candidatePath + "/ietf-system:system/authentication"

// Overview renders the Configure > Users page reading from the candidate.
// GET /configure/users
func (h *ConfigureUsersHandler) Overview(w http.ResponseWriter, r *http.Request) {
	data := cfgUsersPageData{
		PageData: newPageData(r, "configure-users", "Configure: Users"),
	}

	// Read via the full system path (same as configure-system) to avoid
	// sub-path encoding issues. Fall back to running if candidate is empty.
	sysPath := candidatePath + "/ietf-system:system"
	var raw cfgAuthWrapper
	if err := h.RC.Get(r.Context(), sysPath, &raw); err != nil {
		var rcErr *restconf.Error
		if errors.As(err, &rcErr) && rcErr.StatusCode == http.StatusNotFound {
			// Candidate not initialised — read from running as fallback.
			if fallErr := h.RC.Get(r.Context(), "/data/ietf-system:system", &raw); fallErr != nil {
				var rcFall *restconf.Error
				if !errors.As(fallErr, &rcFall) || rcFall.StatusCode != http.StatusNotFound {
					log.Printf("configure users (running fallback): %v", fallErr)
					data.Error = "Could not read user configuration"
				}
			}
		} else {
			log.Printf("configure users: %v", err)
			data.Error = "Could not read user configuration"
		}
	}
	for _, u := range raw.System.Auth.Users {
		data.Users = append(data.Users, cfgUserDisplay{
			cfgUserJSON: u,
			ShellLabel:  shellLabel(u.Shell),
			KeyCount:    len(u.AuthorizedKeys),
		})
	}

	tmplName := "configure-users.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// AddUser creates a new user in the candidate datastore.
// POST /configure/users
func (h *ConfigureUsersHandler) AddUser(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	shell := r.FormValue("shell")
	if name == "" {
		renderSaveError(w, fmt.Errorf("username is required"))
		return
	}

	hash, err := HashPassword(password)
	if err != nil {
		log.Printf("configure users add: hash: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	user := map[string]any{
		"ietf-system:user": map[string]any{
			"name":                 name,
			"password":             hash,
			"infix-system:shell":   shell,
		},
	}
	path := authPath + "/user=" + url.PathEscape(name)
	if err := h.RC.Put(r.Context(), path, user); err != nil {
		log.Printf("configure users add %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	w.Header().Set("HX-Location", `{"path":"/configure/users","target":"#content"}`)
	w.WriteHeader(http.StatusNoContent)
}

// DeleteUser removes a user from the candidate datastore.
// DELETE /configure/users/{name}
func (h *ConfigureUsersHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	path := authPath + "/user=" + url.PathEscape(name)
	if err := h.RC.Delete(r.Context(), path); err != nil {
		log.Printf("configure users delete %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	w.Header().Set("HX-Location", `{"path":"/configure/users","target":"#content"}`)
	w.WriteHeader(http.StatusNoContent)
}

// UpdateShell changes a user's login shell in the candidate datastore.
// POST /configure/users/{name}/shell
func (h *ConfigureUsersHandler) UpdateShell(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	shell := r.FormValue("shell")
	body := map[string]any{
		"ietf-system:user": map[string]any{
			"name":               name,
			"infix-system:shell": shell,
		},
	}
	path := authPath + "/user=" + url.PathEscape(name)
	if err := h.RC.Patch(r.Context(), path, body); err != nil {
		log.Printf("configure users shell %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSaved(w, "Shell updated")
}

// ChangePassword sets a new hashed password for a user in the candidate.
// POST /configure/users/{name}/password
func (h *ConfigureUsersHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	password := r.FormValue("password")
	if password == "" {
		renderSaveError(w, fmt.Errorf("password cannot be empty"))
		return
	}

	hash, err := HashPassword(password)
	if err != nil {
		log.Printf("configure users password %q: hash: %v", name, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	body := map[string]any{
		"ietf-system:user": map[string]any{
			"name":     name,
			"password": hash,
		},
	}
	path := authPath + "/user=" + url.PathEscape(name)
	if err := h.RC.Patch(r.Context(), path, body); err != nil {
		log.Printf("configure users password %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSaved(w, "Password changed")
}

// AddKey adds an SSH authorized key for a user in the candidate.
// POST /configure/users/{name}/keys
func (h *ConfigureUsersHandler) AddKey(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	keyName := strings.TrimSpace(r.FormValue("key_name"))
	keyLine := strings.TrimSpace(r.FormValue("key_data"))

	if keyName == "" || keyLine == "" {
		renderSaveError(w, fmt.Errorf("key name and public key are required"))
		return
	}

	// Parse "algorithm base64data [comment]" from an OpenSSH public key line.
	parts := strings.Fields(keyLine)
	if len(parts) < 2 {
		renderSaveError(w, fmt.Errorf("invalid SSH public key format"))
		return
	}
	algorithm := parts[0]
	keyBytes, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		renderSaveError(w, fmt.Errorf("invalid SSH key data: %w", err))
		return
	}

	body := map[string]any{
		"ietf-system:authorized-key": map[string]any{
			"name":      keyName,
			"algorithm": algorithm,
			"key-data":  keyBytes, // []byte → base64 in JSON
		},
	}
	path := authPath + "/user=" + url.PathEscape(name) +
		"/authorized-key=" + url.PathEscape(keyName)
	if err := h.RC.Put(r.Context(), path, body); err != nil {
		log.Printf("configure users key add %q/%q: %v", name, keyName, err)
		renderSaveError(w, err)
		return
	}
	w.Header().Set("HX-Location", `{"path":"/configure/users","target":"#content"}`)
	w.WriteHeader(http.StatusNoContent)
}

// DeleteKey removes an SSH authorized key for a user in the candidate.
// DELETE /configure/users/{name}/keys/{keyname}
func (h *ConfigureUsersHandler) DeleteKey(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	keyName := r.PathValue("keyname")
	path := authPath + "/user=" + url.PathEscape(name) +
		"/authorized-key=" + url.PathEscape(keyName)
	if err := h.RC.Delete(r.Context(), path); err != nil {
		log.Printf("configure users key delete %q/%q: %v", name, keyName, err)
		renderSaveError(w, err)
		return
	}
	w.Header().Set("HX-Location", `{"path":"/configure/users","target":"#content"}`)
	w.WriteHeader(http.StatusNoContent)
}

