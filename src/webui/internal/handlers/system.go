// SPDX-License-Identifier: MIT

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"infix/webui/internal/restconf"
)

// raucInstallationStatus reads RAUC's Operation/Progress/LastError D-Bus
// properties directly via the rauc-installation-status helper. Used during
// installs because the RESTCONF path goes through the operational-state
// machinery, which runs `rauc status` and blocks while RAUC is busy.
func raucInstallationStatus(ctx context.Context) (swInstallerState, error) {
	var inst swInstallerState
	out, err := exec.CommandContext(ctx, "/usr/bin/rauc-installation-status").Output()
	if err != nil {
		return inst, err
	}
	err = json.Unmarshal(out, &inst)
	return inst, err
}

// SystemHandler provides reboot, config download, and software install actions.
type SystemHandler struct {
	RC           *restconf.Client
	Template     *template.Template // software page template
	SysCtrlTmpl  *template.Template // system control page template
	BackupTmpl   *template.Template // backup & restore page template
	IdentityTmpl *template.Template // topbar device-identity fragment

	// swSlots caches the last successfully-fetched Software card payload
	// so /software?installing=1 can keep rendering slot details — the
	// RESTCONF path blocks on `rauc status` while RAUC is busy, and the
	// user wants to read (and adjust) boot order even between install
	// attempts.
	swSlots swSlotSnapshot
}

// swSlotSnapshot is a tiny RWMutex-guarded copy of the Software page's
// card body. Mirrors the schema.Cache shape (rw lock + payload) from
// internal/schema/refresh.go.
type swSlotSnapshot struct {
	mu        sync.RWMutex
	bootOrder []string
	slots     []slotEntry
}

// raucBusy reports whether RAUC has an install in flight. Best-effort:
// helper-binary failures fall through to "not busy" so the caller's
// pre-flight check doesn't false-positive on an unrelated D-Bus glitch.
func (h *SystemHandler) raucBusy(ctx context.Context) bool {
	inst, err := raucInstallationStatus(ctx)
	if err != nil {
		return false
	}
	return !inst.IsIdle()
}

// Reboot triggers a device restart via the ietf-system:system-restart RPC
// and returns a spinner fragment that polls until the device is back.
func (h *SystemHandler) Reboot(w http.ResponseWriter, r *http.Request) {
	err := h.RC.Post(r.Context(), "/operations/ietf-system:system-restart")
	if err != nil {
		log.Printf("reboot: %v", err)
		http.Error(w, "reboot failed", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, rebootSpinnerHTML)
}

const rebootSpinnerHTML = `<div class="reboot-overlay">
  <div class="reboot-spinner"></div>
  <p class="reboot-message">Rebooting&hellip;</p>
  <p class="reboot-status" id="reboot-status">Waiting for device to shut down&hellip;</p>
</div>`

type systemControlData struct {
	PageData
}

// SystemControl renders the System Control maintenance page.
func (h *SystemHandler) SystemControl(w http.ResponseWriter, r *http.Request) {
	data := systemControlData{
		PageData: newPageData(w, r, "system-control", "System Control"),
	}

	tmplName := "system-control.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.SysCtrlTmpl.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("system-control template: %v", err)
	}
}

