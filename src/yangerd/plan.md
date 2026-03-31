# yangerd Phase 1: Core + ietf-system

**Status**: Complete  
**Started**: 2026-03-28  
**Design doc**: `src/statd/doc/yangerd-design.md`

## Overview

Build the yangerd Go daemon core infrastructure alongside the simplest
module (`ietf-system`). This establishes the project skeleton, in-memory
tree, IPC server, collector framework, and Buildroot packaging. All
subsequent modules build on this foundation.

## Constraints

- No CGo (hard, non-negotiable — design doc Section 2.3)
- Not a sysrepo plugin — yangerd has no sysrepo dependency
- `uint64`/`int64`/`decimal64` YANG types serialized as JSON strings (RFC 7951 / libyang)
- Socket owned by `root:yangerd`, permissions `0660`
- Sources at `src/yangerd/`, module path `github.com/kernelkit/infix/src/yangerd`
- Buildroot: vendored deps (`GOFLAGS=-mod=vendor`), `$(eval $(golang-package))`
- NTP handled by separate `internal/collector/ntp.go` — excluded from system.go
- Ignore gRPC for FRR

## Steps

### Step 1: Project scaffolding
- **Status**: ✅ complete
- `src/yangerd/go.mod` — `module github.com/kernelkit/infix/src/yangerd`, go 1.21
- `cmd/yangerd/main.go` — daemon entry: config → socket → tree → collectors → ready → signal → IPC serve
- `cmd/yangerctl/main.go` — CLI: get/health/dump subcommands with --socket/--timeout flags
- Directory tree per design doc Section 6

### Step 2: Core tree (`internal/tree/tree.go`)
- **Status**: ✅ complete
- `modelEntry` struct: `sync.RWMutex`, `json.RawMessage`, `time.Time`
- `Tree` struct: top-level `sync.RWMutex` protecting `map[string]*modelEntry`
- Methods: `New()`, `Set()`, `Get()`, `GetMulti()`, `Keys()`, `Info()`
- Double-checked locking in `Set()` for new keys
- Eventual consistency in `GetMulti()` (per-model read locks, not snapshot)

### Step 3: IPC protocol + server
- **Status**: ✅ complete
- `internal/ipc/protocol.go` — framing (ver:1 + uint32 BE length + JSON), Request/Response types
- `internal/ipc/server.go` — Unix socket listener, connection handler, method routing, ready flag
- `internal/ipc/client.go` — Dial/query helper for yangerctl

### Step 4: Collector framework
- **Status**: ✅ complete
- `internal/collector/collector.go` — Collector interface + RunAll() scheduling
- `internal/collector/runner.go` — CommandRunner + FileReader interfaces + production impls

### Step 5: System collector (`internal/collector/system.go`)
- **Status**: ✅ complete
- ~658 lines implementing full ietf-system data collection
- Produces: `ietf-system:system` and `ietf-system:system-state` tree keys
- Sub-methods: addHostname, addTimezone, addUsers, addPlatform, addClock,
  addSoftware, addSoftwareSlots, getBootOrder, addDNS, addServices, addResourceUsage,
  addMemory, addLoadAvg, addFilesystems

### Step 6: Config (`internal/config/config.go`)
- **Status**: ✅ complete
- Env var parsing: YANGERD_SOCKET, LOG_LEVEL, STARTUP_TIMEOUT, POLL_INTERVAL_SYSTEM,
  ENABLE_WIFI/CONTAINERS/GPS

### Step 7: Wire main.go startup
- **Status**: ✅ complete
- Config → socket → tree → collectors → ready → signal handling → IPC serve

### Step 8: yangerctl (`cmd/yangerctl/main.go`)
- **Status**: ✅ complete
- Subcommands: get, health, dump with --socket/--timeout flags

### Step 9: Tests
- **Status**: ✅ complete
- `internal/tree/tree_test.go` — Set/Get/GetMulti/Keys/Info + concurrent access (5 tests)
- `internal/ipc/protocol_test.go` — framing round-trip, version mismatch, oversized payload (3 tests)
- `internal/ipc/server_test.go` — get/dump/health/notReady/unknownMethod (5 tests)
- `internal/collector/system_test.go` — 17 tests covering all data sources, edge cases, failures
- `internal/testutil/mock.go` — reusable MockRunner + MockFileReader
- All tests pass with `-race` flag

