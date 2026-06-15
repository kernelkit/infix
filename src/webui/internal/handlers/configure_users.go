// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"encoding/base64"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strings"

	"infix/webui/internal/restconf"
	"infix/webui/internal/schema"
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

// ─── Display helpers ──────────────────────────────────────────────────────────

type cfgUserDisplay struct {
	cfgUserJSON
	ShellLabel string
	KeyCount   int
}

type cfgGroupDisplay struct {
	nacmGroupJSON
	MembersSummary string
	Available      []string // users not currently in this group
}

// ─── Template data ────────────────────────────────────────────────────────────

type cfgUsersPageData struct {
	PageData
	Loading      bool
	Users        []cfgUserDisplay
	Groups       []cfgGroupDisplay
	Error        string
	ShellOptions []schema.IdentityOption
}

// ─── Handler ─────────────────────────────────────────────────────────────────

// ConfigureUsersHandler serves the Configure > Users page.
type ConfigureUsersHandler struct {
	Template *template.Template
	RC       restconf.Fetcher
	Schema   *schema.Cache
}

const authPath = candidatePath + "/ietf-system:system/authentication"
const nacmGroupsPath = candidatePath + "/ietf-netconf-acm:nacm/groups"

// Overview renders the Configure > Users page reading from the candidate.
// GET /configure/users
func (h *ConfigureUsersHandler) Overview(w http.ResponseWriter, r *http.Request) {
	data := cfgUsersPageData{
		PageData: newPageData(w, r, "configure-users", "Users & Groups"),
	}

	// Read via the full system path (same as configure-system) to avoid
	// sub-path encoding issues. Fall back to running if candidate is empty.
	sysPath := candidatePath + "/ietf-system:system"
	var raw cfgAuthWrapper
	if err := h.RC.Get(r.Context(), sysPath, &raw); err != nil {
		if !restconf.IsNotFound(err) {
			log.Printf("configure users: %v", err)
			data.Error = "Could not read user configuration"
		} else if fallErr := h.RC.Get(r.Context(), "/data/ietf-system:system", &raw); fallErr != nil && !restconf.IsNotFound(fallErr) {
			// Candidate not initialised — fall back to running.
			log.Printf("configure users (running fallback): %v", fallErr)
			data.Error = "Could not read user configuration"
		}
	}
	const shellPath = "/ietf-system:system/authentication/user/infix-system:shell"
	mgr := h.Schema.Manager()
	data.Loading = mgr == nil
	if mgr != nil {
		data.ShellOptions = schema.OptionsFor(mgr, shellPath)
	}
	for _, u := range raw.System.Auth.Users {
		data.Users = append(data.Users, cfgUserDisplay{
			cfgUserJSON: u,
			ShellLabel:  schema.StripModulePrefix(u.Shell),
			KeyCount:    len(u.AuthorizedKeys),
		})
	}

	groups, err := h.fetchAllGroups(r.Context())
	if err != nil {
		log.Printf("configure users groups: %v", err)
	}
	allNames := make([]string, 0, len(data.Users))
	for _, u := range data.Users {
		allNames = append(allNames, u.Name)
	}
	for _, g := range groups {
		memberSet := make(map[string]bool, len(g.UserNames))
		for _, u := range g.UserNames {
			memberSet[u] = true
		}
		avail := make([]string, 0)
		for _, u := range allNames {
			if !memberSet[u] {
				avail = append(avail, u)
			}
		}
		summary := strings.Join(g.UserNames, ", ")
		if summary == "" {
			summary = "(none)"
		}
		data.Groups = append(data.Groups, cfgGroupDisplay{
			nacmGroupJSON:  g,
			MembersSummary: summary,
			Available:      avail,
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
		"ietf-system:user": []map[string]any{{
			"name":               name,
			"password":           hash,
			"infix-system:shell": shell,
		}},
	}
	path := authPath + "/user=" + url.PathEscape(name)
	if err := h.RC.Put(r.Context(), path, user); err != nil {
		log.Printf("configure users add %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "User added", "/configure/users")
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
	renderSavedRedirect(w, "User deleted", "/configure/users")
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
		"ietf-system:user": []map[string]any{{
			"name":               name,
			"infix-system:shell": shell,
		}},
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
		"ietf-system:user": []map[string]any{{
			"name":     name,
			"password": hash,
		}},
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

	// PATCH at the system root so libyang has full ancestor-key context.
	// Patching at user=admin or authorized-key leaves libyang without the
	// parent list-key context and produces "List requires N keys" errors.
	body := map[string]any{
		"ietf-system:system": map[string]any{
			"authentication": map[string]any{
				"user": []map[string]any{{
					"name": name,
					"authorized-key": []map[string]any{{
						"name":      keyName,
						"algorithm": algorithm,
						"key-data":  keyBytes,
					}},
				}},
			},
		},
	}
	path := candidatePath + "/ietf-system:system"
	if err := h.RC.Patch(r.Context(), path, body); err != nil {
		log.Printf("configure users key add %q/%q: %v", name, keyName, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "SSH key added", "/configure/users")
}

// DeleteKey removes an SSH authorized key for a user in the candidate.
// DELETE /configure/users/{name}/keys/{keyname}
//
// Direct DELETE to the authorized-key path fails when the key name contains
// characters like '@' that libyang interprets as module@revision syntax in path
// predicates. Work around by GET + filter + PUT at the user level instead.
func (h *ConfigureUsersHandler) DeleteKey(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	keyName := r.PathValue("keyname")

	sysPath := candidatePath + "/ietf-system:system"
	var raw cfgAuthWrapper
	if err := h.RC.Get(r.Context(), sysPath, &raw); err != nil {
		log.Printf("configure users key delete %q/%q: GET: %v", name, keyName, err)
		renderSaveError(w, err)
		return
	}

	var userEntry map[string]any
	for _, u := range raw.System.Auth.Users {
		if u.Name != name {
			continue
		}
		filteredKeys := make([]map[string]any, 0, len(u.AuthorizedKeys))
		found := false
		for _, k := range u.AuthorizedKeys {
			if k.Name == keyName {
				found = true
				continue
			}
			filteredKeys = append(filteredKeys, map[string]any{
				"name":      k.Name,
				"algorithm": k.Algorithm,
				"key-data":  k.KeyData,
			})
		}
		if !found {
			w.WriteHeader(http.StatusOK)
			return
		}
		userEntry = map[string]any{
			"name":               u.Name,
			"authorized-key":     filteredKeys,
		}
		if u.Password != "" {
			userEntry["password"] = u.Password
		}
		if u.Shell != "" {
			userEntry["infix-system:shell"] = u.Shell
		}
		break
	}
	if userEntry == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	// PUT at the user level replaces only this user's entry (including its key
	// list), which avoids the path-predicate Syntax error while not touching
	// other users or system config.
	putPath := authPath + "/user=" + url.PathEscape(name)
	body := map[string]any{
		"ietf-system:user": []map[string]any{userEntry},
	}
	if err := h.RC.Put(r.Context(), putPath, body); err != nil {
		log.Printf("configure users key delete %q/%q: PUT: %v", name, keyName, err)
		renderSaveError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// fetchAllGroups reads NACM groups from candidate, falling back to running on 404.
func (h *ConfigureUsersHandler) fetchAllGroups(ctx context.Context) ([]nacmGroupJSON, error) {
	var raw nacmWrapper
	if err := h.RC.Get(ctx, candidatePath+"/ietf-netconf-acm:nacm", &raw); err != nil {
		if !restconf.IsNotFound(err) {
			return nil, err
		}
		if err2 := h.RC.Get(ctx, "/data/ietf-netconf-acm:nacm", &raw); err2 != nil && !restconf.IsNotFound(err2) {
			return nil, err2
		}
	}
	return raw.NACM.Groups.Group, nil
}

// putGroupMembers overwrites the member list of a single NACM group.
func (h *ConfigureUsersHandler) putGroupMembers(ctx context.Context, groupName string, members []string) error {
	body := map[string]any{
		"ietf-netconf-acm:group": []map[string]any{{
			"name":      groupName,
			"user-name": members,
		}},
	}
	path := nacmGroupsPath + "/group=" + url.PathEscape(groupName)
	return h.RC.Put(ctx, path, body)
}

// AddGroupMembers adds one or more users to a NACM group, moving them out of
// any previous group to maintain the one-group-per-user invariant.
// POST /configure/users/groups/{name}/members
func (h *ConfigureUsersHandler) AddGroupMembers(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	groupName := r.PathValue("name")
	toAdd := r.Form["members"]
	if len(toAdd) == 0 {
		renderSavedRedirect(w, "No users selected", "/configure/users")
		return
	}

	groups, err := h.fetchAllGroups(r.Context())
	if err != nil {
		renderSaveError(w, err)
		return
	}

	toAddSet := make(map[string]bool, len(toAdd))
	for _, u := range toAdd {
		toAddSet[u] = true
	}

	updates := make(map[string][]string)
	var current []string
	for _, g := range groups {
		if g.Name == groupName {
			current = g.UserNames
			continue
		}
		changed := false
		members := make([]string, 0, len(g.UserNames))
		for _, u := range g.UserNames {
			if toAddSet[u] {
				changed = true
				continue
			}
			members = append(members, u)
		}
		if changed {
			updates[g.Name] = members
		}
	}

	inTarget := make(map[string]bool, len(current))
	for _, u := range current {
		inTarget[u] = true
	}
	newTarget := append([]string{}, current...)
	for _, u := range toAdd {
		if !inTarget[u] {
			newTarget = append(newTarget, u)
		}
	}
	updates[groupName] = newTarget

	for gName, members := range updates {
		if err := h.putGroupMembers(r.Context(), gName, members); err != nil {
			log.Printf("configure users groups add: write %q: %v", gName, err)
			renderSaveError(w, err)
			return
		}
	}
	renderSavedRedirect(w, "Members updated", "/configure/users")
}

// RemoveGroupMember removes a single user from a NACM group.
// DELETE /configure/users/groups/{name}/members/{user}
func (h *ConfigureUsersHandler) RemoveGroupMember(w http.ResponseWriter, r *http.Request) {
	groupName := r.PathValue("name")
	userName := r.PathValue("user")

	groups, err := h.fetchAllGroups(r.Context())
	if err != nil {
		renderSaveError(w, err)
		return
	}

	newMembers := make([]string, 0)
	found := false
	for _, g := range groups {
		if g.Name != groupName {
			continue
		}
		for _, u := range g.UserNames {
			if u == userName {
				found = true
				continue
			}
			newMembers = append(newMembers, u)
		}
		break
	}
	if !found {
		w.WriteHeader(http.StatusOK)
		return
	}

	if err := h.putGroupMembers(r.Context(), groupName, newMembers); err != nil {
		log.Printf("configure users groups remove %q/%q: %v", groupName, userName, err)
		renderSaveError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}
