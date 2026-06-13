// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"

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

// pageContext returns the top-level nav group ("Status", "Configure",
// "Maintenance") for a given ActivePage slug.  Used to build breadcrumb-style
// browser-tab titles ("Page · Context") without each handler having to know
// where it lives in the sidebar.
func pageContext(page string) string {
	switch page {
	case "software", "logs", "backup", "system-control":
		return "Maintenance"
	}
	if strings.HasPrefix(page, "configure-") {
		return "Configure"
	}
	return "Status"
}

func newPageData(w http.ResponseWriter, r *http.Request, page, leaf string) PageData {
	title := leaf
	if ctx := pageContext(page); ctx != "" {
		if leaf == "" {
			title = ctx
		} else {
			title = leaf + " · " + ctx
		}
	}

	// On HTMX swaps only #content is replaced, leaving the <title> element in
	// <head> stale.  Fire a setPageTitle event so the JS listener in app.js
	// can update document.title.  Safe to overwrite any prior HX-Trigger
	// header: only GET handlers reach newPageData, and those don't share
	// response paths with the save-side helpers (renderSaved /
	// renderSaveError) that also use HX-Trigger.
	// strconv.QuoteToASCII escapes non-ASCII as \uXXXX so the header value
	// survives transit as 7-bit ASCII; browsers decode header bytes as
	// ISO-8859-1, which would otherwise turn our middle-dot separator into
	// mojibake on the JS side.
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Trigger", `{"setPageTitle":`+strconv.QuoteToASCII(title)+`}`)
	}
	return PageData{
		Username:     restconf.CredentialsFromContext(r.Context()).Username,
		CsrfToken:    csrfToken(r.Context()),
		PageTitle:    title,
		ActivePage:   page,
		Capabilities: CapabilitiesFromContext(r.Context()),
		CfgUnsaved:   cfgUnsavedFromRequest(r),
	}
}