// SetDatetime sets the device clock via the ietf-system:set-current-datetime RPC.
// The form value is a datetime-local string (YYYY-MM-DDTHH:MM) treated as UTC.
func (h *SystemHandler) SetDatetime(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	raw := r.FormValue("datetime") // YYYY-MM-DDTHH:MM or :SS, ISO 24h
	if raw == "" {
		http.Error(w, "datetime required", http.StatusBadRequest)
		return
	}
	// Accept either YYYY-MM-DDTHH:MM (16 chars) or with :SS (19 chars).
	if len(raw) == 16 {
		raw += ":00"
	}

	body := map[string]map[string]string{
		"ietf-system:input": {"current-datetime": raw + "+00:00"},
	}
	if err := h.RC.PostJSON(r.Context(), "/operations/ietf-system:set-current-datetime", body); err != nil {
		msg := err.Error()
		if strings.Contains(msg, "ntp-active") {
			msg = "NTP is active — disable NTP first under Configure > System"
		}
		log.Printf("set datetime: %v", err)
		b, _ := json.Marshal(msg)
		w.Header().Set("HX-Trigger", `{"cfgError":`+string(b)+`}`)
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("HX-Trigger", `{"cfgSaved":"System time updated"}`)
	w.WriteHeader(http.StatusOK)
}

// Shutdown triggers a device power-off via the ietf-system:system-shutdown RPC.
func (h *SystemHandler) Shutdown(w http.ResponseWriter, r *http.Request) {
	if err := h.RC.Post(r.Context(), "/operations/ietf-system:system-shutdown"); err != nil {
		log.Printf("shutdown: %v", err)
		http.Error(w, "shutdown failed", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, shutdownHTML)
}

// shutdownHTML is the overlay shown after a shutdown RPC succeeds.
// Uses .reboot-overlay so the 60 s hard-cap redirect fires (session is dead anyway),
// but omits #reboot-status so the JS does not update the message to "coming back…".
const shutdownHTML = `<div class="reboot-overlay">
  <div class="reboot-spinner"></div>
  <p class="reboot-message">Shutting down&hellip;</p>
  <p class="reboot-status">The device is powering off.</p>
</div>`

// FactoryDefault resets the running datastore to factory defaults without rebooting.
// Uses the infix-factory-default:factory-default RPC.
func (h *SystemHandler) FactoryDefault(w http.ResponseWriter, r *http.Request) {
	if err := h.RC.Post(r.Context(), "/operations/infix-factory-default:factory-default"); err != nil {
		log.Printf("factory-default: %v", err)
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<span class="sc-fd-err">Failed: %s</span>`,
			template.HTMLEscapeString(err.Error()))
		return
	}
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, `<span class="sc-fd-ok">&#10003; Running config reset to factory defaults</span>`)
}

// FactoryReset wipes all datastores and non-volatile storage, then reboots.
// Uses the ietf-factory-default:factory-reset RPC.
func (h *SystemHandler) FactoryReset(w http.ResponseWriter, r *http.Request) {
	if err := h.RC.Post(r.Context(), "/operations/ietf-factory-default:factory-reset"); err != nil {
		log.Printf("factory-reset: %v", err)
		http.Error(w, "factory reset failed", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, factoryResetSpinnerHTML)
}

const factoryResetSpinnerHTML = `<div class="reboot-overlay">
  <div class="reboot-spinner"></div>
  <p class="reboot-message">Factory reset in progress&hellip;</p>
  <p class="reboot-status" id="reboot-status">Wiping configuration and rebooting&hellip;</p>
</div>`

// Backup renders the Backup & Restore maintenance page.
func (h *SystemHandler) Backup(w http.ResponseWriter, r *http.Request) {
	data := newPageData(w, r, "backup", "Backup & Support")
	tmplName := "backup.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.BackupTmpl.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("backup template: %v", err)
	}
}

// SupportBundle runs the on-device `support collect` tool and streams the
// resulting archive back as a download.  The WebUI runs as root, so the
// collection is complete (dmesg, ethtool, etc.).  An optional password
// encrypts the archive via the tool's GPG support and is fed on stdin so
// it never lands in the process list.
//
// Collection emits nothing on stdout until it finishes (~50 s), then the
// whole archive at once.  We buffer it and only commit response headers
// once the tool exits successfully, so a mid-collection failure becomes a
// clean 500 rather than a truncated download.  --work-dir /tmp keeps the
// transient files in tmpfs; the tool cleans up after itself.
// POST /maintenance/support-bundle
func (h *SystemHandler) SupportBundle(w http.ResponseWriter, r *http.Request) {
	// Collection blocks ~50 s with no output, but the server's 15 s
	// WriteTimeout would close the connection long before then (nginx
	// then logs a 502 "upstream prematurely closed connection").  Push
	// the write deadline out for this long-running download.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(4 * time.Minute)); err != nil {
		log.Printf("support bundle: extend write deadline: %v", err)
	}

	password := r.FormValue("password")
	encrypt := password != ""

	args := []string{"--work-dir", "/tmp", "collect"}
	ext, ctype := "tar.gz", "application/gzip"
	if encrypt {
		args = append(args, "-p")
		ext, ctype = "tar.gz.gpg", "application/pgp-encrypted"
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/usr/sbin/support", args...)
	if encrypt {
		cmd.Stdin = strings.NewReader(password + "\n")
	}
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = strings.TrimSpace(string(ee.Stderr))
		}
		log.Printf("support bundle: %v: %s", err, stderr)
		http.Error(w, "Failed to collect support bundle", http.StatusInternalServerError)
		return
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "device"
	}
	fname := fmt.Sprintf("support-%s-%s.%s", hostname, time.Now().UTC().Format("20060102-1504"), ext)

	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fname))
	w.Header().Set("Content-Length", fmt.Sprint(len(out)))
	w.Write(out) //nolint:errcheck
}

