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
	flag.Parse()

	store := auth.NewSessionStore()

	rc := restconf.NewClient(*restconfURL, *insecureTLS)

	schemaCache := schema.NewCache(rc, *yangCacheDir, osVersionID())
	schemaCache.LoadFromCacheBackground() // fast, no HTTP — uses whatever is already on disk

	tmplFS, err := fs.Sub(templateFS, "templates")
	if err != nil {
		log.Fatalf("template fs: %v", err)
	}

	stFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("static fs: %v", err)
	}

	handler, err := server.New(store, rc, schemaCache, tmplFS, stFS, parseHomes(os.Getenv("WEBUI_HOME")))
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

// parseHomes reads WEBUI_HOME, a space separated list of user=path.  A
// user listed here lands on path after login instead of the dashboard,
// and is kept there, see homeAllowed.  The page itself is served by
// nginx, it is not part of this binary.
func parseHomes(v string) map[string]string {
	homes := make(map[string]string)
	for _, f := range strings.Fields(v) {
		user, path, ok := strings.Cut(f, "=")
		// A browser takes //host and /\host for another host
		if !ok || user == "" || path == "/" || !strings.HasPrefix(path, "/") ||
			strings.HasPrefix(path, "//") || strings.Contains(path, "\\") {
			log.Printf("WEBUI_HOME: ignoring %q, want user=/path", f)
			continue
		}
		homes[user] = path
	}
	return homes
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
