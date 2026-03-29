// SPDX-License-Identifier: MIT

package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sort"
	"strconv"

	"github.com/kernelkit/webui/internal/restconf"
)

// ─── RESTCONF JSON types ──────────────────────────────────────────────────────

type servicesWrapper struct {
	SystemState struct {
		Services struct {
			Service []serviceJSON `json:"service"`
		} `json:"infix-system:services"`
	} `json:"ietf-system:system-state"`
}

type serviceJSON struct {
	Name        string          `json:"name"`
	PID         uint32          `json:"pid"`
	Description string          `json:"description"`
	Status      string          `json:"status"`
	Statistics  serviceStatsJSON `json:"statistics"`
}

// memory-usage and uptime are marshalled as strings by the statd Python layer.
type serviceStatsJSON struct {
	MemoryUsage  json.Number `json:"memory-usage"`
	Uptime       json.Number `json:"uptime"`
	RestartCount uint32      `json:"restart-count"`
}

func (s serviceStatsJSON) memoryBytes() uint64 {
	v, _ := strconv.ParseUint(s.MemoryUsage.String(), 10, 64)
	return v
}

func (s serviceStatsJSON) uptimeSeconds() uint64 {
	v, _ := strconv.ParseUint(s.Uptime.String(), 10, 64)
	return v
}

// ─── Template data ────────────────────────────────────────────────────────────

type servicesPageData struct {
	PageData
	Services []serviceEntry
	Error    string
}

type serviceEntry struct {
	Name         string
	Status       string
	StatusClass  string // "svc-running", "svc-stopped", "svc-error", "svc-done"
	PID          string
	Memory       string
	Uptime       string
	RestartCount uint32
	Description  string
}

// ─── Handler ─────────────────────────────────────────────────────────────────

// ServicesHandler serves the system services page.
type ServicesHandler struct {
	Template *template.Template
	RC       *restconf.Client
}

// Overview renders the services page (GET /services).
func (h *ServicesHandler) Overview(w http.ResponseWriter, r *http.Request) {
	data := servicesPageData{
		PageData: newPageData(r, "services", "Services"),
	}

	var raw servicesWrapper
	if err := h.RC.Get(r.Context(), "/data/ietf-system:system-state/infix-system:services", &raw); err != nil {
		log.Printf("restconf services: %v", err)
		data.Error = "Could not fetch services data"
	} else {
		for _, svc := range raw.SystemState.Services.Service {
			data.Services = append(data.Services, buildServiceEntry(svc))
		}
		sort.Slice(data.Services, func(i, j int) bool {
			return data.Services[i].Name < data.Services[j].Name
		})
	}

	tmplName := "services.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func buildServiceEntry(svc serviceJSON) serviceEntry {
	pid := ""
	if svc.PID > 0 {
		pid = fmt.Sprintf("%d", svc.PID)
	}

	return serviceEntry{
		Name:         svc.Name,
		Status:       svc.Status,
		StatusClass:  serviceStatusClass(svc.Status),
		PID:          pid,
		Memory:       formatServiceMemory(svc.Statistics.memoryBytes()),
		Uptime:       formatServiceUptime(svc.Statistics.uptimeSeconds()),
		RestartCount: svc.Statistics.RestartCount,
		Description:  svc.Description,
	}
}

func serviceStatusClass(status string) string {
	switch status {
	case "running", "active", "done":
		return "svc-running" // green
	case "crashed", "failed", "halted", "missing", "dead", "conflict":
		return "svc-error" // red
	default:
		// stopped, paused, and anything else → yellow
		return "svc-stopped"
	}
}

// formatServiceMemory mirrors the CLI's format_memory_bytes.
func formatServiceMemory(b uint64) string {
	switch {
	case b == 0:
		return ""
	case b < 1024:
		return fmt.Sprintf("%dB", b)
	case b < 1024*1024:
		return fmt.Sprintf("%dK", b/1024)
	case b < 1024*1024*1024:
		return fmt.Sprintf("%.1fM", float64(b)/float64(1024*1024))
	default:
		return fmt.Sprintf("%.1fG", float64(b)/float64(1024*1024*1024))
	}
}

// formatServiceUptime mirrors the CLI's format_uptime_seconds.
func formatServiceUptime(s uint64) string {
	switch {
	case s == 0:
		return ""
	case s < 60:
		return fmt.Sprintf("%ds", s)
	case s < 3600:
		return fmt.Sprintf("%dm", s/60)
	case s < 86400:
		return fmt.Sprintf("%dh", s/3600)
	default:
		return fmt.Sprintf("%dd", s/86400)
	}
}
