// SPDX-License-Identifier: MIT

package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"infix/webui/internal/restconf"
)

// DiagnosticsHandler serves the Maintenance > Diagnostics page: ping,
// traceroute, mtr (interactive hop table) and DNS lookup.  Long-running
// tools stream over SSE, mirroring the software-progress and logs-tail
// handlers; client disconnect (the Stop button closing the EventSource)
// cancels the request context, which CommandContext turns into a kill +
// reap of the spawned tool.
type DiagnosticsHandler struct {
	RC       *restconf.Client
	Template *template.Template
}

type diagPageData struct {
	PageData
	Interfaces []string
	Active     string
}

// Overview renders the Diagnostics page with Ping pre-selected.
// GET /maintenance/diagnostics
func (h *DiagnosticsHandler) Overview(w http.ResponseWriter, r *http.Request) {
	data := diagPageData{
		PageData:   newPageData(w, r, "diagnostics", "Diagnostics"),
		Interfaces: h.interfaceNames(r),
		Active:     "ping",
	}
	tmplName := "diagnostics.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// interfaceNames returns configured interface names for the source
// dropdown, best-effort: an error just yields an empty list (the form
// still offers "Auto").
func (h *DiagnosticsHandler) interfaceNames(r *http.Request) []string {
	if h.RC == nil {
		return nil
	}
	var wrap interfacesWrapper
	if err := h.RC.Get(r.Context(), "/data/ietf-interfaces:interfaces", &wrap); err != nil {
		log.Printf("diagnostics: interface list: %v", err)
		return nil
	}
	names := make([]string, 0, len(wrap.Interfaces.Interface))
	for _, i := range wrap.Interfaces.Interface {
		names = append(names, i.Name)
	}
	return names
}

// validIface returns iface only if it's a real configured interface;
// otherwise "" (treated as Auto).  An allow-list is the safe way to let
// a user-supplied string reach a -I/-i flag.
func (h *DiagnosticsHandler) validIface(r *http.Request, iface string) string {
	if iface == "" {
		return ""
	}
	for _, n := range h.interfaceNames(r) {
		if n == iface {
			return iface
		}
	}
	return ""
}

// ─── Input validation ────────────────────────────────────────────────

// validTarget guards the host/target argument before it reaches a
// spawned tool.  Rejects empty input, anything with whitespace, and a
// leading '-' (which a tool would parse as a flag — argument injection).
// Args are always passed as an explicit slice, never a shell string, so
// this plus the leading-dash check is sufficient.
func validTarget(s string) bool {
	if s == "" || len(s) > 255 {
		return false
	}
	if strings.HasPrefix(s, "-") {
		return false
	}
	for _, r := range s {
		if r <= ' ' || r == 0x7f {
			return false
		}
	}
	return true
}

func clampInt(s string, def, min, max int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

// ─── SSE helpers ─────────────────────────────────────────────────────

func sseLine(w io.Writer, f http.Flusher, s string) {
	// SSE data fields must be single-line; collapse any CR/LF.
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r", ""), "\n", " ")
	fmt.Fprintf(w, "event: line\ndata: %s\n\n", s)
	f.Flush()
}

func sseDone(w io.Writer, f http.Flusher) {
	fmt.Fprint(w, "event: done\ndata: end\n\n")
	f.Flush()
}

// ─── Run ─────────────────────────────────────────────────────────────

// diagCommand maps the request to a binary + argument slice, or ok=false
// for an unknown tool.
//
// Address family is forced with an explicit -4/-6 flag on the BASE binary
// rather than via the ping6/traceroute6 name.  A dual-stack hostname (A +
// AAAA) otherwise follows the system's getaddrinfo preference — usually
// IPv6 — so "IPv4" never actually took effect on hosts that only have v4
// routes.  ping, traceroute and mtr all accept -4/-6; nmap defaults to v4
// and only needs -6.  "auto" passes no flag and lets the tool decide
// (IPv6 literals and v4-only names resolve correctly without help).
func diagCommand(tool, target, family, iface string, q url.Values) (bin string, args []string, ok bool) {
	fam := ""
	switch family {
	case "4":
		fam = "-4"
	case "6":
		fam = "-6"
	}

	switch tool {
	case "ping":
		bin = "ping"
		args = []string{"-c", strconv.Itoa(clampInt(q.Get("count"), 3, 1, 100))}
		if fam != "" {
			args = append(args, fam)
		}
		if sz := q.Get("size"); sz != "" {
			args = append(args, "-s", strconv.Itoa(clampInt(sz, 56, 0, 65500)))
		}
		if iface != "" {
			args = append(args, "-I", iface)
		}
		args = append(args, "-W", "2", target)
		return bin, args, true

	case "traceroute":
		bin = "traceroute"
		args = []string{"-n", "-m", strconv.Itoa(clampInt(q.Get("maxhops"), 30, 1, 64))}
		if fam != "" {
			args = append(args, fam)
		}
		if iface != "" {
			args = append(args, "-i", iface)
		}
		args = append(args, target)
		return bin, args, true

	case "mtr":
		bin = "mtr"
		args = []string{"--raw", "-n"}
		if fam != "" {
			args = append(args, fam)
		}
		// A count means the user opted out of the default run-forever
		// mode; otherwise mtr streams continuously until Stop.
		if c := q.Get("count"); c != "" {
			args = append(args, "-c", strconv.Itoa(clampInt(c, 10, 1, 1000)))
		}
		if sz := q.Get("size"); sz != "" {
			args = append(args, "-s", strconv.Itoa(clampInt(sz, 56, 0, 65500)))
		}
		if iface != "" {
			args = append(args, "-I", iface)
		}
		args = append(args, target)
		return bin, args, true

	case "nmap":
		bin = "nmap"
		// -n: skip rDNS so scans stay quick and don't hang on slow
		// resolvers.  Scan profiles map to a fixed flag set — we never
		// pass free-form nmap options from the client.
		args = []string{"-n"}
		if family == "6" {
			args = append(args, "-6")
		}
		switch q.Get("scan") {
		case "quick":
			args = append(args, "-F") // top 100 ports
		case "services":
			args = append(args, "-sV") // service/version detection
		case "ping":
			args = append(args, "-sn") // host discovery only
		case "standard":
			// default top-1000 port scan, no extra flag
		}
		if iface != "" {
			args = append(args, "-e", iface)
		}
		args = append(args, target)
		return bin, args, true
	}
	return "", nil, false
}

// Run streams tool output as SSE.  Event protocol:
//
//	event: line  — a text line to append (ping, traceroute)
//	event: hop   — JSON hop stats to upsert into the mtr table
//	event: done  — the tool exited; client closes the stream
//	: comment    — keep-alive
//
// GET /maintenance/diagnostics/run?tool=&target=&iface=&family=&...
func (h *DiagnosticsHandler) Run(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	q := r.URL.Query()
	tool := q.Get("tool")
	target := q.Get("target")
	family := q.Get("family")
	if family == "" {
		family = "auto"
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	// A few bytes up front so the proxy commits and EventSource.onopen
	// fires promptly even before the tool emits anything.
	fmt.Fprint(w, ": stream opened\n\n")
	flusher.Flush()

	if !validTarget(target) {
		sseLine(w, flusher, "Error: invalid target")
		sseDone(w, flusher)
		return
	}
	iface := h.validIface(r, q.Get("iface"))

	bin, args, ok := diagCommand(tool, target, family, iface, q)
	if !ok {
		sseLine(w, flusher, "Error: unknown tool")
		sseDone(w, flusher)
		return
	}

	if tool == "mtr" {
		h.streamMtr(w, r, flusher, bin, args)
		return
	}
	h.streamLines(w, r, flusher, bin, args, func(line string) {
		sseLine(w, flusher, line)
	})
}

// streamLines spawns bin/args and invokes onLine for each output line,
// emitting a done event on exit.  stdout and stderr are merged so tool
// errors ("Network is unreachable", "unknown host") reach the user.
func (h *DiagnosticsHandler) streamLines(w http.ResponseWriter, r *http.Request, flusher http.Flusher, bin string, args []string, onLine func(string)) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...)

	// Real OS pipe (not io.Pipe): the write-end fd is handed directly to
	// the child, so there's no os/exec copy goroutine that could deadlock
	// when the reader stops draining.  When the child dies — including
	// the CommandContext SIGKILL on ctx cancel — every write end closes
	// and our read sees EOF, no matter whether anyone is reading.
	pr, pw, err := os.Pipe()
	if err != nil {
		sseLine(w, flusher, "Error: "+err.Error())
		sseDone(w, flusher)
		return
	}
	cmd.Stdout = pw
	cmd.Stderr = pw
	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		sseLine(w, flusher, "Error: "+err.Error())
		sseDone(w, flusher)
		return
	}
	pw.Close()       // child holds the only write end now
	defer pr.Close() // unblocks the scanner if we leave first
	// Reap the child so it doesn't linger as a zombie; ctx cancel kills
	// it, Wait then returns.
	go cmd.Wait() //nolint:errcheck

	lines := make(chan string, 128)
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 64*1024), 256*1024)
		for sc.Scan() {
			select {
			case lines <- sc.Text():
			case <-ctx.Done():
				return
			}
		}
	}()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case line, ok := <-lines:
			if !ok {
				sseDone(w, flusher)
				return
			}
			onLine(line)
		case <-heartbeat.C:
			fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
		}
	}
}

