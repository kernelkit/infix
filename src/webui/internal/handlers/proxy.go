// SPDX-License-Identifier: MIT

package handlers

import (
	"io"
	"net/http"
	"strings"

	"infix/webui/internal/restconf"
)

// proxyBodyMax bounds a relayed request body, config edits and action
// input are small.
const proxyBodyMax = 1 << 20

// RestconfProxy serves /api/restconf/, RESTCONF for a page that is not
// part of this binary, see WEBUI_HOME.  Requests are relayed as the
// logged-in user, so what they may read, change and run is up to NACM,
// the same as for the pages here.  Such a page needs nothing added here
// to use a new part of the device's YANG model.
type RestconfProxy struct {
	RC *restconf.Client
}

func (h *RestconfProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// The path as sent, not r.URL.Path: a list key may hold an encoded
	// slash, 10.0.0.0%2F24, and decoding it would split the key in two
	path := strings.TrimPrefix(r.URL.EscapedPath(), "/api/restconf")
	if path != "/data" && !strings.HasPrefix(path, "/data/") &&
		!strings.HasPrefix(path, "/operations/") && !strings.HasPrefix(path, "/ds/") {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, proxyBodyMax))
	if err != nil {
		http.Error(w, "Request too large", http.StatusRequestEntityTooLarge)
		return
	}

	resp, err := h.RC.Do(r.Context(), r.Method, path, r.URL.RawQuery, body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