// RestoreConfig accepts a multipart-uploaded JSON config file and applies it.
// target="running" (default): PUT to running so changes take effect immediately;
// sets the cfg-unsaved cookie so the persistent notification prompts a save.
// target="startup": PUT to startup only; reboot required to apply.
// migrateConfig runs an uploaded .cfg backup through the on-target migrate(1)
// tool so a config saved by an older release is brought up to the running
// syntax before it is applied. The config is written to a temp file and
// migrated in place — migrate only reads stdin from a TTY, so the file path is
// the reliable interface. A config already at the current version comes back
// unchanged. An error carrying migrate's stderr is returned for a config newer
// than the system supports (downgrade) or any other migrate failure.
func migrateConfig(ctx context.Context, raw []byte) ([]byte, error) {
	f, err := os.CreateTemp("", "restore-*.cfg")
	if err != nil {
		return nil, err
	}
	tmp := f.Name()
	defer os.Remove(tmp)

	if _, err := f.Write(raw); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "/usr/sbin/migrate", "-i", "-e", tmp)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		msg = strings.ReplaceAll(msg, tmp+": ", "")
		msg = strings.TrimPrefix(msg, "Error: ")
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("%s", msg)
	}

	if notes := strings.TrimSpace(stderr.String()); notes != "" {
		log.Printf("restore: migrated uploaded config:\n%s", notes)
	}

	return os.ReadFile(tmp)
}

// Identity renders the topbar device-identity widget — hostname, plus an
// optional location/contact hover popover. It is loaded asynchronously via
// hx-trigger="load" so it stays out of the per-page data path; the topbar
// persists across htmx content swaps, so this fetches once per full page load.
// On any fetch error it renders nothing, leaving the topbar slot empty.
func (h *SystemHandler) Identity(w http.ResponseWriter, r *http.Request) {
	var resp struct {
		System struct {
			Hostname string `json:"hostname"`
			Location string `json:"location"`
			Contact  string `json:"contact"`
		} `json:"ietf-system:system"`
	}
	if err := h.RC.Get(r.Context(), "/data/ietf-system:system", &resp); err != nil {
		log.Printf("identity: %v", err)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.IdentityTmpl.ExecuteTemplate(w, "topbar-identity", resp.System); err != nil {
		log.Printf("identity: render: %v", err)
	}
}

func (h *SystemHandler) RestoreConfig(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			r.MultipartForm.RemoveAll()
		}
	}()

	file, _, err := r.FormFile("config")
	if err != nil {
		http.Error(w, "config file required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	raw, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "read error", http.StatusInternalServerError)
		return
	}

	var check json.RawMessage
	if err := json.Unmarshal(raw, &check); err != nil {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<span class="sc-fd-err">Invalid JSON file.</span>`)
		return
	}

	// Bring an older backup up to the running syntax before applying it.
	migrated, err := migrateConfig(r.Context(), raw)
	if err != nil {
		log.Printf("restore: migrate: %v", err)
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<span class="sc-fd-err">Restore failed: %s</span>`,
			template.HTMLEscapeString(err.Error()))
		return
	}

	target := "running"
	if r.FormValue("save-to-startup") == "on" {
		target = "startup"
	}

	if err := h.RC.PutDatastore(r.Context(), target, json.RawMessage(migrated)); err != nil {
		log.Printf("restore(%s): %v", target, err)
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<span class="sc-fd-err">Restore failed: %s</span>`,
			template.HTMLEscapeString(err.Error()))
		return
	}

	if target == "running" {
		setCfgUnsaved(w)
		w.Header().Set("HX-Refresh", "true")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, `<span class="sc-fd-ok">&#10003; Startup configuration restored. Reboot to apply.</span>`)
}