// mtrHop accumulates per-hop statistics from the raw record stream.
type mtrHop struct {
	host              string
	sent, recv        int
	last, best, worst float64
	sum               float64
}

// streamMtr runs mtr --raw and translates its record stream into hop
// updates.  Infix's mtr emits an `x <idx> <txid>` record for every probe
// SENT (not just replies), so loss is exact: 100*(sent-recv)/sent.
//
//	x <idx> <txid>        probe sent for hop idx
//	h <idx> <address>     host discovered at hop idx
//	p <idx> <usec> <txid> reply for hop idx, RTT in microseconds
func (h *DiagnosticsHandler) streamMtr(w http.ResponseWriter, r *http.Request, flusher http.Flusher, bin string, args []string) {
	hops := map[int]*mtrHop{}
	get := func(idx int) *mtrHop {
		hop := hops[idx]
		if hop == nil {
			hop = &mtrHop{host: "???"}
			hops[idx] = hop
		}
		return hop
	}
	emit := func(idx int, hop *mtrHop) {
		loss := 0.0
		if hop.sent > 0 {
			loss = 100 * float64(hop.sent-hop.recv) / float64(hop.sent)
		}
		avg := 0.0
		if hop.recv > 0 {
			avg = hop.sum / float64(hop.recv)
		}
		payload, _ := json.Marshal(map[string]any{
			"idx": idx, "host": hop.host, "loss": loss, "snt": hop.sent,
			"last": hop.last, "avg": avg, "best": hop.best, "worst": hop.worst,
		})
		fmt.Fprintf(w, "event: hop\ndata: %s\n\n", payload)
		flusher.Flush()
	}

	h.streamLines(w, r, flusher, bin, args, func(line string) {
		f := strings.Fields(line)
		if len(f) < 2 {
			return
		}
		idx, err := strconv.Atoi(f[1])
		if err != nil {
			return
		}
		switch f[0] {
		case "x": // probe sent
			hop := get(idx)
			hop.sent++
			emit(idx, hop)
		case "h": // host discovered
			if len(f) >= 3 {
				get(idx).host = f[2]
			}
		case "p": // reply received
			if len(f) >= 3 {
				usec, err := strconv.Atoi(f[2])
				if err != nil {
					return
				}
				rtt := float64(usec) / 1000
				hop := get(idx)
				hop.recv++
				hop.last = rtt
				hop.sum += rtt
				if hop.recv == 1 || rtt < hop.best {
					hop.best = rtt
				}
				if rtt > hop.worst {
					hop.worst = rtt
				}
				emit(idx, hop)
			}
		}
	})
}

// ─── DNS lookup ──────────────────────────────────────────────────────

// Resolve does a one-shot name lookup via getent and returns an HTML
// fragment for the client to swap into the result pane.
// GET /maintenance/diagnostics/resolve?name=&family=
func (h *DiagnosticsHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	render := func(data map[string]any) {
		if err := h.Template.ExecuteTemplate(w, "diag-dns-result", data); err != nil {
			log.Printf("template error: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}
	if !validTarget(name) {
		render(map[string]any{"Name": name, "Error": "invalid name"})
		return
	}

	prog := "ahosts"
	switch r.URL.Query().Get("family") {
	case "4":
		prog = "ahostsv4"
	case "6":
		prog = "ahostsv6"
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, "getent", prog, name).Output()

	// getent ahosts prints each address three times (STREAM/DGRAM/RAW);
	// keep the first occurrence of each, preserving order.
	var addrs []string
	seen := map[string]bool{}
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 || seen[fields[0]] {
			continue
		}
		seen[fields[0]] = true
		addrs = append(addrs, fields[0])
	}

	if len(addrs) == 0 {
		render(map[string]any{"Name": name, "Error": "no addresses found"})
		return
	}
	render(map[string]any{"Name": name, "Addrs": addrs})
}
