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

const cpuFixture = `{"ietf-hardware:hardware":{"component":[
 {"name":"mainboard","class":"iana-hardware:chassis","model-name":"BPi-R2 Pro"},
 {"name":"cpu","class":"iana-hardware:cpu","parent":"mainboard","mfg-name":"Rockchip","model-name":"RK3568",
  "infix-hardware:cpu":{"cluster":[{"id":0,"model":"Cortex-A55","cores":4,
   "frequency":1992,"min-frequency":408,"max-frequency":1992}]}}]}}`

func TestCPUHardwarePage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yang-data+json")
		w.Write([]byte(cpuFixture))
	}))
	defer srv.Close()
	rc := restconf.NewClient(srv.URL+"/restconf", false)
	ctx := restconf.ContextWithCredentials(context.Background(), restconf.Credentials{Username: "admin", Password: "admin"})

	tmpl := realTemplates(t, nil, "layouts/*.html", "fragments/hw-detail.html", "pages/hardware.html")
	h := &HardwareHandler{Template: tmpl, RC: rc}
	req := httptest.NewRequest(http.MethodGet, "/hardware", nil).WithContext(ctx)
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	h.Overview(w, req)
	body := w.Body.String()
	for _, want := range []string{"<th>SoC</th><td>Rockchip RK3568", "<th>Cores</th><td>4x Cortex-A55 @ 1992 MHz (408-1992 MHz)"} {
		if !strings.Contains(body, want) {
			t.Errorf("hardware page lacks %q", want)
		}
	}
	if strings.Contains(body, ">cpu<") {
		t.Error("cpu is listed among the other components")
	}
	board := body[strings.Index(body, "card-header\">Board"):]
	board = board[:strings.Index(board, "</table>")]
	if !strings.Contains(board, "Cortex-A55") {
		t.Error("the cores are not in the Board card")
	}
}

func TestSummarizeCPU(t *testing.T) {
	for _, tc := range []struct {
		comp hwComponentJSON
		want string
	}{
		{hwComponentJSON{MfgName: "Rockchip", ModelName: "RK3568", CPU: &cpuJSON{Cluster: []cpuClusterJSON{
			{Model: "Cortex-A55", Cores: 4, Frequency: 1992}}}},
			"Rockchip RK3568, quad core Cortex-A55 @ 1992 MHz"},
		{hwComponentJSON{MfgName: "Rockchip", ModelName: "RK3588", CPU: &cpuJSON{Cluster: []cpuClusterJSON{
			{Model: "Cortex-A55", Cores: 4, Frequency: 1800}, {ID: 4, Model: "Cortex-A76", Cores: 4, Frequency: 2256}}}},
			"Rockchip RK3588, 4x Cortex-A55 @ 1800 MHz + 4x Cortex-A76 @ 2256 MHz"},
		{hwComponentJSON{CPU: &cpuJSON{Cluster: []cpuClusterJSON{{Model: "Cortex-A53", Cores: 2}}}},
			"Dual core Cortex-A53"},
		{hwComponentJSON{MfgName: "AMD", ModelName: "Ryzen 7 PRO 5850U", CPU: &cpuJSON{Cluster: []cpuClusterJSON{{Cores: 6, Frequency: 1900}}}},
			"AMD Ryzen 7 PRO 5850U, hexa core @ 1900 MHz"},
		{hwComponentJSON{}, ""},
	} {
		if got := summarizeCPU(tc.comp).Summary; got != tc.want {
			t.Errorf("summarizeCPU = %q, want %q", got, tc.want)
		}
	}
}
