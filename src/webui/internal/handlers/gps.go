// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"

	"infix/webui/internal/restconf"
)

// gpsReceiverJSON mirrors infix-hardware:gps-receiver.
type gpsReceiverJSON struct {
	Device            string     `json:"device"`
	Driver            string     `json:"driver"`
	Activated         bool       `json:"activated"`
	SatellitesVisible *yangInt64 `json:"satellites-visible"`
	SatellitesUsed    *yangInt64 `json:"satellites-used"`
	FixMode           string     `json:"fix-mode"`
	Latitude          string     `json:"latitude"`
	Longitude         string     `json:"longitude"`
	Altitude          string     `json:"altitude"`
	PPSAvailable      bool       `json:"pps-available"`
}

func gpsFix(r *gpsReceiverJSON) (fix, class string) {
	switch r.FixMode {
	case "3d":
		return "3D", "status-up"
	case "2d":
		return "2D", "status-warn"
	}
	return "none", "status-down"
}

func gpsSatellites(r *gpsReceiverJSON) string {
	if r.SatellitesVisible == nil {
		return ""
	}
	used := int64(0)
	if r.SatellitesUsed != nil {
		used = int64(*r.SatellitesUsed)
	}
	return fmt.Sprintf("%d of %d", used, int64(*r.SatellitesVisible))
}

func summarizeGPS(c hwComponentJSON) gpsEntry {
	e := gpsEntry{Name: c.Name, Fix: "none", FixClass: "status-down"}
	r := c.GPSReceiver
	if r == nil {
		return e
	}
	e.Device = r.Device
	e.Fix, e.FixClass = gpsFix(r)
	e.Satellites = gpsSatellites(r)
	if r.Latitude != "" && r.Longitude != "" {
		e.Position = r.Latitude + ", " + r.Longitude
	}
	return e
}

// gpsDetailData is the template data for a GPS receiver's detail page.
type gpsDetailData struct {
	PageData
	Name     string
	Error    string
	Fix      string
	FixClass string
	Receiver []infoRow
	Position []infoRow
}

// GPSHandler serves a GPS receiver's detail page.
type GPSHandler struct {
	Template *template.Template
	RC       *restconf.Client
}

// Detail renders GET /hardware/gps/{name}.
func (h *GPSHandler) Detail(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	data := gpsDetailData{
		PageData: newPageData(w, r, "hardware", name),
		Name:     name,
	}

	var hw hardwareWrapper
	if err := h.RC.Get(context.WithoutCancel(r.Context()), "/data/ietf-hardware:hardware", &hw); err != nil {
		log.Printf("restconf hardware (gps %s): %v", name, err)
		data.Error = "Could not fetch GPS receiver information"
	} else if !buildGPSDetail(&data, hw.Hardware.Component) {
		data.Error = fmt.Sprintf("No GPS receiver %s", name)
	}

	tmplName := "gps.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// buildGPSDetail fills the detail cards, false when there is no such receiver.
func buildGPSDetail(d *gpsDetailData, comps []hwComponentJSON) bool {
	var gps *hwComponentJSON
	for i := range comps {
		if comps[i].Name == d.Name && shortClass(comps[i].Class) == classGPS {
			gps = &comps[i]
		}
	}
	if gps == nil {
		return false
	}
	r := gps.GPSReceiver
	if r == nil {
		r = &gpsReceiverJSON{}
	}
	yesNo := map[bool]string{true: "yes", false: "no"}
	add := func(rows *[]infoRow, label, value string) {
		if value != "" {
			*rows = append(*rows, infoRow{label, value})
		}
	}

	d.Fix, d.FixClass = gpsFix(r)
	add(&d.Receiver, "Satellites", gpsSatellites(r))
	add(&d.Receiver, "Activated", yesNo[r.Activated])
	add(&d.Receiver, "Device", r.Device)
	add(&d.Receiver, "Driver", r.Driver)
	add(&d.Receiver, "PPS", yesNo[r.PPSAvailable])
	if len(gps.URI) > 0 {
		add(&d.Receiver, "URI", gps.URI[0])
	}
	add(&d.Receiver, "Attached To", gps.Parent)

	add(&d.Position, "Latitude", r.Latitude)
	add(&d.Position, "Longitude", r.Longitude)
	if r.Altitude != "" {
		add(&d.Position, "Altitude", r.Altitude+" m")
	}
	return true
}