// DownloadConfig serves the startup datastore as a JSON file download.
// Filename includes the device hostname and current UTC date+time.
func (h *SystemHandler) DownloadConfig(w http.ResponseWriter, r *http.Request) {
	data, err := h.RC.GetRaw(r.Context(), "/ds/ietf-datastores:startup")
	if err != nil {
		log.Printf("config download: %v", err)
		http.Error(w, "failed to fetch config", http.StatusBadGateway)
		return
	}

	hostname := "device"
	var sysResp struct {
		System struct {
			Hostname string `json:"hostname"`
		} `json:"ietf-system:system"`
	}
	if err := h.RC.Get(r.Context(), "/data/ietf-system:system", &sysResp); err == nil {
		if hn := sysResp.System.Hostname; hn != "" {
			hostname = hn
		}
	}
	ts := time.Now().UTC().Format("20060102-1504")
	filename := fmt.Sprintf("startup-config-%s-%s.cfg", hostname, ts)

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, filename))
	w.Write(data)
}

// RESTCONF JSON structures for infix-system:software state.

type swStateWrapper struct {
	SystemState struct {
		Software swState `json:"infix-system:software"`
	} `json:"ietf-system:system-state"`
}

type swState struct {
	Compatible string           `json:"compatible"`
	Variant    string           `json:"variant"`
	Booted     string           `json:"booted"`
	BootOrder  []string         `json:"boot-order"`
	Installer  swInstallerState `json:"installer"`
	Slots      []swSlot         `json:"slot"`
}

type swInstallerState struct {
	Operation string              `json:"operation"`
	Progress  swInstallerProgress `json:"progress"`
	LastError string              `json:"last-error"`
}

// IsIdle reports whether RAUC has no install in flight. The YANG model leaves
// Operation empty when no install has run yet and "idle" once one has completed.
func (s swInstallerState) IsIdle() bool {
	return s.Operation == "" || s.Operation == "idle"
}

type swInstallerProgress struct {
	Percentage int    `json:"percentage"`
	Message    string `json:"message"`
}

type swSlot struct {
	Name      string       `json:"name"`
	BootName  string       `json:"bootname"`
	Class     string       `json:"class"`
	State     string       `json:"state"`
	Bundle    swSlotBundle `json:"bundle"`
	Installed struct {
		Datetime string `json:"datetime"`
	} `json:"installed"`
}

type swSlotBundle struct {
	Compatible string `json:"compatible"`
	Version    string `json:"version"`
}

// Template data for the software page.

type softwareData struct {
	PageData
	BootOrder    []string
	Slots        []slotEntry
	Installer    *installerEntry
	Installing   bool // install was triggered this session; keep card visible during RAUC phase gaps
	AutoReboot   bool
	Error        string
	Message      string
}

type slotEntry struct {
	Name        string // bootname: primary, secondary, etc.
	State       string
	Version     string
	InstallDate string
	Booted      bool
}

type installerEntry struct {
	Operation  string
	Percentage int
	Message    string
	LastError  string
	Active     bool
	Done       bool // idle after an install ran (percentage>0 or error set)
	Success    bool // Done with no error
}

