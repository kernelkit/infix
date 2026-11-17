// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"infix/webui/internal/testutil"
)

// patchRecorder also keeps what was merged in.
type patchRecorder struct {
	recordingFetcher
	patches []any
}

func (r *patchRecorder) Patch(_ context.Context, _ string, body any) error {
	r.patches = append(r.patches, body)
	return nil
}

const qosPortE1 = "/ds/ietf-datastores:candidate/ietf-interfaces:interfaces/interface=e1/infix-interfaces:qos"

func saveQoSPort(t *testing.T, form url.Values, current map[string]any) *patchRecorder {
	t.Helper()
	rc := &patchRecorder{recordingFetcher: recordingFetcher{MockFetcher: testutil.NewMockFetcher()}}
	if current != nil {
		rc.SetResponse(qosPortE1, map[string]any{"ietf-interfaces:interfaces": map[string]any{
			"interface": []map[string]any{{"name": "e1", "infix-interfaces:qos": current}}}})
	}
	h := &ConfigureInterfacesHandler{RC: rc}
	postForm(t, h.SaveQoSPort, "/configure/qos/ports/e1", "e1", form)
	return rc
}

func qosPatch(t *testing.T, rc *patchRecorder) map[string]any {
	t.Helper()
	if len(rc.patches) != 1 {
		t.Fatalf("want one patch, got %d", len(rc.patches))
	}
	return rc.patches[0].(map[string]any)["infix-interfaces:qos"].(map[string]any)
}

func TestSaveQoSPort(t *testing.T) {
	all := []string{qosPortE1 + "/profile", qosPortE1 + "/ingress", qosPortE1 + "/egress"}

	t.Run("profile", func(t *testing.T) {
		rc := saveQoSPort(t, url.Values{"source": {"profile:edge"}, "rate": {"100"}, "trust": {"dscp"}}, nil)
		if !reflect.DeepEqual(rc.deletePaths, all) {
			t.Errorf("deletes %v", rc.deletePaths)
		}
		want := map[string]any{"profile": "edge", "egress-shaper": map[string]any{"rate": "100000000"}}
		if got := qosPatch(t, rc); !reflect.DeepEqual(got, want) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("defaults drop everything", func(t *testing.T) {
		rc := saveQoSPort(t, url.Values{"source": {"default"}, "trust": {"dscp"}}, nil)
		if want := append(all, qosPortE1+"/egress-shaper"); !reflect.DeepEqual(rc.deletePaths, want) {
			t.Errorf("deletes %v", rc.deletePaths)
		}
		if len(rc.patches) != 0 {
			t.Errorf("patched %v", rc.patches)
		}
	})

	t.Run("own settings", func(t *testing.T) {
		form := url.Values{"source": {"own"}, "trust": {"dscp"}, "table-preset": {"custom"},
			"algorithm-7": {qosAlgoCBS}, "algorithm-0": {qosAlgoStrict}}
		custom := map[string]any{"priority0": float64(1)}
		rc := saveQoSPort(t, form, map[string]any{"egress": map[string]any{"traffic-class-table": custom}})
		got := qosPatch(t, rc)
		if got["profile"] != nil {
			t.Errorf("profile set: %v", got)
		}
		if trust := got["ingress"].(map[string]any)["trust"]; trust != "dscp" {
			t.Errorf("trust %v", trust)
		}
		eg := got["egress"].(map[string]any)
		if !reflect.DeepEqual(eg["traffic-class-table"], custom) {
			t.Errorf("custom table lost: %v", eg["traffic-class-table"])
		}
		classes := eg["traffic-class"].([]any)
		if len(classes) != 1 || classes[0].(map[string]any)["id"] != float64(7) {
			t.Errorf("classes %v", classes)
		}
	})

	t.Run("bad rate changes nothing", func(t *testing.T) {
		rc := saveQoSPort(t, url.Values{"source": {"default"}, "rate": {"fast"}}, nil)
		if len(rc.deletePaths) != 0 || len(rc.patches) != 0 {
			t.Errorf("deletes %v patches %v", rc.deletePaths, rc.patches)
		}
	})

	t.Run("unknown source changes nothing", func(t *testing.T) {
		rc := saveQoSPort(t, url.Values{"source": {"bogus"}}, nil)
		if len(rc.deletePaths) != 0 || len(rc.patches) != 0 {
			t.Errorf("deletes %v patches %v", rc.deletePaths, rc.patches)
		}
	})
}

func TestConfigureQoSPortsOverview(t *testing.T) {
	mock := testutil.NewMockFetcher()
	mock.SetResponse(ifaceCandPath, map[string]any{
		"ietf-interfaces:interfaces": map[string]any{"interface": []map[string]any{
			{"name": "e10", "type": "infix-if-type:ethernet", "infix-interfaces:qos": map[string]any{"profile": "edge"}},
			{"name": "e2", "type": "infix-if-type:ethernet", "infix-interfaces:qos": map[string]any{
				"ingress": map[string]any{"trust": "dscp"}, "egress-shaper": map[string]any{"rate": "50000000"}}},
			{"name": "e3", "type": "infix-if-type:ethernet"},
			{"name": "lo", "type": "infix-if-type:loopback"},
		}},
	})
	caps := map[string]any{"capabilities": map[string]any{"max-traffic-classes": 8}}
	mock.SetResponse("/data/ietf-interfaces:interfaces", map[string]any{
		"ietf-interfaces:interfaces": map[string]any{"interface": []map[string]any{
			{"name": "e10", "type": "infix-if-type:ethernet", "infix-interfaces:qos": caps},
			{"name": "e2", "type": "infix-if-type:ethernet", "infix-interfaces:qos": caps},
			{"name": "e3", "type": "infix-if-type:ethernet", "infix-interfaces:qos": caps},
			{"name": "lo", "type": "infix-if-type:loopback", "infix-interfaces:qos": caps},
		}},
	})
	mock.SetResponse(candidatePath+"/ietf-system:system", map[string]any{"ietf-system:system": map[string]any{
		"infix-qos:qos-profile": []map[string]any{{"name": "edge"}},
	}})
	h := &ConfigureInterfacesHandler{
		QoSPortsTemplate: pageTemplate(t, "configure-qos-ports.html"),
		RC:               mock,
		Schema:           repoSchema(t, mock),
	}
	w := httptest.NewRecorder()
	h.OverviewQoSPorts(w, testRequest(http.MethodGet, "/configure/qos/ports"))
	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{
		`<td>Profile edge</td>`, `<td>Own settings</td>`, `<span class="text-muted">Defaults</span>`,
		`<option value="profile:edge" selected>`, `<option value="own" selected>`,
		`hx-post="/configure/qos/ports/e2"`, `<details id="qos-own-0" open`,
		`interface=e2/infix-interfaces:qos/ingress/trust&redirect=/configure/qos/ports"`,
		`class="diag-tab active" role="tab" href="/configure/qos/ports"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, `/configure/qos/ports/lo"`) {
		t.Error("lo listed as a QoS port")
	}
	if e2, e10 := strings.Index(body, "ports/e2\""), strings.Index(body, "ports/e10\""); e2 > e10 {
		t.Error("ports not in natural order")
	}
}
