// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"

	"infix/webui/internal/restconf"
	"infix/webui/internal/schema"
)

// ─── RESTCONF JSON types (candidate datastore) ────────────────────────────────

// cfgAdvancedWrapper models /ietf-system:system down to the infix-system
// advanced container; everything else in system is ignored on decode.
type cfgAdvancedWrapper struct {
	System struct {
		Advanced cfgAdvancedJSON `json:"infix-system:advanced"`
	} `json:"ietf-system:system"`
}

type cfgAdvancedJSON struct {
	RCD     cfgRCDJSON     `json:"rc.ds"`
	Default cfgDefaultJSON `json:"defaults"`
}

// cfgRCDJSON is the rc.ds container; the rc.d list is ordered-by user.
type cfgRCDJSON struct {
	Scripts []cfgAdvEntryJSON `json:"rc.d,omitempty"`
}

// cfgDefaultJSON is the defaults container; the default list is keyed by name only.
type cfgDefaultJSON struct {
	Files []cfgAdvEntryJSON `json:"default,omitempty"`
}

// cfgAdvEntryJSON is one rc.d script or one /etc/default file; both lists
// share the same leaves.  Enabled is a pointer so an unset leaf stays unset
// when an entry is written back.
type cfgAdvEntryJSON struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Enabled     *bool  `json:"enabled,omitempty"`
	Content     []byte `json:"content"` // YANG binary, base64 on the wire
}

// ─── Template data ────────────────────────────────────────────────────────────

type cfgAdvEntryView struct {
	Name        string
	Description string
	Enabled     bool
	Content     string // decoded text
}

type cfgAdvancedPageData struct {
	PageData
	Loading bool // true while YANG schema is still downloading
	Error   string
	Scripts []cfgAdvEntryView // list order == execution order
	Files   []cfgAdvEntryView
	Desc    map[string]string // leaf name -> YANG description
}

// ─── Handler ─────────────────────────────────────────────────────────────────

// ConfigureAdvancedHandler serves the Configure > Advanced page: rc.d boot
// scripts and /etc/default daemon files, backed by the candidate datastore.
type ConfigureAdvancedHandler struct {
	Template *template.Template
	RC       restconf.Fetcher
	Schema   *schema.Cache
}

const (
	advancedSchemaPath = "/ietf-system:system/infix-system:advanced"
	rcdCandPath        = candidatePath + advancedSchemaPath + "/rc.ds"
	defaultCandPath    = candidatePath + advancedSchemaPath + "/defaults"
)

func rcdScriptCandPath(name string) string {
	return rcdCandPath + "/rc.d=" + restconf.EscapeKey(name)
}

func defaultFileCandPath(name string) string {
	return defaultCandPath + "/default=" + restconf.EscapeKey(name)
}

// loadAdvanced reads the advanced container from the candidate datastore,
// falling back to running when candidate is uninitialised.  The whole
// system container is fetched so the response shape is independent of how
// deep GETs are nested by the server.
func (h *ConfigureAdvancedHandler) loadAdvanced(ctx context.Context) (cfgAdvancedJSON, error) {
	var raw cfgAdvancedWrapper
	if err := h.RC.Get(ctx, candidatePath+"/ietf-system:system", &raw); err != nil {
		if !restconf.IsNotFound(err) {
			return cfgAdvancedJSON{}, err
		}
		if fallErr := h.RC.Get(ctx, "/data/ietf-system:system", &raw); fallErr != nil && !restconf.IsNotFound(fallErr) {
			return cfgAdvancedJSON{}, fallErr
		}
	}
	return raw.System.Advanced, nil
}

// rewriteRCD applies fn to the current rc.ds container and PUTs the result
// back as a whole.  The list is ordered-by user, so replacing the container
// is the only way to make the stored order match exactly what fn produced.
func (h *ConfigureAdvancedHandler) rewriteRCD(ctx context.Context, fn func(*cfgRCDJSON) error) error {
	adv, err := h.loadAdvanced(ctx)
	if err != nil {
		return err
	}
	rcd := adv.RCD
	if err := fn(&rcd); err != nil {
		return err
	}
	return h.RC.Put(ctx, rcdCandPath, map[string]any{"infix-system:rc.ds": rcd})
}

func (h *ConfigureAdvancedHandler) render(w http.ResponseWriter, r *http.Request, data any) {
	tmplName := "configure-advanced.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func advEntryViews(entries []cfgAdvEntryJSON) []cfgAdvEntryView {
	views := make([]cfgAdvEntryView, 0, len(entries))
	for _, e := range entries {
		views = append(views, cfgAdvEntryView{
			Name:        e.Name,
			Description: e.Description,
			Enabled:     e.Enabled == nil || *e.Enabled,
			Content:     string(e.Content),
		})
	}
	return views
}

