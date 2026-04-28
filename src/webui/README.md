# Infix WebUI

A lightweight web management interface for [Infix][1] network devices,
built with Go and [htmx][2].

The WebUI communicates with the device over [RESTCONF][3] (RFC 8040),
presenting the same operational data available through the Infix CLI in
a browser-friendly format.

## Features

- **Dashboard** -- system info, hardware, sensors, and interface summary
  with bridge member grouping
- **Interfaces** -- list with status, addresses, and per-type detail;
  click through to a detail page with live-updating counters, WiFi
  station table, scan results, WireGuard peers, and ethernet frame
  statistics
- **Firewall** -- zone-to-zone policy matrix
- **Keystore** -- symmetric and asymmetric key display
- **Firmware** -- slot overview, install from URL with live progress
- **Reboot** -- two-phase status polling (wait down, wait up)
- **Config download** -- startup datastore as JSON


## Building

Requires Go 1.22 or later.

```sh
make build
```

Produces a statically linked `webui` binary with all templates,
CSS, and JS embedded.

Cross-compile for the target:

```sh
GOOS=linux GOARCH=arm64 make build
```


## Running

```sh
./webui --restconf https://192.168.0.1/restconf --listen :8080
```

| **Flag**          | **Default**                       | **Description**                           |
|-------------------|-----------------------------------|-------------------------------------------|
| `--listen`        | `:8080`                           | Address to listen on                      |
| `--restconf`      | `http://localhost:8080/restconf`  | RESTCONF base URL of the device           |
| `--session-key`   | `/var/lib/misc/webui-session.key` | Path to persistent session encryption key |
| `--insecure-tls`  | `false`                           | Disable TLS certificate verification      |

The RESTCONF URL can also be set via the `RESTCONF_URL` environment
variable.


## Development

Point `RESTCONF_URL` at a running Infix device and start the dev
server:

```sh
make dev ARGS="--restconf https://192.168.0.1/restconf"
```

This runs `go run .` on port 8080 with `--insecure-tls` already set.


## Architecture

```
Browser ──htmx──▶ Go server ──RESTCONF──▶ Infix device (rousette/sysrepo)
```

- **Single binary** -- templates, CSS, JS, and images are embedded via
  `go:embed`
- **Server-side rendering** -- Go `html/template` with per-page parsing
  to avoid `{{define "content"}}` collisions
- **htmx SPA navigation** -- sidebar links use `hx-get` / `hx-target`
  for partial page updates with `hx-push-url` for browser history
- **Stateless sessions** -- AES-256-GCM encrypted cookies carry
  credentials (needed for every RESTCONF call); no server-side session
  store
- **Live polling** -- counters update every 5s, firmware progress every
  3s, all via htmx triggers

```
main.go                          Entry point, flags, embedded FS
internal/
  auth/                          Login, logout, session (AES-GCM cookies)
  restconf/                      HTTP client (Get, GetRaw, Post, PostJSON)
  handlers/                      Page handlers
    dashboard.go                   Dashboard, hardware, sensors
    interfaces.go                  Interface list, detail, counters
    firewall.go                    Zone matrix
    keystore.go                    Key display
    system.go                      Firmware, reboot, config download
  server/
    server.go                    Route registration, template wiring, middleware
templates/
  layouts/                       base.html (shell), sidebar.html
  pages/                         Per-page templates (one per route)
  fragments/                     htmx partial fragments
static/
  css/style.css                  All styles
  js/htmx.min.js                htmx library
  img/                           Logo, favicon
```


## License

See [LICENSE](LICENSE).

[1]: https://github.com/kernelkit/infix
[2]: https://htmx.org
[3]: https://datatracker.ietf.org/doc/html/rfc8040
