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
	"strings"
	"time"

	"github.com/kernelkit/webui/internal/restconf"
)

// SystemHandler provides reboot, config download, and firmware update actions.
type SystemHandler struct {
	RC          *restconf.Client
	Template    *template.Template // firmware page template
	SysCtrlTmpl *template.Template // system control page template
	BackupTmpl  *template.Template // backup & restore page template
}

// DeviceStatus returns 200 if the RESTCONF device is reachable, 502 otherwise.
// Used by the reboot spinner to detect when the device goes down and comes back.
// A short timeout keeps the poll snappy during the reboot window.
func (h *SystemHandler) DeviceStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	var target struct{}
	err := h.RC.Get(ctx, "/data/ietf-system:system-state/platform", &target)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusOK)
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
	CurrentDatetime string // device clock, empty when unavailable
}

// SystemControl renders the System Control maintenance page.
func (h *SystemHandler) SystemControl(w http.ResponseWriter, r *http.Request) {
	data := systemControlData{
		PageData: newPageData(r, "system-control", "System Control"),
	}

	var clockResp struct {
		SystemState struct {
			Clock struct {
				CurrentDatetime string `json:"current-datetime"`
			} `json:"clock"`
		} `json:"ietf-system:system-state"`
	}
	if err := h.RC.Get(r.Context(), "/data/ietf-system:system-state/clock", &clockResp); err == nil {
		dt := clockResp.SystemState.Clock.CurrentDatetime
		if len(dt) > 19 {
			dt = dt[:19]
		}
		data.CurrentDatetime = strings.Replace(dt, "T", " ", 1) + " UTC"
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
	raw := r.FormValue("datetime") // YYYY-MM-DDTHH:MM from datetime-local input
	if raw == "" {
		http.Error(w, "datetime required", http.StatusBadRequest)
		return
	}

	body := map[string]map[string]string{
		"ietf-system:input": {"current-datetime": raw + ":00+00:00"},
	}
	err := h.RC.PostJSON(r.Context(), "/operations/ietf-system:set-current-datetime", body)

	w.Header().Set("Content-Type", "text/html")
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "ntp-active") {
			fmt.Fprint(w, `<span class="sc-fd-err">NTP is active &mdash; disable NTP first under Configure &gt; System.</span>`)
		} else {
			fmt.Fprintf(w, `<span class="sc-fd-err">Failed: %s</span>`, template.HTMLEscapeString(msg))
		}
		return
	}
	fmt.Fprint(w, `<span class="sc-fd-ok">&#10003; System time updated</span>`)
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
	data := newPageData(r, "backup", "Backup & Restore")
	tmplName := "backup.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.BackupTmpl.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("backup template: %v", err)
	}
}

