// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"net/http"

	"infix/webui/internal/restconf"
	"infix/webui/internal/security"
)

// PageData is the base template data passed to every page.
type PageData struct {
	Username     string
	CsrfToken    string
	PageTitle    string
	ActivePage   string
	Capabilities *Capabilities
	CfgUnsaved   bool // running config differs from startup (Apply was used without ApplyAndSave)
	// RetryAfter, when > 0, renders a <meta http-equiv="refresh">
	// in the page head with that many seconds. Used by transient
	// fetch failures (e.g. post-upgrade dashboard fetch racing
	// yanger / sysrepo startup) to self-recover without the user
	// having to remember to reload.
	RetryAfter int
}

func csrfToken(ctx context.Context) string {
	return security.TokenFromContext(ctx)
}

func newPageData(r *http.Request, page, title string) PageData {
	return PageData{
		Username:     restconf.CredentialsFromContext(r.Context()).Username,
		CsrfToken:    csrfToken(r.Context()),
		PageTitle:    title,
		ActivePage:   page,
		Capabilities: CapabilitiesFromContext(r.Context()),
		CfgUnsaved:   cfgUnsavedFromRequest(r),
	}
}
