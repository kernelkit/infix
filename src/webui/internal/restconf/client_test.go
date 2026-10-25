// SPDX-License-Identifier: MIT

package restconf

import (
	"context"
	"net/http"
	"net/http/httptest"
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
