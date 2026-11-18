// SPDX-License-Identifier: MIT

package handlers

import (
	"encoding/json"
	"net/http"

	"infix/webui/internal/restconf"
	"infix/webui/internal/security"
)

// Session serves GET /api/session, the logged-in user and the CSRF
// token.  A page served outside this binary has no template to get the
// token from, and needs it to post to /logout.
func Session(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"username": restconf.CredentialsFromContext(r.Context()).Username,
		"csrf":     security.TokenFromContext(r.Context()),
	})
}
