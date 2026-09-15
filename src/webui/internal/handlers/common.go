// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"encoding/json"
	"infix/webui/internal/restconf"
	"infix/webui/internal/security"
	"unicode/utf16"
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
	case "software", "logs", "diagnostics", "backup", "system-control":
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
	if r.Header.Get("HX-Request") == "true" {
		hxTrigger(w, "setPageTitle", title)
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

// IfaceTemplateFuncs is the FuncMap the configure-interfaces template is
// parsed with. Exported so tests can parse the real template the same way.
func IfaceTemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"shortPMD": ShortenPMD,
		"add":      func(a, b int) int { return a + b },
		"deref": func(v any) any {
			switch p := v.(type) {
			case *bool:
				if p != nil {
					return *p
				}
			case *uint32:
				if p != nil {
					return *p
				}
			case *int:
				if p != nil {
					return *p
				}
			}
			return nil
		},
		// dict lets callers pass keyed args to nested templates, e.g.
		// {{template "foo" (dict "Key" .X "Selected" "")}}.
		"dict": func(values ...any) (map[string]any, error) {
			if len(values)%2 != 0 {
				return nil, fmt.Errorf("dict: odd argument count")
			}
			m := make(map[string]any, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				k, ok := values[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict: non-string key at position %d", i)
				}
				m[k] = values[i+1]
			}
			return m, nil
		},
	}
}

// hxTrigger sets an HX-Trigger header that fires event with a string
// detail. The JSON is escaped to 7-bit ASCII: browsers decode header
// bytes as ISO-8859-1, so raw UTF-8 (an en dash in a range, a middle dot
// in a title) arrives as mojibake in the event.
func hxTrigger(w http.ResponseWriter, event, value string) {
	w.Header().Set("HX-Trigger", `{"`+event+`":`+asciiJSON(value)+`}`)
}

func asciiJSON(s string) string {
	b, _ := json.Marshal(s)
	var sb strings.Builder
	for _, r := range string(b) {
		switch {
		case r < 0x80:
			sb.WriteRune(r)
		case r > 0xFFFF:
			hi, lo := utf16.EncodeRune(r)
			fmt.Fprintf(&sb, `\u%04x\u%04x`, hi, lo)
		default:
			fmt.Fprintf(&sb, `\u%04x`, r)
		}
	}
	return sb.String()
}
