// SPDX-License-Identifier: MIT

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"infix/webui/internal/restconf"
)

const cfgUnsavedCookie = "cfg-unsaved"

// webuiStartTime is captured when the process starts. The cfg-unsaved
// cookie carries the timestamp it was set at; cookies whose timestamp
// predates this value are from a previous boot (or a previous webui
// restart) and the running/startup state they referred to no longer
// applies — running was reloaded from startup at boot, so they're
// equal and the banner must not show. Using process-start instead of
// kernel boot keeps the check entirely in-process: a webui restart
// from configd flush also resets the marker, which matches "fresh
// running" semantics.
var webuiStartTime = time.Now()

// ConfigureHandler manages the candidate datastore lifecycle.
type ConfigureHandler struct {
	RC       restconf.Fetcher
	Template *template.Template
}

func setCfgUnsaved(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cfgUnsavedCookie,
		Value:    strconv.FormatInt(time.Now().Unix(), 10),
		Path:     "/",
		MaxAge:   86400,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearCfgUnsaved(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: cfgUnsavedCookie, Value: "", Path: "/", MaxAge: -1, SameSite: http.SameSiteLaxMode})
}

// cfgUnsavedFromRequest returns true when the cfg-unsaved cookie is
// present AND its timestamp is from this webui session. Stale cookies
// (process restart / device reboot since the cookie was set) are
// treated as absent — running has been reloaded from startup, so the
// "unsaved" condition the cookie referred to no longer holds.
func cfgUnsavedFromRequest(r *http.Request) bool {
	cookie, err := r.Cookie(cfgUnsavedCookie)
	if err != nil {
		return false
	}
	ts, perr := strconv.ParseInt(cookie.Value, 10, 64)
	if perr != nil {
		// Legacy cookie format ("1") from before the timestamp change,
		// or unparseable junk. Treat as stale.
		return false
	}
	return time.Unix(ts, 0).After(webuiStartTime)
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

// applyError surfaces a failed datastore copy from Apply / Apply&Save / Save.
// A RESTCONF error means the device answered and rejected the config — almost
// always a validation failure (e.g. deleting an interface other nodes still
// reference). The device is reachable, so report the reason inline via an
// HX-Trigger and a 200, NOT a 502: the front-end's connection monitor reads
// 502/503/504 as "device disconnected", which is exactly wrong here. Only a
// transport failure (no RESTCONF response at all) is a real gateway error.
func applyError(w http.ResponseWriter, op string, err error) {
	log.Printf("configure %s: %v", op, err)
	var re *restconf.Error
	if errors.As(err, &re) {
		// parseError always populates Message (server text, else HTTP status
		// text), so a single fallback covers the otherwise-unreachable empty.
		msg := re.Message
		if msg == "" {
			msg = "the device rejected the configuration"
		}
		b, _ := json.Marshal(msg)
		w.Header().Set("HX-Trigger", `{"cfgApplyError":`+string(b)+`}`)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Error(w, "Could not reach the device: "+err.Error(), http.StatusBadGateway)
}

// Apply copies candidate → running, activating all staged changes atomically.
// Sets the cfg-unsaved cookie so the persistent banner appears until startup is saved.
// POST /configure/apply
func (h *ConfigureHandler) Apply(w http.ResponseWriter, r *http.Request) {
	if err := h.RC.CopyDatastore(r.Context(), "candidate", "running"); err != nil {
		applyError(w, "apply", err)
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
		applyError(w, "apply-and-save", err)
		return
	}
	if err := h.RC.CopyDatastore(r.Context(), "running", "startup"); err != nil {
		applyError(w, "apply-and-save (save)", err)
		return
	}
	clearCfgUnsaved(w)
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusNoContent)
}

// configDiffLine is one classified line of unified-diff output.
type configDiffLine struct {
	Class string
	Text  string
}

// configDiffData is the template payload for the diff modal body.  Neither
// field set means the configs are identical (rendered as such by the template).
type configDiffData struct {
	Lines []configDiffLine
	Err   string
}

// ConfigDiff renders a unified diff between startup-config and running-config
// for the unsaved-changes modal.  Both datastores are fetched over RESTCONF
// (same serializer → deterministic, low-noise diff), written to temp files,
// and compared with busybox `diff -u`.  Always responds 200 with an HTML
// fragment — including on error — so the connection monitor never reads a 5xx
// as "device disconnected" (same reasoning as applyError).
// GET /configure/diff
func (h *ConfigureHandler) ConfigDiff(w http.ResponseWriter, r *http.Request) {
	var data configDiffData
	out, err := h.configDiff(r.Context())
	switch {
	case err != nil:
		log.Printf("configure diff: %v", err)
		data.Err = err.Error()
	case strings.TrimSpace(out) != "":
		data.Lines = classifyDiff(out)
	}
	if err := h.Template.ExecuteTemplate(w, "config-diff.html", data); err != nil {
		log.Printf("configure diff: render: %v", err)
	}
}

// configDiff fetches startup and running config, writes each to a temp file,
// and returns the `diff -u` output (empty when the two are identical).
func (h *ConfigureHandler) configDiff(ctx context.Context) (string, error) {
	startup, err := h.RC.GetDatastore(ctx, "startup")
	if err != nil {
		return "", fmt.Errorf("read startup-config: %w", err)
	}
	running, err := h.RC.GetDatastore(ctx, "running")
	if err != nil {
		return "", fmt.Errorf("read running-config: %w", err)
	}

	startupFile, err := writeTempConfig("cfgdiff-startup-*.json", startup)
	if err != nil {
		return "", err
	}
	defer os.Remove(startupFile)
	runningFile, err := writeTempConfig("cfgdiff-running-*.json", running)
	if err != nil {
		return "", err
	}
	defer os.Remove(runningFile)

	var stdout, stderr bytes.Buffer
	// busybox diff supports -L (not --label); repeat it for each file.
	cmd := exec.CommandContext(ctx, "diff", "-u",
		"-L", "startup-config", "-L", "running-config", startupFile, runningFile)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	// diff exit codes: 0 = identical, 1 = differences (normal), >1 = error.
	var exit *exec.ExitError
	if err != nil && (!errors.As(err, &exit) || exit.ExitCode() > 1) {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("diff failed: %s", msg)
	}
	return stdout.String(), nil
}

// writeTempConfig writes data to a fresh 0600 temp file and returns its path.
func writeTempConfig(pattern string, data []byte) (string, error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// classifyDiff tags each unified-diff line with a CSS class for colorization.
func classifyDiff(out string) []configDiffLine {
	raw := strings.Split(strings.TrimRight(out, "\n"), "\n")
	lines := make([]configDiffLine, 0, len(raw))
	for _, ln := range raw {
		class := "diff-ctx"
		switch {
		case strings.HasPrefix(ln, "+++"), strings.HasPrefix(ln, "---"):
			class = "diff-meta"
		case strings.HasPrefix(ln, "@@"):
			class = "diff-hunk"
		case strings.HasPrefix(ln, "+"):
			class = "diff-add"
		case strings.HasPrefix(ln, "-"):
			class = "diff-del"
		}
		lines = append(lines, configDiffLine{Class: class, Text: ln})
	}
	return lines
}

// DeleteLeaf removes a single leaf from the candidate datastore so the YANG
// default takes effect. Used by curated-page reset buttons.
// DELETE /configure/leaf?path=...&redirect=...
func (h *ConfigureHandler) DeleteLeaf(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	redirect := r.URL.Query().Get("redirect")
	if path == "" || redirect == "" {
		http.Error(w, "path and redirect required", http.StatusBadRequest)
		return
	}
	// Swallow data-missing: the leaf was already absent, so the reset
	// semantically succeeded — there was nothing left to remove.
	if err := h.RC.Delete(r.Context(), candidatePath+path); err != nil && !restconf.IsDataMissing(err) {
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Reset to default", redirect)
}

// Save copies running → startup, persisting the active configuration.
// Clears the cfg-unsaved cookie and does a full-page refresh so the banner disappears.
// POST /configure/save
func (h *ConfigureHandler) Save(w http.ResponseWriter, r *http.Request) {
	if err := h.RC.CopyDatastore(r.Context(), "running", "startup"); err != nil {
		applyError(w, "save", err)
		return
	}
	clearCfgUnsaved(w)
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusNoContent)
}