### Step 10: Buildroot packaging
- **Status**: ✅ complete
- `package/yangerd/yangerd.mk` — golang-package recipe with build targets + feature flags
- `package/yangerd/Config.in` — Kconfig entry with Go arch dependency
- `package/yangerd/yangerd.svc` — Finit service definition
- `package/Config.in` — source entry added

## Bug fixes found during testing

- **`fmt.Sprintf("%v", float64)` producing scientific notation**: In `addServices()`, JSON-decoded
  float64 values like `4096000.0` were formatted as `"4.096e+06"` instead of `"4096000"`. Fixed by
  using `strconv.Itoa(toInt(...))` for uint64 string encoding. Same fix applied to slot size in
  `addSoftwareSlots()`.

## File inventory

### Production code (12 files)
| File | Lines | Description |
|------|-------|-------------|
| `go.mod` | 3 | Module definition |
| `internal/config/config.go` | ~85 | Environment variable parsing |
| `internal/tree/tree.go` | 119 | Per-model locked tree store |
| `internal/ipc/protocol.go` | ~120 | IPC framing + request/response types |
| `internal/ipc/server.go` | ~150 | Unix socket server |
| `internal/ipc/client.go` | ~80 | Client helper for yangerctl |
| `internal/collector/collector.go` | 51 | Collector interface + RunAll scheduler |
| `internal/collector/runner.go` | 40 | CommandRunner/FileReader interfaces |
| `internal/collector/system.go` | 658 | System data collector |
| `cmd/yangerd/main.go` | ~100 | Daemon entry point |
| `cmd/yangerctl/main.go` | ~120 | CLI diagnostic tool |

### Test code (5 files)
| File | Tests | Description |
|------|-------|-------------|
| `internal/tree/tree_test.go` | 5 | Tree operations + concurrency |
| `internal/ipc/protocol_test.go` | 3 | Framing protocol |
| `internal/ipc/server_test.go` | 5 | IPC server end-to-end |
| `internal/collector/system_test.go` | 17 | System collector with mocks |
| `internal/testutil/mock.go` | — | Shared test mocks |

### Buildroot packaging (4 files)
| File | Description |
|------|-------------|
| `package/yangerd/yangerd.mk` | Buildroot Go package recipe |
| `package/yangerd/Config.in` | Kconfig menu entry |
| `package/yangerd/yangerd.svc` | Finit service definition |
| `package/Config.in` | Updated: added yangerd source |

## Reference files

| File | Purpose |
|------|---------|
| `src/statd/doc/yangerd-design.md` | Authoritative design document |
| `src/statd/python/yanger/ietf_system.py` | Python reference impl (461 lines) |
| `src/confd/yang/confd/ietf-system@2014-08-06.yang` | Standard YANG model |
| `src/confd/yang/confd/infix-system.yang` | Infix augmentations |
| `src/confd/yang/confd/infix-system-software.yang` | Software submodule |
| `src/netbrowse/` | Existing Go project (pattern reference) |
| `package/netbrowse/netbrowse.mk` | Buildroot Go package template |

## Notes

- NTP data is handled by `internal/collector/ntp.go` (Phase 2), not system.go
- DNS statistics (cache-size, cache-hits, cache-misses from infix-system.yang) not in current Python impl
- `addFilesystems` uses `syscall.Statfs()` directly (not mockable via FileReader); not unit-tested
- `addHostname` uses `os.Hostname()` directly; not unit-tested in isolation
- `addTimezone` uses `filepath.EvalSymlinks()` directly; not unit-tested in isolation

---

# yangerd Phase 2: Polling Collectors

**Status**: Complete
**Started**: 2026-03-28
**Completed**: 2026-03-29

## Overview

Implement all polling-based collectors that replace the Python yanger
scripts. Reactive collectors (netlink/interfaces, ZAPI/routing-table,
LLDP, D-Bus/DHCP/firewall) are deferred to Phase 3.

## Collectors implemented

