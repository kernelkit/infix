// SPDX-License-Identifier: MIT

package handlers

import (
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// LogsHandler serves the Maintenance > Logs page.  Reads from a fixed
// allow-list of /var/log files; the file path is never user-controlled,
// so no traversal risk.  Live tail spawns `tail -F` whose lifetime is
// tied to the SSE request context — client disconnect kills the child.
type LogsHandler struct {
	Template *template.Template
}

// ─── Source allow-list ───────────────────────────────────────────────

type logSource struct {
	Key   string
	Label string
	Path  string
}

var logSources = []logSource{
	{"messages", "Messages", "/var/log/messages"},
	{"syslog", "Syslog", "/var/log/syslog"},
	{"kern", "Kernel", "/var/log/kern.log"},
	{"auth", "Auth", "/var/log/auth.log"},
	{"routing", "Routing", "/var/log/routing"},
	{"firewall", "Firewall", "/var/log/firewall.log"},
	{"upgrade", "Upgrade", "/var/log/upgrade.log"},
	{"debug", "Debug", "/var/log/debug"},
	{"container", "Container", "/var/log/container"},
	{"mail", "Mail", "/var/log/mail.log"},
}

const (
	initialLines    = 250
	earlierLines    = 250
	tailFlushDelay  = 200 * time.Millisecond
	tailMaxBatch    = 50
	tailHeartbeatMs = 15 * time.Second
)

func lookupSource(key string) (logSource, bool) {
	for _, s := range logSources {
		if s.Key == key {
			return s, true
		}
	}
	return logSource{}, false
}

// ─── Template data ───────────────────────────────────────────────────

type logsPageData struct {
	PageData
	Sources []sourceMeta
	Active  string
	Source  sourceContent
}

type sourceMeta struct {
	Key   string
	Label string
	Path  string
	Empty bool
}

// loadCursor describes the next Load earlier / Load previous action.
// File is "current" while still paging within the active log file, or
// "rot0", "rot1", … once the user has crossed into older rotations.
// FileLabel is the rotation file's basename, shown on the button when
// IsPrev is true.  A nil cursor means "no more older content."
type loadCursor struct {
	File      string
	Skip      int
	FileLabel string
	IsPrev    bool
}

type sourceContent struct {
	sourceMeta
	Lines []logLine
	Next  *loadCursor // nil when no older content remains
}

type logLine struct {
	Text  string
	Class string
}

// earlierData mirrors sourceContent's field names for the logs-earlier
// template so the same fragment renders both the initial buffer and
// the Earlier-handler response.
type earlierData struct {
	Key   string
	Lines []logLine
	Next  *loadCursor
}

// ─── Classification ──────────────────────────────────────────────────

func classify(text string) string {
	lower := strings.ToLower(text)
	for _, kw := range []string{"panic", "kernel: bug", "fatal", "error", "fail", "crit"} {
		if strings.Contains(lower, kw) {
			return "err"
		}
	}
	for _, kw := range []string{"warning", "warn:", "warn ", "deprecated"} {
		if strings.Contains(lower, kw) {
			return "warn"
		}
	}
	return ""
}

func classifyLines(raw []string) []logLine {
	out := make([]logLine, len(raw))
	for i, t := range raw {
		out[i] = logLine{Text: t, Class: classify(t)}
	}
	return out
}

// ─── File reading ────────────────────────────────────────────────────

func openReader(path string) (io.Reader, func(), error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	if strings.HasSuffix(path, ".gz") {
		zr, err := gzip.NewReader(f)
		if err != nil {
			f.Close()
			return nil, nil, err
		}
		return zr, func() { zr.Close(); f.Close() }, nil
	}
	return f, func() { f.Close() }, nil
}

func readAllLines(path string) ([]string, error) {
	r, done, err := openReader(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer done()

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 256*1024), 1024*1024)

	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

// rotationsFor returns rotation files for a source, newest first.  The
// current file is NOT included.  Suffix order matches logrotate
// convention: .0 = most recent rotation, .1 / .1.gz next, and so on.
// Files that don't match the numeric suffix pattern are ignored.
func rotationsFor(path string) []string {
	matches, err := filepath.Glob(path + ".*")
	if err != nil {
		return nil
	}
	type rot struct {
		Path string
		N    int
	}
	var rots []rot
	for _, m := range matches {
		rest := strings.TrimPrefix(m, path+".")
		rest = strings.TrimSuffix(rest, ".gz")
		if n, err := strconv.Atoi(rest); err == nil {
			rots = append(rots, rot{Path: m, N: n})
		}
	}
	sort.Slice(rots, func(i, j int) bool { return rots[i].N < rots[j].N })

	out := make([]string, len(rots))
	for i, r := range rots {
		out[i] = r.Path
	}
	return out
}

// resolveCursor maps a cursor's File field to the actual filesystem
// path.  Returns the empty string if the cursor points past the end of
// available history (e.g. "rot3" on a source with only two rotations).
func resolveCursor(src logSource, file string) string {
	if file == "" || file == "current" {
		return src.Path
	}
	if !strings.HasPrefix(file, "rot") {
		return ""
	}
	idx, err := strconv.Atoi(strings.TrimPrefix(file, "rot"))
	if err != nil || idx < 0 {
		return ""
	}
	rots := rotationsFor(src.Path)
	if idx >= len(rots) {
		return ""
	}
	return rots[idx]
}

// nextCursor computes the cursor for the click AFTER the one we're
// about to serve.  `file` is the file just read; `skip+returned` is
// where in that file the next paging click would resume.  When the
// file is exhausted we cross into the next older rotation; when no
// more rotations exist, returns nil.
func nextCursor(src logSource, file string, skip, returned, total int) *loadCursor {
	nextSkip := skip + returned
	if nextSkip < total {
		// Same file — continuing back through it.
		c := &loadCursor{File: file, Skip: nextSkip}
		if file != "current" {
			c.IsPrev = true
			c.FileLabel = filepath.Base(resolveCursor(src, file))
		}
		return c
	}

	// Current/this file exhausted.  Cross into next older rotation.
	var nextIdx int
	if file == "current" {
		nextIdx = 0
	} else {
		idx, _ := strconv.Atoi(strings.TrimPrefix(file, "rot"))
		nextIdx = idx + 1
	}
	rots := rotationsFor(src.Path)
	if nextIdx >= len(rots) {
		return nil
	}
	return &loadCursor{
		File:      "rot" + strconv.Itoa(nextIdx),
		Skip:      0,
		FileLabel: filepath.Base(rots[nextIdx]),
		IsPrev:    true,
	}
}

// readFileWindow returns up to `limit` lines from the end of `path`,
// skipping the last `skip` lines, plus the file's total line count.
func readFileWindow(path string, skip, limit int) (lines []string, total int, err error) {
	all, err := readAllLines(path)
	if err != nil {
		return nil, 0, err
	}
	total = len(all)
	end := total - skip
	if end < 0 {
		end = 0
	}
	start := end - limit
	if start < 0 {
		start = 0
	}
	return all[start:end], total, nil
}

func sourceMetas() []sourceMeta {
	out := make([]sourceMeta, 0, len(logSources))
	for _, s := range logSources {
		empty := true
		if info, err := os.Stat(s.Path); err == nil {
			empty = info.Size() == 0
		}
		out = append(out, sourceMeta{Key: s.Key, Label: s.Label, Path: s.Path, Empty: empty})
	}
	return out
}

func buildContent(src logSource) sourceContent {
	lines, total, err := readFileWindow(src.Path, 0, initialLines)
	if err != nil {
		log.Printf("logs read %s: %v", src.Key, err)
	}
	return sourceContent{
		sourceMeta: sourceMeta{Key: src.Key, Label: src.Label, Path: src.Path, Empty: total == 0},
		Lines:      classifyLines(lines),
		Next:       nextCursor(src, "current", 0, len(lines), total),
	}
}

// ─── Handlers ────────────────────────────────────────────────────────

func (h *LogsHandler) Overview(w http.ResponseWriter, r *http.Request) {
	src, _ := lookupSource("messages")
	data := logsPageData{
		PageData: newPageData(w, r, "logs", "Logs"),
		Sources:  sourceMetas(),
		Active:   "messages",
		Source:   buildContent(src),
	}
	tmplName := "logs.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (h *LogsHandler) Fragment(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("name")
	src, ok := lookupSource(key)
	if !ok {
		http.NotFound(w, r)
		return
	}
	data := logsPageData{
		Sources: sourceMetas(),
		Active:  key,
		Source:  buildContent(src),
	}
	if err := h.Template.ExecuteTemplate(w, "logs-card", data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// Earlier returns the next page of older lines plus the button to load
// the page after that.  The button does an outerHTML swap on itself so
// the OLD button is replaced by [NEW button, NEW lines], keeping
// chronological order in the DOM.
// GET /maintenance/logs/{name}/earlier?file=current|rot0|…&skip=N
func (h *LogsHandler) Earlier(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("name")
	src, ok := lookupSource(key)
	if !ok {
		http.NotFound(w, r)
		return
	}
	file := r.URL.Query().Get("file")
	if file == "" {
		file = "current"
	}
	path := resolveCursor(src, file)
	if path == "" {
		http.NotFound(w, r)
		return
	}
	skip, _ := strconv.Atoi(r.URL.Query().Get("skip"))

	lines, total, err := readFileWindow(path, skip, earlierLines)
	if err != nil {
		log.Printf("logs earlier %s: %v", path, err)
	}
	data := earlierData{
		Key:   key,
		Lines: classifyLines(lines),
		Next:  nextCursor(src, file, skip, len(lines), total),
	}
	if err := h.Template.ExecuteTemplate(w, "logs-earlier", data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// Tail streams new lines as Server-Sent Events.  `tail -F -n 0`
// handles rotation reopens and starts from "now" — the client already
// has the existing buffer rendered.  Lines are debounced and emitted
// in HTML fragments matching the regular logs-line markup, so the
// client just appends.
// GET /maintenance/logs/{name}/tail
func (h *LogsHandler) Tail(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("name")
	src, ok := lookupSource(key)
	if !ok {
		http.NotFound(w, r)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Send an immediate comment so the response body has a few bytes on
	// the wire right away.  Some intermediate proxies hold the headers
	// until they see enough body content to commit to a buffering
	// strategy; without this nudge, EventSource onopen can wait many
	// seconds before firing on the client side.
	fmt.Fprint(w, ": stream opened\n\n")
	flusher.Flush()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	cmd := exec.CommandContext(ctx, "tail", "-F", "-n", "0", src.Path)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("logs tail stdout %s: %v", key, err)
		return
	}
	if err := cmd.Start(); err != nil {
		log.Printf("logs tail start %s: %v", key, err)
		return
	}
	// Without this Wait, every client reconnect leaves the killed tail
	// child as a zombie until webui itself exits — the SSE auto-reconnect
	// cycle accumulated 50+ <defunct> tails in a few minutes of idle.
	// cmd.Wait blocks until the process is reaped; CommandContext SIGKILLs
	// it on ctx cancel, so Wait returns "signal: killed" which we ignore.
	defer func() { _ = cmd.Wait() }()

	linesCh := make(chan string, 200)
	go func() {
		defer close(linesCh)
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 256*1024), 1024*1024)
		for scanner.Scan() {
			select {
			case linesCh <- scanner.Text():
			case <-ctx.Done():
				return
			}
		}
	}()

	flush := func(pending []string) {
		if len(pending) == 0 {
			return
		}
		var buf strings.Builder
		for _, text := range pending {
			cls := classify(text)
			if cls != "" {
				fmt.Fprintf(&buf, `<div class="logs-line logs-%s">%s</div>`, cls, template.HTMLEscapeString(text))
			} else {
				fmt.Fprintf(&buf, `<div class="logs-line">%s</div>`, template.HTMLEscapeString(text))
			}
		}
		payload := strings.ReplaceAll(buf.String(), "\n", " ")
		fmt.Fprintf(w, "event: lines\ndata: %s\n\n", payload)
		flusher.Flush()
	}

	heartbeat := time.NewTicker(tailHeartbeatMs)
	defer heartbeat.Stop()

	var pending []string
	var batchTimer *time.Timer
	var batchC <-chan time.Time

	for {
		select {
		case <-ctx.Done():
			return
		case text, ok := <-linesCh:
			if !ok {
				flush(pending)
				return
			}
			pending = append(pending, text)
			if len(pending) >= tailMaxBatch {
				if batchTimer != nil {
					batchTimer.Stop()
					batchTimer = nil
					batchC = nil
				}
				flush(pending)
				pending = pending[:0]
			} else if batchTimer == nil {
				batchTimer = time.NewTimer(tailFlushDelay)
				batchC = batchTimer.C
			}
		case <-batchC:
			batchTimer = nil
			batchC = nil
			flush(pending)
			pending = pending[:0]
		case <-heartbeat.C:
			fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
		}
	}
}

// Download streams the full archive for a source as a single
// text/plain attachment — every rotation file (oldest first, gunzipped
// on the fly) followed by the current file.  Users running this from a
// web browser want one click and a complete chronological log; if they
// need just the current file there's always `cat /var/log/X` over SSH.
// GET /maintenance/logs/{name}/download
func (h *LogsHandler) Download(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("name")
	src, ok := lookupSource(key)
	if !ok {
		http.NotFound(w, r)
		return
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "device"
	}
	ts := time.Now().UTC().Format("20060102-1504")
	fname := fmt.Sprintf("%s-%s-%s.log", key, hostname, ts)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, fname))

	// Concatenate oldest-first: highest-N rotation down through .0, then
	// current.  rotationsFor returns newest-first so iterate in reverse.
	rots := rotationsFor(src.Path)
	for i := len(rots) - 1; i >= 0; i-- {
		streamFile(w, rots[i])
	}
	streamFile(w, src.Path)
}

func streamFile(w io.Writer, path string) {
	rd, done, err := openReader(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("logs stream %s: %v", path, err)
		}
		return
	}
	defer done()
	io.Copy(w, rd)
}
