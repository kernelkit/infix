// SPDX-License-Identifier: MIT

// Package netbrowse serves the mDNS network browser, the page behind
// https://network.local.  It needs no login and has a listener of its own.
package netbrowse

import (
	"embed"
	"encoding/json"
	"log"
	"net/http"
)

//go:embed browse.html
var browseHTML []byte

//go:embed static
var staticFS embed.FS

func browseHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(browseHTML)
}

func dataHandler(scanFn func() map[string]Host) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hosts := scanFn()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if err := json.NewEncoder(w).Encode(hosts); err != nil {
			log.Printf("json: %v", err)
		}
	}
}

// New returns the handler for the network browser.  The discovery backend
// is auto, avahi or operational.
func New(backend string) http.Handler {
	var scanFn func() map[string]Host
	switch backend {
	case "operational":
		scanFn = scanOperational
	case "avahi":
		scanFn = scan
	default:
		scanFn = scanAuto
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", browseHandler)
	mux.HandleFunc("/netbrowse", browseHandler)
	mux.HandleFunc("/data", dataHandler(scanFn))
	mux.HandleFunc("/netbrowse/data", dataHandler(scanFn))
	mux.Handle("/static/", http.FileServer(http.FS(staticFS)))

	return mux
}