### RoutingCollector (`routing.go`, ~748 lines)
- **Tree key**: `ietf-routing:routing`
- **Interval**: 10s (configurable: `YANGERD_POLL_INTERVAL_ROUTING`)
- Merges OSPF, RIP, and BFD into a single `control-plane-protocols` list
- OSPF: `ospf-status` helper + `vtysh show ip ospf json` for areas, interfaces, neighbors, routes
- RIP: `vtysh -c 'show ip rip status'` text parsing + `show ip route rip json`
- BFD: `vtysh -c 'show bfd peers json'`, filters out multihop sessions

### NTPCollector (`ntp.go`, ~434 lines)
- **Tree key**: `ietf-ntp:ntp`
- **Interval**: 60s (configurable: `YANGERD_POLL_INTERVAL_NTP`)
- Parses `chronyc -c` CSV output: sources, sourcestats, tracking, serverstats
- Detects NTP listening port via `ss -ulnp`
- Infix augmentations: clock-state frequency/offset details

### HardwareCollector (`hardware.go`, ~1122 lines)
- **Tree key**: `ietf-hardware:hardware`
- **Interval**: 10s (configurable: `YANGERD_POLL_INTERVAL_HARDWARE`)
- Motherboard from `/run/system.json`
- VPD components with vendor extensions
- USB port components with lock/unlock state
- hwmon sensors: temp, fan, PWM, voltage, current, power with parent/child relationships
- Thermal zones from `/sys/class/thermal/`
- WiFi radios via `iw.py` (gated by `YANGERD_ENABLE_WIFI`)
- GPS receivers via gpsd TCP (gated by `YANGERD_ENABLE_GPS`)

### ContainerCollector (`containers.go`, ~466 lines)
- **Tree key**: `infix-containers:containers`
- **Interval**: 10s (configurable: `YANGERD_POLL_INTERVAL_CONTAINERS`)
- Feature-gated by `YANGERD_ENABLE_CONTAINERS`
- Podman ps/inspect/stats integration
- Cgroup v2 resource limits (memory.max, cpu.max)
- Network info (host/bridge + port publishing)
- Resource usage stats (memory, CPU, block I/O, net I/O, PIDs)

## Bug fixes

- **NTP `ss` parsing**: `addServerStatus()` used `fields[4]` for local address
  in `ss -ulnp` output, but `strings.Fields` puts local address at index 3.
  Fixed to `fields[3]`.

## Test summary

| File | Tests | Status |
|------|-------|--------|
| `routing_test.go` | 16 | ✅ pass |
| `ntp_test.go` | 10 | ✅ pass |
| `hardware_test.go` | 10 | ✅ pass |
| `containers_test.go` | 12 | ✅ pass |
| **Phase 2 total** | **48** | **✅ all pass** |
| **Overall total** | **65** | **✅ all pass** |

## File inventory (Phase 2 additions)

### Production code (4 files)
| File | Lines | Description |
|------|-------|-------------|
| `internal/collector/routing.go` | 748 | OSPF+RIP+BFD routing collector |
| `internal/collector/ntp.go` | 434 | NTP collector (chronyc) |
| `internal/collector/hardware.go` | 1122 | Hardware/sensor collector |
| `internal/collector/containers.go` | 466 | Container collector (podman) |

### Test code (4 files)
| File | Tests | Description |
|------|-------|-------------|
| `internal/collector/routing_test.go` | 16 | Routing protocol tests |
| `internal/collector/ntp_test.go` | 10 | NTP chronyc parsing tests |
| `internal/collector/hardware_test.go` | 10 | Sensor/VPD/thermal tests |
| `internal/collector/containers_test.go` | 12 | Podman/cgroup parsing tests |

### Modified files
| File | Change |
|------|--------|
| `internal/config/config.go` | Added PollRouting/NTP/Hardware/Containers fields |
| `cmd/yangerd/main.go` | Registered all Phase 2 collectors |

---

# yangerd Phase 3: Reactive/Event-Driven Collectors

**Status**: Complete
**Started**: 2026-03-29
**Completed**: 2026-03-29

## Overview

Reactive collectors replace polling with persistent subscriptions (netlink,
ZAPI, D-Bus signals, subprocess watchers). They implement a
`Run(ctx context.Context) error` goroutine pattern instead of the polling
`Collector` interface.

## Packages implemented

### Foundation infrastructure

