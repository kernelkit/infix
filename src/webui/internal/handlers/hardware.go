// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"sort"

	"github.com/kernelkit/webui/internal/restconf"
)

// Hardware page data — populated from /ietf-hardware:hardware.
// Mirrors CLI `show hardware`, but with browser affordances: sensor groups
// keep their child rows together, USB ports get a state column, and any
// uncategorised components fall through to a "Other" section so the page
// shows everything ietf-hardware exposes.

type hardwarePageData struct {
	PageData
	Board        boardInfo
	USBPorts     []usbPortEntry
	WiFiRadios   []wifiEntry
	GPSReceivers []gpsEntry
	SensorGroups []hwSensorGroup
	OtherComps   []otherCompEntry
	Error        string
}

type usbPortEntry struct {
	Name        string
	Description string
	AdminState  string // locked / unlocked
	OperState   string // enabled / disabled / etc.
}

type gpsEntry struct {
	Name         string
	Manufacturer string
	ModelName    string
	HardwareRev  string
}

type hwSensorGroup struct {
	Parent  string
	Sensors []hwSensorEntry
}

type hwSensorEntry struct {
	Name       string
	Value      string
	Type       string // temperature / fan / volts-DC / etc.
	OperStatus string // ok / unavailable / nonoperational
}

type otherCompEntry struct {
	Name        string
	Class       string
	Parent      string
	Description string
	Manufacturer string
	ModelName    string
	SerialNum    string
	HardwareRev  string
}

// HardwareHandler serves the Status → Hardware page (GET /hardware).
type HardwareHandler struct {
	Template *template.Template
	RC       *restconf.Client
}

func (h *HardwareHandler) Overview(w http.ResponseWriter, r *http.Request) {
	data := hardwarePageData{
		PageData: newPageData(r, "hardware", "Hardware"),
	}

	// Detach from r.Context() so the RESTCONF call (and yanger behind it)
	// survives a client disconnect mid-fetch (e.g. login redirect). The
	// RESTCONF client's own 10s timeout still bounds each call.
	ctx := context.WithoutCancel(r.Context())

	var hw hardwareWrapper
	if err := h.RC.Get(ctx, "/data/ietf-hardware:hardware", &hw); err != nil {
		if !restconf.IsNotFound(err) {
			log.Printf("restconf hardware: %v", err)
			data.Error = "Could not fetch hardware information"
		}
	} else {
		buildHardwarePage(&data, hw.Hardware.Component)
	}

	tmplName := "hardware.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// buildHardwarePage walks the ietf-hardware components and routes each to
// the section it belongs in. Components with sensor-data show up under
// SensorGroups regardless of class (a chassis can carry a temp sensor too);
// any component not handled by an explicit section falls through to
// OtherComps so the page is exhaustive.
//
// Unlike the dashboard's curated view, this one keeps non-ok sensors
// visible — the redesigned Overview (Phase 2) will surface those via a
// summary indicator instead of hiding them.
func buildHardwarePage(data *hardwarePageData, comps []hwComponentJSON) {
	sensorMap := make(map[string][]hwSensorEntry)
	var sensorParents []string

	for _, c := range comps {
		class := shortClass(c.Class)
		handled := true
		switch class {
		case classChassis:
			data.Board = boardInfo{
				Model:        c.ModelName,
				Manufacturer: c.MfgName,
				SerialNum:    c.SerialNum,
				HardwareRev:  c.HardwareRev,
				BaseMAC:      c.PhysAddress,
			}
		case classUSB:
			data.USBPorts = append(data.USBPorts, usbPortEntry{
				Name:        c.Name,
				Description: c.Description,
				AdminState:  compAdminState(c),
				OperState:   compOperState(c),
			})
		case classWiFi:
			data.WiFiRadios = append(data.WiFiRadios, summarizeWiFiRadio(c))
		case classGPS:
			data.GPSReceivers = append(data.GPSReceivers, gpsEntry{
				Name:         c.Name,
				Manufacturer: c.MfgName,
				ModelName:    c.ModelName,
				HardwareRev:  c.HardwareRev,
			})
		default:
			handled = false
		}

		if c.SensorData != nil {
			entry := hwSensorEntry{
				Name:       c.Name,
				Value:      formatSensor(c.SensorData.ValueType, int64(c.SensorData.Value), c.SensorData.ValueScale),
				Type:       c.SensorData.ValueType,
				OperStatus: c.SensorData.OperStatus,
			}
			if _, ok := sensorMap[c.Parent]; !ok {
				sensorParents = append(sensorParents, c.Parent)
			}
			sensorMap[c.Parent] = append(sensorMap[c.Parent], entry)
			handled = true
		}

		if !handled {
			data.OtherComps = append(data.OtherComps, otherCompEntry{
				Name:         c.Name,
				Class:        class,
				Parent:       c.Parent,
				Description:  c.Description,
				Manufacturer: c.MfgName,
				ModelName:    c.ModelName,
				SerialNum:    c.SerialNum,
				HardwareRev:  c.HardwareRev,
			})
		}
	}

	sort.Strings(sensorParents)
	for _, p := range sensorParents {
		data.SensorGroups = append(data.SensorGroups, hwSensorGroup{
			Parent:  p,
			Sensors: sensorMap[p],
		})
	}
	sort.Slice(data.USBPorts, func(i, j int) bool { return data.USBPorts[i].Name < data.USBPorts[j].Name })
	sort.Slice(data.OtherComps, func(i, j int) bool {
		if data.OtherComps[i].Class != data.OtherComps[j].Class {
			return data.OtherComps[i].Class < data.OtherComps[j].Class
		}
		return data.OtherComps[i].Name < data.OtherComps[j].Name
	})
}

func compAdminState(c hwComponentJSON) string {
	if c.State == nil {
		return ""
	}
	return c.State.AdminState
}

func compOperState(c hwComponentJSON) string {
	if c.State == nil {
		return ""
	}
	return c.State.OperState
}
