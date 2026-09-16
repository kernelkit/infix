// SPDX-License-Identifier: MIT

package restconf

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestYangPatchRequest(t *testing.T) {
	var (
		method, path, ctype string
		body                map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path, ctype = r.Method, r.URL.Path, r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &body)
		w.Header().Set("Content-Type", "application/yang-data+json")
		w.Write([]byte(`{"ietf-yang-patch:yang-patch-status":{"patch-id":"webui","ok":[null]}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL+"/restconf", false)
	p := NewYangPatch("/ds/ietf-datastores:candidate").
		Merge("/ietf-hardware:hardware", map[string]any{"ietf-hardware:hardware": map[string]any{}}).
		Replace("/ietf-interfaces:interfaces/interface=wlan0/infix-interfaces:wifi", map[string]any{"infix-interfaces:wifi": map[string]any{"radio": "radio0"}}).
		Remove("/ietf-interfaces:interfaces/interface=wlan1")
	if err := c.YangPatch(context.Background(), p); err != nil {
		t.Fatalf("YangPatch: %v", err)
	}
	if method != http.MethodPatch || path != "/restconf/ds/ietf-datastores:candidate" {
		t.Fatalf("request %s %s", method, path)
	}
	if ctype != "application/yang-patch+json" {
		t.Fatalf("content-type %q", ctype)
	}
	patch, _ := body["ietf-yang-patch:yang-patch"].(map[string]any)
	edits, _ := patch["edit"].([]any)
	if patch["patch-id"] == "" || len(edits) != 3 {
		t.Fatalf("body %v", body)
	}
	for i, want := range []struct {
		op, target string
		value      bool
	}{
		{"merge", "/ietf-hardware:hardware", true},
		{"replace", "/ietf-interfaces:interfaces/interface=wlan0/infix-interfaces:wifi", true},
		{"remove", "/ietf-interfaces:interfaces/interface=wlan1", false},
	} {
		e := edits[i].(map[string]any)
		_, hasValue := e["value"]
		if e["operation"] != want.op || e["target"] != want.target || hasValue != want.value || e["edit-id"] == "" {
			t.Errorf("edit %d: %v", i, e)
		}
	}
}

func TestYangPatchEmptySendsNothing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()
	c := NewClient(srv.URL+"/restconf", false)
	if err := c.YangPatch(context.Background(), NewYangPatch("/ds/ietf-datastores:candidate")); err != nil {
		t.Fatal(err)
	}
}

func TestYangPatchStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yang-data+json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"ietf-yang-patch:yang-patch-status":{"patch-id":"webui","edit-status":{"edit":[{"edit-id":"e2","errors":{"error":[{"error-type":"protocol","error-tag":"invalid-value","error-path":"/ietf-interfaces:interfaces/interface[name='wlan0']/infix-interfaces:wifi","error-message":"Invalid channel."}]}}]}}}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL+"/restconf", false)
	p := NewYangPatch("/ds/ietf-datastores:candidate").Remove("/ietf-interfaces:interfaces/interface=wlan0")
	err := c.YangPatch(context.Background(), p)
	re, ok := err.(*Error)
	if !ok {
		t.Fatalf("err %T %v", err, err)
	}
	if re.StatusCode != 400 || re.Tag != "invalid-value" || !strings.Contains(re.Message, "Invalid channel.") {
		t.Fatalf("error %+v", re)
	}
}