| Package | File | Lines | Description |
|---------|------|-------|-------------|
| `ipbatch` | `internal/ipbatch/ipbatch.go` | 212 | Persistent `ip -json` subprocess with mutex-serialized Query(), exponential backoff restart (100ms→30s), canary queries, 4MiB scanner |
| `bridgebatch` | `internal/bridgebatch/bridgebatch.go` | 201 | Persistent `bridge -json` subprocess, same pattern as ipbatch |
| `fswatcher` | `internal/fswatcher/fswatcher.go` | 176 | Inotify watcher with per-path debounce, glob expansion, initial read |

### Reactive monitors

| Package | File | Lines | Description |
|---------|------|-------|-------------|
| `monitor` | `internal/monitor/monitor.go` | 509 | NLMonitor — netlink link/addr/neigh/MDB subscription, stores raw ip-json at sub-paths |
| `ethmonitor` | `internal/ethmonitor/ethmonitor.go` | 223 | EthMonitor — ethtool genetlink for speed/duplex/auto-negotiation |
| `iwmonitor` | `internal/iwmonitor/iwmonitor.go` | 311 | IWMonitor — `iw event -t` parser + station dump/info queries |
| `lldpmonitor` | `internal/lldpmonitor/lldpmonitor.go` | 315 | LLDPMonitor — `lldpcli json0 watch` for neighbor discovery |
| `zapiwatcher` | `internal/zapiwatcher/zapiwatcher.go` | 305 | ZAPIWatcher — FRR ZAPI v6 route redistribution |
| `dbusmonitor` | `internal/dbusmonitor/dbusmonitor.go` | 1020 | DBusMonitor — dnsmasq lease events + firewalld Reloaded/NameOwnerChanged |

### Transformer

| Package | File | Lines | Description |
|---------|------|-------|-------------|
| `iface` | `internal/iface/iface.go` | 813 | Pure interface transformer: raw `ip -json` → YANG JSON (type mapping, oper-state, counters, IPv4/IPv6, VLAN/VETH/GRE/VXLAN/LAG/bridge augments) |

## External dependencies added

- `github.com/fsnotify/fsnotify` v1.9.0
- `github.com/vishvananda/netlink`
- `github.com/mdlayher/genetlink` + `github.com/mdlayher/ethtool`
- `github.com/osrg/gobgp/v4/pkg/zebra`
- `github.com/godbus/dbus/v5`

## Key design decisions

- **Event-as-trigger pattern**: Most reactive sources use events solely as triggers
  and then re-read canonical data (e.g., netlink notification triggers `ip -json link show`)
- **Persistent subprocess managers** (IPBatch/BridgeBatch): Mutex-serialized Query(),
  dead/alive atomic state, ErrBatchDead sentinel error, exponential backoff restart
- **Iface transformer is pure**: Takes raw `ip -json` arrays, returns YANG JSON.
  Uses `FileChecker` interface for IPv6 MTU and WiFi detection
- **Interface filtering**: Skip `group=="internal"` or `link_type` in `("can","vcan")`
- **RFC 7951 compliance**: Counter values (uint64) encoded as JSON strings

## Test summary

| File | Tests | Status |
|------|-------|--------|
| `internal/iface/iface_test.go` | 20+ | ✅ pass |
| `internal/iwmonitor/iwmonitor_test.go` | 8+ | ✅ pass |
| `internal/lldpmonitor/lldpmonitor_test.go` | 8+ | ✅ pass |
| `internal/monitor/monitor_test.go` | 8+ | ✅ pass |
| `internal/dbusmonitor/dbusmonitor_test.go` | 25+ | ✅ pass |
| `internal/zapiwatcher/zapiwatcher_test.go` | 8 | ✅ pass |
| `internal/fswatcher/fswatcher_test.go` | 11 | ✅ pass |
| **Phase 3 total** | **93** | **✅ all pass** |
| **Overall total** | **158** | **✅ all pass with -race** |

## Modified files

| File | Change |
|------|--------|
| `internal/config/config.go` | Added reactive config fields (ZAPISocket, DBus paths, WiFi/LLDP enables) |
| `cmd/yangerd/main.go` | All reactive subsystems wired (229 lines total) |

## Packages without tests (intentional)