// Software renders the software overview page (GET /software).
func (h *SystemHandler) Software(w http.ResponseWriter, r *http.Request) {
	data := softwareData{
		PageData:   newPageData(w, r, "software", "Software"),
		Message:    r.URL.Query().Get("msg"),
		Installing: r.URL.Query().Get("installing") == "1",
		AutoReboot: r.URL.Query().Get("auto-reboot") == "1",
	}

	// When an install is in progress, the RESTCONF path blocks on
	// `rauc status` until RAUC is done. Skip the slow path and read
	// the installer state directly so the progress card can render
	// immediately; SSE then drives the visual update during the
	// install. The Software card body falls back to the last cached
	// slot snapshot so it doesn't go blank.
	if data.Installing {
		if inst, err := raucInstallationStatus(r.Context()); err == nil {
			data.Installer = newInstallerEntry(inst)
		} else {
			log.Printf("software page (installing): %v", err)
		}
		h.swSlots.mu.RLock()
		data.BootOrder = slices.Clone(h.swSlots.bootOrder)
		data.Slots = slices.Clone(h.swSlots.slots)
		h.swSlots.mu.RUnlock()
	} else {
		var sw swStateWrapper
		err := h.RC.Get(r.Context(), "/data/ietf-system:system-state", &sw)
		if err != nil {
			log.Printf("restconf software: %v", err)
			data.Error = "Could not fetch software status"
		} else {
			data.BootOrder = sw.SystemState.Software.BootOrder
			for _, s := range sw.SystemState.Software.Slots {
				if s.Class != "rootfs" {
					continue
				}
				name := s.BootName
				if name == "" {
					name = s.Name
				}
				date := s.Installed.Datetime
				if len(date) > 19 {
					date = date[:19]
				}
				data.Slots = append(data.Slots, slotEntry{
					Name:        name,
					State:       s.State,
					Version:     s.Bundle.Version,
					InstallDate: date,
					Booted:      s.BootName == sw.SystemState.Software.Booted,
				})
			}

			data.Installer = newInstallerEntry(sw.SystemState.Software.Installer)

			h.swSlots.mu.Lock()
			h.swSlots.bootOrder = slices.Clone(data.BootOrder)
			h.swSlots.slots = slices.Clone(data.Slots)
			h.swSlots.mu.Unlock()
		}
	}

	tmplName := "software.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// SetBootOrder calls the infix-system:set-boot-order RPC with the ordered
// boot-order form values submitted by the boot order card.
// On success it refreshes the page so the Software card badges update.
func (h *SystemHandler) SetBootOrder(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	order := r.Form["boot-order"]
	if len(order) == 0 || len(order) > 3 {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<span class="sc-fd-err">Invalid boot order.</span>`)
		return
	}
	body := map[string]any{
		"infix-system:input": map[string]any{
			"boot-order": order,
		},
	}
	if err := h.RC.PostJSON(r.Context(), "/operations/infix-system:set-boot-order", body); err != nil {
		log.Printf("set-boot-order: %v", err)
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<span class="sc-fd-err">Failed: %s</span>`,
			template.HTMLEscapeString(err.Error()))
		return
	}
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusNoContent)
}

// SoftwareUpload accepts a .pkg file upload, saves it to a temp file, and
// kicks off the install-bundle RPC asynchronously so the response (a
// plain-text redirect target) reaches the browser before RAUC starts
// writing slots. Upload size is capped at the nginx layer.
func (h *SystemHandler) SoftwareUpload(w http.ResponseWriter, r *http.Request) {
	if h.raucBusy(r.Context()) {
		http.Error(w, "software install already in progress", http.StatusConflict)
		return
	}

	// 1 MiB in-RAM threshold; larger parts spill to $TMPDIR (/var/tmp on
	// the target, eMMC-backed) instead of the RAM-backed /tmp.
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			r.MultipartForm.RemoveAll()
		}
	}()

	file, _, err := r.FormFile("pkg")
	if err != nil {
		http.Error(w, "pkg file required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	tmp, err := os.CreateTemp("", "webui-bundle-*.pkg")
	if err != nil {
		log.Printf("software upload: create temp: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	tmpPath := tmp.Name()

	if _, err := io.Copy(tmp, file); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		log.Printf("software upload: write: %v", err)
		http.Error(w, "failed to save bundle", http.StatusInternalServerError)
		return
	}
	tmp.Close()

	body := map[string]map[string]string{
		"infix-system:input": {"url": tmpPath},
	}
	creds := restconf.CredentialsFromContext(r.Context())
	go h.runInstall(creds, body, tmpPath)

	target := "/software?installing=1"
	if r.FormValue("auto-reboot") == "1" {
		target += "&auto-reboot=1"
	}
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, target)
}

// runInstall fires the install-bundle RPC and then waits for RAUC to finish
// before deleting the uploaded .pkg. The 30-minute outer timeout is the
// safety net for a stuck RAUC.
func (h *SystemHandler) runInstall(creds restconf.Credentials, body any, tmpPath string) {
	ctx, cancel := context.WithTimeout(
		restconf.ContextWithCredentials(context.Background(), creds),
		30*time.Minute,
	)
	defer cancel()
	defer os.Remove(tmpPath)

	if err := h.RC.PostJSON(ctx, "/operations/infix-system:install-bundle", body); err != nil {
		log.Printf("software upload: install-bundle: %v", err)
		return
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			inst, err := raucInstallationStatus(ctx)
			if err != nil {
				continue
			}
			if inst.IsIdle() {
				return
			}
		}
	}
}

