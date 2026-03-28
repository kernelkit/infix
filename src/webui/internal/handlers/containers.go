// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"sync"

	"github.com/kernelkit/webui/internal/restconf"
)

// containerJSON matches the RESTCONF JSON for a single container entry.
type containerJSON struct {
	Name    string   `json:"name"`
	Image   string   `json:"image"`
	Running yangBool `json:"running"`
	Status  string   `json:"status"`
	Network struct {
		Publish []string `json:"publish"`
	} `json:"network"`
	ResourceUsage containerResourceUsageJSON `json:"resource-usage"`
	ResourceLimit containerResourceLimitJSON `json:"resource-limit"`
}

// containerResourceUsageJSON matches the RESTCONF JSON for resource-usage.
type containerResourceUsageJSON struct {
	Memory yangInt64   `json:"memory"` // KiB
	CPU    yangFloat64 `json:"cpu"`    // percent
}

// containerResourceLimitJSON matches the RESTCONF JSON for resource-limit.
type containerResourceLimitJSON struct {
	Memory yangInt64 `json:"memory"` // KiB
}

// containerListWrapper wraps the top-level RESTCONF containers response.
// The server returns the full "containers" object; the list lives inside it.
type containerListWrapper struct {
	Containers struct {
		Container []containerJSON `json:"container"`
	} `json:"infix-containers:containers"`
}

// containerResourceUsageWrapper wraps the RESTCONF resource-usage response.
type containerResourceUsageWrapper struct {
	ResourceUsage containerResourceUsageJSON `json:"infix-containers:resource-usage"`
}

// ContainerEntry holds display-ready data for a single container row.
type ContainerEntry struct {
	Name     string
	Image    string
	Status   string
	Running  bool
	CPUPct   int
	MemUsed  string
	MemLimit string
	MemPct   int
	Uptime   string
	Ports    []string
}

// containersData is the template data for the containers page.
type containersData struct {
	PageData
	Containers []ContainerEntry
	Error      string
}

// ContainersHandler serves the containers status page.
type ContainersHandler struct {
	Template *template.Template
	RC       *restconf.Client
}

// Overview renders the containers list page.
func (h *ContainersHandler) Overview(w http.ResponseWriter, r *http.Request) {
	data := containersData{
		PageData: newPageData(r, "containers", "Containers"),
	}

	// Detach from the request context so that RESTCONF calls survive
	// browser connection resets.
	ctx := context.WithoutCancel(r.Context())

	var listResp containerListWrapper
	if err := h.RC.Get(ctx, "/data/infix-containers:containers", &listResp); err != nil {
		log.Printf("restconf containers list: %v", err)
		data.Error = "Could not fetch container information"
	} else {
		containers := listResp.Containers.Container

		// Fetch resource-usage for each container concurrently.
		usages := make([]containerResourceUsageJSON, len(containers))
		var mu sync.Mutex
		var wg sync.WaitGroup

		for i, c := range containers {
			wg.Add(1)
			go func(idx int, name string) {
				defer wg.Done()
				path := fmt.Sprintf("/data/infix-containers:containers/container=%s/resource-usage",
					url.PathEscape(name))
				var w containerResourceUsageWrapper
				if err := h.RC.Get(ctx, path, &w); err != nil {
					log.Printf("restconf resource-usage %s: %v", name, err)
					return
				}
				mu.Lock()
				usages[idx] = w.ResourceUsage
				mu.Unlock()
			}(i, c.Name)
		}
		wg.Wait()

		for i, c := range containers {
			entry := ContainerEntry{
				Name:    c.Name,
				Image:   c.Image,
				Status:  c.Status,
				Running: bool(c.Running),
				Ports:   c.Network.Publish,
			}

			// CPU usage — round to int.
			entry.CPUPct = int(float64(usages[i].CPU) + 0.5)
			if entry.CPUPct > 100 {
				entry.CPUPct = 100
			}

			// Memory usage — resource-usage.memory is in KiB.
			memUsedKiB := int64(usages[i].Memory)
			if memUsedKiB > 0 {
				entry.MemUsed = humanBytes(memUsedKiB * 1024)
			}

			// Memory limit — resource-limit.memory is in KiB.
			memLimitKiB := int64(c.ResourceLimit.Memory)
			if memLimitKiB > 0 {
				entry.MemLimit = humanBytes(memLimitKiB * 1024)
				if memUsedKiB > 0 {
					entry.MemPct = int(float64(memUsedKiB) / float64(memLimitKiB) * 100)
					if entry.MemPct > 100 {
						entry.MemPct = 100
					}
				}
			}

			// Uptime: extract from status string (e.g., "Up About a minute", "Up 3 hours").
			entry.Uptime = extractUptime(c.Status)

			data.Containers = append(data.Containers, entry)
		}
	}

	tmplName := "containers.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// extractUptime returns the uptime portion of a container status string.
// E.g., "Up About a minute" → "About a minute", "Up 3 hours" → "3 hours",
// "Exited (0) 2 hours ago" → "".
func extractUptime(status string) string {
	const prefix = "Up "
	if len(status) > len(prefix) && status[:len(prefix)] == prefix {
		return status[len(prefix):]
	}
	return ""
}
