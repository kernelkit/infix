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

	"infix/webui/internal/auth"
	"infix/webui/internal/restconf"
	"infix/webui/internal/schema"
	"infix/webui/internal/server"
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

	listen := flag.String("listen", ":10000", "address to listen on")
	restconfURL := flag.String("restconf", defaultRC, "RESTCONF base URL")
	insecureTLS := flag.Bool("insecure-tls", envBool("INSECURE_TLS"), "disable TLS certificate verification")
	yangCacheDir := flag.String("yang-cache-dir", "/var/cache/webui/yang", "directory for cached YANG schema files")
	schemaFile := flag.String("schema", "/usr/share/webui/schema.json", "schema compiled at build time, used instead of the YANG cache when it exists")
	compileDir := flag.String("compile-schema", "", "compile the YANG files in `dir` to the -schema file and exit")
	flag.Parse()

	if *compileDir != "" {
		mgr, err := schema.Load(*compileDir)
		if err == nil {
			err = mgr.Write(*schemaFile)
		}
		if err != nil {
			log.Fatalf("compile schema: %v", err)
		}
		return
	}

	store := auth.NewSessionStore()

	rc := restconf.NewClient(*restconfURL, *insecureTLS)

	schemaCache := schema.NewCache(rc, *yangCacheDir, osVersionID())
	if err := schemaCache.LoadFile(*schemaFile); err == nil {
		log.Printf("schema: loaded %s", *schemaFile)
	} else {
		if !os.IsNotExist(err) {
			log.Printf("schema: %v, using the YANG cache", err)
		}
		schemaCache.LoadFromCacheBackground() // fast, no HTTP — uses whatever is already on disk
	}

	tmplFS, err := fs.Sub(templateFS, "templates")
	if err != nil {
		log.Fatalf("template fs: %v", err)
	}

	stFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("static fs: %v", err)
	}

	handler, err := server.New(store, rc, schemaCache, tmplFS, stFS, auth.ParseHomes(os.Getenv("WEBUI_HOME")))
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

// osVersionID is VERSION_ID from /etc/os-release, empty when unavailable.
func osVersionID() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, "VERSION_ID="); ok {
			return strings.Trim(v, "\"")
		}
	}
	return ""
}
