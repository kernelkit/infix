// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"infix/webui/internal/restconf"
)

// A receiver with a 3D fix, and one gpsd has not heard from yet.
const gpsFixture = `{"ietf-hardware:hardware":{"component":[
 {"name":"gps0","class":"infix-hardware:gps","uri":["usb:///sys/bus/usb/devices/1-1.2"],"parent":"usb-1-1.2",
  "infix-hardware:gps-receiver":{"device":"/dev/gps0","driver":"u-blox","activated":true,
   "satellites-visible":11,"satellites-used":7,"fix-mode":"3d","latitude":"59.329300",
   "longitude":"18.068600","altitude":"28.0","pps-available":true}},
 {"name":"gps1","class":"infix-hardware:gps",
  "infix-hardware:gps-receiver":{"device":"/dev/gps1","activated":false,"satellites-visible":0,
   "satellites-used":0,"fix-mode":"none","pps-available":false}}]}}`

func TestGPSHardwareAndDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yang-data+json")
		w.Write([]byte(gpsFixture))
	}))
	defer srv.Close()
	rc := restconf.NewClient(srv.URL+"/restconf", false)
	ctx := restconf.ContextWithCredentials(context.Background(), restconf.Credentials{Username: "admin", Password: "admin"})

	var hw hardwareWrapper
	if err := rc.Get(ctx, "/data/ietf-hardware:hardware", &hw); err != nil {
		t.Fatal(err)
	}
	var data hardwarePageData
	buildHardwarePage(&data, hw.Hardware.Component)
	if len(data.GPSReceivers) != 2 {
		t.Fatalf("receivers %+v", data.GPSReceivers)
	}
	fix, none := data.GPSReceivers[0], data.GPSReceivers[1]
	if fix.Fix != "3D" || fix.Satellites != "7 of 11" || fix.Position != "59.329300, 18.068600" {
		t.Errorf("gps0 = %+v", fix)
	}
	if none.Fix != "none" || none.FixClass != "status-down" || none.Position != "" {
		t.Errorf("gps1 = %+v", none)
	}

	tmpl := realTemplates(t, nil, "layouts/*.html", "fragments/hw-detail.html", "pages/gps.html")
	h := &GPSHandler{Template: tmpl, RC: rc}
	for name, wants := range map[string][]string{
		"gps0": {"3D", "7 of 11", "u-blox", "/dev/gps0", "usb-1-1.2", "59.329300", "28.0 m", "data-auto-refresh"},
		"gps1": {"none", "No position without a fix."},
		"gps9": {"No GPS receiver gps9"},
	} {
		req := httptest.NewRequest(http.MethodGet, "/hardware/gps/"+name, nil).WithContext(ctx)
		req.SetPathValue("name", name)
		req.Header.Set("HX-Request", "true")
		w := httptest.NewRecorder()
		h.Detail(w, req)
		for _, want := range wants {
			if !strings.Contains(w.Body.String(), want) {
				t.Errorf("%s: page lacks %q", name, want)
			}
		}
	}
}
