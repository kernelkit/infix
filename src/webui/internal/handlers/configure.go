// SPDX-License-Identifier: MIT

package handlers

import (
	"log"
	"net/http"

	"github.com/kernelkit/webui/internal/restconf"
)

const cfgUnsavedCookie = "cfg-unsaved"

// ConfigureHandler manages the candidate datastore lifecycle.
type ConfigureHandler struct {
	RC restconf.Fetcher
}

func setCfgUnsaved(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: cfgUnsavedCookie, Value: "1", Path: "/", MaxAge: 86400, SameSite: http.SameSiteLaxMode})
}

func clearCfgUnsaved(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: cfgUnsavedCookie, Value: "", Path: "/", MaxAge: -1, SameSite: http.SameSiteLaxMode})
}

// Enter copies running → candidate, initialising a fresh edit session.
// Called when the user opens the Configure accordion.
// POST /configure/enter
func (h *ConfigureHandler) Enter(w http.ResponseWriter, r *http.Request) {
	if err := h.RC.CopyDatastore(r.Context(), "running", "candidate"); err != nil {
		log.Printf("configure enter: %v", err)
		http.Error(w, "Could not initialise candidate datastore", http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Apply copies candidate → running, activating all staged changes atomically.
// Sets the cfg-unsaved cookie so the persistent banner appears until startup is saved.
// POST /configure/apply
func (h *ConfigureHandler) Apply(w http.ResponseWriter, r *http.Request) {
	if err := h.RC.CopyDatastore(r.Context(), "candidate", "running"); err != nil {
		log.Printf("configure apply: %v", err)
		http.Error(w, "Could not apply configuration: "+err.Error(), http.StatusBadGateway)
		return
	}
	setCfgUnsaved(w)
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusNoContent)
}

// Abort copies running → candidate, discarding all staged changes.
// POST /configure/abort
func (h *ConfigureHandler) Abort(w http.ResponseWriter, r *http.Request) {
	if err := h.RC.CopyDatastore(r.Context(), "running", "candidate"); err != nil {
		log.Printf("configure abort: %v", err)
		// Best-effort reset; refresh regardless.
	}
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusNoContent)
}

// ApplyAndSave copies candidate → running then running → startup in one step.
// Clears the cfg-unsaved cookie.
// POST /configure/apply-and-save
func (h *ConfigureHandler) ApplyAndSave(w http.ResponseWriter, r *http.Request) {
	if err := h.RC.CopyDatastore(r.Context(), "candidate", "running"); err != nil {
		log.Printf("configure apply-and-save: %v", err)
		http.Error(w, "Could not apply configuration: "+err.Error(), http.StatusBadGateway)
		return
	}
	if err := h.RC.CopyDatastore(r.Context(), "running", "startup"); err != nil {
		log.Printf("configure apply-and-save (save): %v", err)
		http.Error(w, "Could not save configuration: "+err.Error(), http.StatusBadGateway)
		return
	}
	clearCfgUnsaved(w)
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusNoContent)
}

// Save copies running → startup, persisting the active configuration.
// Clears the cfg-unsaved cookie and does a full-page refresh so the banner disappears.
// POST /configure/save
func (h *ConfigureHandler) Save(w http.ResponseWriter, r *http.Request) {
	if err := h.RC.CopyDatastore(r.Context(), "running", "startup"); err != nil {
		log.Printf("configure save: %v", err)
		http.Error(w, "Could not save configuration: "+err.Error(), http.StatusBadGateway)
		return
	}
	clearCfgUnsaved(w)
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusNoContent)
}
