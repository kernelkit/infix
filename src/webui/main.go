// SPDX-License-Identifier: MIT

package main

import (
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kernelkit/webui/internal/auth"
	"github.com/kernelkit/webui/internal/restconf"
	"github.com/kernelkit/webui/internal/schema"
	"github.com/kernelkit/webui/internal/server"
)

//go:embed templates/*
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

func main() {
	defaultRC := "http://localhost:8080/restconf"
	if env := os.Getenv("RESTCONF_URL"); env != "" {
		defaultRC = env
	}

	listen := flag.String("listen", ":8080", "address to listen on")
	restconfURL := flag.String("restconf", defaultRC, "RESTCONF base URL")
	sessionKey := flag.String("session-key", "/var/lib/misc/webui-session.key", "path to persistent session key file")
	insecureTLS := flag.Bool("insecure-tls", envBool("INSECURE_TLS"), "disable TLS certificate verification")
	yangCacheDir := flag.String("yang-cache-dir", "/var/cache/webui/yang", "directory for cached YANG schema files")
	flag.Parse()

	store, err := auth.NewSessionStore(*sessionKey)
	if err != nil {
		log.Fatalf("session store: %v", err)
	}

	rc := restconf.NewClient(*restconfURL, *insecureTLS)

	schemaCache := schema.NewCache(rc, *yangCacheDir)
	schemaCache.LoadFromCacheBackground() // fast, no HTTP — uses whatever is already on disk

	tmplFS, err := fs.Sub(templateFS, "templates")
	if err != nil {
		log.Fatalf("template fs: %v", err)
	}

	stFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("static fs: %v", err)
	}

	handler, err := server.New(store, rc, schemaCache, tmplFS, stFS)
	if err != nil {
		log.Fatalf("server setup: %v", err)
	}

	log.Printf("listening on %s (restconf %s)", *listen, *restconfURL)
	srv := &http.Server{
		Addr:              *listen,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("listen: %v", err)
	}
}

func envBool(key string) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return false
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