- `ipbatch`, `bridgebatch` — require real subprocesses (`ip`, `bridge`)
- `ethmonitor` — requires genetlink socket (kernel interface)

---

# yangerd Phase 4: Architecture Fix — Transform-on-Write

**Status**: Complete
**Started**: 2026-03-29
**Completed**: 2026-03-29

## Problem

NLMonitor stored raw ip-json fragments at per-interface sub-paths
(e.g. `/ietf-interfaces:interfaces/interface[name='eth0']`,
`/addresses`, `/statistics`). These are not valid YANG keys and the
output did not match what Python yanger produces — a single complete
`{"ietf-interfaces:interfaces":{"interface":[...]}}` document.

The `iface.Transform()` function existed but was never called.
EthMonitor and IWMonitor also stored at fragment paths.

## Fix: Transform-on-write

NLMonitor is now the central coordinator. It owns staging data (raw
link/addr/stats arrays) and after every netlink event:

1. Runs `iface.Transform(links, addrs, stats, fc)` to produce the base
   YANG document
2. Merges augment data (ethernet, wifi, bridge FDB/MDB) from staging
   maps into the matching interface entries
3. Stores the complete result at a single tree key `ietf-interfaces:interfaces`

EthMonitor and IWMonitor no longer write to the tree directly. They call
`NLMonitor.SetEthernetData(ifname, data)` and
`NLMonitor.SetWifiData(ifname, data)` which update staging maps and
trigger a rebuild.

## Changes

| File | Change |
|------|--------|
| `internal/monitor/monitor.go` | Added staging fields (`links`, `addrs`, `stats`, `fdb`, `mdb`, `ethernet`, `wifi`), `rebuild()` method calling `iface.Transform()` + `mergeAugments()`, `replaceByIfName()` helper, `SetEthernetData()`/`SetWifiData()` public methods. Removed per-interface fragment tree paths. Added `iface.FileChecker` parameter to `New()`. |
| `internal/monitor/monitor_test.go` | Removed `TestPathHelpers` (old fragment paths). Added tests: `TestReplaceByIfName`, `TestReplaceByIfNamePreservesUpdatedData`, `TestMergeAugments`, `TestMergeAugmentsNoOp`, `TestMergeAugmentsInvalidDoc`, `TestTreeKey`. |
| `internal/ethmonitor/ethmonitor.go` | Removed `tree` field and tree import. Added `onUpdate` callback. `New()` no longer takes `*tree.Tree`. `refreshEthernetSettings()` calls `onUpdate(ifname, data)` instead of `tree.Set()`. |
| `internal/iwmonitor/iwmonitor.go` | Removed `tree` field and tree import. Added `onUpdate` callback + `publishWifi()` method. `New()` no longer takes `*tree.Tree`. Assembles combined wifi JSON per interface. |
| `cmd/yangerd/main.go` | Added `osFileChecker` type implementing `iface.FileChecker`. Updated `monitor.New()` call with FileChecker. Wired `ethMon.SetOnUpdate(nlmon.SetEthernetData)` and `iwmon.SetOnUpdate(nlmon.SetWifiData)`. Updated `ethmonitor.New()` and `iwmonitor.New()` signatures. |

## Test summary

| Metric | Value |
|--------|-------|
| New tests added | 5 |
| Tests removed | 1 (TestPathHelpers — obsolete fragment paths) |
| **Total tests** | **163** |
| **Status** | **✅ all pass with -race** |

---

# yangerd Phase 5: Buildroot Compatibility Fixes

**Status**: Complete
**Completed**: 2026-03-29

## Problem

