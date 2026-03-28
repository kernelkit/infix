// SPDX-License-Identifier: MIT

package handlers

import (
	"fmt"
	"html/template"
	"log"
	"net/http"

	"github.com/kernelkit/webui/internal/restconf"
)

// SystemHandler provides reboot, config download, and firmware update actions.
type SystemHandler struct {
	RC       *restconf.Client
	Template *template.Template // firmware page template
}

// DeviceStatus returns 200 if the RESTCONF device is reachable, 502 otherwise.
// Used by the reboot spinner to detect when the device goes down and comes back.
func (h *SystemHandler) DeviceStatus(w http.ResponseWriter, r *http.Request) {
	var target struct{}
	err := h.RC.Get(r.Context(), "/data/ietf-system:system-state/platform", &target)
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

const rebootSpinnerHTML = `<div class="reboot-overlay" data-timeout="120000" data-interval="2000">
  <div class="reboot-spinner"></div>
  <p class="reboot-message">Rebooting&hellip;</p>
  <p class="reboot-status" id="reboot-status">Waiting for device to shut down&hellip;</p>
</div>`

// DownloadConfig serves the startup datastore as a JSON file download.
func (h *SystemHandler) DownloadConfig(w http.ResponseWriter, r *http.Request) {
	data, err := h.RC.GetRaw(r.Context(), "/ds/ietf-datastores:startup")
	if err != nil {
		log.Printf("config download: %v", err)
		http.Error(w, "failed to fetch config", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="startup-config.json"`)
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
}

// Firmware renders the firmware overview page (GET /firmware).
func (h *SystemHandler) Firmware(w http.ResponseWriter, r *http.Request) {
	data := firmwareData{
		PageData: newPageData(r, "firmware", "Firmware"),
		Message:  r.URL.Query().Get("msg"),
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

		inst := sw.SystemState.Software.Installer
		data.Installer = &installerEntry{
			Operation:  inst.Operation,
			Percentage: inst.Progress.Percentage,
			Message:    inst.Progress.Message,
			LastError:  inst.LastError,
			Active:     inst.Operation != "" && inst.Operation != "idle",
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

	w.Header().Set("HX-Redirect", "/firmware?msg=Install+started")
	w.WriteHeader(http.StatusNoContent)
}
