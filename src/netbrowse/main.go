// SPDX-License-Identifier: MIT
package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

//go:embed browse.html
var browseHTML []byte

//go:embed static
var staticFS embed.FS

func browseHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(browseHTML)
}

func dataHandler(w http.ResponseWriter, r *http.Request) {
	hosts := scan()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(hosts); err != nil {
		log.Printf("json: %v", err)
	}
}

func main() {
	listen := flag.String("l", "127.0.0.1:8000", "listen address:port")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/", browseHandler)
	mux.HandleFunc("/netbrowse", browseHandler)
	mux.HandleFunc("/data", dataHandler)
	mux.HandleFunc("/netbrowse/data", dataHandler)

	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	srv := &http.Server{Addr: *listen, Handler: mux}

	go func() {
		sigch := make(chan os.Signal, 1)
		signal.Notify(sigch, syscall.SIGTERM, syscall.SIGINT)
		<-sigch
		log.Println("shutting down")
		srv.Shutdown(context.Background())
	}()

	log.Printf("listening on %s", *listen)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
