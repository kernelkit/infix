package main

import (
	"html/template"
	"io/fs"
	"strings"
	"testing"

	"infix/webui/internal/handlers"
	"infix/webui/internal/schema"
)

// Every page must parse together with the layouts and fragments, as
// server.New loads them, so an unbalanced action fails here rather than
// at daemon start.
func TestTemplatesParse(t *testing.T) {
	templates, err := fs.Sub(templateFS, "templates")
	if err != nil {
		t.Fatal(err)
	}
	pages, err := fs.Glob(templates, "pages/*.html")
	if err != nil || len(pages) == 0 {
		t.Fatalf("no pages found: %v", err)
	}
	funcs := handlers.IfaceTemplateFuncs()
	funcs["stripPrefix"] = schema.StripModulePrefix
	for _, page := range pages {
		patterns := []string{"layouts/*.html", "fragments/*.html", page}
		if strings.HasSuffix(page, "/login.html") {
			patterns = []string{"layouts/icons.html", page}
		}
		if _, err := template.New("").Funcs(funcs).ParseFS(templates, patterns...); err != nil {
			t.Errorf("%s: %v", page, err)
		}
	}
}