// Overview renders the Configure > Advanced page.
// GET /configure/advanced
func (h *ConfigureAdvancedHandler) Overview(w http.ResponseWriter, r *http.Request) {
	data := cfgAdvancedPageData{
		PageData: newPageData(w, r, "configure-advanced", "Advanced"),
	}

	adv, err := h.loadAdvanced(r.Context())
	if err != nil {
		log.Printf("configure advanced: %v", err)
		data.Error = "Could not read candidate configuration"
	} else {
		data.Scripts = advEntryViews(adv.RCD.Scripts)
		data.Files = advEntryViews(adv.Default.Files)
	}

	mgr := h.Schema.Manager()
	data.Loading = mgr == nil
	if mgr != nil {
		rcdPath := advancedSchemaPath + "/rc.ds"
		defPath := advancedSchemaPath + "/defaults"
		data.Desc = map[string]string{
			"rc.d":          schema.DescriptionOf(mgr, rcdPath),
			"name":          schema.DescriptionOf(mgr, rcdPath+"/rc.d/name"),
			"description":   schema.DescriptionOf(mgr, rcdPath+"/rc.d/description"),
			"s-enabled":     schema.DescriptionOf(mgr, rcdPath+"/rc.d/enabled"),
			"content":       schema.DescriptionOf(mgr, rcdPath+"/rc.d/content"),
			"default":       schema.DescriptionOf(mgr, defPath),
			"f-name":        schema.DescriptionOf(mgr, defPath+"/default/name"),
			"f-description": schema.DescriptionOf(mgr, defPath+"/default/description"),
			"f-enabled":     schema.DescriptionOf(mgr, defPath+"/default/enabled"),
			"f-content":     schema.DescriptionOf(mgr, defPath+"/default/content"),
		}
	}

	h.render(w, r, data)
}

// entryFromForm reads the shared add/edit form fields.  Content is plain
// text in the form and becomes the YANG binary leaf as-is; CRLF from the
// textarea is normalised so the file lands on the device unchanged.
func entryFromForm(r *http.Request, name string) (cfgAdvEntryJSON, error) {
	content := strings.ReplaceAll(r.FormValue("content"), "\r\n", "\n")
	if strings.TrimSpace(content) == "" {
		return cfgAdvEntryJSON{}, fmt.Errorf("content is required")
	}
	enabled := r.FormValue("enabled") == "true"
	return cfgAdvEntryJSON{
		Name:        name,
		Description: strings.TrimSpace(r.FormValue("description")),
		Enabled:     &enabled,
		Content:     []byte(content),
	}, nil
}

// ─── rc.d scripts ─────────────────────────────────────────────────────────────

// AddScript appends a script to the end of the list.
// POST /configure/advanced/rc.d/scripts
func (h *ConfigureAdvancedHandler) AddScript(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		renderSaveError(w, fmt.Errorf("script name is required"))
		return
	}
	script, err := entryFromForm(r, name)
	if err != nil {
		renderSaveError(w, err)
		return
	}
	err = h.rewriteRCD(r.Context(), func(rcd *cfgRCDJSON) error {
		for _, s := range rcd.Scripts {
			if s.Name == name {
				return fmt.Errorf("script %q already exists", name)
			}
		}
		rcd.Scripts = append(rcd.Scripts, script)
		return nil
	})
	if err != nil {
		log.Printf("configure advanced add script %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Script added", "/configure/advanced")
}