// RestoreConfig accepts a multipart-uploaded JSON config file and applies it.
// target="running" (default): PUT to running so changes take effect immediately;
// sets the cfg-unsaved cookie so the persistent notification prompts a save.
// target="startup": PUT to startup only; reboot required to apply.
func (h *SystemHandler) RestoreConfig(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

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

	target := "running"
	if r.FormValue("save-to-startup") == "on" {
		target = "startup"
	}

	if err := h.RC.PutDatastore(r.Context(), target, check); err != nil {
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

type fwSoftwareWrapper struct {
	SystemState struct {
		Platform struct {
			Machine string `json:"machine"`
		} `json:"platform"`
		Software fwSoftwareState `json:"infix-system:software"`
	} `json:"ietf-system:system-state"`
}

type fwSoftwareState struct {
	Compatible string           `json:"compatible"`
	Variant    string           `json:"variant"`
	Booted     string           `json:"booted"`
	BootOrder  []string         `json:"boot-order"`
	Installer  fwInstallerState `json:"installer"`
	Slots      []fwSlot         `json:"slot"`
}

type fwInstallerState struct {
	Operation string              `json:"operation"`
	Progress  fwInstallerProgress `json:"progress"`
	LastError string              `json:"last-error"`
}

type fwInstallerProgress struct {
	Percentage int    `json:"percentage"`
	Message    string `json:"message"`
}

type fwSlot struct {
	Name      string       `json:"name"`
	BootName  string       `json:"bootname"`
	Class     string       `json:"class"`
	State     string       `json:"state"`
	Bundle    fwSlotBundle `json:"bundle"`
	Installed struct {
		Datetime string `json:"datetime"`
	} `json:"installed"`
}

type fwSlotBundle struct {
	Compatible string `json:"compatible"`
	Version    string `json:"version"`
}

// Template data for the firmware page.

type firmwareData struct {
	PageData
	Machine      string
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

// Firmware renders the firmware overview page (GET /firmware).
func (h *SystemHandler) Firmware(w http.ResponseWriter, r *http.Request) {
	data := firmwareData{
		PageData:   newPageData(r, "firmware", "Firmware"),
		Message:    r.URL.Query().Get("msg"),
		Installing: r.URL.Query().Get("installing") == "1",
		AutoReboot: r.URL.Query().Get("auto-reboot") == "1",
	}

	var sw fwSoftwareWrapper
	err := h.RC.Get(r.Context(), "/data/ietf-system:system-state", &sw)
	if err != nil {
		log.Printf("restconf firmware: %v", err)
		data.Error = "Could not fetch firmware status"
	} else {
		data.Machine = sw.SystemState.Platform.Machine
		if data.Machine == "arm64" {
			data.Machine = "aarch64"
		}
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
		// Don't re-open the SSE progress card for an already-finished install
		// (e.g. user navigating back to /firmware?installing=1 after reboot).
		if data.Installer.Done {
			data.Installing = false
		}
	}

	tmplName := "firmware.html"
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

// FirmwareUpload accepts a .pkg file upload, saves it to a temp file, and
// triggers RAUC installation via the install-bundle RPC with a file:// URL.
// The response is a plain-text redirect target which the JS XHR handler follows.
func (h *SystemHandler) FirmwareUpload(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(10 * time.Minute))

	if err := r.ParseMultipartForm(1 << 30); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("pkg")
	if err != nil {
		http.Error(w, "pkg file required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	tmp, err := os.CreateTemp("", "webui-fw-*.pkg")
	if err != nil {
		log.Printf("firmware upload: create temp: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	tmpPath := tmp.Name()

	if _, err := io.Copy(tmp, file); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		log.Printf("firmware upload: write: %v", err)
		http.Error(w, "failed to save firmware", http.StatusInternalServerError)
		return
	}
	tmp.Close()

	body := map[string]map[string]string{
		"infix-system:input": {"url": "file://" + tmpPath},
	}
	if err := h.RC.PostJSON(r.Context(), "/operations/infix-system:install-bundle", body); err != nil {
		os.Remove(tmpPath)
		log.Printf("firmware upload: install-bundle: %v", err)
		http.Error(w, "install failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	creds := restconf.CredentialsFromContext(r.Context())
	go h.cleanupFirmwareTemp(creds, tmpPath)

	target := "/firmware?installing=1"
	if r.FormValue("auto-reboot") == "1" {
		target += "&auto-reboot=1"
	}
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, target)
}

// cleanupFirmwareTemp polls the installer state and removes the temp file once
// RAUC goes idle. Falls back to deletion after 30 minutes in any case.
func (h *SystemHandler) cleanupFirmwareTemp(creds restconf.Credentials, path string) {
	ctx, cancel := context.WithTimeout(
		restconf.ContextWithCredentials(context.Background(), creds),
		30*time.Minute,
	)
	defer cancel()
	defer os.Remove(path)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var sw fwSoftwareWrapper
			if err := h.RC.Get(ctx, "/data/ietf-system:system-state", &sw); err != nil {
				continue
			}
			op := sw.SystemState.Software.Installer.Operation
			if op == "" || op == "idle" {
				return
			}
		}
	}
}

// FirmwareInstall triggers a firmware install via the install-bundle RPC (POST /firmware/install).
func (h *SystemHandler) FirmwareInstall(w http.ResponseWriter, r *http.Request) {
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
		log.Printf("firmware install: %v", err)
		w.Header().Set("HX-Redirect", "/firmware?msg=Install+failed:+"+err.Error())
		w.WriteHeader(http.StatusNoContent)
		return
	}

	target := "/firmware?installing=1"
	if r.FormValue("auto-reboot") == "1" {
		target += "&auto-reboot=1"
	}
	w.Header().Set("HX-Redirect", target)
	w.WriteHeader(http.StatusNoContent)
}

// FirmwareProgress streams installer status as SSE so the Go server does the
// polling and the browser just receives rendered HTML fragments.
// GET /firmware/progress
func (h *SystemHandler) FirmwareProgress(w http.ResponseWriter, r *http.Request) {
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
				continue
			}
			lastKey = key

			var buf bytes.Buffer
			if err := h.Template.ExecuteTemplate(&buf, "fw-progress-body", data); err != nil {
				log.Printf("firmware progress template: %v", err)
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

// installerSnapshot fetches the current installer state from RESTCONF and
// builds the template data for the fw-progress-body fragment.
func (h *SystemHandler) installerSnapshot(r *http.Request, autoReboot bool) firmwareProgressData {
	data := firmwareProgressData{
		AutoReboot: autoReboot,
	}

	var sw fwSoftwareWrapper
	if err := h.RC.Get(r.Context(), "/data/ietf-system:system-state", &sw); err != nil {
		// RESTCONF temporarily unavailable during upgrade — leave Installer nil
		// so the template renders an indeterminate "Installing…" state.
		log.Printf("firmware progress poll: %v", err)
		return data
	}

	data.Installer = newInstallerEntry(sw.SystemState.Software.Installer)
	return data
}

// newInstallerEntry converts a raw YANG installer state to the template-facing struct.
func newInstallerEntry(inst fwInstallerState) *installerEntry {
	idle := inst.Operation == "" || inst.Operation == "idle"
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

// firmwareProgressData is the template data for the fw-progress-body fragment.
type firmwareProgressData struct {
	AutoReboot bool
	Installer  *installerEntry
}