`make yangerd-rebuild` failed: Buildroot ships Go 1.23.12 but go.mod
had `go 1.24.5` (from gobgp/v4's requirement). Two other dependencies
also required go 1.24+.

## Fixes

| Change | Before | After | Reason |
|--------|--------|-------|--------|
| `go.mod` go directive | `go 1.24.5` | `go 1.23.0` | Buildroot has Go 1.23.12 |
| `osrg/gobgp` | v4 (go 1.24.5) | v3 v3.37.0 (go 1.23.0) | All v4 releases need 1.24+ |
| `mdlayher/ethtool` | v0.5.1 (go 1.24.0) | v0.4.1 (go 1.23.0) | API-compatible downgrade |
| `golang.org/x/sys` | v0.40.0 (go 1.24.0) | v0.35.0 (go 1.23.0) | Sufficient for all deps |

### gobgp v3 vs v4 API changes

- `Nexthop.Gate`: `netip.Addr` (v4) → `net.IP` (v3)
- `Prefix.Prefix`: `netip.Addr` (v4) → `net.IP` (v3)
- `NewClient()`: `*slog.Logger` (v4) → `log.Logger` interface (v3)
- Created `slogAdapter` in zapiwatcher.go to bridge `*slog.Logger` → gobgp v3 `log.Logger`

### ip batch `-s` flag fix

`ip -json -force -batch -` does not accept `-s` as a batch line subcommand.
Added `-s -d` as global flags to the persistent batch process args instead.
Removed separate `-s link show` queries and the `stats` staging field.

## Verification

- All 163 tests pass with `-mod=vendor` and `-race`
- `go build ./cmd/yangerd` and `go build ./cmd/yangerctl` succeed
- `go vet ./...` clean

---

# yangerd Phase 6: statd C Integration

**Status**: Complete (code-complete, pending real device testing)
**Completed**: 2026-03-29

## Overview

statd (C daemon) now queries yangerd over Unix socket IPC instead of
fork/exec'ing Python yanger scripts, with automatic fallback to yanger
when yangerd is unavailable.

## Architecture

```
statd (C)  ──yangerd_query()──► /run/yangerd.sock ──► yangerd (Go)
   │                                                      │
   │ fallback if yangerd unavailable                      │ tree.Get(key)
   ▼                                                      ▼
ly_add_yanger_data()                              in-memory Tree store
  (fork/exec Python)                              (per-module JSON blobs)
```

## Wire protocol (C↔Go)

```
Frame: [ver:1byte=0x01] [length:4bytes big-endian] [JSON body]
Request:  {"method":"get","path":"ietf-interfaces:interfaces"}
Response: {"status":"ok","data":{"ietf-interfaces:interfaces":{...}}}
```

## Files created

| File | Lines | Description |
|------|-------|-------------|
| `src/statd/yangerd.h` | 28 | Header: socket path, timeout, max payload, proto version, `yangerd_query()` |
| `src/statd/yangerd.c` | 245 | Full C IPC client: connect, framed I/O, jansson JSON parsing |

## Files modified

| File | Change |
|------|--------|
| `src/statd/statd.c` | Added `#include "yangerd.h"`, `ly_add_yangerd_data()` wrapper, `xpath_to_yangerd_path()` helper; updated all 5 sysrepo callbacks |
| `src/statd/Makefile.am` | Added `yangerd.c yangerd.h` to `statd_SOURCES` |

## Key functions

- **`yangerd_query(path, &buf, &len)`** — Connect to socket, send framed "get" request, receive framed response, extract "data" JSON field
- **`ly_add_yangerd_data(ctx, parent, path, yanger_args)`** — Try yangerd first; on failure, fall back to `ly_add_yanger_data()` (fork/exec Python)
- **`xpath_to_yangerd_path(xpath, buf, bufsz)`** — Strip leading `/`, take first path segment (maps sysrepo xpath to yangerd tree key)

## Subscription → yangerd key mapping

| statd subscription xpath | yangerd tree key |
|---|---|
| `/ietf-interfaces:interfaces` | `ietf-interfaces:interfaces` |
| `/ietf-routing:routing/ribs` | `ietf-routing:routing` |
| `/ietf-hardware:hardware` | `ietf-hardware:hardware` |
| `/ietf-system:system` | `ietf-system:system` |
| `/ietf-system:system-state` | `ietf-system:system-state` |
| `/ieee802-dot1ab-lldp:lldp` | `ieee802-dot1ab-lldp:lldp` |
| `/infix-containers:containers` | `infix-containers:containers` |
| `/infix-dhcp-server:dhcp-server` | `infix-dhcp-server:dhcp-server` |
| `/infix-firewall:firewall` | `infix-firewall:firewall` |
| `/ietf-ntp:ntp` | `ietf-ntp:ntp` |
| OSPF/RIP/BFD callbacks | `ietf-routing:routing` (hardcoded) |

## Remaining

- ~~Real device testing~~ Done — bugs found and fixed (see Phase 7)
- ~~Verify fallback~~ Removed — no fallback by design
- Performance comparison (optional): yangerd IPC vs fork/exec Python yanger

---

# yangerd Phase 7: Real Device Bug Fixes

**Status**: Complete (Bugs 1-7)
**Completed**: 2026-03-30

## Overview

Two rounds of `yangerctl dump` on a real Infix x86_64 device exposed
seven bugs across four categories. All seven are now fixed.

## Bug 1: Double-wrapping (FIXED)

### Problem

The IPC server's `handleGet()` wraps stored data in `{key: data}`, but
some collectors ALSO stored their data pre-wrapped. This caused
double-nesting, e.g. `{"ietf-interfaces:interfaces":{"ietf-interfaces:interfaces":{...}}}`.

### Contract established

Collectors store data WITHOUT the module key wrapper. The server adds it.

### Fixes

| File | Change |
|------|--------|
| `internal/iface/iface.go` | `Transform()` returns `{"interface":[...]}` (removed outer wrapper) |
| `internal/iface/iface_test.go` | Updated `mustInterfaces()` to parse unwrapped format |
| `internal/monitor/monitor.go` | `mergeAugments()` updated to parse unwrapped format |
| `internal/monitor/monitor_test.go` | `TestMergeAugments`/`TestMergeAugmentsNoOp` updated |
| `internal/dbusmonitor/dbusmonitor.go` | `buildDHCPTree()`/`buildFirewallTree()` removed wrappers |
| `internal/dbusmonitor/dbusmonitor_test.go` | Tests updated for unwrapped format |

## Bug 2: Fragmented routing tree keys (FIXED)

### Problem

`ietf-routing:routing` is a shared tree written by three different
sources — RoutingCollector (control-plane-protocols), ZAPIWatcher (ribs),
and FSWatcher (forwarding interfaces). Each wrote to its own sub-path
key or used `tree.Set()` which overwrote the others' data.

### Solution: `Tree.Merge()`

Added a shallow first-level JSON merge method to `Tree` that allows each
writer to merge its fields into the shared key without overwriting
others' data. All three writers now use `tree.Merge("ietf-routing:routing", ...)`.

### Fixes

| File | Change |
|------|--------|
| `internal/tree/tree.go` | Added `Merge()` and `Delete()` methods |
| `internal/tree/tree_test.go` | 7 new tests (Merge subtests, empty, non-object, delete, concurrent) |
| `internal/zapiwatcher/zapiwatcher.go` | Refactored: internal `routes` map, builds complete ribs, writes via `Merge()` |
| `internal/zapiwatcher/zapiwatcher_test.go` | Removed obsolete `routePath`/`routeKey` tests |
| `internal/collector/routing.go` | Changed `t.Set()` → `t.Merge()` on line 59 |
| `internal/fswatcher/fswatcher.go` | Added `UseMerge` field to `WatchHandler`; `InitialRead()`/`fireHandler()` respect it |
| `cmd/yangerd/main.go` | Replaced per-file `forwardingTreeKey()` with `forwardingAggregator` that scans all forwarding files and writes a complete `interfaces` list via `Merge()` |

### Forwarding aggregator

Replaces the old per-file approach (each `/proc/sys/net/ipv{4,6}/conf/*/forwarding`
file got its own sub-path tree key) with an aggregator that:
1. On any forwarding file change, rescans ALL forwarding files
2. Builds the complete `{"interfaces":{"interface":["e1","e2",...]}}` list
3. Writes via `tree.Merge("ietf-routing:routing", ...)` — coexists with ribs and control-plane-protocols
4. Matches Python yanger `get_routing_interfaces()` output format
5. Uses `forwarding` for IPv4, `force_forwarding` for IPv6 (matching Python behavior)

## Bug 3: Duplicate interface entries (FIXED)

### Problem

When Infix renames `eth0` to `e1`, `ip -json link show` reports both the
old and new names with the same ifindex. Both appeared in the YANG output.

### Solution: `dedup()` in iface transformer

Added `dedup()` function that runs before `skipInterface()`. When
multiple entries share the same ifindex, keeps the one with
`operstate=="UP"` (or the first seen if neither is UP).

### Fixes

| File | Change |
|------|--------|
| `internal/iface/iface.go` | Added `dedup()` function; `Transform()` calls `dedup(decodeObjects(linkData))` |
| `internal/iface/iface_test.go` | 4 new test cases: UP-over-DOWN, both-DOWN-keeps-first, different-ifindex, zero-ifindex |

## Verification

- All tests pass (including new tests for Merge, dedup, and forwarding aggregator)
- `go build ./cmd/yangerd` and `go build ./cmd/yangerctl` succeed
- `go vet ./...` clean

## Bug 4: Interface removal not handled (FIXED)

### Problem

When an interface is removed, its `/proc/sys/net/*/conf/IFNAME/forwarding`
file disappears. The inotify Remove event was only calling `rewatch()`,
not updating the tree — the removed interface stayed in the YANG data.

### Solution: `handleRemove()` in FSWatcher

Replaced the old Remove→handleEvent→rewatch sequence with a dedicated
`handleRemove()` method that handles the two handler types differently:

- **UseMerge handlers** (forwarding aggregator): fires the handler, which
  rescans via glob and naturally excludes the removed file
- **Plain handlers**: calls `tree.Delete()` to clear stale data

After handling, attempts to re-add the inotify watch. If the file is
permanently gone, cleans up the handler and debounce timer entries.

### Fixes

| File | Change |
|------|--------|
| `internal/fswatcher/fswatcher.go` | Added `handleRemove()`, removed `rewatch()`, `Run()` dispatches Remove to `handleRemove()` |
| `internal/fswatcher/fswatcher_test.go` | 4 new tests: merge-handler removal, plain-handler removal, unknown path, rewatch-succeeds |

## Bug 5: Phantom GPS devices (FIXED)

### Problem

`gps0`–`gps3` appeared in hardware output even when `/dev/gps*` didn't
exist. `readlink -f` on non-existent paths succeeds (returns the
canonical form), so the loop at `hardware.go:941` never skipped them.

### Solution

Added an `ls /dev/gpsN` existence check before the `readlink -f` call,
matching the Python reference (`ietf_hardware.py:727`: `HOST.exists()`).

### Fixes

| File | Change |
|------|--------|
| `internal/collector/hardware.go` | Added `ls` existence check before `readlink -f` in GPS loop |
| `internal/collector/hardware_test.go` | `TestHardwareGPSDeviceNotFound` — verifies no phantom GPS when devices missing |

## Bug 6: `null` JSON arrays instead of `[]` (FIXED)

### Problem

Go `nil` slices marshal to `null` in JSON. YANG lists must be arrays,
so libyang rejects `null`. Three locations used `var slice []Type`
(nil) instead of `make([]Type, 0)` (empty array).

### Fixes

| File | Line | Change |
|------|------|--------|
| `cmd/yangerd/main.go` | 282 | `var ifnames []string` → `ifnames := make([]string, 0)` |
| `internal/collector/system.go` | 172 | `var users []interface{}` → `users := make([]interface{}, 0)` |
| `internal/collector/system.go` | 422 | `var servers []interface{}` → `servers := make([]interface{}, 0)` |
| `internal/collector/system_test.go` | — | `TestSystemCollectorNoUsersEmptyArray`, `TestSystemCollectorNoDNSEmptyArray` |

## Bug 7: Firewall data without firewalld running (FIXED)

### Problem

`refreshFirewall()` was called on initial D-Bus connect. When firewalld
wasn't on the bus, all D-Bus calls failed but execution continued.
`getFirewallPolicies()` unconditionally appended a hardcoded
"default-drop" policy, producing phantom firewall data.

### Solution

Made `getDefaultZone` the gate: if it fails, `refreshFirewall()` returns
early without writing to the tree. This is the minimal fix — the
`NameOwnerChanged` handler already clears the tree when firewalld exits.

### Fixes

| File | Change |
|------|--------|
| `internal/dbusmonitor/dbusmonitor.go` | `refreshFirewall()` returns early if `getDefaultZone` fails |

## Verification

- All tests pass (173+ tests)
- `go build ./cmd/yangerd` and `go build ./cmd/yangerctl` succeed
- `go vet ./...` clean
- Ready for re-test on real device
