// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"net/http"

	"github.com/kernelkit/webui/internal/restconf"
	"github.com/kernelkit/webui/internal/security"
)

// PageData is the base template data passed to every page.
type PageData struct {
	Username     string
	CsrfToken    string
	PageTitle    string
	ActivePage   string
	Capabilities *Capabilities
	CfgUnsaved   bool // running config differs from startup (Apply was used without ApplyAndSave)
}

func csrfToken(ctx context.Context) string {
	return security.TokenFromContext(ctx)
}

func newPageData(r *http.Request, page, title string) PageData {
	_, cookieErr := r.Cookie(cfgUnsavedCookie)
	return PageData{
		Username:     restconf.CredentialsFromContext(r.Context()).Username,
		CsrfToken:    csrfToken(r.Context()),
		PageTitle:    title,
		ActivePage:   page,
		Capabilities: CapabilitiesFromContext(r.Context()),
		CfgUnsaved:   cookieErr == nil,
	}
}
