// SPDX-License-Identifier: MIT
package main

import (
	"context"
	"embed"
	"flag"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

//go:embed browse.html
var browseHTML string

//go:embed static
var staticFS embed.FS

var tmpl = template.Must(template.New("browse").Parse(browseHTML))

type pageData struct {
	Hosts map[string][]Service
}

func browseHandler(w http.ResponseWriter, r *http.Request) {
	hosts := scan()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, pageData{Hosts: hosts}); err != nil {
		log.Printf("template: %v", err)
	}
}

func main() {
	listen := flag.String("l", "127.0.0.1:8000", "listen address:port")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/", browseHandler)
	mux.HandleFunc("/netbrowse", browseHandler)

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
