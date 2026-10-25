// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"infix/webui/internal/restconf"
)

// An EP06-E on an IoT SIM, as yangerd reports it.
const modemFixture = `{"ietf-hardware:hardware":{"component":[
 {"name":"modem0","class":"infix-hardware:modem","uri":["usb:///sys/bus/usb/devices/2-1"],
  "infix-hardware:modem-state":{"manufacturer":"Quectel","model":"EP06-E","firmware-version":"EP06ELAR04A08M4G",
   "usb-vendor-id":"2c7c","usb-product-id":"0306","state":"connected","power-state":"on",
   "enabled-locks":["fixed-dialing"],"signal-quality":71,"signal-quality-recent":true,
   "signal-rsrp":-105,"signal-rsrq":"-10.0","signal-sinr":"14.0",
   "cellular":{"registration-state":"roaming","service-state":"attached","operator-name":"Onomondo",
    "operator-id":"24007","network-type":"lte"}}},
 {"name":"sim0","class":"infix-hardware:sim","infix-hardware:sim-state":{"slot":1,"state":"unlocked"}},
 {"name":"usb1","class":"infix-hardware:usb"}]}}`

const wwanFixture = `{"ietf-interfaces:interfaces":{"interface":[
 {"name":"wwan0","type":"infix-if-type:modem","oper-status":"up",
  "infix-interfaces:wwan":{"modem":"modem0","bearer-state":{"connected":true,
   "ipv4-address":"100.86.5.73","ipv4-prefix-length":30,"total-duration":"8040"}}},
 {"name":"lan1","type":"infix-if-type:ethernet","oper-status":"down"}]}}`

func modemComponents(t *testing.T) []hwComponentJSON {
	t.Helper()
	var hw hardwareWrapper
	if err := json.Unmarshal([]byte(modemFixture), &hw); err != nil {
		t.Fatal(err)
	}
	return hw.Hardware.Component
}

func TestSummarizeModem(t *testing.T) {
	m := summarizeModem(modemComponents(t)[0])
	if m.State != "connected" || m.StateClass != "status-up" || m.Network != "LTE" ||
		m.Operator != "Onomondo" || !m.Roaming {
		t.Errorf("summary = %+v", m)
	}
	if !m.HasSignal || m.SignalPct != 71 || m.SignalWord != "good" {
		t.Errorf("signal = %d%% %s", m.SignalPct, m.SignalWord)
	}
	if m.SignalDetail != "RSRP -105 dBm · RSRQ -10.0 dB · SINR 14.0 dB" {
		t.Errorf("signal detail = %q", m.SignalDetail)
	}

	// The words follow the CLI's thresholds
	for pct, word := range map[int64]string{80: "excellent", 60: "good", 40: "poor", 39: "bad"} {
		c := modemComponents(t)[0]
		q := yangInt64(pct)
		c.ModemState.SignalQuality = &q
		if got := summarizeModem(c).SignalWord; got != word {
			t.Errorf("%d%%: %q, want %q", pct, got, word)
		}
	}
}

func TestHardwarePageModems(t *testing.T) {
	var data hardwarePageData
	buildHardwarePage(&data, modemComponents(t))
	if len(data.Modems) != 1 || data.Modems[0].SIM == nil {
		t.Fatalf("modems %+v", data.Modems)
	}
	if sim := data.Modems[0].SIM; sim.Name != "sim0" || sim.Slot != "1" || sim.StateClass != "status-up" {
		t.Errorf("sim = %+v", sim)
	}
	for _, c := range data.OtherComps {
		if c.Class == classModem || c.Class == classSIM {
			t.Errorf("%s still among other components", c.Name)
		}
	}
}

func TestModemDetail(t *testing.T) {
	var actions []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yang-data+json")
		switch {
		case r.Method == http.MethodPost:
			actions = append(actions, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "ietf-hardware:hardware"):
			w.Write([]byte(modemFixture))
		case strings.HasSuffix(r.URL.Path, "ietf-interfaces:interfaces"):
			w.Write([]byte(wwanFixture))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	tmpl := realTemplates(t, nil, "layouts/*.html", "fragments/modem-signal.html", "pages/modem.html")
	h := &ModemHandler{Template: tmpl, RC: restconf.NewClient(srv.URL+"/restconf", false)}
	ctx := restconf.ContextWithCredentials(context.Background(), restconf.Credentials{Username: "admin", Password: "admin"})

	req := httptest.NewRequest(http.MethodGet, "/hardware/modem/modem0", nil).WithContext(ctx)
	req.SetPathValue("name", "modem0")
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	h.Detail(w, req)
	body := w.Body.String()
	for _, want := range []string{"EP06-E", "Onomondo", "roaming", "71% good", "fixed-dialing",
		"usb:///sys/bus/usb/devices/2-1", "2c7c:0306", "unlocked",
		`href="/interfaces/wwan0"`, "100.86.5.73/30", "2h 14m", "data-auto-refresh"} {
		if !strings.Contains(body, want) {
			t.Errorf("detail lacks %q", want)
		}
	}
	if strings.Contains(body, "lan1") {
		t.Error("an interface not on the modem listed as its bearer")
	}

	req = httptest.NewRequest(http.MethodPost, "/hardware/modem/modem0/restart", nil).WithContext(ctx)
	req.SetPathValue("name", "modem0")
	w = httptest.NewRecorder()
	h.Restart(w, req)
	if len(actions) != 1 || actions[0] != "/restconf/data/ietf-hardware:hardware/component=modem0/infix-hardware:restart" {
		t.Errorf("restart called %v", actions)
	}
	if !strings.Contains(w.Body.String(), "Restarting") {
		t.Errorf("restart answered %s", w.Body.String())
	}

	// No such modem
	d := modemDetailData{Name: "modem9"}
	if buildModemDetail(&d, modemComponents(t)) {
		t.Error("modem9 found")
	}
}
