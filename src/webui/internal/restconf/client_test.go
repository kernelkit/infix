// SPDX-License-Identifier: MIT

package restconf

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestCheckPath(t *testing.T) {
	ok := []string{
		"/data/ietf-interfaces:interfaces",
		"/ds/ietf-datastores:candidate/ietf-interfaces:interfaces/interface=e0%2F1",
		"/data/ietf-system:system/ntp/server=2001%3Adb8%3A%3A1",
		"/operations/ietf-system:system-restart",
	}
	bad := []string{
		"",
		"data/ietf-interfaces:interfaces",
		"/ds/ietf-datastores:candidate/../../yang/x.yang",
		"/data/./x",
		"/ds/ietf-datastores:candidate/%2e%2e/%2E%2E/yang/x.yang",
		"/data/x%2F..%2F..%2Fyang",
		"/data/x%zz",
		"/data/x?content=config",
		"/data/x#frag",
		"/data/interface=a b",
		"/data/x\r\nInjected: 1",
	}
	for _, p := range ok {
		if err := checkPath(p); err != nil {
			t.Errorf("%q rejected: %v", p, err)
		}
	}
	for _, p := range bad {
		if err := checkPath(p); err == nil {
			t.Errorf("%q accepted", p)
		}
	}
}

// The copy reads the source with defaults trimmed.  The query must reach
// the server as a query: checkPath refuses one inside the path.
func TestCopyDatastoreTrimsDefaults(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.RequestURI())
		if r.Method == http.MethodGet {
			w.Write([]byte(`{"ietf-system:system":{}}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewClient(srv.URL+"/restconf", false)
	if err := c.CopyDatastore(context.Background(), "running", "candidate"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"GET /restconf/ds/ietf-datastores:running?with-defaults=trim",
		"PUT /restconf/ds/ietf-datastores:candidate",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("requests %v, want %v", got, want)
	}
}

// rousette answers a POST without a Content-Type with 400, also for an
// RPC without input, like infix-hardware:locate.
func TestPostWithoutBodyHasContentType(t *testing.T) {
	var ctype string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctype = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewClient(srv.URL+"/restconf", false)
	if err := c.Post(context.Background(), "/operations/infix-hardware:locate"); err != nil {
		t.Fatal(err)
	}
	if ctype != "application/yang-data+json" {
		t.Errorf("Content-Type = %q", ctype)
	}
}
