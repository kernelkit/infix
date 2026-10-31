// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"infix/webui/internal/restconf"
	"infix/webui/internal/schema"
)

const ethIfaceFixture = `{"ietf-interfaces:interfaces":{"interface":[
{"name":"eth0","type":"infix-if-type:ethernet","oper-status":"up",
 "ieee802-ethernet-interface:ethernet":{"duplex":"full"}},
{"name":"br0","type":"infix-if-type:bridge","oper-status":"up"}]}}`

// The detail page of an Ethernet port reads its frame counters with
// the get-statistics action, a bridge has none to read.
func TestInterfaceDetailEthStatistics(t *testing.T) {
	var actions []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yang-data+json")
		switch {
		case r.Method == http.MethodPost:
			actions = append(actions, r.URL.Path)
			w.Write([]byte(`{"infix-ethernet-interface:output":{"frame":{` +
				`"in-frames":"1234","in-good-octets":"98765","out-frames":"42"}}}`))
		case strings.HasSuffix(r.URL.Path, "ietf-interfaces:interfaces"):
			w.Write([]byte(ethIfaceFixture))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	tmpl := realTemplates(t, nil, "layouts/*.html", "pages/iface-detail.html",
		"fragments/iface-counters.html", "fragments/hw-detail.html")
	h := &InterfacesHandler{DetailTemplate: tmpl, CountersTemplate: tmpl, Schema: &schema.Cache{},
		RC: restconf.NewClient(srv.URL+"/restconf", false)}
	ctx := restconf.ContextWithCredentials(context.Background(),
		restconf.Credentials{Username: "admin", Password: "admin"})

	detail := func(name string) string {
		req := httptest.NewRequest(http.MethodGet, "/interfaces/"+name, nil).WithContext(ctx)
		req.SetPathValue("name", name)
		req.Header.Set("HX-Request", "true")
		w := httptest.NewRecorder()
		h.Detail(w, req)
		return w.Body.String()
	}

	body := detail("eth0")
	want := "/restconf/data/ietf-interfaces:interfaces/interface=eth0" +
		"/ieee802-ethernet-interface:ethernet/infix-ethernet-interface:get-statistics"
	if len(actions) != 1 || actions[0] != want {
		t.Fatalf("eth0 called %v", actions)
	}
	for _, s := range []string{"Ethernet Frame Statistics", "in-frames", "1,234", "out-frames"} {
		if !strings.Contains(body, s) {
			t.Errorf("eth0 detail lacks %q", s)
		}
	}

	body = detail("br0")
	if len(actions) != 1 {
		t.Errorf("br0 called %v", actions[1:])
	}
	if strings.Contains(body, "Ethernet Frame Statistics") {
		t.Error("br0 detail shows Ethernet frame statistics")
	}

	// The counters fragment, polled every few seconds, carries them too
	req := httptest.NewRequest(http.MethodGet, "/interfaces/eth0/counters", nil).WithContext(ctx)
	req.SetPathValue("name", "eth0")
	w := httptest.NewRecorder()
	h.Counters(w, req)
	body = w.Body.String()
	if len(actions) != 2 || actions[1] != want {
		t.Errorf("eth0 counters called %v", actions[1:])
	}
	for _, s := range []string{`id="iface-counters"`, "Ethernet Frame Statistics", "1,234"} {
		if !strings.Contains(body, s) {
			t.Errorf("eth0 counters lacks %q", s)
		}
	}
}