// SoftwareInstall triggers a bundle install via the install-bundle RPC (POST /software/install).
func (h *SystemHandler) SoftwareInstall(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	url := r.FormValue("url")
	if url == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}

	body := map[string]map[string]string{
		"infix-system:input": {
			"url": url,
		},
	}

	err := h.RC.PostJSON(r.Context(), "/operations/infix-system:install-bundle", body)
	if err != nil {
		log.Printf("software install: %v", err)
		w.Header().Set("HX-Redirect", "/software?msg=Install+failed:+"+err.Error())
		w.WriteHeader(http.StatusNoContent)
		return
	}

	target := "/software?installing=1"
	if r.FormValue("auto-reboot") == "1" {
		target += "&auto-reboot=1"
	}
	w.Header().Set("HX-Redirect", target)
	w.WriteHeader(http.StatusNoContent)
}

// SoftwareProgress streams installer status as SSE so the Go server does the
// polling and the browser just receives rendered HTML fragments.
// GET /software/progress
func (h *SystemHandler) SoftwareProgress(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	autoReboot := r.URL.Query().Get("auto-reboot") == "1"

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var lastKey string // change-detection: suppress redundant SSE frames
	lastFrame := time.Now()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			data := h.installerSnapshot(r, autoReboot)

			// Build a cheap key for change detection; skip frames with identical state.
			var key string
			if data.Installer != nil {
				key = fmt.Sprintf("%s|%d|%s|%s", data.Installer.Operation, data.Installer.Percentage, data.Installer.Message, data.Installer.LastError)
			}
			if key == lastKey && key != "" {
				// RAUC sometimes parks Progress at the same percentage
				// for tens of seconds (e.g., "Checking bundle" while
				// verifying signatures, or quiet during slot write).
				// Emit a comment as a keep-alive every 15 s so nginx's
				// proxy_read_timeout and the browser EventSource both
				// keep the stream warm — otherwise the connection gets
				// torn down mid-install and the UI freezes on the last
				// rendered frame until a manual page reload.
				if time.Since(lastFrame) > 15*time.Second {
					fmt.Fprint(w, ": keep-alive\n\n")
					flusher.Flush()
					lastFrame = time.Now()
				}
				continue
			}
			lastKey = key
			lastFrame = time.Now()

			var buf bytes.Buffer
			if err := h.Template.ExecuteTemplate(&buf, "sw-progress-body", data); err != nil {
				log.Printf("software progress template: %v", err)
				continue
			}

			// SSE data must not contain raw newlines; collapse to spaces.
			line := strings.ReplaceAll(buf.String(), "\n", " ")

			eventName := "progress"
			if data.Installer != nil && data.Installer.Done {
				if autoReboot && data.Installer.Success {
					eventName = "reboot"
				} else {
					eventName = "done"
				}
			}

			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventName, line)
			flusher.Flush()

			if data.Installer != nil && data.Installer.Done {
				return
			}
		}
	}
}

// installerSnapshot reads RAUC's installer state via rauc-installation-status
// (direct D-Bus property read) and builds the template data for the
// sw-progress-body fragment. The RESTCONF path is avoided because it runs
// `rauc status`, which blocks while an install is in progress.
func (h *SystemHandler) installerSnapshot(r *http.Request, autoReboot bool) softwareProgressData {
	data := softwareProgressData{
		AutoReboot: autoReboot,
	}

	inst, err := raucInstallationStatus(r.Context())
	if err != nil {
		// Leave Installer nil so the template renders an indeterminate
		// "Installing…" state on transient failures.
		log.Printf("software progress poll: %v", err)
		return data
	}
	data.Installer = newInstallerEntry(inst)
	return data
}

// newInstallerEntry converts a raw YANG installer state to the template-facing struct.
func newInstallerEntry(inst swInstallerState) *installerEntry {
	idle := inst.IsIdle()
	done := idle && (inst.Progress.Percentage > 0 || inst.LastError != "")
	return &installerEntry{
		Operation:  inst.Operation,
		Percentage: inst.Progress.Percentage,
		Message:    inst.Progress.Message,
		LastError:  inst.LastError,
		Active:     !idle,
		Done:       done,
		Success:    done && inst.LastError == "",
	}
}

// softwareProgressData is the template data for the sw-progress-body fragment.
type softwareProgressData struct {
	AutoReboot bool
	Installer  *installerEntry
}