// SaveScript replaces one script in place, keeping its list position.
// POST /configure/advanced/rc.d/scripts/{name}
func (h *ConfigureAdvancedHandler) SaveScript(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	script, err := entryFromForm(r, name)
	if err != nil {
		renderSaveError(w, err)
		return
	}
	err = h.rewriteRCD(r.Context(), func(rcd *cfgRCDJSON) error {
		for i := range rcd.Scripts {
			if rcd.Scripts[i].Name == name {
				rcd.Scripts[i] = script
				return nil
			}
		}
		return fmt.Errorf("script %q not found", name)
	})
	if err != nil {
		log.Printf("configure advanced save script %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSaved(w, "Script saved")
}

// ToggleScript sets the per-script enabled leaf from the table row checkbox.
// POST /configure/advanced/rc.d/scripts/{name}/enabled
func (h *ConfigureAdvancedHandler) ToggleScript(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	enabled := r.FormValue("enabled") == "true"
	err := h.rewriteRCD(r.Context(), func(rcd *cfgRCDJSON) error {
		for i := range rcd.Scripts {
			if rcd.Scripts[i].Name == name {
				rcd.Scripts[i].Enabled = &enabled
				return nil
			}
		}
		return fmt.Errorf("script %q not found", name)
	})
	if err != nil {
		log.Printf("configure advanced toggle script %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	if enabled {
		renderSaved(w, "Script enabled")
	} else {
		renderSaved(w, "Script disabled")
	}
}

// DeleteScript removes a script.
// DELETE /configure/advanced/rc.d/scripts/{name}
func (h *ConfigureAdvancedHandler) DeleteScript(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.RC.Delete(r.Context(), rcdScriptCandPath(name)); err != nil && !restconf.IsNotFound(err) {
		log.Printf("configure advanced delete script %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Script deleted", "/configure/advanced")
}

// ReorderScripts rewrites the list in the order given by the repeated
// "order" form value, posted by the drag-and-drop table.  The submitted set
// must match the stored set exactly; otherwise nothing is written.
// POST /configure/advanced/rc.d/order
func (h *ConfigureAdvancedHandler) ReorderScripts(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	order := r.Form["order"]
	if len(order) == 0 {
		renderSaveError(w, fmt.Errorf("script order is required"))
		return
	}
	err := h.rewriteRCD(r.Context(), func(rcd *cfgRCDJSON) error {
		if len(order) != len(rcd.Scripts) {
			return fmt.Errorf("script list changed, reload the page")
		}
		byName := make(map[string]cfgAdvEntryJSON, len(rcd.Scripts))
		for _, s := range rcd.Scripts {
			byName[s.Name] = s
		}
		sorted := make([]cfgAdvEntryJSON, 0, len(order))
		for _, name := range order {
			s, ok := byName[name]
			if !ok {
				return fmt.Errorf("script list changed, reload the page")
			}
			delete(byName, name)
			sorted = append(sorted, s)
		}
		rcd.Scripts = sorted
		return nil
	})
	if err != nil {
		log.Printf("configure advanced reorder: %v", err)
		renderSaveError(w, err)
		return
	}
	renderSaved(w, "Script order saved")
}

// ─── /etc/default files ───────────────────────────────────────────────────────

// The file list is keyed by name only, so each entry is written on its own:
// PUT replaces, PATCH merges a leaf, DELETE removes.

// AddFile creates a new /etc/default file entry.
// POST /configure/advanced/default/files
func (h *ConfigureAdvancedHandler) AddFile(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		renderSaveError(w, fmt.Errorf("file name is required"))
		return
	}
	file, err := entryFromForm(r, name)
	if err != nil {
		renderSaveError(w, err)
		return
	}
	adv, err := h.loadAdvanced(r.Context())
	if err != nil {
		log.Printf("configure advanced add file %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	for _, f := range adv.Default.Files {
		if f.Name == name {
			renderSaveError(w, fmt.Errorf("file %q already exists", name))
			return
		}
	}
	body := map[string]any{"infix-system:default": []cfgAdvEntryJSON{file}}
	if err := h.RC.Put(r.Context(), defaultFileCandPath(name), body); err != nil {
		log.Printf("configure advanced add file %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "File added", "/configure/advanced")
}

// SaveFile replaces one /etc/default file entry.
// POST /configure/advanced/default/files/{name}
func (h *ConfigureAdvancedHandler) SaveFile(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	file, err := entryFromForm(r, name)
	if err != nil {
		renderSaveError(w, err)
		return
	}
	body := map[string]any{"infix-system:default": []cfgAdvEntryJSON{file}}
	if err := h.RC.Put(r.Context(), defaultFileCandPath(name), body); err != nil {
		log.Printf("configure advanced save file %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSaved(w, "File saved")
}

// ToggleFile merges the enabled leaf of one file entry.
// POST /configure/advanced/default/files/{name}/enabled
func (h *ConfigureAdvancedHandler) ToggleFile(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	enabled := r.FormValue("enabled") == "true"
	body := map[string]any{
		"infix-system:default": []map[string]any{{"name": name, "enabled": enabled}},
	}
	if err := h.RC.Patch(r.Context(), defaultFileCandPath(name), body); err != nil {
		log.Printf("configure advanced toggle file %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	if enabled {
		renderSaved(w, "File enabled")
	} else {
		renderSaved(w, "File disabled")
	}
}

// DeleteFile removes a file entry.
// DELETE /configure/advanced/default/files/{name}
func (h *ConfigureAdvancedHandler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.RC.Delete(r.Context(), defaultFileCandPath(name)); err != nil && !restconf.IsNotFound(err) {
		log.Printf("configure advanced delete file %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "File deleted", "/configure/advanced")
}
