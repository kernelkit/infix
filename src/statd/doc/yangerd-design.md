# yangerd — Design Document

**Status:** DRAFT  
**Date:** 2026-02-24  
**Author:** (Engineering Team)

## Table of Contents

- [Revision History](#revision-history)
- [1. Introduction](#1-introduction)
  - [1.1 Purpose](#11-purpose)
  - [1.2 Problem Statement](#12-problem-statement)
  - [1.3 Solution Summary](#13-solution-summary)
  - [1.4 Relationship to Infix Components](#14-relationship-to-infix-components)
- [2. Requirements & Constraints](#2-requirements--constraints)
  - [2.1 Functional Requirements](#21-functional-requirements)
  - [2.2 Non-Functional Requirements](#22-non-functional-requirements)
  - [2.3 Explicit Scope Boundaries](#23-explicit-scope-boundaries-what-this-is-not)
    - [Not a sysrepo plugin](#not-a-sysrepo-plugin)
    - [Not a NETCONF or RESTCONF server](#not-a-netconf-or-restconf-server)
    - [Not CGo](#not-cgo)
    - [Not a replacement for confd](#not-a-replacement-for-confd)
    - [Not a YANG validator](#not-a-yang-validator)
    - [Not a push or streaming daemon](#not-a-push-or-streaming-daemon)
    - [Not responsible for container namespace data (Phase 1)](#not-responsible-for-container-namespace-data-phase-1)
  - [2.4 Hard Constraints](#24-hard-constraints)
  - [2.5 YANG-Model JSON Output Compatibility](#25-yang-model-json-output-compatibility)
    - [2.5.1 Top-level JSON Structure Per Module](#251-top-level-json-structure-per-module)
    - [2.5.2 Concrete JSON Output Examples](#252-concrete-json-output-examples)
    - [2.5.3 Structural Rules](#253-structural-rules)
    - [2.5.4 Field Transformation Reference](#254-field-transformation-reference)
    - [2.5.5 Validation Strategy](#255-validation-strategy)
- [3. Architecture Overview](#3-architecture-overview)
  - [3.1 Component Diagram](#31-component-diagram)
  - [3.2 Data Flow Diagrams](#32-data-flow-diagrams)
  - [3.3 Component Responsibilities](#33-component-responsibilities)
- [4. Detailed Design](#4-detailed-design)
  - [4.1 Netlink Monitor Subsystem](#41-netlink-monitor-subsystem)
  - [4.1bis ip batch Subprocess Manager](#41bis-ip-batch-subprocess-manager)
  - [4.1ter File Watcher Subsystem](#41ter-file-watcher-subsystem)
  - [4.1quater Bridge Monitor Subsystem](#41quater-bridge-monitor-subsystem)
  - [4.1quinquies IW Event Monitor Subsystem](#41quinquies-iw-event-monitor-subsystem)
  - [4.1sexies Ethtool Netlink Monitor Subsystem](#41sexies-ethtool-netlink-monitor-subsystem)
  - [4.1octies ZAPI Watcher Subsystem (Zebra Route Redistribution)](#41octies-zapi-watcher-subsystem-zebra-route-redistribution)
  - [4.1novies D-Bus Monitor Subsystem](#41novies-d-bus-monitor-subsystem)
  - [4.1decies LLDP Monitor Subsystem](#41decies-lldp-monitor-subsystem)
  - [4.1undecies mDNS Monitor Subsystem](#41undecies-mdns-monitor-subsystem)
  - [4.1septies Event-Triggered Batch Re-read Pattern (All Netlink Events)](#41septies-event-triggered-batch-re-read-pattern-all-netlink-events)
  - [4.2 In-Memory Data Tree](#42-in-memory-data-tree)
  - [4.3 IPC Protocol Specification](#43-ipc-protocol-specification)
  - [4.4 Supplementary Collectors](#44-supplementary-collectors)
  - [4.5 statd Integration](#45-statd-integration)
  - [4.6 yangerctl CLI](#46-yangerctl-cli)
  - [4.7 Design Decisions](#47-design-decisions)
  - [4.8 Monitoring & Observability](#48-monitoring--observability)
    - [Health Endpoint](#health-endpoint)
    - [Metrics Tracked](#metrics-tracked)
    - [Log Levels](#log-levels)
  - [4.9 Security Considerations](#49-security-considerations)
    - [Socket Permissions](#socket-permissions)
    - [Linux Capabilities](#linux-capabilities)
    - [Trust Boundary](#trust-boundary)
- [5. Data Source Matrix](#5-data-source-matrix)
  - [5.1 ietf-interfaces](#51-ietf-interfaces)
  - [5.2 ietf-routing](#52-ietf-routing)
  - [5.3 ietf-hardware](#53-ietf-hardware)
  - [5.4 ietf-system](#54-ietf-system)
  - [5.5 ietf-ntp](#55-ietf-ntp)
  - [5.6 ieee802-dot1ab-lldp](#56-ieee802-dot1ab-lldp)
  - [5.7 infix-containers](#57-infix-containers)
  - [5.8 infix-dhcp-server](#58-infix-dhcp-server)
  - [5.9 infix-firewall](#59-infix-firewall)
  - [5.9bis infix-services](#59bis-infix-services)
  - [5.10 Summary Table](#510-summary-table)
  - [5.11 Module-by-Module Mapping](#511-module-by-module-mapping)
- [6. Project Structure](#6-project-structure)
  - [6.1 Go Project Layout](#61-go-project-layout)
  - [6.2 Package Descriptions](#62-package-descriptions)
  - [6.3 Key Dependencies](#63-key-dependencies)
  - [6.4 Buildroot Integration](#64-buildroot-integration)
- [7. Deployment & Operations](#7-deployment--operations)
  - [7.1 Finit Service File](#71-finit-service-file)
  - [7.2 Socket Permissions](#72-socket-permissions)
  - [7.3 Environment Variables](#73-environment-variables)
  - [7.4 Startup Sequence](#74-startup-sequence)
  - [7.5 Local Development](#75-local-development)
  - [7.6 Buildroot Package](#76-buildroot-package)
  - [7.7 Cross-Compilation](#77-cross-compilation)
- [8. Testing Strategy](#8-testing-strategy)
  - [8.1 Unit Tests](#81-unit-tests)
  - [8.2 Integration Tests](#82-integration-tests)
  - [8.3 Regression Tests](#83-regression-tests)
  - [8.4 Race Detector Policy](#84-race-detector-policy)
  - [8.5 Testability Contracts (Interface Boundaries)](#85-testability-contracts-interface-boundaries)
  - [8.6 Verification Loop (Definition of Done)](#86-verification-loop-definition-of-done)
- [9. Migration Plan](#9-migration-plan)
  - [9.1 Module Inventory](#91-module-inventory)
- [10. Risk Assessment](#10-risk-assessment)
  - [10.1 Detailed Risks](#101-detailed-risks)
  - [10.2 Risk Summary](#102-risk-summary)
- [Appendices](#appendices)
  - [A.1 Netlink Group Reference](#a1-netlink-group-reference)
  - [A.2 YANG Module Registry](#a2-yang-module-registry)
  - [A.3 Glossary](#a3-glossary)
- [Troubleshooting Guide](#troubleshooting-guide)
  - [IPC Connection Issues](#ipc-connection-issues)
  - [Stale Data in the Tree](#stale-data-in-the-tree)
  - [Performance Bottlenecks](#performance-bottlenecks)
- [Detailed IPC Examples](#detailed-ipc-examples)
  - [Example 1: Full Interface List Query](#example-1-full-interface-list-query)
  - [Example 2: Routing Table Query](#example-2-routing-table-query)

## Revision History

| Date | Revision | Description | Author |
|------|----------|-------------|--------|
| 2026-02-24 | 0.1 | Initial draft from implementation proposal. | Assistant |
| 2026-02-24 | 0.2 | Added reactive file watcher and bridge monitor subsystems; converted ~8 data sources to REACTIVE. | Assistant |
| 2026-02-24 | 0.3 | Added iw event monitor subsystem for reactive 802.11 wireless monitoring; converted WiFi from POLLING to REACTIVE. | Assistant |
| 2026-02-24 | 0.4 | Moved `last-change` from NOT COLLECTED to REACTIVE; added oper-status tracking in link event handler with `time.Now()` timestamp. | Assistant |
| 2026-02-24 | 0.5 | Added ethtool netlink monitor subsystem for reactive speed/duplex/auto-negotiation via `ETHNL_MCGRP_MONITOR` genetlink multicast; converted 3 ethtool leaves from POLLING 30s to REACTIVE; ethtool collector becomes hybrid (reactive settings + polling statistics). | Assistant |
| 2026-02-24 | 0.6 | RTM_NEWLINK full interface re-read: on any link event, the event dispatcher now writes a full set of queries (`link show dev`, `-s link show dev`, `addr show dev`) to ip batch for atomic interface state; also triggers `ethmonitor.RefreshInterface()` for ethtool re-query since `ETHNL_MCGRP_MONITOR` does NOT fire on link up/down. Cross-subsystem coordination between link monitor and ethmonitor. | Assistant |
| 2026-02-24 | 0.7 | Kernel 6.18 cleanup: stripped all fallback/degradation hedging for ethtool netlink. Infix targets Linux 6.18 exclusively; ethtool netlink is unconditionally available. Removed all `kernel < 5.6`, `graceful degradation`, and `polling fallback` references across 23 locations. | Assistant |
| 2026-02-24 | 0.8 | Event-triggered batch re-read for ALL netlink event types: address, route, and neighbor events (both RTM_NEW* and RTM_DEL*) now use the same event-as-trigger pattern as link events. Each event triggers a full re-read of the affected state via ip batch; event content is not parsed for data. Delete events produce a re-read that omits the removed entity. Updated data source matrix, design decisions, appendix A.1, module-by-module mapping, and project structure throughout. | Assistant |
| 2026-02-24 | 0.9 | Added section 2.5: YANG-Model JSON Output Compatibility. Formal requirement that yangerd must produce JSON output structurally identical to the current Python yanger scripts (RFC 7951 module-qualified keys, list-as-array, augmentation prefixes, counter-as-string, presence-as-null). Includes top-level JSON structure table for all 14 models, concrete JSON examples for every module, 14 structural rules, field transformation reference, and validation strategy. | Assistant |
| 2026-02-24 | 0.10 | Corrected routing data source attribution: route table re-reads now use `vtysh` (FRRouting) instead of `ip batch`, because vtysh is the authoritative source for the complete routing table including all protocol routes (kernel, connected, static, OSPF, RIP) with enriched metadata (source protocol, distance, metric, active/installed flags). Updated data source matrix (section 5.2), route event handler code, initial state dump, batch query examples, event-trigger tables, module-by-module mapping, appendix A.1, glossary, and all related prose throughout the document. Added dedicated Route Table Collector section (5b) for the reactive vtysh-based RIB collection. | Assistant |
| 2026-02-24 | 0.11 | Build-time feature flags and binary-present assumption. WiFi (`YANGERD_ENABLE_WIFI`), containers (`YANGERD_ENABLE_CONTAINERS`), and GPS (`YANGERD_ENABLE_GPS`) are now opt-in build features controlled by runtime environment variables in `/etc/default/yangerd`, written by the Buildroot recipe based on `BR2_PACKAGE_*` selections. When a feature is disabled, its collectors and monitors are not started — no runtime binary detection is performed. All tool binaries (`iw`, `iproute2`, `bridge`, `vtysh`, `nft`, `chronyc`, `dmidecode`, etc.) are guaranteed present on target when their feature is enabled; removed all "if binary absent" hedging. Updated env vars table, startup sequence, Buildroot recipe, `internal/config/` description, collector failure behaviors, data source matrix, module-by-module mapping, migration table, project tree, appendix model table, risk assessment, and design rationale throughout. | Assistant |
| 2026-02-25 | 0.12 | Replaced `ip monitor -json` and `bridge monitor -json` subprocess-based event monitoring with native Go netlink subscriptions via `vishvananda/netlink`. iproute2 investigation confirmed that `ip monitor -json` and `bridge monitor -json` never produce JSON output (the `-json` flag is parsed globally but the JSON writer `_jw` is never allocated in `do_ipmonitor()` or `bridge/monitor.c`). Events are now received as typed Go structs (`LinkUpdate`, `AddrUpdate`, `RouteUpdate`, `NeighUpdate`) on dedicated channels. Bridge FDB events arrive via `NeighSubscribeWithOptions`; bridge VLAN via `LinkSubscribeWithOptions`; bridge MDB via raw netlink `RTNLGRP_MDB` subscription. Event-as-trigger pattern preserved: all events trigger full re-reads via `ip batch`, `bridge batch`, or `vtysh`. Subprocess count drops from FIVE to THREE (`ip batch`, `bridge batch`, `iw event`). Updated architecture diagrams, component table, `EventMonitor` code (now `NLMonitor`), bridge monitor code, design rationale, data source matrix, module-by-module mapping, project structure, risk assessment, and glossary throughout. | Assistant |
| 2026-02-25 | 0.13 | Replaced vtysh-based route table collection with a streaming ZAPI watcher (`internal/zapiwatcher/`) that connects directly to FRR zebra's zserv unix socket (`/var/run/frr/zserv.api`), subscribes to route redistribution notifications via ZAPI v6, and receives both the initial RIB dump and incremental route add/delete updates. This captures routes in zebra's RIB that are not present in the Linux kernel FIB (unresolvable nexthop, lost admin-distance election, ECMP overflow, table-map filtered). Automatic reconnection with exponential backoff handles zebra restarts; stale routes are cleared atomically on reconnect via full replacement. vtysh is retained for OSPF/RIP/BFD protocol-specific collectors only. Uses `github.com/osrg/gobgp/v4/pkg/zebra` for ZAPI message framing. Updated architecture diagrams (section 3), NLMonitor (section 4.1), event-triggered batch re-read (section 4.1septies), added new section 4.1octies (ZAPI Watcher Subsystem), design decisions (section 4.7), data source matrix (section 5), module-by-module mapping (section 5.11), project structure (section 6), deployment startup sequence (section 7), migration plan (section 9), risk assessment (section 10, including new Risk 11), and appendix A.1 throughout. | Assistant |
| 2026-02-25 | 0.14 | Removed hwmon/thermal sensor files from inotify-based fswatcher -- sysfs pseudo-files do not emit inotify events (kernel generates values on `read()`, never calls `fsnotify_modify()`). Hardware sensors are now collected exclusively by `collector/hardware.go` via polling at 10-second intervals. Updated fswatcher watched paths table (removed 4 hwmon/thermal entries), glob expansion paragraph, inotify limitations section, hardware collector interval (30s->10s), data source matrix (REACTIVE->POLLING for sensors), summary table counts, module-by-module mapping strategy, fswatcher package description, and fsnotify dependency description throughout. | Assistant |
| 2026-02-25 | 0.15 | Review-driven fixes. (1) Socket ownership corrected from `root:yangerd` to `root:statd`. (2) OSPF/RIP/BFD collector intervals normalized to 10s. (3) Hardware collector interval corrected from 30s to 10s (missed in 0.14). (4) Grammar fix: "every 1 seconds" to "every second". (5) IPC protocol version field added: 1-byte version header before 4-byte length in framing (`[ver:1][length:4][JSON body]`); updated framing diagram, architecture intro, yangerd.c defines/read/write code, test descriptions, and glossary. (6) yangerd.c partial read bug fixed: replaced single `read()` with accumulating loop for short reads on Unix sockets. (7) Per-model locking redesign: replaced single `sync.RWMutex` with per-model `modelEntry` structs each containing their own `sync.RWMutex`; writers for different YANG modules never block each other; added `GetMulti()` for multi-module IPC concatenation; updated solution summary, architecture diagram, ethmonitor/ZAPI watcher concurrency sections, core Tree type, design rationale, design decision, project tree, package descriptions, startup sequence, test descriptions, and race policy throughout. | Assistant |
| 2026-02-26 | 0.16 | Removed all remaining netlink route subscription references (routes are sourced exclusively from the ZAPI watcher). Removed `RTNLGRP_IPV4_ROUTE` and `RTNLGRP_IPV6_ROUTE` rows from appendix A.1 table. Updated appendix A.1 intro text, RTNLGRP glossary entry (six groups → four groups), NLMonitor architecture, event-triggered re-read section, design decisions, project tree, package descriptions, and dependency table throughout. Added VRF out-of-scope declaration to Section 2.3. | Assistant |
| 2026-03-02 | 0.17 | Removed all yanger.py coexistence, fallback, and phased-migration references. yangerd ships all 13 modules as a single delivery and completely replaces the Python yanger scripts -- no fallback path, no rollback to Python, no phased rollout. Rewrote Section 9 Migration Plan (single delivery), Risk 4 (503 = retry, not Python fallback), risk summary table, Appendix A.2 (removed Phase column), glossary `sr_oper_get_subscribe` entry, validation strategy (golden-file based, not Python comparison), regression tests (YANG-schema validation, not live Python comparison), and removed fallback integration test. Removed Coexistence Strategy and Rollback subsections. | Assistant |
| 2026-03-02 | 0.18 | Bridge data collection is now fully reactive via netlink events as triggers + `bridge -json -batch -` for state re-reads. Removed all bridge polling references. STP port state is now sourced from netlink `RTM_NEWLINK` events carrying `IFLA_BRPORT_STATE` in `IFLA_PROTINFO` (not inotify on `/sys/class/net/<br>/brport/state`). Updated Section 4.1.2 (bridge event channels with STP, event-as-trigger pattern), Section 4.4.3 intro (bridge excluded from polling collectors), design rationale (inotify: removed STP; bridge batch: confirmed STP), summary table (fswatcher: removed brport state; REACTIVE row: added bridge event triggers), migration section `bridge.py` (removed fswatcher/inotify for STP), summary table bridge row (removed fswatcher reference), fswatcher package description (removed brport/state), nlmonitor package description (added STP events), migration module table bridge row (removed `/sys/class/net` reference), Risk 7 (removed bridge ports from inotify exhaustion), Risk 8 (added STP to reactive data list), glossary inotify (removed STP), glossary bridge netlink events (added STP), and appendix A.1 RTNLGRP_MDB notes throughout. | Assistant |
| 2026-03-04 | 0.19 | D-Bus reactive monitoring: dnsmasq DHCP and firewall data collection moved from polling to reactive via D-Bus signal subscriptions. Added D-Bus Monitor Subsystem (Section 4.1novies) using `godbus/dbus/v5` `AddMatchSignal()` for dnsmasq (`DHCPLeaseAdded/Deleted/Updated`) and firewalld (`Reloaded`, `NameOwnerChanged`) signals. dnsmasq lease file watching moved from fswatcher inotify to D-Bus signal triggers (re-read lease file + `GetMetrics()` on each signal). Firewall data moved from 30-second polling of `nft list ruleset -j` to firewalld D-Bus signal triggers. Updated architecture diagram, data flow diagrams (added 3.2.8 D-Bus Monitor Reactive Path), component responsibilities, File Watcher Subsystem (removed DHCP leases from watched paths), collector specifications (#10 DHCP, #11 Firewall), design rationale (added D-Bus Monitor rationale; updated inotify rationale), data source matrix (DHCP/firewall rows now REACTIVE), summary table (moved leaf counts from POLLING to REACTIVE), module-by-module migration (DHCP/firewall strategies), project structure (added `dbusmonitor/`), package descriptions (added dbusmonitor, updated fswatcher/collector), dependency table (updated godbus/fsnotify), startup sequence, risk assessment (updated Risk 3, added Risk 12 for D-Bus service unavailability), and glossary (added D-Bus monitor entry, updated inotify/reactive entries) throughout. | Assistant |
| 2026-03-04 | 0.20 | NTP data collection optimized: replaced `exec chronyc` subprocess spawning with native Go cmdmon protocol via `github.com/facebook/time/ntp/chrony` (Apache-2.0). Investigation confirmed chrony has no D-Bus interface, no event-driven socket protocol, and no subscribe mechanism -- the cmdmon UDS protocol (`/var/run/chrony/chronyd.sock`) is strictly request-response. Polling remains the only supported monitoring approach. Updated NTP collector (#8), data source matrix (NTP rows), module-by-module mapping (ietf_ntp.py), migration table, project tree, dependency table, summary table description, appendix model table, field transformation reference, and polling glossary entry throughout. | Assistant |
| 2026-03-04 | 0.21 | Architectural review fixes. (1) IPBatch/BridgeBatch error handling: documented pipe EOF detection, immediate error return, restart coordination with ErrBatchDead sentinel and canary-query validation. (2) errgroup lifecycle: clarified that all Run() methods swallow errors internally, only returning on ctx.Done() -- errgroup is purely a goroutine join point, not a failure propagation mechanism. (3) ZAPI disconnect behavior: route subtree is cleared immediately on disconnect (not served stale). (4) Netlink resubscription: full-scope re-read of all entities for the affected event type after any subscription error. (5) IPC server: explicitly documented that tree serves last-known-good state during subprocess restart windows. (6) Health endpoint: defined response schema with per-subsystem state/restart-count/PID and per-model last-updated timestamps; added `updated time.Time` to modelEntry struct. (7) OSPF/RIP/BFD polling intervals corrected from 5s to 10s in data source matrix and collector specifications. (8) Socket group corrected from `yangerd` to `statd`. (9) NLMonitor terminology standardized. (10) Ethmonitor: no fallback on genetlink failure, must use target kernel. (11) Feature flags renamed from "build-time" to "runtime feature flags." (12) D-Bus error paths: log+serve-stale for parse errors, explicit timeouts for D-Bus calls and nft. (13) External command timeouts: all exec.Command uses exec.CommandContext with per-command timeouts. (14) GetMulti eventual consistency documented as explicit design choice. (15) Text parser test fixtures for iw/vtysh. (16) Fswatcher path-to-YANG-leaf mapping. (17) GetMulti lock ordering safety comment. (18) Added YANGERD_POLL_INTERVAL_NTP env var (default 60s). | Assistant |
| 2026-03-04 | 0.22 | Second review pass: fixed 6 copy-paste regressions (duplicated modelEntry/Set/GetMulti/health schema, misplaced consistency note, missing package header), 8 consistency issues (timeout policy, failure philosophy exceptions, BridgeBatch ErrBatchDead, D-Bus code timeouts, yangerctl health output, dead/alive mapping, socket group, NTP env var), and 9 architectural additions (startup readiness protocol, graceful shutdown, memory bounds, security model with Finit snippet, IPC method mapping, config reload policy, signal handling, iw parser robustness, Phase-2 container namespace design). | Assistant |
| 2026-03-04 | 0.23 | Firewall data source corrected: replaced all `nft list ruleset -j` references with firewalld D-Bus method calls, matching the Python `infix_firewall.py` implementation. `refreshFirewall()` now takes `conn *dbus.Conn` and queries firewalld directly (`getDefaultZone()`, `getActiveZones()`, `getZoneSettings2()`, `getPolicies()`, `getPolicySettings()`, `listServices()`, `getServiceSettings2()`, `getLogDenied()`, `queryPanicMode()`). Updated data source matrix (nftables YANG paths replaced with firewalld zone/policy/service paths), signal subscription table, differences table, collector #11 spec, design rationale, external command timeouts, migration section (reversed: D-Bus is kept, not replaced), summary migration table, project tree (`transformNftRuleset()` renamed to `buildFirewallTree()`), dbusmonitor package description, appendix model table, and glossary D-Bus Monitor entry throughout. | Assistant |
| 2026-03-05 | 0.24 | Added testability architecture: Section 8.5 defines Go interface contracts for all 9 external dependencies (netlink, ip batch, bridge batch, D-Bus, ZAPI, ethtool, chrony, command execution, file I/O), with interface definitions, production/mock implementation table, and import restriction rule. Section 8.6 defines the verification loop (definition of done): 4-step build/vet/test/golden-file workflow executable on a developer workstation with no target hardware, golden-file capture process from running Python yanger, YANG schema validation via yanglint in CI, and 8-point per-module completion checklist. | Assistant |
| 2026-03-27 | 0.25 | Review-driven corrections: LLDP converted from polling to reactive (`lldpcli -f json0 watch`); added `infix-services:mdns` module (migrated from statd/avahi.c via avahi D-Bus); added LLDPMonitor and mDNS Monitor subsystem sections; reconciled health endpoint schema (4.3.5 vs 4.8); fixed module counts; added container lifecycle reactive recommendation; added polling justification notes; fixed `routing-state` deprecation, typos, and formatting throughout. | Assistant |

---

## 1. Introduction

### 1.1 Purpose
This document specifies the design for `yangerd`, a high-performance Go daemon that manages operational data for the Infix network OS. It serves as the authoritative technical reference for implementation, deployment, and testing.

### 1.2 Problem Statement
`statd` is the operational data daemon for Infix. On every NETCONF or RESTCONF poll that touches an operational subtree, `statd` invokes `ly_add_yanger_data()`, which calls `fsystemv()` to fork and exec the `yanger` Python script. Each invocation starts a fresh CPython interpreter, imports the relevant module (one of 14 total YANG modules in the target design; 13 current migration modules in legacy Python/C paths), runs the collection logic, prints JSON to stdout, and exits. 

The interpreter start-up cost alone is approximately 200 milliseconds per invocation. With 13 `sr_oper_get_subscribe()` callbacks registered in `subscribe_to_all()`, a worst-case full-tree poll triggers 13 sequential forks, for a cumulative delay of roughly 2.6 seconds before sysrepo can return data to the requestor.

Beyond latency, the architecture has two structural weaknesses:
1.  **No state preservation:** Every fork re-reads the same kernel interfaces, re-parses the same `ip` command output, and re-queries the same D-Bus services, even when nothing has changed.
2.  **Memory churn:** Each Python process allocates its own heap and module cache, producing high memory churn under repeated polling.

### 1.3 Solution Summary
`yangerd` (Architecture Option C -- IPC Indirection) is a pure Go daemon with no CGo dependency. It monitors Linux netlink events natively via `vishvananda/netlink` subscriptions (`LinkSubscribeWithOptions`, `AddrSubscribeWithOptions`, `NeighSubscribeWithOptions`), receiving typed Go structs on dedicated channels. Each event triggers a full re-read of the affected state: link, address, and neighbor data are re-queried through a persistent `ip -json -force -batch -` subprocess, bridge state through `bridge -json -batch -`. Route data is sourced from a streaming ZAPI connection to FRR zebra's zserv socket (not from netlink events or vtysh). Supplementary collectors handle data not exposed via netlink (ethtool genetlink, iw event, D-Bus, /proc/sys). All collected data is maintained in an in-memory YANG JSON tree with per-model `sync.RWMutex` locking -- each YANG module key has its own read-write mutex, so writers for different modules never block each other and readers only contend with writers of the same module. `statd` queries this tree over a Unix domain socket (`/run/yangerd.sock`) using a lightweight JSON/length-prefixed framing protocol, replacing the fork/exec path with a socket read. On multi-module IPC requests, per-model read locks are acquired individually, data is read and concatenated into the response.

### 1.4 Relationship to Infix Components
- **statd:** The primary consumer. It translates sysrepo operational data requests into `yangerd` IPC queries.
- **sysrepo/libyang:** `yangerd` produces JSON fragments that `statd` parses into libyang trees for sysrepo.
- **confd:** Operates in parallel. `confd` handles the configuration (write) path, while `yangerd` handles the operational (read) path.
- **netopeer2/rousette:** External management endpoints that eventually receive data collected by `yangerd`.

---

## 2. Requirements & Constraints

### 2.1 Functional Requirements
- **Real-time Monitoring:** Must subscribe to netlink events for link, address, and neighbor changes. Route data is sourced from a streaming ZAPI connection to FRR zebra.
- **Comprehensive Collection:** Must implement collectors/monitors for all 14 supported YANG modules (13 migrated modules + 1 new module: `infix-services:mdns` migrated from `statd/avahi.c`).
- **In-Memory Cache:** Maintain a synchronized, pre-serialized JSON tree of all operational state.
- **IPC Server:** Provide a Unix socket server for concurrent client queries.
- **Health Reporting:** Expose internal monitor and collector status.
- **CLI Tool:** Provide a `yangerctl` utility for manual inspection and debugging.

### 2.2 Non-Functional Requirements
- **Sub-millisecond query latency:** `statd` callbacks receive a JSON response from an in-memory read — no process spawning, no disk I/O on the hot path.
- **Reactive link state:** netlink events update the in-memory tree within microseconds of the kernel event, eliminating staleness.
- **Elimination of Python startup overhead:** the 200 milliseconds per-invocation interpreter cost is removed entirely; current per-subtree fork chains are replaced by in-memory IPC reads.
- **Single consolidated daemon:** `yangerd` replaces 25+ Python collector scripts with typed Go collector functions, simplifying deployment, logging, and error handling.
- **Pure Go cross-compilation:** No CGo dependency for easy cross-builds across ARM, AArch64, RISC-V, and x86_64.

### 2.3 Explicit Scope Boundaries (What This Is NOT)

This section defines explicit scope boundaries for yangerd. Its purpose is to prevent future scope creep, clarify integration responsibility, and help contributors quickly determine whether a proposed change belongs in yangerd, statd, confd, or elsewhere.

#### Not a sysrepo plugin

yangerd has no sysrepo dependency and registers no `sr_*` callbacks. It does not link against `libsysrepo.so`, does not open a sysrepo connection, and has no knowledge of sysrepo session handles, subscription IDs, or event types. The sysrepo integration layer lives entirely in `statd.c`: it is statd that calls `sr_oper_get_subscribe()`, receives the sysrepo callback, queries yangerd over the Unix socket, and calls `lyd_parse_data_mem()` to parse the result into a libyang tree.

Adding sysrepo to yangerd would defeat the zero-C-dependency constraint (sysrepo is a C library with no Go bindings), reintroduce link-time complexity against `libyang` and `libsysrepo`, and blur the separation of concerns that makes yangerd testable in isolation. The IPC boundary between yangerd and statd is intentional and permanent.

#### Not a NETCONF or RESTCONF server

yangerd does not speak NETCONF XML, RESTCONF JSON+XML, gRPC, YANG push, or any IETF management protocol. It has a private, non-standard IPC protocol (1-byte version + 4-byte big-endian length + JSON payload over a Unix domain socket) whose sole consumer is statd. It cannot be queried directly by a NETCONF client, a RESTCONF client, or a browser.

The management protocol endpoints in Infix remain `netopeer2` (NETCONF) and `rousette` (RESTCONF). yangerd is not a replacement for, competitor to, or extension of either. It is a data collection and aggregation daemon that feeds statd, which feeds sysrepo, which feeds the management protocol layer.

#### Not CGo

yangerd contains zero C code. There are no `import "C"` directives, no `#cgo LDFLAGS` or `#cgo CFLAGS` pragmas, no `.c` source files, and no calls to `C.*` functions. This is a hard, non-negotiable constraint.

The reason is Buildroot cross-compilation. CGo requires a matching C cross-compiler toolchain (sysroot, headers, and libraries) for each target architecture (arm, aarch64, riscv64, x86_64). Managing four CGo toolchains in Buildroot is brittle and error-prone. Pure-Go cross-compilation requires only `GOARCH` and `GOOS` environment variables — no sysroot, no linker flags, no host-target library matching.

Any future requirement that would necessitate calling a C library (e.g., direct access to a vendor-specific kernel module via an ioctl not wrapped by any Go package) must be implemented as a separate standalone binary that yangerd invokes as a subprocess, maintaining the CGo boundary outside yangerd itself.

#### Not a replacement for confd

yangerd collects and serves operational (read-only, runtime) data. It never writes to the sysrepo running datastore, never handles a NETCONF `<edit-config>` or `<copy-config>` RPC, never processes a RESTCONF PATCH, PUT, or POST, and never modifies the system configuration in any way.

Configuration management — translating NETCONF/RESTCONF configuration changes into Linux network configuration (via `ip`, `bridge`, `nft`, and other tools) — remains entirely within `confd`. There is no proposed overlap or merge between confd and yangerd. They are complementary daemons with non-overlapping responsibilities: confd handles the write path, yangerd handles the read path.

#### Not a YANG validator

yangerd does not parse YANG module files, does not load `.yang` schemas via libyang, and does not validate that JSON values conform to YANG type constraints (ranges, patterns, enumerations, must-expressions, etc.). It stores and retrieves opaque `json.RawMessage` blobs keyed by YANG path string. The blobs are produced by yangerd's own collector functions and are assumed to be structurally valid.

YANG validation — ensuring that the JSON returned by yangerd is well-typed, range-checked, and list-keyed correctly — is performed by libyang inside statd when `lyd_parse_data_mem()` is called on the JSON blob. If a collector produces malformed JSON or a value outside a YANG type's range, libyang will reject it and statd will log the error. yangerd is deliberately schema-agnostic to avoid introducing a libyang dependency.

#### Not a push or streaming daemon

yangerd does not emit spontaneous outbound messages. It does not implement YANG push (RFC 8641), does not maintain persistent subscriptions, and does not send SSE, WebSocket, or gRPC stream frames. Its communication model is strictly pull-on-demand: statd connects, sends a request, receives a response, and disconnects. (Or, if the connection is kept alive, sends the next request on the same connection — but there is no server-initiated message.)

Reactive netlink events update yangerd's internal tree continuously, but these updates are internal state changes only — they do not trigger any outbound notification to statd or to any other consumer. Consumers see the updates only when they issue the next pull request. This simplicity is intentional: it avoids the complexity of managing subscriber lists, flow control, and partial-failure handling in a push model.

#### Not responsible for container namespace data (Phase 1)

Collecting operational data from inside a container namespace (e.g., the interface list or routing table as seen from within a podman container) requires opening a netlink socket in the specific network namespace of that container. This involves calling `netlink.NewHandleAt(ns)` with a namespace file descriptor, which in turn requires reading `/proc/<pid>/ns/net` for the container's PID — a non-trivial and error-prone operation that differs between rootful and rootless podman.

This complexity is explicitly deferred to Phase 2. In Phase 1, yangerd's netlink monitors operate exclusively in the host (init) network namespace and report the host's view of all interfaces, routes, and neighbours. Container-internal interfaces that appear in the host namespace (veth pairs) are included; interfaces visible only from inside the container are not.

#### Not VRF-aware

yangerd operates exclusively in the default VRF. It does not subscribe to non-default VRF route tables, does not open netlink sockets in non-default VRF contexts, and does not distinguish routes by VRF ID in its in-memory tree. The ZAPI watcher connects to zebra using `zebra.VRFDefault` and subscribes to route redistribution for the default VRF only. The ZAPI v6 wire format includes a VRF ID field in every message header, but yangerd treats all messages as belonging to VRF 0 (default) and ignores messages with non-zero VRF IDs. Multi-VRF support is explicitly out of scope for both Phase 1 and Phase 2.

### 2.4 Hard Constraints
- **No CGo.**
- **No direct sysrepo access.**
- **No YANG validation in yangerd.**

### 2.5 YANG-Model JSON Output Compatibility

yangerd MUST produce JSON output that is structurally identical to the current Python yanger scripts. The output is consumed by `statd`, which passes it to `lyd_parse_data_mem()` in libyang for validation against the installed YANG schemas. Any deviation in key names, nesting structure, module prefixes, or value encoding will cause libyang to reject the data.

This is a hard implementation constraint, not a best-effort goal. The Go collectors must transform `iproute2`, `ethtool`, `iw`, `vtysh`, D-Bus, and filesystem data into the exact same YANG-model JSON structure that the Python yanger scripts produce today. The canonical format specification is the Python source code in `src/statd/python/yanger/`.

#### 2.5.1 Top-Level JSON Structure Per Module

Each yanger module returns a JSON object with one or more YANG-module-prefixed top-level keys (RFC 7951 module-qualified names). yangerd must produce the same top-level keys for each module path.

| YANG Module | yanger Python Module | Top-Level JSON Key(s) |
|-------------|---------------------|-----------------------|
| `ietf-interfaces` | `ietf_interfaces` | `"ietf-interfaces:interfaces"` |
| `ietf-routing` | `ietf_routing` | `"ietf-routing:routing"` |
| `ietf-hardware` | `ietf_hardware` | `"ietf-hardware:hardware"` |
| `ietf-system` | `ietf_system` | `"ietf-system:system"`, `"ietf-system:system-state"` |
| `ietf-ntp` | `ietf_ntp` | `"ietf-ntp:ntp"` (nested inside out dict via `insert()`) |
| `ieee802-dot1ab-lldp` | `infix_lldp` | `"ieee802-dot1ab-lldp:lldp"` |
| `infix-containers` | `infix_containers` | `"infix-containers:containers"` |
| `infix-dhcp-server` | `infix_dhcp_server` | `"infix-dhcp-server:dhcp-server"` |
| `infix-firewall` | `infix_firewall` | `"infix-firewall:firewall"` |
| `infix-services` | `(new — migrated from statd/avahi.c)` | `"infix-services:mdns"` |
| `ietf-ospf` | `ietf_ospf` | `"ietf-routing:routing"` (with nested `control-plane-protocols`) |
| `ietf-rip` | `ietf_rip` | `"ietf-routing:routing"` (with nested `control-plane-protocols`) |
| `ietf-bfd-ip-sh` | `ietf_bfd_ip_sh` | `"ietf-routing:routing"` (with nested `control-plane-protocols`) |


#### 2.5.2 Concrete JSON Output Examples

The following examples show the exact JSON structures that yangerd must produce for each module. These are derived directly from the Python source code.

**ietf-interfaces** (`ietf_interfaces/__init__.py`, `link.py`):
```json
{
  "ietf-interfaces:interfaces": {
    "interface": [
      {
        "type": "infix-if-type:ethernet",
        "name": "eth0",
        "if-index": 2,
        "admin-status": "up",
        "oper-status": "up",
        "phys-address": "02:00:00:00:00:01",
        "statistics": {
          "in-octets": "123456789012",
          "out-octets": "987654321098"
        },
        "ietf-ip:ipv4": {
          "mtu": 1500,
          "address": [
            {
              "ip": "192.168.1.1",
              "prefix-length": 24,
              "origin": "static"
            }
          ]
        },
        "ietf-ip:ipv6": {
          "mtu": 1500,
          "address": [
            {
              "ip": "fe80::1",
              "prefix-length": 64,
              "origin": "link-layer"
            }
          ]
        },
        "ieee802-ethernet-interface:ethernet": {
          "auto-negotiation": {
            "enable": true
          },
          "speed": "1.0",
          "duplex": "full",
          "statistics": {
            "frame": {
              "out-frames": "12345",
              "out-multicast-frames": "100",
              "out-broadcast-frames": "50",
              "in-frames": "67890",
              "in-multicast-frames": "200",
              "in-broadcast-frames": "75",
              "in-total-frames": "68000",
              "in-error-fcs-frames": "0",
              "in-error-undersize-frames": "0",
              "in-error-oversize-frames": "0",
              "infix-ethernet-interface:out-good-octets": "9876543",
              "infix-ethernet-interface:in-good-octets": "12345678"
            }
          }
        }
      }
    ]
  }
}
```

**ietf-interfaces with bridge augmentation** (`bridge.py`):
```json
{
  "name": "br0",
  "type": "infix-if-type:bridge",
  "infix-interfaces:bridge": {
    "vlans": {
      "proto": "ieee802-dot1q-types:c-vlan",
      "vlan": [
        {
          "vid": 1,
          "untagged": ["br0", "eth0"],
          "tagged": ["eth1"],
          "multicast": {
            "snooping": true,
            "querier": "auto"
          },
          "multicast-filters": {
            "multicast-filter": [
              {
                "group": "239.1.1.1",
                "ports": [
                  {
                    "port": "eth0",
                    "state": "permanent"
                  }
                ]
              }
            ]
          }
        }
      ]
    },
    "stp": {
      "force-protocol": "rstp",
      "hello-time": 2,
      "forward-delay": 15,
      "max-age": 20,
      "transmit-hold-count": 6,
      "max-hops": 20,
      "cist": {
        "priority": 32768,
        "bridge-id": {
          "priority": 32768,
          "system-id": 0,
          "address": "02:00:00:00:00:01"
        },
        "root-id": {
          "priority": 32768,
          "system-id": 0,
          "address": "02:00:00:00:00:01"
        },
        "root-port": "eth0",
        "topology-change": {
          "count": 1,
          "in-progress": false,
          "port": "eth0",
          "time": "2026-02-24T11:00:00+0000"
        }
      }
    }
  }
}

**ietf-interfaces with WireGuard augmentation** (`wireguard.py`):
```json
{
  "name": "wg0",
  "type": "infix-if-type:wireguard",
  "infix-interfaces:wireguard": {
    "peer-status": {
      "peer": [
        {
          "public-key": "aGVsbG8gd29ybGQ=",
          "connection-status": "up",
          "latest-handshake": "2026-02-24T12:00:00+00:00",
          "endpoint-address": "192.168.1.1",
          "endpoint-port": 51820,
          "transfer": {
            "tx-bytes": "123456",
            "rx-bytes": "654321"
          }
        }
      ]
    }
  }
}
```

**ietf-interfaces with WiFi augmentation** (`wifi.py`):

WiFi output depends on the interface mode.  In AP mode:
```json
{
  "name": "wlan0",
  "type": "infix-if-type:wifi",
  "infix-interfaces:wifi": {
    "access-point": {
      "ssid": "MyNetwork",
      "stations": {
        "station": [
          {
            "mac": "02:00:00:00:00:05",
            "signal": -45,
            "rx_bitrate": 400.0,
            "tx_bitrate": 866.7,
            "connected_time": 3600,
            "inactive_time": 100
          }
        ]
      }
    }
  }
}
```

In station (client) mode:
```json
{
  "name": "wlan0",
  "type": "infix-if-type:wifi",
  "infix-interfaces:wifi": {
    "station": {
      "ssid": "MyNetwork",
      "signal-strength": -45,
      "rx-speed": 400,
      "tx-speed": 866,
      "scan-results": [
        {
          "bssid": "02:00:00:00:00:01",
          "ssid": "MyNetwork",
          "signal-strength": -42,
          "encryption": ["WPA2-Personal"],
          "channel": 36
        }
      ]
    }
  }
}
```

**ietf-interfaces with VLAN augmentation** (`vlan.py`):
```json
{
  "name": "eth0.10",
  "type": "infix-if-type:vlan",
  "infix-interfaces:vlan": {
    "tag-type": "ieee802-dot1q-types:c-vlan",
    "id": 10,
    "lower-layer-if": "eth0"
  }
}
```

**ietf-interfaces with LAG augmentation** (`lag.py`):

LACP mode:
```json
{
  "name": "bond0",
  "type": "infix-if-type:lag",
  "infix-interfaces:lag": {
    "mode": "lacp",
    "lacp": {
      "mode": "active",
      "rate": "fast",
      "hash": "layer3-4",
      "aggregator-id": 1,
      "actor-key": 13,
      "partner-key": 13,
      "partner-mac": "02:00:00:00:00:03",
      "system-priority": 65535
    },
    "link-monitor": {
      "debounce": {
        "up": 0,
        "down": 0
      }
    }
  }
}
```

Static (non-LACP) mode:
```json
{
  "name": "bond0",
  "type": "infix-if-type:lag",
  "infix-interfaces:lag": {
    "mode": "static",
    "static": {
      "mode": "balance-xor",
      "hash": "layer3+4"
    },
    "link-monitor": {
      "debounce": {
        "up": 0,
        "down": 0
      }
    }
  }
}
```

**ietf-interfaces with LAG member augmentation** (`lag.py:lower()`):
```json
{
  "name": "eth0",
  "infix-interfaces:lag-port": {
    "lag": "bond0",
    "state": "active",
    "link-failures": 0,
    "lacp": {
      "aggregator-id": 1,
      "actor-state": "AD",
      "partner-state": "AD"
    }
  }
}
```

**ietf-interfaces with tunnel augmentation** (`tun.py`):
```json
{
  "name": "gre0",
  "type": "infix-if-type:gre",
  "infix-interfaces:gre": {
    "local": "10.0.0.1",
    "remote": "10.0.0.2"
  }
}
```

**ietf-interfaces with veth augmentation** (`veth.py`):
```json
{
  "name": "veth0",
  "type": "infix-if-type:veth",
  "infix-interfaces:veth": {
    "peer": "veth1"
  }
}
```

**ietf-interfaces with container-network augmentation** (`container.py`):

Container network interfaces include `interface_common()` fields plus
container-specific data.  The `description` holds the kernel-internal
interface name while `name` is the user-facing container network name:
```json
{
  "type": "iana-if-type:ethernetCsmacd",
  "name": "cni0",
  "if-index": 42,
  "admin-status": "up",
  "oper-status": "up",
  "phys-address": "02:00:00:00:00:01",
  "description": "real-kernel-name",
  "statistics": {
    "in-octets": "1234",
    "out-octets": "5678"
  },
  "infix-interfaces:container-network": {
    "containers": ["mycontainer"]
  }
}
```

**ietf-routing** (`ietf_routing.py`):

The routing module produces two ribs named `ipv4` and `ipv6`.  The
`interfaces` list is populated with interfaces that have forwarding
enabled.  Route fields use fully qualified YANG names:
```json
{
  "ietf-routing:routing": {
    "interfaces": {
      "interface": ["eth0", "eth1"]
    },
    "ribs": {
      "rib": [
        {
          "name": "ipv4",
          "address-family": "ipv4",
          "routes": {
            "route": [
              {
                "ietf-ipv4-unicast-routing:destination-prefix": "192.168.1.0/24",
                "source-protocol": "infix-routing:kernel",
                "route-preference": 100,
                "active": [null],
                "last-updated": "2026-02-24T12:00:00+00:00",
                "next-hop": {
                  "next-hop-list": {
                    "next-hop": [
                      {
                        "ietf-ipv4-unicast-routing:address": "10.0.0.1",
                        "infix-routing:installed": [null]
                      }
                    ]
                  }
                }
              },
              {
                "ietf-ipv4-unicast-routing:destination-prefix": "10.0.0.0/8",
                "source-protocol": "direct",
                "route-preference": 0,
                "active": [null],
                "last-updated": "2026-02-24T00:00:00+00:00",
                "next-hop": {
                  "outgoing-interface": "eth0"
                }
              }
            ]
          }
        },
        {
          "name": "ipv6",
          "address-family": "ipv6"
        }
      ]
    }
  }
}
```

**ietf-hardware** (`ietf_hardware.py`):

The hardware module builds a component list from multiple sources:
mainboard, VPD, USB ports, hwmon sensors, thermal zones, WiFi radios,
and GPS receivers.  Sensor data uses `value-type` + `value-scale`
instead of combined types:
```json
{
  "ietf-hardware:hardware": {
    "component": [
      {
        "name": "mainboard",
        "class": "iana-hardware:chassis",
        "mfg-name": "Kernelkit",
        "serial-num": "ABC123",
        "state": {
          "admin-state": "unknown",
          "oper-state": "enabled"
        }
      },
      {
        "name": "cpu",
        "class": "iana-hardware:sensor",
        "sensor-data": {
          "value": 42000,
          "value-type": "celsius",
          "value-scale": "milli",
          "value-precision": 0,
          "value-timestamp": "2026-02-24T12:00:00+00:00",
          "oper-status": "ok"
        }
      },
      {
        "name": "sfp0",
        "class": "iana-hardware:module"
      },
      {
        "name": "sfp0-temperature",
        "class": "iana-hardware:sensor",
        "parent": "sfp0",
        "sensor-data": {
          "value": 35500,
          "value-type": "celsius",
          "value-scale": "milli",
          "value-precision": 0,
          "value-timestamp": "2026-02-24T12:00:00+00:00",
          "oper-status": "ok"
        }
      },
      {
        "name": "sfp0-voltage",
        "class": "iana-hardware:sensor",
        "parent": "sfp0",
        "description": "Vcc",
        "sensor-data": {
          "value": 3300,
          "value-type": "volts-DC",
          "value-scale": "milli",
          "value-precision": 0,
          "value-timestamp": "2026-02-24T12:00:00+00:00",
          "oper-status": "ok"
        }
      },
      {
        "name": "usb1",
        "class": "infix-hardware:usb",
        "state": {
          "admin-state": "unlocked",
          "oper-state": "enabled"
        }
      },
      {
        "name": "radio0",
        "class": "infix-hardware:wifi",
        "description": "WiFi Radio radio0",
        "infix-hardware:wifi-radio": {
          "bands": [{"band": "1", "name": "2.4 GHz", "ht-capable": true}],
          "driver": "mt7915e",
          "max-interfaces": {"ap": 4},
          "supported-channels": [1, 6, 11],
          "num-virtual-interfaces": 1
        }
      },
      {
        "name": "gps0",
        "class": "infix-hardware:gps",
        "description": "GPS/GNSS Receiver",
        "infix-hardware:gps-receiver": {
          "device": "/dev/gps0",
          "driver": "u-blox",
          "activated": true,
          "fix-mode": "3d",
          "latitude": "57.708870",
          "longitude": "11.974560",
          "altitude": "45.2",
          "satellites-visible": 12,
          "satellites-used": 8,
          "pps-available": true
        }
      }
    ]
  }
}
```

**ietf-system** (`ietf_system.py`):

The system module splits output between `ietf-system:system` (config-
visible state) and `ietf-system:system-state` (operational).  DNS
resolver includes both static and DHCP-learned servers with origin
tracking.  NTP source state is under `infix-system:ntp`.  Resource
usage includes filesystem utilization:
```json
{
  "ietf-system:system": {
    "hostname": "infix",
    "authentication": {
      "user": [
        {
          "name": "admin",
          "password": "$6$...",
          "infix-system:shell": "infix-system:clish",
          "authorized-key": [
            {
              "name": "admin-key-0",
              "algorithm": "ssh-ed25519",
              "key-data": "AAAA..."
            }
          ]
        }
      ]
    },
    "clock": {
      "timezone-name": "Europe/Stockholm"
    }
  },
  "ietf-system:system-state": {
    "platform": {
      "os-name": "Infix",
      "os-version": "25.02.0",
      "os-release": "20260224",
      "machine": "x86_64"
    },
    "clock": {
      "current-datetime": "2026-02-24T12:00:00+00:00",
      "boot-datetime": "2026-02-24T00:00:00+00:00"
    },
    "infix-system:software": {
      "compatible": "infix-x86_64",
      "booted": "rootfs.0",
      "slot": [],
      "installer": {}
    },
    "infix-system:ntp": {
      "sources": {
        "source": [
          {
            "address": "192.168.1.1",
            "mode": "server",
            "state": "selected",
            "stratum": 2,
            "poll": 6
          }
        ]
      }
    },
    "infix-system:services": {
      "service": [
        {
          "pid": 1234,
          "name": "syslogd",
          "status": "running",
          "description": "System log daemon",
          "statistics": {
            "memory-usage": "4096",
            "uptime": "86400",
            "restart-count": 0
          }
        }
      ]
    },
    "infix-system:dns-resolver": {
      "options": {
        "timeout": 5,
        "attempts": 2
      },
      "server": [
        {
          "address": "8.8.8.8",
          "origin": "static"
        },
        {
          "address": "192.168.1.1",
          "origin": "dhcp",
          "interface": "eth0"
        }
      ],
      "search": ["example.com"]
    },
    "infix-system:resource-usage": {
      "memory": {
        "total": "4096000",
        "free": "2048000",
        "available": "3072000"
      },
      "load-average": {
        "load-1min": "0.15",
        "load-5min": "0.10",
        "load-15min": "0.05"
      },
      "filesystem": [
        {
          "mount-point": "/",
          "size": "2097152",
          "used": "524288",
          "available": "1572864"
        },
        {
          "mount-point": "/var",
          "size": "1048576",
          "used": "262144",
          "available": "786432"
        },
        {
          "mount-point": "/cfg",
          "size": "65536",
          "used": "4096",
          "available": "61440"
        }
      ]
    }
  }
}
```

**ietf-ntp** (`ietf_ntp.py`):
```json
{
  "ietf-ntp:ntp": {
    "associations": {
      "association": [
        {
          "address": "192.168.1.1",
          "local-mode": "ietf-ntp:client",
          "isconfigured": true,
          "stratum": 2,
          "prefer": true,
          "reach": 255,
          "poll": 6,
          "now": 12,
          "offset": "0.123",
          "delay": "1.456",
          "dispersion": "0.089"
        }
      ]
    },
    "clock-state": {
      "system-status": {
        "clock-state": "ietf-ntp:synchronized",
        "clock-stratum": 2,
        "clock-refid": "GPS ",
        "nominal-freq": "1000000000.0000",
        "actual-freq": "1000000000.0012",
        "clock-precision": -20,
        "clock-offset": "0.001",
        "root-delay": "1.234",
        "root-dispersion": "0.567",
        "reference-time": "2026-02-25T10:30:15.12Z",
        "sync-state": "ietf-ntp:clock-synchronized",
        "infix-ntp:last-offset": "0.000001234",
        "infix-ntp:rms-offset": "0.000002345",
        "infix-ntp:residual-freq": "0.012",
        "infix-ntp:skew": "0.034",
        "infix-ntp:update-interval": "64.0"
      }
    },
    "refclock-master": {
      "master-stratum": 2
    },
    "port": 123,
    "ntp-statistics": {
      "packet-received": 1000,
      "packet-dropped": 5,
      "packet-sent": 950,
      "packet-sent-fail": 0
    }
  }
}
```

**ieee802-dot1ab-lldp** (`infix_lldp.py`):
```json
{
  "ieee802-dot1ab-lldp:lldp": {
    "port": [
      {
        "name": "eth0",
        "dest-mac-address": "01:80:C2:00:00:0E",
        "remote-systems-data": [
          {
            "time-mark": 3600,
            "remote-index": 1,
            "chassis-id-subtype": "mac-address",
            "chassis-id": "02:00:00:00:00:01",
            "port-id-subtype": "interface-name",
            "port-id": "eth0"
          }
        ]
      }
    ]
  }
}
```

**infix-containers** (`infix_containers.py`):
```json
{
  "infix-containers:containers": {
    "container": [
      {
        "name": "mycontainer",
        "id": "abc123def456",
        "image": "docker.io/library/alpine:latest",
        "image-id": "sha256:abc123",
        "running": true,
        "status": "Up 2 hours",
        "command": "/bin/sh",
        "network": {
          "interface": [{"name": "podnet"}],
          "publish": ["0.0.0.0:8080:80/tcp"]
        },
        "resource-limit": {
          "memory": "524288",
          "cpu": 1000
        },
        "resource-usage": {
          "memory": "32768",
          "cpu": "2.50",
          "block-io": {
            "read": "1024",
            "write": "512"
          },
          "net-io": {
            "received": "2048",
            "sent": "1024"
          },
          "pids": 5
        }
      }
    ]
  }
}
```

**infix-dhcp-server** (`infix_dhcp_server.py`):
```json
{
  "infix-dhcp-server:dhcp-server": {
    "statistics": {
      "out-offers": 95,
      "out-acks": 88,
      "out-naks": 2,
      "in-declines": 0,
      "in-discovers": 100,
      "in-requests": 90,
      "in-releases": 10,
      "in-informs": 3
    },
    "leases": {
      "lease": [
        {
          "expires": "2026-02-25T12:00:00+00:00",
          "address": "192.168.1.100",
          "phys-address": "02:00:00:00:00:01",
          "hostname": "client1",
          "client-id": "01:02:00:00:00:00:01"
        }
      ]
    }
  }
}
```

**infix-firewall** (`infix_firewall.py`):
```json
{
  "infix-firewall:firewall": {
    "default": "public",
    "logging": "off",
    "lockdown": false,
    "zone": [
      {
        "name": "public",
        "short": "Public",
        "immutable": false,
        "description": "For use in public areas",
        "interface": ["eth0"],
        "network": [],
        "action": "reject",
        "service": ["ssh", "dhcpv6-client"],
        "port-forward": [
          {
            "lower": 443,
            "proto": "tcp",
            "to": {
              "addr": "192.168.2.10",
              "port": 443
            }
          }
        ]
      }
    ],
    "policy": [
      {
        "name": "allow-host-ipv6",
        "action": "accept",
        "priority": -15000,
        "ingress": ["HOST"],
        "egress": ["ANY"]
      }
    ],
    "service": [
      {
        "name": "ssh",
        "description": "Secure Shell",
        "port": [
          {
            "lower": 22,
            "proto": "tcp"
          }
        ]
      }
    ]
  }
}
```

**ietf-ospf** (`ietf_ospf.py`):
```json
{
  "ietf-routing:routing": {
    "control-plane-protocols": {
      "control-plane-protocol": [
        {
          "type": "infix-routing:ospfv2",
          "name": "default",
          "ietf-ospf:ospf": {
            "ietf-ospf:router-id": "10.0.0.1",
            "ietf-ospf:address-family": "ipv4",
            "ietf-ospf:areas": {
              "ietf-ospf:area": [
                {
                  "ietf-ospf:area-id": "0.0.0.0",
                  "ietf-ospf:interfaces": {
                    "ietf-ospf:interface": [
                      {
                        "name": "eth0",
                        "state": "dr",
                        "enabled": true,
                        "passive": false,
                        "interface-type": "broadcast",
                        "ietf-ospf:neighbors": {
                          "ietf-ospf:neighbor": []
                        }
                      }
                    ]
                  }
                }
              ]
            },
            "ietf-ospf:local-rib": {
              "ietf-ospf:route": [
                {
                  "prefix": "192.168.1.0/24",
                  "route-type": "intra-area",
                  "metric": 10,
                  "next-hops": {
                    "next-hop": [
                      {
                        "next-hop": "10.0.0.2"
                      }
                    ]
                  }
                }
              ]
            }
          }
        }
      ]
    }
  }
}
```

**ietf-rip** (`ietf_rip.py`):
```json
{
  "ietf-routing:routing": {
    "control-plane-protocols": {
      "ietf-routing:control-plane-protocol": [
        {
          "type": "infix-routing:ripv2",
          "name": "default",
          "ietf-rip:rip": {
            "distance": 120,
            "default-metric": 1,
            "timers": {
              "update-interval": 30,
              "invalid-interval": 180,
              "flush-interval": 240
            },
            "interfaces": {
              "interface": [
                {
                  "interface": "eth0",
                  "oper-status": "up",
                  "send-version": "2",
                  "receive-version": "2"
                }
              ]
            },
            "ipv4": {
              "routes": {
                "route": [
                  {
                    "ipv4-prefix": "192.168.50.0/24",
                    "metric": 2,
                    "route-type": "rip",
                    "next-hop": "192.168.50.2",
                    "interface": "eth0"
                  }
                ]
              },
              "neighbors": {
                "neighbor": [
                  {
                    "ipv4-address": "192.168.50.2",
                    "bad-packets-rcvd": 0,
                    "bad-routes-rcvd": 0
                  }
                ]
              }
            },
            "num-of-routes": 1
          }
        }
      ]
    }
  }
}
```

**ietf-bfd-ip-sh** (`ietf_bfd_ip_sh.py`):
```json
{
  "ietf-routing:routing": {
    "control-plane-protocols": {
      "control-plane-protocol": [
        {
          "type": "infix-routing:bfdv1",
          "name": "bfd",
          "ietf-bfd:bfd": {
            "ietf-bfd-ip-sh:ip-sh": {
              "sessions": {
                "session": [
                  {
                    "interface": "eth0",
                    "dest-addr": "10.0.0.2",
                    "local-discriminator": 1,
                    "remote-discriminator": 2,
                    "session-running": {
                      "local-state": "up",
                      "remote-state": "up",
                      "local-diagnostic": "none",
                      "detection-mode": "async-without-echo",
                      "negotiated-rx-interval": 300000,
                      "negotiated-tx-interval": 300000,
                      "detection-time": 900000
                    },
                    "path-type": "ietf-bfd-types:path-ip-sh",
                    "ip-encapsulation": true
                  }
                ]
              }
            }
          }
        }
      ]
    }
  }
}
```

#### 2.5.3 Structural Rules

The following rules govern the JSON output format. These MUST be followed by all Go collectors.

1. **Module-prefixed top-level keys (RFC 7951).** Every top-level key in the returned JSON object uses the YANG module name as a prefix, separated by a colon: `"ietf-interfaces:interfaces"`, `"ietf-routing:routing"`, `"infix-firewall:firewall"`. This is mandated by RFC 7951 section 4 for module-qualified names.

2. **Module-prefixed augmentation keys.** When a YANG augmentation from a different module adds nodes to a container, the augmented nodes use the augmenting module's prefix: `"ietf-ip:ipv4"`, `"infix-interfaces:bridge"`, `"ieee802-ethernet-interface:ethernet"`, `"infix-system:shell"`, `"infix-routing:area-id"`, `"ietf-ospf:ospf"`. Nodes from the same module as their parent do NOT carry a prefix (e.g., `"name"`, `"type"`, `"oper-status"` inside `ietf-interfaces`).

3. **YANG lists are JSON arrays.** Every YANG `list` is encoded as a JSON key whose value is an array of objects. The key name is the YANG list name: `"interface": [...]`, `"component": [...]`, `"lease": [...]`, `"route": [...]`, `"association": [...]`, `"session": [...]`.

4. **YANG leaf-lists are JSON arrays of scalars.** YANG `leaf-list` nodes are encoded as arrays of strings or numbers: `"boot-order": ["rootfs.0", "rootfs.1"]`, `"containers": ["mycontainer"]`.

5. **Presence containers as `[null]`.** YANG presence containers (containers whose mere existence carries semantic meaning) are encoded as `[null]` per RFC 7951 section 6.9. Example from `wifi.py`: `"active": [null]`.

6. **Large counters as strings.** Any YANG type that can exceed 32-bit range (`uint64`, `counter64`, `yang:gauge64`, `yang:zero-based-counter64`) MUST be encoded as a JSON string, not a number. JavaScript/JSON numbers lose precision beyond 2^53. Examples: `"in-octets": "123456789012"`, `"tx-bytes": "123456"`, `"memory-usage": "4096"`, `"size": "12345678"`.

7. **Decimal values with fixed fraction digits.** YANG `decimal64` types and certain formatted numeric strings must use a fixed number of fraction digits matching the YANG type definition. Examples from NTP: `"offset": "0.123"` (3 fraction digits), `"nominal-freq": "1000000000.0000"` (4 fraction digits), `"infix-ntp:last-offset": "0.000000001"` (9 fraction digits). CPU percentage: `"cpu": "2.50"` (2 fraction digits).

8. **Boolean values are JSON booleans.** YANG `boolean` leaves are encoded as JSON `true` or `false`, not strings: `"running": true`, `"masquerade": false`, `"immutable": true`, `"passive": false`.

9. **Integer values are JSON numbers.** YANG integer types (`int8`, `int16`, `int32`, `uint8`, `uint16`, `uint32`) are encoded as JSON numbers: `"if-index": 2`, `"vid": 1`, `"stratum": 2`, `"priority": 32767`. Exception: `uint64` and `counter64` are strings (rule 6).

10. **YANG identity references use module-prefixed strings.** YANG `identityref` values include the defining module's prefix: `"ietf-ntp:client"`, `"infix-routing:ospfv2"`, `"infix-if-type:ethernet"`, `"ietf-bfd-types:path-ip-sh"`, `"ieee802-dot1q-types:c-vlan"`, `"infix-system:bash"`.

11. **YANG enumeration values are lowercase strings.** YANG `enumeration` values are encoded as their enum string: `"oper-status": "up"`, `"duplex": "full"`, `"state": "active"`, `"action": "reject"`.

12. **Empty or absent containers are omitted.** If a collector has no data for an optional container, it omits the key entirely rather than including an empty object. Exception: containers that serve as structural anchors (e.g., `"ietf-ospf:neighbors": {}`) may be included empty when required by the YANG schema for list parent nodes.

13. **Timestamps use YANG `date-and-time` format.** All timestamps follow RFC 3339 / ISO 8601 with timezone offset using colon separator: `"2026-02-24T12:00:00+00:00"`. The Python `YangDate` class in `common.py` formats this as `strftime("%Y-%m-%dT%H:%M:%S%z")` with a colon inserted in the timezone offset.

14. **The `insert()` helper pattern.** The Python code uses `common.insert(obj, *path_and_value)` to build nested structures. This is equivalent to creating nested dicts along a path. Go collectors should build the equivalent nested `map[string]interface{}` structure directly.

#### 2.5.4 Field Transformation Reference

The following table documents key transformations from Linux data sources to YANG JSON keys. Go collectors must replicate these transformations exactly.

| Linux Source | Linux Field | YANG JSON Key | Transform | Example |
|-------------|-------------|---------------|-----------|---------|
| `ip -j link show` | `ifname` | `"name"` | Direct copy | `"eth0"` |
| `ip -j link show` | `link_type` + `info_kind` | `"type"` | `iplink2yang_type()` map | `"infix-if-type:ethernet"` |
| `ip -j link show` | `operstate` | `"oper-status"` | Lowercase | `"up"`, `"down"` |
| `ip -j link show` | `ifindex` | `"if-index"` | Direct copy (int) | `2` |
| `ip -j link show` | `address` | `"phys-address"` | Direct copy | `"02:00:00:00:00:01"` |
| `ip -j link show` | `mtu` | `"mtu"` | Direct copy (int) | `1500` |
| `ip -s -j link show` | `stats64.rx.bytes` | `"in-octets"` | `str()` (string) | `"123456789012"` |
| `ip -s -j link show` | `stats64.tx.bytes` | `"out-octets"` | `str()` (string) | `"987654321098"` |
| `ip -s -j link show` | `stats64.rx.packets` | `"in-unicast-pkts"` | `str()` (string) | `"12345"` |
| `ip -s -j link show` | `stats64.tx.packets` | `"out-unicast-pkts"` | `str()` (string) | `"67890"` |
| `ip -j addr show` | `local` | `"ip"` (in address list) | Direct copy | `"192.168.1.1"` |
| `ip -j addr show` | `prefixlen` | `"prefix-length"` | Direct copy (int) | `24` |
| `ethtool --json <if>` | `speed` | `"speed"` | Mbps string | `"1000"` |
| `ethtool --json <if>` | `duplex` | `"duplex"` | Lowercase | `"full"` |
| `ethtool --json -S <if>` | group counters | `"statistics"."frame".*` | `str()` for uint64 | `"12345"` |
| `wg show <if> dump` | `rx_bytes` | `"rx-bytes"` | `str()` (string) | `"654321"` |
| `wg show <if> dump` | `tx_bytes` | `"tx-bytes"` | `str()` (string) | `"123456"` |
| `wg show <if> dump` | `latest_handshake` | `"latest-handshake"` | RFC 3339 timestamp | `"2026-02-24T12:00:00+00:00"` |
| `/proc/meminfo` | `MemTotal` | `"total"` | KiB as string | `"4096000"` |
| `/proc/loadavg` | fields 0-2 | `"load-1min"` etc. | Direct copy (string) | `"0.15"` |
| chrony cmdmon protocol (sources) | fields 2-9 | `"address"`, `"stratum"`, etc. | Typed Go struct | see `ietf_ntp.py` |
| `vtysh -c 'show ip ospf ...'` | JSON fields | `"ietf-ospf:*"` | Module-prefixed keys | see `ietf_ospf.py` |
| `lldpcli show neighbors -f json` | `interface.*` | `"port"` list | Restructure per-port | see `infix_lldp.py` |
| firewalld D-Bus | zone/policy/service | `"infix-firewall:firewall"` | D-Bus to YANG map | see `infix_firewall.py` |
| `podman ps/inspect/stats` | container fields | `"infix-containers:containers"` | Restructure + cgroup parse | see `infix_containers.py` |

#### 2.5.5 Validation Strategy

To ensure yangerd's JSON output is correct and schema-compliant:

1. **Golden-file tests.** For each YANG module, golden reference files capture the expected JSON structure from a known-good system state (`golden/<model>.json`). The Go integration tests compare yangerd's output against these golden files using a structural JSON diff (ignoring value differences for volatile fields like timestamps and counters, but requiring identical key structure and nesting). Golden files are committed to the repository and updated when YANG models change or output format is intentionally modified.

2. **libyang validation.** The Go integration tests pass yangerd's output through `lyd_parse_data_mem()` (via a small C test harness or by running `yanglint`) to verify that libyang accepts the JSON. This is the ultimate acceptance criterion -- if libyang rejects the output, it is wrong regardless of what the golden file says.

3. **Replay-based testing.** yangerd supports a replay mode (reading captured `ip`, `ethtool`, `iw`, etc. output from files) to enable deterministic testing. This allows byte-exact comparison of output for the same input data across test runs.

4. **Per-module smoke tests.** Each Go collector function has unit tests that verify the JSON key structure for representative inputs. These tests assert the presence of module-prefixed keys, correct list-vs-object encoding, and string-vs-number encoding for counter types.
---

## 3. Architecture Overview

### 3.1 Component Diagram

```
                        ┌─────────────────────────────────────────────────────────────────┐
                        │                         yangerd (Go)                            │
                        │                                                                 │
  netlink subs ────────►│  NLMonitor (event dispatcher)  in-memory YANG tree                │
  (vishvananda/netlink)  │        │                      ┌──────────────────┐              │
   LinkUpdate ch         │        ▼                      │                  │              │
   AddrUpdate ch         │  ip -json -batch - ◄────────► │  per-model       │              │
   NeighUpdate ch        │  (persistent query            │  RWMutex-guarded  │              │
                        │   subprocess)                 │  subtrees        │              │
                        │                               │                  │              │
  bridge netlink ──────►│  bridge event dispatcher       │  /ietf-interfaces │              │
  (NeighSub + LinkSub   │        │                      │  /ietf-routing    │              │
   + raw RTNLGRP_MDB)   │        ▼                      │  /ietf-hardware   │              │
                        │  bridge -json -batch - ◄────► │  /ietf-system     │              │
                        │  (persistent query            │  ...              │              │
                        │   subprocess)                 │                  │              │
                        │                               └────────┬─────────┘              │
  iw event -t ─────────►│  wifi event dispatcher                 │                        │
  (802.11 station,       │        │                               │                        │
   auth, scan,           │        ▼                               │                        │
   channel events)       │  iw dev <if> info/station ────────────►│                        │
                        │  (text parse + re-query)                │                        │
                        │                                        │                        │
  D-Bus Monitor ────────►│  ┌──────────────┐                      │                        │
  (godbus/dbus/v5)       │  │ dbusmonitor  │──────────────────────►│                        │
  dnsmasq signals        │  │ (reactive    │                      │                        │
  (DHCPLease*)           │  │  D-Bus sigs) │                      │                        │
  firewalld signals      │  └──────────────┘                      │                        │
  (Reloaded,             │                                        │                        │
   NameOwnerChanged)     │                                        │                        │
                        │                                        │                        │
  File Watcher ────────────▶│  ┌──────────────┐                      │                        │
  (fsnotify/inotify)    │  │ fswatcher    │──────────────────────▶│                        │
  /proc/sys forwarding  │  │ (reactive    │                      │                        │
                        │  │  file I/O)   │                      │                        │
                        │                                        │                        │
  ethtool genetlink ───►│  ┌──────────────┐                      │                        │
  ETHNL_MCGRP_MONITOR   │  │ ethmonitor   │──────────────────────►│                        │
  (speed, duplex,        │  │ (reactive    │                      │                        │
   autoneg NTFs)         │  │  genetlink)  │                      │                        │
                        │  └──────────────┘                      │                        │
                        │                                        │                        │
  ZAPI (zserv) ────────►│  ┌──────────────┐                      │                        │
  /var/run/frr/         │  │ zapiwatcher  │──────────────────────►│                        │
  zserv.api             │  │ (streaming   │                      │                        │
  (REDISTRIBUTE_        │  │  ZAPI v6)    │                      │                        │
   ROUTE_ADD/DEL)       │  └──────────────┘                      │                        │
                        │                                        │                        │
  Supplementary ───────►│  ┌──────────────┐                      │                        │
  ethtool stats poll    │  │ ethtool coll │──────────────────────►│                        │
  vtysh CLI (OSPF/RIP)  │  │ vtysh coll   │──────────────────────►│                        │
  wgctrl WireGuard      │  │ wgctrl coll  │──────────────────────►│                        │
  /proc/sys polling     │  └──────────────┘                      │                        │
                        │             IPC server ◄───────────────┘                        │
                        │          /run/yangerd.sock                                      │
                        │          SOCK_STREAM, ver(1) + 4-byte BE length + JSON        │
                        └─────────────────────────────────────────────────────────────────┘
                                          ▲
                                          │  yangerd_query(path)
                                          │  Unix socket read/write
                        ┌─────────────────┴───────────────────────────────────┐
                        │                    statd (C daemon)                  │
                        │                                                     │
                        │  13 x sr_oper_get_subscribe()                       │
                        │  on callback:                                       │
                        │    ly_add_yangerd_data()                            │
                        │      -> yangerd_query(path)      -- primary path     │
                        │                                                     │
                        │    lyd_parse_data_mem(ctx, buf)                     │
                        └──────────────────────────────────┬──────────────────┘
                                                           │
                                          ┌────────────────▼────────────────┐
                                          │         sysrepo / libyang        │
                                          │   operational datastore          │
                                          │   NETCONF / RESTCONF consumers   │
                                          └─────────────────────────────────┘
```

### 3.2 Data Flow Diagrams

#### 3.2.1 Netlink Reactive Path (Event to Tree)
```
Kernel          NLMonitor        event dispatcher   ip batch         ethmonitor       Tree
  |                   |                |               |              |              |
  |--RTM_NEWLINK----->|                |               |              |              |
  |                   |---LinkUpdate-->|               |              |              |
  |                   |                |               |              |              |
  |                   |                |  === Full Interface Re-read (3 queries) ===|
  |                   |                |---link show--->|              |              |
  |                   |                |<--JSON resp----|              |              |
  |                   |                |--s link show-->|              |              |
  |                   |                |<--JSON resp----|              |              |
  |                   |                |---addr show--->|              |              |
  |                   |                |<--JSON resp----|              |              |
  |                   |                |               |              |              |
  |                   |                |---tree.Set(link+stats+addr)-------------->|
  |                   |                |               |              |              |
  |                   |                |  === Cross-subsystem ethtool re-query === |
  |                   |                |---RefreshInterface()------->|              |
  |                   |                |               |    etClient.LinkInfo()     |
  |                   |                |               |    etClient.LinkMode()     |
  |                   |                |               |---tree.Set(ethernet)------>|
  |                   |                |               |              |              |

Note: Netlink event types link, addr, and neigh use the event as a trigger to re-read
full current state via ip batch. Route data is sourced exclusively from the ZAPI
watcher's streaming connection to zebra (see Section 4.1octies) -- yangerd does not
subscribe to netlink route groups. For link events specifically, the re-read is a
3-query set plus ethtool cross-trigger.
```

#### 3.2.2 Statd Query Path (Request to Response)
```
statd (sysrepo cb)      yangerd IPC Server      In-Memory Tree
        |                       |                      |
        |---Length + JSON Req-->|                      |
        |                       |---tree.Get(path)---->|
        |                       |   (Read Lock)        |
        |                       |<-------JSON Blob-----|
        |<--Length + JSON Resp--|                      |
```


#### 3.2.4 File Watcher Reactive Path (File Change to Tree)
```
Kernel          fsnotify         fswatcher        os.ReadFile      Tree
  |                   |                |               |              |
  |--inotify event--->|                |               |              |
  |                   |---IN_MODIFY--->|               |              |
  |                   |                |--debounce 200ms              |
  |                   |                |---read file-->|              |
  |                   |                |               |---file data->|
  |                   |                |---tree.Set()---------------->|
  |                   |                |               |              |
```

Note: The fswatcher monitors procfs forwarding flags (`/proc/sys/net/ipv4/conf/*/forwarding`) via inotify. DHCP lease file watching has been moved to the D-Bus Monitor Subsystem (Section 3.2.8), which reacts to dnsmasq D-Bus signals instead of inotify file events.

#### 3.2.5 Bridge Monitor Reactive Path (Bridge Event to Tree)
```

Kernel        bridge netlink   bridge dispatcher  bridge batch      Tree
  |                   |                |               |              |
  |--RTNL bridge evt->|                |               |              |
  |                   |---NL event---->|               |              |
  |                   |                |---query cmd-->|              |
  |                   |                |               |---JSON resp->|
  |                   |                |---tree.Set()---------------->|
  |                   |                |               |              |
```

#### 3.2.6 IW Event Reactive Path (802.11 Event to Tree)
```
Kernel        iw event -t      wifi dispatcher   exec iw dev      Tree
  |                   |                |               |              |
  |--nl80211 event--->|                |               |              |
  |                   |---text line--->|               |              |
  |                   |                |---parse line   |              |
  |                   |                |---exec iw---->|              |
  |                   |                |               |---text out-->|
  |                   |                |---parse + tree.Set()-------->|
  |                   |                |               |              |
```

Note: Unlike the core netlink subscriptions (which receive typed Go structs via `vishvananda/netlink` channels) and bridge netlink events, `iw event` produces human-readable text, not structured data. The wifi dispatcher must parse each text line to extract the event type and interface name, then run `iw dev <ifname> info` / `iw dev <ifname> station dump` and parse their text output into structured JSON for tree storage. This adds a text-parsing layer absent from the netlink reactive paths.

#### 3.2.7 Ethtool Netlink Reactive Path (Settings Change to Tree)
```
Kernel        genetlink        ethmonitor       ethtool.Client    Tree
  |                   |                |               |              |
  |--ETHTOOL_MSG_*_NTF>|                |               |              |
  |                   |---NTF message->|               |              |
  |                   |                |---parse cmd   |              |
  |                   |                |---LinkInfo()--->|              |
  |                   |                |               |---JSON resp->|
  |                   |                |---tree.Set()---------------->|
  |                   |                |               |              |
```

#### 3.2.8 D-Bus Monitor Reactive Path (D-Bus Signal to Tree)
```
D-Bus Daemon    godbus/dbus/v5   dbusmonitor       re-read          Tree
  |                   |                |               |              |
  |--D-Bus signal---->|                |               |              |
  |                   |---signal msg-->|               |              |
  |                   |                |---dispatch     |              |
  |                   |                |---re-read---->|              |
  |                   |                |               |---data------>|
  |                   |                |---tree.Set()---------------->|
  |                   |                |               |              |
```

Note: The D-Bus Monitor subscribes to signals from dnsmasq (`uk.org.thekelleys.dnsmasq`, signals `DHCPLeaseAdded`, `DHCPLeaseDeleted`, `DHCPLeaseUpdated`) and firewalld (`org.fedoraproject.FirewallD1`, signal `Reloaded`; plus `org.freedesktop.DBus.NameOwnerChanged` for service restart detection). On dnsmasq signals, the monitor re-reads `/var/lib/misc/dnsmasq.leases` and calls `GetMetrics()` via D-Bus method call. On firewalld signals, the monitor re-reads firewall state via firewalld D-Bus method calls (`getDefaultZone()`, `getActiveZones()`, `getZoneSettings2()`, `getPolicies()`, `getPolicySettings()`, `listServices()`, `getServiceSettings2()`, `getLogDenied()`, `queryPanicMode()`). This follows the same event-as-trigger pattern used by the netlink and bridge subsystems.

Note: Unlike the ip/bridge/iw subsystems, the ethtool netlink monitor is NOT a subprocess. It is a native Go genetlink socket subscription using `mdlayher/genetlink`. The `EthMonitor` goroutine joins the `"monitor"` multicast group of the `"ethtool"` genetlink family and receives `_NTF` notification messages directly. On receiving `ETHTOOL_MSG_LINKMODES_NTF` or `ETHTOOL_MSG_LINKINFO_NTF`, it re-queries the affected interface via `ethtool.Client.LinkInfo()` and `ethtool.Client.LinkMode()` to obtain updated speed, duplex, and auto-negotiation state. Statistics (counters) have no `_NTF` message type and remain polling-based via the ethtool collector. Importantly, `ETHNL_MCGRP_MONITOR` does **NOT** fire on link up/down events — only on explicit settings renegotiation. To close this gap, the link event handler calls `ethmonitor.RefreshInterface()` on every RTM_NEWLINK, triggering an ethtool re-query for the affected interface.

### 3.3 Component Responsibilities

| Component | Responsibility |
|-----------|----------------|
| **NLMonitor** (netlink event subscriptions) | Native Go netlink subscriptions via `vishvananda/netlink` (`LinkSubscribeWithOptions`, `AddrSubscribeWithOptions`, `NeighSubscribeWithOptions`) for kernel events; persistent `ip -json -force -batch -` subprocess for state queries; event dispatch and tree updates. Link, addr, and neigh events trigger full re-reads of the affected state via ip batch. Route events are **not** handled by NLMonitor -- route data is sourced exclusively from the ZAPI watcher (Section 4.1octies). For link events: 3-query full interface re-read via ip batch + `ethmonitor.RefreshInterface()`. For addr/neigh: single-query re-read of the affected subtree via ip batch. |
| **File Watcher Subsystem** | Watches selected procfs paths (IP forwarding flags) via Linux inotify; triggers re-read and tree update on file change; falls back to polling for pseudo-filesystem paths that do not support inotify. Note: sysfs sensor files (`/sys/class/hwmon`, `/sys/class/thermal`) do NOT emit inotify events and are handled by the hardware polling collector instead. STP bridge port state is handled reactively via netlink events (Bridge Monitor Subsystem), not via fswatcher. DHCP lease file watching is handled by the D-Bus Monitor Subsystem, not via fswatcher. |
| **Bridge Netlink / Bridge Batch Subsystem** | Netlink events trigger full bridge state re-reads via the persistent `bridge -json -batch -` subprocess. FDB entries arrive via `NeighSubscribeWithOptions` (entries with `NDA_MASTER` flag); VLAN and STP port state changes arrive via `LinkSubscribeWithOptions`; MDB events arrive via raw netlink subscription to `RTNLGRP_MDB` (group 26). Each event triggers the appropriate `bridge -json -batch -` query (`fdb show`, `vlan show`, `mdb show`) to re-read full state. STP root/topology data is re-queried whenever a port state change event arrives. |
| **IW Event Monitor Subsystem** | Persistent `iw event -t` subprocess for 802.11 wireless events (station associations, disconnections, channel switches, regulatory changes); triggers re-query of WiFi state via short-lived `iw dev <iface> info` and `iw dev <iface> station dump` commands. Enabled only when `YANGERD_ENABLE_WIFI=true` (set by Buildroot when WiFi support is included in the build). When enabled, the `iw` binary is guaranteed present on the target. |
| **Ethtool Netlink Monitor Subsystem** | Native Go genetlink subscription to the `"ethtool"` family's `"monitor"` multicast group (`ETHNL_MCGRP_MONITOR`); receives `ETHTOOL_MSG_LINKMODES_NTF` and `ETHTOOL_MSG_LINKINFO_NTF` notifications for speed, duplex, and auto-negotiation changes; re-queries via `ethtool.Client.LinkInfo()` + `ethtool.Client.LinkMode()`. Also exposes `RefreshInterface()` for cross-subsystem use by the link event handler (RTM_NEWLINK), since `ETHNL_MCGRP_MONITOR` does NOT fire on link up/down. Not a subprocess — runs as a goroutine with a genetlink socket. |
| **ZAPI Watcher Subsystem** | Persistent streaming connection to FRRouting's zebra daemon via the zserv Unix socket (`/var/run/frr/zserv.api`), using ZAPI protocol v6. Subscribes to route redistribution for kernel, connected, static, OSPF, and RIP route types. Receives incremental `REDISTRIBUTE_ROUTE_ADD` and `REDISTRIBUTE_ROUTE_DEL` messages and updates the in-memory tree. Handles zebra restarts with automatic reconnection and exponential backoff. **Sole source** for route table data -- replaces `vtysh` for route collection. See Section 4.1octies. |
| **D-Bus Monitor Subsystem** | Subscribes to D-Bus signals from dnsmasq and firewalld via `godbus/dbus/v5`. dnsmasq signals (`DHCPLeaseAdded`, `DHCPLeaseDeleted`, `DHCPLeaseUpdated`) trigger re-read of the lease file (`/var/lib/misc/dnsmasq.leases`) and a `GetMetrics()` D-Bus method call for DHCP packet counters. firewalld signals (`Reloaded`, plus `NameOwnerChanged` for restart detection) trigger re-read of firewall state via firewalld D-Bus method calls (`getDefaultZone()`, `getActiveZones()`, `getZoneSettings2()`, `getPolicies()`, `getPolicySettings()`, `listServices()`, `getServiceSettings2()`, `getLogDenied()`, `queryPanicMode()`). Follows the event-as-trigger pattern: D-Bus signals provide the notification, actual data is re-read from the canonical source (firewalld D-Bus API). See Section 4.1novies. |
| **LLDP Monitor Subsystem** | Persistent `lldpcli -f json0 watch` subprocess for reactive LLDP neighbor discovery. Receives pretty-printed JSON objects (blank-line delimited) with root keys `lldp-added`, `lldp-updated`, `lldp-deleted` — each carrying the full neighbor payload. Parses the stream using brace-depth counting, extracts neighbor data, and replaces the `ieee802-dot1ab-lldp:lldp` subtree in the in-memory tree. Uses lldpd's own CLI rather than reimplementing LLDP — lldpd is the system's LLDP authority. See Section 4.1decies. |
| **mDNS Monitor Subsystem** | Subscribes to Avahi's D-Bus API (`org.freedesktop.Avahi`) for reactive mDNS/DNS-SD neighbor discovery. Uses the existing `godbus/dbus/v5` dependency to interact with Avahi's `ServiceTypeBrowser`, `ServiceBrowser`, and `ServiceResolver` objects. Updates `/infix-services:mdns/neighbors` on add/update/remove signals. Uses Avahi rather than a standalone mDNS library — Avahi is already running on the system and is the canonical mDNS authority. See Section 4.1undecies. |
| **Collectors** | Poll external sources (vtysh for OSPF/RIP/BFD protocol data, chrony cmdmon for NTP, `podman` CLI for container state, sysfs for hardware sensors) at configured intervals. Container collection invokes `podman ps`, `podman inspect`, and `podman stats` for runtime data; lifecycle events (start/stop/create/remove) are candidates for reactive monitoring via `podman events --format json` in Phase 2. Route table collection is **not** performed by collectors -- it is handled by the ZAPI watcher. DHCP and firewall data are handled reactively by the D-Bus Monitor Subsystem, not by polling collectors. |
| **In-Memory Tree** | Thread-safe storage of pre-serialized YANG JSON subtrees. |
| **IPC Server** | Handle Unix socket requests from `statd` and `yangerctl`. |
| **statd (C)** | Bridge sysrepo to `yangerd`. |
| **yangerctl** | CLI interface for monitoring and debugging `yangerd`. |

---

## 4. Detailed Design

### 4.1 Netlink Monitor Subsystem

`yangerd` manages THREE persistent subprocesses plus two native netlink subsystems:
1. `ip -json -batch -` -- data queries (yangerd writes commands to stdin, reads JSON arrays from stdout)
2. `bridge -json -batch -` -- bridge state queries (see [4.1quater](#41quater-bridge-monitor-subsystem))
3. `iw event -t` -- 802.11 wireless event notification (see [4.1quinquies](#41quinquies-iw-event-monitor-subsystem))
4. **Netlink event subscriptions** -- native Go netlink subscriptions via `vishvananda/netlink` (`LinkSubscribeWithOptions`, `AddrSubscribeWithOptions`, `NeighSubscribeWithOptions`) for link, address, and neighbor events. Bridge FDB events arrive via `NeighSubscribeWithOptions` (FDB entries are neighbor-like with `NDA_MASTER`); bridge VLAN changes via `LinkSubscribeWithOptions`; bridge MDB via raw netlink subscription to `RTNLGRP_MDB`. These are not subprocesses -- they are goroutines reading from typed Go channels. yangerd does NOT subscribe to netlink route groups (`RTNLGRP_IPV4_ROUTE`, `RTNLGRP_IPV6_ROUTE`) -- route data is sourced exclusively from the ZAPI watcher (Section 4.1octies).
5. **Ethtool genetlink monitor** -- native Go genetlink socket subscription to `ETHNL_MCGRP_MONITOR` for ethtool settings change notifications (see [4.1sexies](#41sexies-ethtool-netlink-monitor-subsystem)). This is not a subprocess -- it is a goroutine that opens a genetlink socket, joins the `"monitor"` multicast group, and calls `Receive()` in a loop.

When a netlink event arrives on any subscription channel (e.g., `LinkUpdate` for a state change on eth0), the NLMonitor goroutine:
1. Extracts the affected entity from the typed Go struct (interface name from `update.Link.Attrs().Name`, address from `update.LinkAddress`, etc.)
2. **For link events (RTM_NEWLINK/RTM_DELLINK)**: writes a **full interface re-read** set of commands to `ip batch` stdin -- `link show dev eth0\n`, `-s link show dev eth0\n` (with stats), and `addr show dev eth0\n` -- to obtain the entire interface state atomically. This ensures all interface data in the tree is coherent at a single point in time. Additionally, the dispatcher calls `ethmonitor.RefreshInterface("eth0")` to re-query ethtool settings (speed/duplex/autoneg), because the ethtool genetlink monitor (`ETHNL_MCGRP_MONITOR`) does NOT emit notifications on link up/down events. On RTM_DELLINK, the re-read returns empty/error, causing the interface subtree to be removed from the tree.
3. **For address events (RTM_NEWADDR/RTM_DELADDR)**: writes `addr show dev <iface>` to `ip batch` stdin and replaces the entire address subtree for that interface. Both add and remove events trigger the same re-read -- the result after a delete simply omits the removed address.
4. **For neighbor events (RTM_NEWNEIGH/RTM_DELNEIGH)**: writes `neigh show dev <iface>` to `ip batch` stdin and replaces the neighbor subtree for that interface.
5. Reads the JSON array response(s) from `ip batch` stdout (one `[...]` per command, one per line)
6. Calls `tree.Set()` with the parsed JSON to replace the affected subtree

#### 4.1.2 Netlink Subscription Event Channels
- **Link events**: `netlink.LinkSubscribeWithOptions(linkCh, ctx.Done(), LinkSubscribeOptions{ErrorCallback: errCb})` -- receives `LinkUpdate` structs containing `Link.Attrs().Name`, `Link.Attrs().OperState`, etc.
- **Address events**: `netlink.AddrSubscribeWithOptions(addrCh, ctx.Done(), AddrSubscribeOptions{ErrorCallback: errCb})` -- receives `AddrUpdate` structs containing `LinkAddress`, `LinkIndex`, `NewAddr` bool
- **Neighbor events**: `netlink.NeighSubscribeWithOptions(neighCh, ctx.Done(), NeighSubscribeOptions{ErrorCallback: errCb})` -- receives `NeighUpdate` structs containing `Neigh`, event `Type` (RTM_NEWNEIGH/RTM_DELNEIGH)
- **Bridge FDB events**: arrive on the neighbor channel (`NeighSubscribeWithOptions`) -- FDB entries have `NDA_MASTER` flag and are distinguishable from ARP/NDP neighbors. Used as trigger only; full FDB state re-read via `bridge -json -batch -`.
- **Bridge VLAN events**: arrive on the link channel (`LinkSubscribeWithOptions`) -- VLAN attributes are carried on link update messages. Used as trigger only; full VLAN state re-read via `bridge -json -batch -`.
- **Bridge MDB events**: require raw netlink socket subscribed to `RTNLGRP_MDB` (multicast group 26) -- `vishvananda/netlink` does not expose a dedicated MDB subscription API. Used as trigger only; full MDB state re-read via `bridge -json -batch -`.
- **Bridge STP events**: STP port state changes arrive as `RTM_NEWLINK` events carrying `IFLA_BRPORT_STATE` in `IFLA_PROTINFO` on the link channel. The link event handler detects bridge port events and triggers a bridge batch re-query for STP state. STP root/topology data is not proactively notified by the kernel, so it is re-read from the bridge device via batch whenever a port state change is detected.
- Events include: link up/down, address add/remove, neighbor add/remove, bridge FDB/VLAN/MDB/STP changes
- **Event-as-trigger pattern**: All netlink events -- link, addr, neigh, and bridge (FDB, VLAN, MDB, STP) -- use the event only to identify WHAT entity changed, then re-read the FULL current state via `ip batch` or `bridge batch`. Route data is sourced exclusively from the ZAPI watcher's streaming connection to zebra (Section 4.1octies). The event content itself is not parsed for data. This applies equally to RTM_NEW* and RTM_DEL* events; a delete event triggers a re-read that returns the state without the deleted entity.
- **Full interface re-read on RTM_NEWLINK/RTM_DELLINK**: When a link event arrives on `linkCh`, the NLMonitor writes a full set of three queries to ip batch (`link show dev <iface>`, `-s link show dev <iface>`, `addr show dev <iface>`) and updates the entire YANG subtree for that interface. This ensures consistency -- all interface data (flags, MTU, operstate, statistics, addresses) is captured at a single coherent point in time. On RTM_DELLINK, the re-read returns empty/error, causing the interface subtree to be removed.
- **Full address re-read on RTM_NEWADDR/RTM_DELADDR**: When an address event arrives on `addrCh`, the NLMonitor writes `addr show dev <iface>` to ip batch and replaces the entire address subtree for that interface. Both add and remove produce the same re-read; after a remove, the result simply omits the deleted address.
- **Full neighbor re-read on RTM_NEWNEIGH/RTM_DELNEIGH**: When a neighbor event arrives on `neighCh`, the NLMonitor writes `neigh show dev <iface>` to ip batch and replaces the neighbor subtree for that interface.
- **Cross-subsystem ethtool trigger**: RTM_NEWLINK link events also trigger `ethmonitor.RefreshInterface(<iface>)` to re-query ethtool data (speed, duplex, auto-negotiation). This is necessary because `ETHNL_MCGRP_MONITOR` does NOT fire notifications when a link goes up/down -- only when settings are explicitly renegotiated.

#### 4.1.3 ip batch Query Engine
- Started as: `ip -json -batch -` (reads commands from stdin, `-` means stdin)
- Flag order matters: `-json` MUST come before `-batch`
- Each command written to stdin produces one JSON array `[...]` on stdout
- Use `-force` flag (`ip -json -force -batch -`) so errors don't abort the batch process — it continues past failed commands
- Error output goes to stderr (e.g., "Device does not exist"); JSON goes to stdout — clean separation
- Example commands written to stdin: `link show dev eth0`, `-s link show dev eth0` (with stats), `addr show dev eth0`, `neigh show`
- **Full interface re-read set** (written atomically on RTM_NEWLINK for interface eth0):
  ```
  link show dev eth0
  -s link show dev eth0
  addr show dev eth0
  ```
  This produces three JSON array responses on stdout (one per line). The first gives link state (flags, MTU, operstate, qdisc, etc.), the second adds hardware counters (rx/tx bytes/packets/errors/dropped), and the third gives all IPv4/IPv6 addresses. Together they provide the complete interface snapshot needed to update the entire YANG subtree.
- **Address re-read** (written on RTM_NEWADDR or RTM_DELADDR for interface eth0): `addr show dev eth0` — single query, single JSON response replacing the full address subtree.
- **Route data**: Route data is NOT queried via `ip batch`. Route data is sourced exclusively from the ZAPI watcher's streaming connection to zebra's zserv socket (see Section 4.1octies). yangerd does not subscribe to netlink route groups.
- **Neighbor re-read** (written on RTM_NEWNEIGH or RTM_DELNEIGH for interface eth0): `neigh show dev eth0` — single query, single JSON response replacing the neighbor subtree.
- For bridge data: a separate `bridge -json -batch -` subprocess (see [4.1quater](#41quater-bridge-monitor-subsystem))

#### 4.1.4 Initial State Dump
- On startup, before subscribing to netlink events, `yangerd` populates the tree from three sources:
  - **ip batch** (link, address, neighbor data):
    - `link show` (all links)
    - `-s link show` (all links with stats)
    - `addr show` (all addresses)
    - `neigh show` (all neighbors)
  - **ZAPI watcher** (routing table — zebra is the authoritative source for all route types):
    - Streaming connection to `/var/run/frr/zserv.api` via ZAPI v6
    - `ZEBRA_REDISTRIBUTE_ADD` per route type triggers full RIB dump from zebra
    - Receives `REDISTRIBUTE_ROUTE_ADD` / `REDISTRIBUTE_ROUTE_DEL` messages incrementally
  - **fswatcher** (procfs forwarding flags):
    - After glob expansion and inotify watch setup, calls `InitialRead()` to read the current value of every watched file (see Section 4.1ter)
    - `/proc/sys/net/ipv4/conf/*/forwarding` for all existing interfaces
    - Completes sub-millisecond (procfs reads are kernel-generated)
- This populates the tree before any events arrive

#### 4.1.5 Subprocess and Socket Lifecycle
- The `ip batch` and `bridge batch` subprocesses are started in `yangerd`'s `main()` and run for the daemon's lifetime
- Netlink subscription channels are created and subscriptions established before the initial state dump (subscribe-first-then-list pattern, following Antrea's approach)
- If a netlink subscription channel closes (indicating kernel-side error), `yangerd` re-establishes all subscriptions (following OVN-Kubernetes' re-subscribe-on-close pattern)
- If either batch subprocess exits unexpectedly, `yangerd` restarts it with exponential backoff (100ms to 30s)
- On daemon shutdown (SIGTERM/SIGINT), `ctx.Done()` closes all netlink subscriptions, and stdin pipes are closed for batch subprocesses
- The shared `ErrorCallback` logs warnings and triggers context cancellation, following the Cilium/Docker production pattern

#### 4.1.6 Concurrency Model
- One goroutine runs the NLMonitor select loop, reading from three netlink subscription channels (`linkCh`, `addrCh`, `neighCh`) plus bridge MDB raw netlink channel
- One goroutine reads `ip batch` stdout (response reader)
- One goroutine writes to `ip batch` stdin (query writer, serialized via channel)
- The query writer and response reader coordinate via a request/response queue (channel of pending queries)

### 4.1bis ip batch Subprocess Manager

`yangerd` uses a dedicated manager to interact with the persistent `ip batch` process. This manager handles the stdin/stdout pipes and ensures that queries are serialized and paired with their responses.

#### IPBatch Manager Implementation

The following Go code demonstrates the core logic for the `IPBatch` manager:

```go
type IPBatch struct {
    cmd    *exec.Cmd
    stdin  io.WriteCloser
    stdout *bufio.Scanner
    stderr io.ReadCloser
    mu     sync.Mutex // serializes queries
    log    *slog.Logger
}

func NewIPBatch(ctx context.Context, log *slog.Logger) (*IPBatch, error) {
    cmd := exec.CommandContext(ctx, "ip", "-json", "-force", "-batch", "-")
    stdin, err := cmd.StdinPipe()
    if err != nil {
        return nil, fmt.Errorf("stdin pipe: %w", err)
    }
    stdout, err := cmd.StdoutPipe()
    if err != nil {
        return nil, fmt.Errorf("stdout pipe: %w", err)
    }
    stderr, err := cmd.StderrPipe()
    if err != nil {
        return nil, fmt.Errorf("stderr pipe: %w", err)
    }
    if err := cmd.Start(); err != nil {
        return nil, fmt.Errorf("start ip batch: %w", err)
    }
    b := &IPBatch{
        cmd:    cmd,
        stdin:  stdin,
        stdout: bufio.NewScanner(stdout),
        stderr: stderr,
        log:    log,
    }
    go b.drainStderr()
    return b, nil
}

// Query sends a command to the ip batch process and returns the JSON response.
// Commands are newline-terminated (e.g., "link show dev eth0\n").
// Each command produces exactly one line of JSON array output on stdout.
func (b *IPBatch) Query(command string) (json.RawMessage, error) {
    b.mu.Lock()
    defer b.mu.Unlock()
    
    if _, err := fmt.Fprintf(b.stdin, "%s\n", command); err != nil {
        return nil, fmt.Errorf("write command: %w", err)
    }
    if !b.stdout.Scan() {
        if err := b.stdout.Err(); err != nil {
            return nil, fmt.Errorf("read response: %w", err)
        }
        return nil, fmt.Errorf("ip batch process exited")
    }
    return json.RawMessage(b.stdout.Bytes()), nil
}

func (b *IPBatch) drainStderr() {
    scanner := bufio.NewScanner(b.stderr)
    for scanner.Scan() {
        b.log.Warn("ip batch stderr", "line", scanner.Text())
    }
}
```
#### IPBatch Error Handling and Restart

The `IPBatch` manager detects subprocess death via pipe EOF: when the `ip batch` process exits, `b.stdout.Scan()` returns `false` and `fmt.Fprintf(b.stdin, ...)` returns a broken-pipe error. Either condition causes `Query()` to return an error immediately to the caller. There is no per-query timeout — pipe EOF detection is instantaneous.

**Restart coordination:**

1. On subprocess death, the `IPBatch` manager transitions to a `dead` state. All subsequent `Query()` calls return `ErrBatchDead` immediately without acquiring the mutex.
2. A dedicated restart goroutine respawns the subprocess with exponential backoff (100ms initial, 30s max, factor 2x).
3. After a successful restart, a canary query (`link show lo`) validates the new process. Only on canary success does the manager transition back to `alive`, allowing `Query()` calls to proceed.
4. During the restart window, callers (monitor goroutines) receive `ErrBatchDead` and simply skip the current event. The next netlink event will retry the query against the restored subprocess. No event data is lost — the event-as-trigger pattern means the next event triggers a full re-read that captures all accumulated state changes. Note: `ErrBatchDead` is a transient sentinel error that must be handled by the monitor's `select` loop, not propagated as a fatal daemon error.

**Terminology mapping**: The `dead`/`alive` states used internally by `IPBatch` and `BridgeBatch` map to the health API states as follows: `alive` → `"running"`, `dead` (during restart with backoff) → `"restarting"`, `dead` (after max retries exhausted) → `"failed"`. See Section 4.3.5 for the health response schema.


The `BridgeBatch` manager follows the identical error handling and restart protocol, using `vlan show` as its canary query.

Specifically, on `bridge -json -batch -` subprocess death, `BridgeBatch.Query()` returns `ErrBatchDead` immediately to all callers. A restart goroutine respawns the subprocess with the same exponential backoff parameters (100ms initial, 30s max, factor 2x). After a successful restart, a `vlan show` canary query validates the new process before transitioning back to the alive state. During the restart window, bridge event handlers that receive `ErrBatchDead` skip the current re-query; the next netlink event triggers a full re-read against the restored subprocess.


#### NLMonitor Event Loop

The NLMonitor subscribes to netlink events via `vishvananda/netlink` channels and triggers full-state queries via the `IPBatch` manager:

```go
// NLMonitor tracks per-interface oper-status for last-change timestamps.
// The lastOperStatus map records the most recent operstate string per interface;
// when a LinkUpdate arrives with a different oper-status, time.Now() is
// recorded as the interface's last-change timestamp.
//
// On ANY LinkUpdate, the monitor performs a full interface re-read:
// three ip batch queries (link show, -s link show, addr show) to capture the
// complete interface state atomically, plus an ethtool re-query via
// ethmonitor.RefreshInterface() since ETHNL_MCGRP_MONITOR does NOT fire on
// link up/down events.
type NLMonitor struct {
    batch           *IPBatch
    brBatch         *BridgeBatch
    tree            *tree.Tree
    ethMon          *ethmonitor.EthMonitor  // for cross-subsystem ethtool re-query
    log             *slog.Logger
    lastOperStatus  map[string]string  // iface -> "UP"/"DOWN"/"DORMANT"/...
}

func (m *NLMonitor) Run(ctx context.Context) error {
    ctx, cancel := context.WithCancel(ctx)
    defer cancel()

    // Shared error callback (Cilium/Docker production pattern).
    // Any netlink socket error triggers context cancellation, which
    // closes all subscription channels and allows the supervisor to
    // re-establish subscriptions.
    errorCallback := func(err error) {
        m.log.Warn("netlink subscription error, restarting", "err", err)
        cancel()
    }

    // Subscribe to all three netlink event types.
    // Subscribe BEFORE initial dump (Antrea subscribe-first-then-list pattern)
    // to ensure no events are missed between dump and subscription.
    linkCh := make(chan netlink.LinkUpdate)
    if err := netlink.LinkSubscribeWithOptions(linkCh, ctx.Done(), netlink.LinkSubscribeOptions{
        ErrorCallback: errorCallback,
    }); err != nil {
        return fmt.Errorf("link subscribe: %w", err)
    }

    addrCh := make(chan netlink.AddrUpdate)
    if err := netlink.AddrSubscribeWithOptions(addrCh, ctx.Done(), netlink.AddrSubscribeOptions{
        ErrorCallback: errorCallback,
    }); err != nil {
        return fmt.Errorf("addr subscribe: %w", err)
    }

    neighCh := make(chan netlink.NeighUpdate)
    if err := netlink.NeighSubscribeWithOptions(neighCh, ctx.Done(), netlink.NeighSubscribeOptions{
        ErrorCallback: errorCallback,
    }); err != nil {
        return fmt.Errorf("neigh subscribe: %w", err)
    }

    // Initial state dump AFTER subscribe (subscribe-first-then-list pattern).
    // Any events that arrive during the dump are queued in the channels
    // and will be processed once we enter the select loop.
    m.initialDump(ctx)

    // Main event loop: select across all subscription channels.
    for {
        select {
        case u, ok := <-linkCh:
            if !ok {
                // Channel closed — netlink socket error (OVN-K re-subscribe pattern).
                return fmt.Errorf("link subscription channel closed")
            }
            iface := u.Link.Attrs().Name
            if iface == "" {
                continue
            }

            // === FULL INTERFACE RE-READ ===
            // On ANY LinkUpdate, re-read the ENTIRE interface to ensure
            // all data in the tree is coherent at a single point in time.
            // Three queries: link state, link stats, addresses.
            ifPath := fmt.Sprintf("/ietf-interfaces:interfaces/interface[name='%s']", iface)

            linkData, err := m.batch.Query(fmt.Sprintf("link show dev %s", iface))
            if err != nil {
                m.log.Warn("batch link query failed", "iface", iface, "err", err)
                continue
            }
            m.tree.Set(ifPath, linkData)

            statsData, err := m.batch.Query(fmt.Sprintf("-s link show dev %s", iface))
            if err != nil {
                m.log.Warn("batch stats query failed", "iface", iface, "err", err)
                // Non-fatal: link data already written, stats are supplementary
            } else {
                m.tree.Set(ifPath+"/statistics", statsData)
            }

            addrData, err := m.batch.Query(fmt.Sprintf("addr show dev %s", iface))
            if err != nil {
                m.log.Warn("batch addr query failed", "iface", iface, "err", err)
            } else {
                m.tree.Set(ifPath+"/addresses", addrData)
            }

            // === CROSS-SUBSYSTEM ETHTOOL RE-QUERY ===
            // ETHNL_MCGRP_MONITOR does NOT fire on link up/down -- only on
            // explicit settings renegotiation. When a link goes up, the kernel
            // negotiates speed/duplex/autoneg but the ethtool genetlink monitor
            // is silent. We must explicitly re-query ethtool here.
            if m.ethMon != nil {
                m.ethMon.RefreshInterface(iface)
            }

            // Track oper-status transitions for last-change (RFC 7223 sec 2.2).
            // Since we receive every LinkUpdate, recording time.Now() at
            // the moment of oper-status change gives last-change for free.
            newStatus := extractOperStatus(linkData)
            if oldStatus, ok := m.lastOperStatus[iface]; !ok || oldStatus != newStatus {
                m.lastOperStatus[iface] = newStatus
                ts := time.Now().Format(time.RFC3339)
                m.tree.Set(
                    ifPath+"/last-change",
                    json.RawMessage(fmt.Sprintf("%q", ts)),
                )
                m.log.Info("oper-status changed", "iface", iface,
                    "old", oldStatus, "new", newStatus, "last-change", ts)
            }

        case u, ok := <-addrCh:
            if !ok {
                return fmt.Errorf("addr subscription channel closed")
            }
            // === FULL ADDRESS RE-READ ===
            // On ANY AddrUpdate (new or removed), re-read all addresses
            // for this interface. The event is just a trigger -- we don't parse its
            // content. After a delete, the re-read result simply omits the removed address.
            link, err := netlink.LinkByIndex(u.LinkIndex)
            if err != nil {
                m.log.Warn("resolve link index", "index", u.LinkIndex, "err", err)
                continue
            }
            iface := link.Attrs().Name
            ifPath := fmt.Sprintf("/ietf-interfaces:interfaces/interface[name='%s']", iface)
            addrData, err := m.batch.Query(fmt.Sprintf("addr show dev %s", iface))
            if err != nil {
                m.log.Warn("batch addr query failed", "iface", iface, "err", err)
                continue
            }
            m.tree.Set(ifPath+"/addresses", addrData)

        case u, ok := <-neighCh:
            if !ok {
                return fmt.Errorf("neigh subscription channel closed")
            }
            // === FULL NEIGHBOR RE-READ ===
            // On ANY NeighUpdate (new or deleted), re-read all neighbors
            // for this interface.
            iface := ""
            if link, err := netlink.LinkByIndex(u.LinkIndex); err == nil {
                iface = link.Attrs().Name
            }
            if iface == "" {
                continue
            }
            ifPath := fmt.Sprintf("/ietf-interfaces:interfaces/interface[name='%s']", iface)
            neighData, err := m.batch.Query(fmt.Sprintf("neigh show dev %s", iface))
            if err != nil {
                m.log.Warn("batch neigh query failed", "iface", iface, "err", err)
                continue
            }
            m.tree.Set(ifPath+"/neighbors", neighData)

        case <-ctx.Done():
            return ctx.Err()
        }
    }
}

// extractOperStatus pulls the operstate string from ip -json link output.
// Returns "UP", "DOWN", "DORMANT", "LOWERLAYERDOWN", etc.
func extractOperStatus(data json.RawMessage) string {
    var links []struct {
        OperState string `json:"operstate"`
    }
    if err := json.Unmarshal(data, &links); err != nil || len(links) == 0 {
        return ""
    }
    return links[0].OperState
}
```


### 4.1ter File Watcher Subsystem

The File Watcher Subsystem provides reactive monitoring of filesystem-based data sources, replacing traditional polling for files in `procfs` that support inotify. By leveraging the Linux `inotify` mechanism, `yangerd` can detect and process updates to IP forwarding flags immediately upon their modification, significantly reducing latency and CPU wake-ups for data that changes infrequently. Note: sysfs pseudo-files (hwmon sensors, thermal zones) do not support inotify and are handled by the polling-based hardware collector instead -- see the note after the Watched Paths table below. STP bridge port state is not watched via inotify either; it is handled reactively via netlink events (see Section 4.1quater). DHCP lease updates and firewall configuration changes are handled reactively via D-Bus signals (see Section 4.1novies).

#### FSWatcher Implementation

The following Go code defines the `FSWatcher` type and its core event loop in `internal/fswatcher/fswatcher.go`:

```go
type FSWatcher struct {
    watcher  *fsnotify.Watcher
    tree     *tree.Tree
    handlers map[string]WatchHandler // path -> handler
    debounce map[string]*time.Timer  // path -> debounce timer
    mu       sync.Mutex
    log      *slog.Logger
}

type WatchHandler struct {
    TreeKey  string                                    // YANG tree key to update
    ReadFunc func(path string) (json.RawMessage, error) // read and transform
    Debounce time.Duration                              // coalescing window
}

func New(tree *tree.Tree, log *slog.Logger) (*FSWatcher, error) {
    w, err := fsnotify.NewWatcher()
    if err != nil {
        return nil, fmt.Errorf("fsnotify: %w", err)
    }
    return &FSWatcher{
        watcher:  w,
        tree:     tree,
        handlers: make(map[string]WatchHandler),
        debounce: make(map[string]*time.Timer),
        log:      log,
    }, nil
}

func (fw *FSWatcher) Watch(path string, handler WatchHandler) error {
    fw.mu.Lock()
    fw.handlers[path] = handler
    fw.mu.Unlock()
    return fw.watcher.Add(path)
}

func (fw *FSWatcher) Run(ctx context.Context) error {
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case event, ok := <-fw.watcher.Events:
            if !ok {
                return fmt.Errorf("watcher closed")
            }
            if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
                fw.handleEvent(event.Name)
            }
            if event.Has(fsnotify.Remove) {
                // inotify sends IN_IGNORED after IN_DELETE; re-add watch
                fw.rewatch(event.Name)
            }
        case err, ok := <-fw.watcher.Errors:
            if !ok {
                return fmt.Errorf("watcher error channel closed")
            }
            fw.log.Warn("fsnotify error", "err", err)
        }
    }
}
```

#### Watched Paths

| Watched Path Pattern | Handler | Tree Key | Debounce | Notes |
|-----|------|------|------|------|
| `/proc/sys/net/ipv4/conf/*/forwarding` | readForwardingState | `ietf-routing:routing` | 200ms | May not support inotify on some procfs paths; falls back to polling |

**Note**: sysfs pseudo-files under `/sys/class/hwmon/` and `/sys/class/thermal/` do **not** emit inotify events. The kernel does not call `fsnotify_modify()` when hardware sensor values change — these files generate their values on `read()`, not on write. Additionally, sensor values (temperature, fan speed, voltage) fluctuate continuously, which would produce event storms even if inotify worked. Hardware sensor data is therefore collected by the polling-based hardware collector (`collector/hardware.go`) at a 10-second interval, not by the fswatcher. See Section 5, collector #6.

#### Debouncing Strategy

To prevent excessive tree updates during rapid filesystem writes (e.g., multiple interface forwarding state changes during reconfiguration), the file watcher implements a per-path debouncing mechanism. When a file modification event is received, `FSWatcher` starts or resets a timer for that specific path. Only after the timer expires (the "coalescing window") is the file read and the tree updated. This ensures that only the final state is committed to the in-memory tree during bulk write operations.

#### inotify Limitations and Fallback

While inotify is highly efficient, it has certain kernel-level limitations that `yangerd` must handle. The `/proc/sys/fs/inotify/max_user_watches` limit (default 65536) can be exhausted on systems with many interfaces. Additionally, some pseudo-filesystems do not support inotify events at all: `sysfs` files under `/sys/class/hwmon/` and `/sys/class/thermal/` are generated dynamically by the kernel on `read()` — the kernel never calls `fsnotify_modify()` when hardware sensor values change, so inotify watches on these paths would never fire. For this reason, hardware sensor data is collected by the polling-based `collector/hardware.go` (see Section 5, item 6), not by the fswatcher. If a watch on a supported path cannot be established, `yangerd` logs a warning and the affected path falls back to the polling collector at its configured interval.

#### Glob Expansion at Startup

Some watched paths contain wildcards that must be resolved at startup. For example, procfs forwarding flags (`/proc/sys/net/ipv4/conf/*/forwarding`) use shell-style globs. These patterns are expanded using `filepath.Glob` during daemon initialization. For each matching path discovered, an individual inotify watch is added to the `FSWatcher` instance. If new interfaces appear at runtime, `yangerd` must be notified to re-scan and add new watches.

#### Initial Read at Startup

After all watches are established (including glob-expanded paths), the fswatcher performs a synchronous initial read of every watched file before entering the `Run()` event loop. For each path registered in `fw.handlers`, it calls the handler's `ReadFunc` to read the current value and populates the tree immediately. This ensures that forwarding flags are present in the tree from daemon start, rather than remaining empty until the first inotify event fires — which may never happen if the forwarding state does not change after boot.

```go
// InitialRead reads the current value of every watched file and populates
// the tree.  Called once after all Watch() calls and glob expansion, before
// Run().  Errors on individual files are logged but do not prevent startup.
func (fw *FSWatcher) InitialRead() {
    fw.mu.Lock()
    defer fw.mu.Unlock()
    for path, handler := range fw.handlers {
        data, err := handler.ReadFunc(path)
        if err != nil {
            fw.log.Warn("initial read failed", "path", path, "err", err)
            continue
        }
        fw.tree.Set(handler.TreeKey, data)
        fw.log.Debug("initial read", "path", path, "key", handler.TreeKey)
    }
}
```

The initial read is fast (sub-millisecond per file — procfs reads are kernel-generated values) and completes synchronously before the daemon signals readiness.

#### Concurrency Model

The `FSWatcher` runs as a single goroutine executing the `Run()` event loop. All incoming inotify events are processed sequentially within this loop. When a debounce timer expires, it calls `handleEvent` in its own goroutine via `time.AfterFunc`, which then posts the event back to the main event loop or acquires the necessary locks to perform the read and update the tree. This ensures thread-safe access to the internal `handlers` and `debounce` maps.


### 4.1quater Bridge Monitor Subsystem

The Bridge Monitor Subsystem provides fully reactive updates for the Forwarding Database (FDB), VLAN membership, Multicast Database (MDB), and Spanning Tree Protocol (STP) states. All bridge data follows the same event-as-trigger pattern used for link/addr/neigh: netlink events identify WHAT changed, then full state re-reads via the persistent `bridge -json -batch -` subprocess provide the authoritative data. No bridge netlink attributes are parsed directly — `iproute2`'s `bridge` tool handles all attribute parsing, ensuring complete coverage of kernel-exposed bridge data including attributes not yet wrapped by Go netlink libraries.

#### BridgeBatch Subprocess Manager

The `BridgeBatch` manager interacts with a persistent `bridge -json -batch -` subprocess, ensuring serialized queries and response pairing. Its structure is identical to the `IPBatch` manager but utilizes the `bridge` binary for all operations.

```go
type BridgeBatch struct {
    cmd    *exec.Cmd
    stdin  io.WriteCloser
    stdout *bufio.Scanner
    stderr io.ReadCloser
    mu     sync.Mutex
    log    *slog.Logger
}

func NewBridgeBatch(ctx context.Context, log *slog.Logger) (*BridgeBatch, error) {
    cmd := exec.CommandContext(ctx, "bridge", "-json", "-batch", "-")
    stdin, err := cmd.StdinPipe()
    if err != nil {
        return nil, fmt.Errorf("bridge batch stdin pipe: %w", err)
    }
    stdout, err := cmd.StdoutPipe()
    if err != nil {
        return nil, fmt.Errorf("bridge batch stdout pipe: %w", err)
    }
    stderr, err := cmd.StderrPipe()
    if err != nil {
        return nil, fmt.Errorf("bridge batch stderr pipe: %w", err)
    }
    if err := cmd.Start(); err != nil {
        return nil, fmt.Errorf("start bridge batch: %w", err)
    }
    b := &BridgeBatch{
        cmd:    cmd,
        stdin:  stdin,
        stdout: bufio.NewScanner(stdout),
        stderr: stderr,
        log:    log,
    }
    go b.drainStderr()
    return b, nil
}
```

#### Bridge Netlink Event Handling

The bridge event monitor receives events from multiple netlink channels and uses each event solely as a trigger for a full state re-read via `bridge -json -batch -`. Bridge FDB entries arrive via `NeighSubscribeWithOptions` (FDB entries are neighbor-like with `NDA_MASTER` flag), bridge VLAN and STP port state changes arrive via `LinkSubscribeWithOptions` (as `RTM_NEWLINK` events on bridge port interfaces), and bridge MDB events arrive via a raw netlink socket subscribed to `RTNLGRP_MDB` (multicast group 26). The event content is not parsed for data — only the affected bridge name is extracted to scope the re-query:

```go
// Bridge event handling is integrated into the NLMonitor's select loop.
// Bridge events are used as triggers only -- the event content is not parsed
// for data.  Full state is always re-read via bridge -json -batch -.
//
// In the LinkUpdate handler (linkCh):
//   - If the link is a bridge port (has MasterIndex), trigger bridge vlan
//     and STP state re-read via bridge batch
//   - Regular link processing continues as normal
//
// In the NeighUpdate handler (neighCh):
//   - If neigh has NDA_MASTER flag (bridge FDB entry), trigger FDB re-read
//     via bridge batch
//   - Otherwise, regular neighbor re-read via ip batch
//
// Bridge MDB events require a dedicated raw netlink socket:
func (m *NLMonitor) subscribeBridgeMDB(ctx context.Context) (<-chan struct{}, error) {
    // Raw netlink socket for RTNLGRP_MDB (group 26)
    sock, err := nl.Subscribe(syscall.NETLINK_ROUTE, 26) // RTNLGRP_MDB
    if err != nil {
        return nil, fmt.Errorf("subscribe RTNLGRP_MDB: %w", err)
    }

    mdbCh := make(chan struct{}, 1)
    go func() {
        defer sock.Close()
        for {
            msgs, _, err := sock.Receive()
            if err != nil {
                if ctx.Err() != nil {
                    return
                }
                m.log.Warn("MDB netlink receive error", "err", err)
                continue
            }
            if len(msgs) > 0 {
                select {
                case mdbCh <- struct{}{}:
                default: // coalesce if unread
                }
            }
        }
    }()
    return mdbCh, nil
}

// handleBridgeFDB is called when a NeighUpdate has NDA_MASTER flag.
func (m *NLMonitor) handleBridgeFDB(u netlink.NeighUpdate) {
    bridge := ""
    if link, err := netlink.LinkByIndex(u.MasterIndex); err == nil {
        bridge = link.Attrs().Name
    }
    if bridge == "" {
        return
    }
    data, err := m.brBatch.Query(fmt.Sprintf("fdb show br %s", bridge))
    if err != nil {
        m.log.Warn("bridge fdb query failed", "bridge", bridge, "err", err)
        return
    }
    m.tree.Set(
        fmt.Sprintf("/ietf-interfaces:interfaces/interface[name='%s']/bridge:bridge/fdb", bridge),
        data,
    )
}

// handleBridgeMDB is called when an MDB event arrives.
func (m *NLMonitor) handleBridgeMDB() {
    data, err := m.brBatch.Query("mdb show")
    if err != nil {
        m.log.Warn("bridge mdb query failed", "err", err)
        return
    }
    m.tree.Set("/ieee802-dot1q-bridge:bridges/bridge/mdb", data)
}
```

#### Initial State Dump

On startup, the bridge subsystem populates the tree by issuing the following commands to the `BridgeBatch` process:
- `vlan show`
- `fdb show`
- `mdb show`

#### Subprocess Lifecycle

The bridge batch subprocess is managed with the same exponential backoff restart pattern as the ip batch subsystem. A canary query (`vlan show`) is performed upon restart to verify the health of the bridge batch process. Bridge netlink subscriptions are established alongside the main NLMonitor subscriptions and share the same error callback and context cancellation pattern.

#### Concurrency Model

Bridge FDB and VLAN events are handled within the NLMonitor's main select loop (they arrive on the `neighCh` and `linkCh` channels respectively). Bridge MDB events arrive on a dedicated raw netlink channel and are also included in the NLMonitor select loop. The `BridgeBatch` manager serializes bridge state queries via a mutex, mirroring the `IPBatch` design.

### 4.1quinquies IW Event Monitor Subsystem

#### Overview

The IW Event Monitor subsystem provides reactive 802.11 wireless monitoring by running a persistent `iw event -t` subprocess. Unlike the NLMonitor (which receives typed Go structs from `vishvananda/netlink` channels) and the bridge netlink subscriptions, the `iw event` command produces human-readable text lines that require custom parsing. Additionally, `iw` has no batch query mode -- re-queries spawn individual short-lived `exec.Command("iw", ...)` subprocesses. This is acceptable because WiFi events occur at a much lower rate than netlink link/addr/neigh events (typically single-digit events per minute during normal operation, compared to hundreds of netlink events per second during convergence).

The subsystem is governed by the `YANGERD_ENABLE_WIFI` feature flag: when WiFi support is included in the Infix build, the Buildroot recipe sets `YANGERD_ENABLE_WIFI=true` in `/etc/default/yangerd`, and the `iw` binary is guaranteed present on the target. When WiFi is not included in the build, the flag is set to `false` and the IW Event Monitor is not started at all.

#### IW Event Output Format

The `iw event -t` command produces timestamped, human-readable text lines on stdout. Each line follows one of several formats:

```
1708984743.123456: wlan0 (phy #0): new station aa:bb:cc:dd:ee:ff
1708984743.456789: wlan0 (phy #0): del station aa:bb:cc:dd:ee:ff
1708984800.111222: wlan0 (phy #0): connected to aa:bb:cc:dd:ee:ff
1708984800.333444: wlan0 (phy #0): disconnected
1708984900.555666: wlan0 (phy #0): scan started
1708984901.777888: wlan0 (phy #0): scan aborted
1708984950.999000: wlan0 (phy #0): ch_switch_started_notify freq 5180 width 80 MHz
1708985000.111222: phy #0: reg_change
```

Key differences from NLMonitor (netlink subscription) output:
- **Text, not JSON**: Each line must be parsed with string splitting and regular expressions rather than `json.Unmarshal()`
- **No batch mode**: There is no `iw -batch -` equivalent; re-queries use individual short-lived `exec.Command` invocations
- **Timestamp format**: Floating-point Unix epoch seconds (e.g., `1708984743.123456`), not ISO 8601
- **Variable structure**: Different event types have different numbers of fields after the interface identifier

#### Key Event Types

| Event | Meaning | Action |
|-------|---------|--------|
| `new station` | A wireless client associated (AP mode) | Re-query `iw dev <iface> station dump` |
| `del station` | A wireless client disassociated (AP mode) | Re-query station list; remove from tree |
| `connected` | This station connected to an AP (station mode) | Re-query `iw dev <iface> info` and `iw dev <iface> link` |
| `disconnected` | This station disconnected from AP (station mode) | Clear link info from tree |
| `auth` | Authentication event | Logged; no tree update (transient) |
| `scan started` | Background scan initiated | Logged for observability |
| `scan aborted` | Scan was aborted | Logged for observability |
| `ch_switch_started_notify` | Channel switch in progress | Re-query `iw dev <iface> info` for new frequency |
| `reg_change` | Regulatory domain changed | Re-query all wireless interfaces |

#### IWMonitor Go Struct

```go
// IWMonitor manages the persistent `iw event -t` subprocess.
// Started only when YANGERD_ENABLE_WIFI=true (WiFi included in build).
type IWMonitor struct {
    cmd    *exec.Cmd       // persistent iw event -t subprocess
    stdout *bufio.Scanner  // line scanner for subprocess stdout
    tree   *tree.Tree      // shared in-memory data tree
    log    *slog.Logger     // structured logger
    ctx    context.Context  // lifecycle context
    cancel context.CancelFunc
}

// IWEvent represents a single parsed line from `iw event -t`.
type IWEvent struct {
    Timestamp float64  // Unix epoch seconds (e.g., 1708984743.123456)
    Interface string   // Wireless interface name (e.g., "wlan0")
    Phy       string   // Physical device identifier (e.g., "phy #0")
    Type      string   // Event type (e.g., "new station", "disconnected")
    Addr      string   // MAC address (if applicable, empty otherwise)
}
```

#### Event Parser

```go
// parseIWEvent parses a single line from `iw event -t` output.
// Returns an IWEvent and true on success, or zero-value and false
// if the line does not match any known event format.
func parseIWEvent(line string) (IWEvent, bool) {
    // Format: "<timestamp>: <iface> (<phy>): <event type> [<addr>]"
    // Example: "1708984743.123456: wlan0 (phy #0): new station aa:bb:cc:dd:ee:ff"
    parts := strings.SplitN(line, ": ", 3)
    if len(parts) < 3 {
        return IWEvent{}, false
    }

    ts, err := strconv.ParseFloat(parts[0], 64)
    if err != nil {
        return IWEvent{}, false
    }

    // Parse "wlan0 (phy #0)" portion
    ifacePhy := parts[1]
    parenIdx := strings.Index(ifacePhy, " (")
    if parenIdx < 0 {
        return IWEvent{}, false
    }
    iface := ifacePhy[:parenIdx]
    phy := strings.Trim(ifacePhy[parenIdx+2:], ")")

    // Remaining is event type + optional address
    eventStr := parts[2]
    ev := IWEvent{Timestamp: ts, Interface: iface, Phy: phy}

    switch {
    case strings.HasPrefix(eventStr, "new station "):
        ev.Type = "new station"
        ev.Addr = strings.TrimPrefix(eventStr, "new station ")
    case strings.HasPrefix(eventStr, "del station "):
        ev.Type = "del station"
        ev.Addr = strings.TrimPrefix(eventStr, "del station ")
    case strings.HasPrefix(eventStr, "connected to "):
        ev.Type = "connected"
        ev.Addr = strings.TrimPrefix(eventStr, "connected to ")
    case eventStr == "disconnected":
        ev.Type = "disconnected"
    case strings.HasPrefix(eventStr, "ch_switch_started_notify"):
        ev.Type = "ch_switch_started_notify"
    case eventStr == "scan started":
        ev.Type = "scan started"
    case eventStr == "scan aborted":
        ev.Type = "scan aborted"
    case strings.HasPrefix(eventStr, "reg_change"):
        ev.Type = "reg_change"
    case strings.HasPrefix(eventStr, "auth"):
        ev.Type = "auth"
    default:
        ev.Type = eventStr // preserve unknown events for logging
    }

    return ev, true
}
```

#### Event Handler and Re-Query

```go
// handleEvent processes a parsed IW event by re-querying the appropriate
// iw subcommands and updating the in-memory tree.
func (m *IWMonitor) handleEvent(ev IWEvent) {
    switch ev.Type {
    case "new station", "del station":
        m.refreshStationList(ev.Interface)
    case "connected", "ch_switch_started_notify":
        m.refreshInterfaceInfo(ev.Interface)
    case "disconnected":
        m.clearLinkInfo(ev.Interface)
    case "reg_change":
        m.refreshAllInterfaces()
    default:
        m.log.Debug("unhandled iw event", "type", ev.Type, "iface", ev.Interface)
    }
}

// refreshStationList runs `iw dev <iface> station dump` and updates
// the tree with the current list of associated stations.
func (m *IWMonitor) refreshStationList(iface string) {
    ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
    defer cancel()
    out, err := exec.CommandContext(ctx, "iw", "dev", iface, "station", "dump").Output()
    if err != nil {
        m.log.Warn("iw station dump failed", "iface", iface, "err", err)
        return
    }
    stations := parseStationDump(string(out))
    m.tree.Set("wifi/"+iface+"/stations", stations)
}

// refreshInterfaceInfo runs `iw dev <iface> info` to update SSID,
// frequency, channel width, and TX power in the tree.
func (m *IWMonitor) refreshInterfaceInfo(iface string) {
    ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
    defer cancel()
    out, err := exec.CommandContext(ctx, "iw", "dev", iface, "info").Output()
    if err != nil {
        m.log.Warn("iw dev info failed", "iface", iface, "err", err)
        return
    }
    info := parseIWInfo(string(out))
    m.tree.Set("wifi/"+iface+"/info", info)
}
```

#### Differences from IP/Bridge Monitor Subsystems

| Aspect | NLMonitor (netlink subscriptions) | iw event |
|--------|---------------------------|----------|
| Output format | Typed Go structs (`LinkUpdate`, `AddrUpdate`, etc.) on channels | Human-readable text lines |
| Batch query mode | `ip -json -batch -` / `bridge -json -batch -` (persistent stdin/stdout) | None -- each query spawns a short-lived `exec.Command` |
| Event rate | High (100s/sec during convergence) | Low (single-digit/min typical) |
| Parser implementation | Direct struct field access (`u.Link.Attrs().Name`) | `strings.SplitN()` + `strconv.ParseFloat()` + switch |
| Event source | Native Go netlink channels (`vishvananda/netlink`) | `iw event -t` subprocess (stdout) |
| Absence handling | Netlink always available (kernel 6.18) | Governed by `YANGERD_ENABLE_WIFI` feature flag; when enabled, `iw` is guaranteed present |

#### Subprocess Lifecycle

The `iw event -t` subprocess is started during yangerd initialization when `YANGERD_ENABLE_WIFI=true`. If the subprocess exits unexpectedly, it is restarted with the same exponential backoff pattern used by the NLMonitor and bridge batch subsystems (initial delay 100ms, max delay 30s, backoff factor 2x). Upon restart, a full re-query of all known wireless interfaces is performed to synchronize the in-memory tree with the current kernel state.

Unlike the ip and bridge subsystems, there is no canary query mechanism because `iw event` has no query/response mode—it only emits events. Health monitoring is instead based on subprocess liveness: if the process exits or its stdout is closed, the monitor goroutine detects this via `scanner.Err()` and initiates the restart sequence.

#### Concurrency Model

The IW Event Monitor uses a single goroutine that reads lines from `iw event -t` stdout via a `bufio.Scanner`. For each parsed event, re-queries are executed synchronously within the same goroutine because WiFi event rates are low enough that sequential processing does not introduce meaningful latency. This avoids the complexity of a separate query goroutine and its associated synchronization. If future deployments reveal that re-query latency becomes problematic (e.g., on systems with dozens of wireless interfaces), the design can be extended to dispatch re-queries to a bounded worker pool without changing the event parsing goroutine.

### 4.1sexies Ethtool Netlink Monitor Subsystem

#### Overview

The Linux kernel's ethtool subsystem exposes a genetlink family named `"ethtool"`. This family includes a multicast group named `"monitor"` (`ETHNL_MCGRP_MONITOR`) that delivers notification messages whenever ethtool-managed settings change on any network interface. Infix targets Linux kernel 6.18, where ethtool netlink is unconditionally available. By subscribing to this multicast group, yangerd receives immediate notification of speed, duplex, link mode, and auto-negotiation changes without polling.

Unlike the ip, bridge, and iw subsystems, the ethtool netlink monitor is **not a subprocess**. It is a native Go goroutine that opens a genetlink socket using `mdlayher/genetlink`, joins the monitor multicast group, and calls `conn.Receive()` in a loop. This avoids the overhead of managing an external process, parsing its output, and supervising its lifecycle.

#### Notification Types

The kernel's `ethnl_default_notify_ops[]` array (defined in `net/ethtool/netlink.c`) registers the following notification message types:

| Notification Message | Trigger | Relevant YANG Leaves |
|---------------------|---------|---------------------|
| `ETHTOOL_MSG_LINKINFO_NTF` | Link info change (PHY type, transceiver) | speed, duplex |
| `ETHTOOL_MSG_LINKMODES_NTF` | Link modes change (advertised/supported speeds, autoneg) | speed, duplex, auto-negotiation |
| `ETHTOOL_MSG_FEATURES_NTF` | Offload feature toggle | (not mapped to YANG leaves in Phase 1) |
| `ETHTOOL_MSG_WOL_NTF` | Wake-on-LAN setting change | (not mapped) |
| `ETHTOOL_MSG_RINGS_NTF` | Ring buffer size change | (not mapped) |
| `ETHTOOL_MSG_CHANNELS_NTF` | Channel count change | (not mapped) |
| `ETHTOOL_MSG_COALESCE_NTF` | Interrupt coalescing change | (not mapped) |
| `ETHTOOL_MSG_PAUSE_NTF` | Pause frame setting change | (not mapped) |
| `ETHTOOL_MSG_EEE_NTF` | Energy-Efficient Ethernet change | (not mapped) |
| `ETHTOOL_MSG_FEC_NTF` | Forward Error Correction change | (not mapped) |
| `ETHTOOL_MSG_MODULE_NTF` | Transceiver module event | (not mapped) |
| `ETHTOOL_MSG_PLCA_NTF` | Physical Layer Collision Avoidance change | (not mapped) |
| `ETHTOOL_MSG_MM_NTF` | MAC Merge (802.3br) change | (not mapped) |

Of these, yangerd acts on `ETHTOOL_MSG_LINKINFO_NTF` and `ETHTOOL_MSG_LINKMODES_NTF` — these are the only notifications that affect YANG-modeled operational leaves (speed, duplex, auto-negotiation). All other notification types are logged at DEBUG level and discarded.

**Important**: Statistics and counters (e.g., `ethtool -S` output) have **no corresponding `_NTF` message type**. They must remain polling-based via the ethtool collector at a 30-second interval.

#### What Does NOT Have Notifications

The following ethtool data categories are explicitly **not** covered by the genetlink monitor and remain polling-based:

- **Per-interface statistics** (`ethtool -S <ifname>`, `ETHTOOL_MSG_STATS_GET`): No `ETHTOOL_MSG_STATS_NTF` exists. Counters are monotonically increasing values that change on every packet; event-based notification would be impractical.
- **String sets** (`ethtool -i <ifname>`, `ETHTOOL_MSG_STRSET_GET`): Driver name, firmware version — effectively static, queried once at startup.

#### Hybrid Model

The ethtool data acquisition becomes a **hybrid** of reactive and polling:

| Data Category | Method | Interval/Trigger | Go Package |
|--------------|--------|-----------------|-----------|
| Speed, duplex, auto-negotiation | REACTIVE (genetlink monitor) | On `_NTF` notification | `internal/ethmonitor/` |
| Extended statistics (counters) | POLLING | 30 seconds | `internal/collector/ethtool.go` |
| Advertised/supported link modes | REACTIVE (genetlink monitor) | On `ETHTOOL_MSG_LINKMODES_NTF` | `internal/ethmonitor/` |

Both the reactive ethmonitor and the polling ethtool collector write to the same tree paths under `ietf-interfaces:interfaces` (specifically the `infix-ethernet-interface:ethernet` augment subtrees). The per-model `sync.RWMutex` for the `ietf-interfaces:interfaces` key ensures that concurrent writes from the monitor goroutine and the collector goroutine are serialized.

#### Core Types

```go
// internal/ethmonitor/ethmonitor.go

package ethmonitor

import (
    "context"
    "log/slog"

    "github.com/mdlayher/ethtool"
    "github.com/mdlayher/genetlink"
    "github.com/kernelkit/infix/src/yangerd/internal/tree"
)

// EthMonitor subscribes to the ethtool genetlink monitor multicast
// group and updates the in-memory tree on settings change notifications.
type EthMonitor struct {
    conn     *genetlink.Conn
    family   genetlink.Family
    groupID  uint32          // monitor multicast group ID
    tree     *tree.Tree
    etClient *ethtool.Client // for re-queries on notification
    log      *slog.Logger
}

// NTF command constants from include/uapi/linux/ethtool_netlink_generated.h
const (
    ETHTOOL_MSG_LINKINFO_NTF  = 28
    ETHTOOL_MSG_LINKMODES_NTF = 29
)
```

#### Subscription and Event Loop

```go
// New creates an EthMonitor by dialing the genetlink socket,
// resolving the "ethtool" family, and finding the "monitor" multicast group.
func New(t *tree.Tree, log *slog.Logger) (*EthMonitor, error) {
    conn, err := genetlink.Dial(nil)
    if err != nil {
        return nil, fmt.Errorf("genetlink dial: %w", err)
    }

    family, err := conn.GetFamily("ethtool")
    if err != nil {
        conn.Close()
        return nil, fmt.Errorf("get ethtool family: %w", err)
    }

    var groupID uint32
    for _, g := range family.Groups {
        if g.Name == "monitor" {
            groupID = g.ID
            break
        }
    }
    if groupID == 0 {
        conn.Close()
        return nil, fmt.Errorf("ethtool monitor multicast group not found")
    }

    if err := conn.JoinGroup(groupID); err != nil {
        conn.Close()
        return nil, fmt.Errorf("join monitor group: %w", err)
    }

    etClient, err := ethtool.New()
    if err != nil {
        conn.Close()
        return nil, fmt.Errorf("ethtool client: %w", err)
    }

    return &EthMonitor{
        conn:     conn,
        family:   family,
        groupID:  groupID,
        tree:     t,
        etClient: etClient,
        log:      log,
    }, nil
}

// Run listens for ethtool notifications and updates the tree.
// It blocks until ctx is cancelled.
func (m *EthMonitor) Run(ctx context.Context) error {
    defer m.conn.Close()
    defer m.etClient.Close()

    // Set read deadline so we can check ctx.Done() periodically
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
        }

        msgs, _, err := m.conn.Receive()
        if err != nil {
            if ctx.Err() != nil {
                return ctx.Err()
            }
            m.log.Warn("ethmonitor receive error", "err", err)
            continue
        }

        for _, msg := range msgs {
            m.handleNotification(msg)
        }
    }
}
```

#### Notification Handler

```go
func (m *EthMonitor) handleNotification(msg genetlink.Message) {
    switch msg.Header.Command {
    case ETHTOOL_MSG_LINKINFO_NTF, ETHTOOL_MSG_LINKMODES_NTF:
        ifname := extractIfname(msg.Data)
        if ifname == "" {
            m.log.Debug("ethmonitor: NTF without ifname", "cmd", msg.Header.Command)
            return
        }
        m.refreshEthernetSettings(ifname)
    default:
        m.log.Debug("ethmonitor: ignored NTF", "cmd", msg.Header.Command)
    }
}

// refreshEthernetSettings re-queries speed, duplex, and auto-negotiation
// for the given interface and updates the tree.
func (m *EthMonitor) refreshEthernetSettings(ifname string) {
    iface, err := net.InterfaceByName(ifname)
    if err != nil {
        m.log.Warn("ethmonitor: interface lookup failed", "iface", ifname, "err", err)
        return
    }

    linkInfo, err := m.etClient.LinkInfo(ethtool.Interface{Index: iface.Index})
    if err != nil {
        m.log.Warn("ethmonitor: LinkInfo failed", "iface", ifname, "err", err)
        return
    }

    linkMode, err := m.etClient.LinkMode(ethtool.Interface{Index: iface.Index})
    if err != nil {
        m.log.Warn("ethmonitor: LinkMode failed", "iface", ifname, "err", err)
        return
    }

    data := map[string]interface{}{
        "speed":            linkMode.SpeedMegabits,
        "duplex":           duplexString(linkInfo.Duplex),
        "auto-negotiation": autonegString(linkMode.AutoNegotiation),
    }
    jsonData, _ := json.Marshal(data)
    m.tree.Set("ietf-interfaces:interfaces/interface["+ifname+"]/ethernet", json.RawMessage(jsonData))
}
```

#### Public RefreshInterface API (Cross-Subsystem)

The `EthMonitor` exposes a public `RefreshInterface()` method that the link event handler (`monitor/link.go`) calls on every RTM_NEWLINK event. This is necessary because the ethtool genetlink monitor (`ETHNL_MCGRP_MONITOR`) does **NOT** emit notifications when a link goes up or down — only when settings are explicitly renegotiated (e.g., by `ethtool -s`). When the kernel brings a link up, it negotiates speed, duplex, and auto-negotiation with the link partner, but this negotiation result is invisible to the ethtool monitor.

`RefreshInterface()` is a thin public wrapper around the private `refreshEthernetSettings()`:

```go
// RefreshInterface is called by the link event handler (monitor/link.go)
// whenever an RTM_NEWLINK event arrives. Since ETHNL_MCGRP_MONITOR does NOT
// emit notifications on link up/down (only on explicit settings changes),
// this method ensures that speed/duplex/autoneg are re-queried after every
// link state change.
func (m *EthMonitor) RefreshInterface(ifname string) {
    m.refreshEthernetSettings(ifname)
}
```

This cross-subsystem coordination ensures that ethtool data is always current after link events:

| Trigger | Source | Ethtool Action |
|---------|--------|----------------|
| `ETHTOOL_MSG_LINKINFO_NTF` | ethtool genetlink monitor (settings change) | `refreshEthernetSettings()` (internal) |
| `ETHTOOL_MSG_LINKMODES_NTF` | ethtool genetlink monitor (mode change) | `refreshEthernetSettings()` (internal) |
| `RTM_NEWLINK` (any) | link event handler (`monitor/link.go`) | `RefreshInterface()` (public, cross-subsystem) |

Without `RefreshInterface()`, after a link-up event the tree would show stale speed/duplex/autoneg values until the next 30-second polling cycle (if ethmonitor failed) or indefinitely (if ethmonitor was active but the kernel never sent an explicit ethtool NTF).
```

#### Differences from Other Reactive Subsystems

| Aspect | NLMonitor (netlink subscriptions) | iw event | ethtool genetlink |
|--------|-------------------|----------|-------------------|
| Implementation | Native Go netlink channels (`vishvananda/netlink`) | External subprocess | Native Go genetlink socket |
| Output format | Typed Go structs (`LinkUpdate`, `AddrUpdate`, etc.) | Human-readable text | Binary genetlink messages |
| Process management | Goroutine with channel re-subscribe on close | Persistent subprocess with restart | Goroutine -- no process to manage |
| Batch query mode | Yes (`ip -batch -` / `bridge -batch -`) for re-reads | None (short-lived exec) | No -- re-queries via `ethtool.Client` |
| Failure mode | Channel close -> re-subscribe (OVN-K pattern) | Subprocess crash -> restart | Socket error -> reconnect |
| Event rate | High (100s/sec during convergence) | Low (single-digit/min) | Very low (link negotiation events only) |
| Absence handling | Netlink always available (kernel 6.18) | Governed by `YANGERD_ENABLE_WIFI` flag | Always active (ethtool netlink unconditionally available on kernel 6.18) |

#### Lifecycle

The `EthMonitor` is created during yangerd initialization by calling `ethmonitor.New()`. Since Infix targets Linux kernel 6.18, the `"ethtool"` genetlink family and its `"monitor"` multicast group are unconditionally available. If the subscription fails for any unexpected reason (e.g., permission denied, kernel module not loaded), the error is treated as fatal and logged at ERROR — this indicates a misconfigured system, not a kernel capability gap.

On clean shutdown (context cancellation), the genetlink connection is closed via `defer m.conn.Close()`, which causes `conn.Receive()` to return an error and the goroutine to exit.

#### Concurrency Model

The `EthMonitor` uses a single goroutine that calls `conn.Receive()` in a loop. Each notification triggers a synchronous re-query via `ethtool.Client.LinkInfo()` and `ethtool.Client.LinkMode()`. This sequential model is appropriate because ethtool settings change notifications are extremely infrequent — they occur only during physical link negotiation events (cable plug/unplug, speed forced by operator, autoneg toggled). Even on a system with hundreds of interfaces, link negotiation storms are rare and short-lived.

The tree write from the ethmonitor goroutine and the tree write from the ethtool polling collector are serialized by the per-model `sync.RWMutex` for their shared key (`ietf-interfaces:interfaces`). No additional synchronization is needed between these two components -- they write to the same tree paths but at different times (reactive on notification vs. periodic on 30-second tick).

### 4.1octies ZAPI Watcher Subsystem (Zebra Route Redistribution)

#### Overview

The ZAPI (Zebra API) watcher replaces the previous `vtysh`-based route table collection with a persistent, streaming connection to FRRouting's zebra daemon. Instead of forking `vtysh -c 'show ip route json'` on every netlink route event, yangerd opens a Unix domain socket to zebra's zserv API, subscribes to route redistribution notifications, and receives incremental route add/delete messages as they occur.

This design is motivated by a fundamental limitation of the Linux kernel FIB: **routes may exist in zebra's RIB that are not installed in the kernel**. These include:

- Routes with unresolvable next-hops (`"installed": false` in FRR)
- Routes that lost the administrative distance election (`"selected": false`)
- ECMP paths exceeding the kernel's maximum next-hop count
- Routes filtered by FRR's `table-map` policy

The `ip route` command (and netlink `RTM_NEWROUTE` events) only reflect routes that zebra has successfully installed in the kernel FIB. To expose the complete routing state through the YANG operational datastore, yangerd must query zebra directly.

#### ZAPI Protocol

FRRouting uses the Zebra Serv (zserv) protocol for inter-daemon communication. All FRR daemons (bgpd, ospfd, ripd, staticd) use this same protocol to exchange routes with zebra. The protocol version is ZSERV_VERSION 6, which has been stable across FRR 8.x, 9.x, and 10.x (including the target FRR 10.5.1).

```
ZAPI v6 Header (10 bytes):

  0                   1                   2                   3
  0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
 +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
 |          Length (2)           |  Marker 0xFE  | Version (6)   |
 +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
 |                        VRF ID (4)                             |
 +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
 |         Command (2)          |
 +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

Socket path: /var/run/frr/zserv.api (Unix domain socket)
```

The connection flow for a route redistribution client is:

```
yangerd (ZAPI client)                           zebra
       |                                          |
       |--- ZEBRA_HELLO (daemon type=0) --------->|  Register as client
       |                                          |
       |--- ZEBRA_ROUTER_ID_ADD ----------------->|  Request router-id updates
       |                                          |
       |--- ZEBRA_REDISTRIBUTE_ADD (kernel) ----->|  Subscribe: type 1
       |--- ZEBRA_REDISTRIBUTE_ADD (connected) -->|  Subscribe: type 2
       |--- ZEBRA_REDISTRIBUTE_ADD (static) ----->|  Subscribe: type 3
       |--- ZEBRA_REDISTRIBUTE_ADD (rip) -------->|  Subscribe: type 4
       |--- ZEBRA_REDISTRIBUTE_ADD (ospf) ------->|  Subscribe: type 6
       |                                          |
       |<-- ZEBRA_REDISTRIBUTE_ROUTE_ADD ---------|  Full dump of existing
       |<-- ZEBRA_REDISTRIBUTE_ROUTE_ADD ---------|  routes matching the
       |<-- ZEBRA_REDISTRIBUTE_ROUTE_ADD ---------|  subscribed types
       |<-- ...                                   |
       |                                          |
       |    (incremental updates from here on)    |
       |<-- ZEBRA_REDISTRIBUTE_ROUTE_ADD ---------|  New route installed
       |<-- ZEBRA_REDISTRIBUTE_ROUTE_DEL ---------|  Route withdrawn
       |                                          |
```

After the initial dump, zebra sends incremental `REDISTRIBUTE_ROUTE_ADD` and `REDISTRIBUTE_ROUTE_DEL` messages whenever a route matching a subscribed type is added, modified, or withdrawn. Each message includes the full route body: prefix, prefix length, address family, route type, distance, metric, tag, next-hop list (with interface index and gateway address), and flags indicating whether the route is selected and installed in the kernel.

#### Go Implementation

The ZAPI watcher uses the `github.com/osrg/gobgp/v4/pkg/zebra` package, which implements ZAPI protocol versions 2 through 6. This library is production-tested in Cilium and kube-vip. It provides `NewClient()` for connection setup, `SendRedistribute()` for subscription, and a `Receive()` channel for incoming messages.

```go
package zapiwatcher

import (
    "context"
    "log/slog"
    "math"
    "net"
    "time"

    "github.com/osrg/gobgp/v4/pkg/zebra"
    "github.com/kernelkit/infix/src/yangerd/internal/tree"
)

const (
    zapiSocketPath = "/var/run/frr/zserv.api"
    zapiVersion    = 6
    zapiSoftware   = "frr10.5"

    // Reconnection parameters
    reconnectInitial = 100 * time.Millisecond
    reconnectMax     = 30 * time.Second
    reconnectFactor  = 2.0
)

// Route types to subscribe for redistribution.
var subscribeTypes = []zebra.RouteType{
    zebra.RouteKernel,    // type 1: kernel routes (from ip route add)
    zebra.RouteConnect,   // type 2: connected (interface subnets)
    zebra.RouteStatic,    // type 3: static routes (from staticd)
    zebra.RouteRIP,       // type 4: RIP-learned routes
    zebra.RouteOSPF,      // type 6: OSPF-learned routes
}

// ZAPIWatcher maintains a persistent connection to zebra's zserv
// socket and updates the in-memory tree with route redistribution
// messages. It handles zebra restarts with automatic reconnection.
type ZAPIWatcher struct {
    tree *tree.Tree
    log  *slog.Logger
}

func New(t *tree.Tree, log *slog.Logger) *ZAPIWatcher {
    return &ZAPIWatcher{tree: t, log: log}
}
```

#### Connection and Subscription

```go
// connect establishes a ZAPI session and subscribes to route
// redistribution for all configured route types.
func (w *ZAPIWatcher) connect(ctx context.Context) (*zebra.Client, error) {
    conn, err := net.Dial("unix", zapiSocketPath)
    if err != nil {
        return nil, fmt.Errorf("dial zserv: %w", err)
    }

    cli, err := zebra.NewClient(conn, zebra.MaxSoftware(zapiSoftware),
        zebra.Version(uint8(zapiVersion)))
    if err != nil {
        conn.Close()
        return nil, fmt.Errorf("zapi handshake: %w", err)
    }

    // Send HELLO to register as a redistribution client.
    if err := cli.SendHello(); err != nil {
        cli.Close()
        return nil, fmt.Errorf("zapi hello: %w", err)
    }

    // Request router-id updates (needed for some route attributes).
    if err := cli.SendRouterIDAdd(); err != nil {
        cli.Close()
        return nil, fmt.Errorf("zapi router-id: %w", err)
    }

    // Subscribe to redistribution for each route type.
    for _, rt := range subscribeTypes {
        if err := cli.SendRedistribute(rt, zebra.VRFDefault); err != nil {
            cli.Close()
            return nil, fmt.Errorf("zapi redistribute %v: %w", rt, err)
        }
    }

    w.log.Info("zapi: connected to zebra", "socket", zapiSocketPath,
        "version", zapiVersion, "types", len(subscribeTypes))
    return cli, nil
}
```

#### Main Run Loop with Reconnection

The gobgp zebra client's `Receive()` channel delivers incoming ZAPI messages. When zebra restarts (the zserv socket is deleted and recreated), the channel closes with an EOF. The watcher detects this and reconnects with exponential backoff.

```go
// Run starts the ZAPI watcher. It blocks until ctx is cancelled.
// On disconnect, it reconnects with exponential backoff.
func (w *ZAPIWatcher) Run(ctx context.Context) error {
    delay := reconnectInitial

    for {
        cli, err := w.connect(ctx)
        if err != nil {
            w.log.Warn("zapi: connect failed, retrying",
                "error", err, "delay", delay)
            select {
            case <-ctx.Done():
                return ctx.Err()
            case <-time.After(delay):
            }
            delay = time.Duration(math.Min(
                float64(delay)*reconnectFactor,
                float64(reconnectMax)))
            continue
        }

        // Reset backoff on successful connection.
        delay = reconnectInitial

        // Process messages until disconnect.
        w.processMessages(ctx, cli)

        // If we reach here, the connection was lost.
        // Clear stale routes before reconnecting.
        w.clearAllRoutes()
        w.log.Warn("zapi: disconnected from zebra, reconnecting")
    }
}
```

#### Message Processing

```go
func (w *ZAPIWatcher) processMessages(ctx context.Context, cli *zebra.Client) {
    for {
        select {
        case <-ctx.Done():
            cli.Close()
            return
        case msg, ok := <-cli.Receive():
            if !ok {
                // Channel closed -- zebra disconnected.
                return
            }
            w.handleMessage(msg)
        }
    }
}

func (w *ZAPIWatcher) handleMessage(msg *zebra.Message) {
    switch body := msg.Body.(type) {
    case *zebra.IPRouteBody:
        switch msg.Header.Command {
        case zebra.RedistributeRouteAdd:
            w.addRoute(body)
        case zebra.RedistributeRouteDel:
            w.deleteRoute(body)
        }
    case *zebra.RouterIDUpdateBody:
        w.log.Debug("zapi: router-id update", "id", body.Prefix)
    default:
        // Ignore unhandled message types.
    }
}
```

#### Route Tree Updates

Route messages are transformed into YANG-compatible structures and written to the in-memory tree. The `IPRouteBody` from gobgp's zebra package contains:

| Field | Description | YANG mapping |
|-------|-------------|--------------|
| `Prefix` | Route prefix (net.IPNet) | `destination-prefix` |
| `Type` | Route type (kernel/connected/static/ospf/rip) | `source-protocol` |
| `Distance` | Administrative distance | `route-preference` (when supported) |
| `Metric` | Route metric | `metric` |
| `Nexthops` | Next-hop list (gateway + interface index) | `next-hop-list/next-hop` |
| `Flags` | Selected, installed, etc. | `active` leaf |

```go
func (w *ZAPIWatcher) addRoute(body *zebra.IPRouteBody) {
    rib := ribName(body.Prefix) // "ipv4-master" or "ipv6-master"
    key := routeKey(body)       // prefix + protocol composite key

    entry := transformRoute(body) // -> YANG-compatible JSON structure
    w.tree.SetRoute(rib, key, entry)

    w.log.Debug("zapi: route add", "prefix", body.Prefix,
        "type", body.Type, "nexthops", len(body.Nexthops),
        "installed", body.IsInstalled())
}

func (w *ZAPIWatcher) deleteRoute(body *zebra.IPRouteBody) {
    rib := ribName(body.Prefix)
    key := routeKey(body)

    w.tree.DeleteRoute(rib, key)

    w.log.Debug("zapi: route del", "prefix", body.Prefix,
        "type", body.Type)
}
```

#### Stale Route Cleanup on Reconnect

When the connection to zebra is lost (zebra restart, socket error), all routes in the tree sourced from ZAPI become potentially stale. The watcher clears all ZAPI-sourced routes from the tree before reconnecting. Upon successful reconnection, zebra performs a full dump of all routes matching the subscribed types, which repopulates the tree with current data.

```go
func (w *ZAPIWatcher) clearAllRoutes() {
    w.tree.ClearRIB("ipv4-master")
    w.tree.ClearRIB("ipv6-master")
    w.log.Info("zapi: cleared stale routes from tree")
}
```

This full-replacement strategy is simpler and more reliable than mark-and-sweep. Since zebra's post-connection dump is complete (it sends every route matching the subscribed types), the tree converges to the correct state within seconds of reconnection. The brief window where routes are absent from the tree is acceptable because:

1. RESTCONF/NETCONF clients querying during reconnection get an empty (but valid) routing table rather than stale data.
2. The reconnection window is short (typically under 1 second for a local Unix socket).
3. Zebra restarts are infrequent operational events, not steady-state behavior.

#### Differences from Other Reactive Subsystems

| Aspect | NLMonitor (netlink) | iw event | ethmonitor (genetlink) | ZAPI watcher |
|--------|-------------------|----------|----------------------|--------------|
| Implementation | Native Go netlink channels (`vishvananda/netlink`) | External subprocess | Native Go genetlink socket | Native Go Unix socket (`osrg/gobgp/v4/pkg/zebra`) |
| Output format | Typed Go structs (`LinkUpdate`, etc.) | Human-readable text | Binary genetlink messages | Typed Go structs (`IPRouteBody`, etc.) |
| Process management | Goroutine with channel re-subscribe on close | Persistent subprocess with restart | Goroutine -- no process to manage | Goroutine with reconnection and re-subscription |
| Failure mode | Channel close -> re-subscribe | Subprocess crash -> restart | Socket error -> reconnect | EOF -> clear routes -> reconnect with backoff |
| Event rate | High (100s/sec during convergence) | Low (single-digit/min) | Very low (link negotiation only) | Moderate (proportional to route churn) |
| Absence handling | Netlink always available (kernel 6.18) | Governed by `YANGERD_ENABLE_WIFI` flag | Always active (kernel 6.18) | Requires FRR zebra running; reconnects if absent |
| Data exclusivity | Supplements ip batch re-reads | Supplements iw queries | Supplements ethtool polling | **Sole source** for route table data |

The key distinction from other subsystems is that the ZAPI watcher is the **sole source** for route table data. The NLMonitor, iw event monitor, and ethmonitor all supplement batch/polling collectors that perform the same queries. The ZAPI watcher fully replaces `vtysh` for route collection -- there is no parallel polling or batch query for routes.

#### Lifecycle

The `ZAPIWatcher` is created during yangerd initialization by calling `zapiwatcher.New()`. Its `Run()` method is started as a goroutine that blocks until context cancellation. If zebra is not yet running at startup (e.g., yangerd starts before FRR), the watcher's reconnection loop handles this transparently -- it retries with exponential backoff until zebra becomes available.

On clean shutdown (context cancellation), the `processMessages` loop detects `ctx.Done()`, closes the zebra client, and the `Run()` goroutine returns.

#### Concurrency Model

The ZAPI watcher uses a single goroutine that reads from the `cli.Receive()` channel. Route messages are processed synchronously within this goroutine: each `REDISTRIBUTE_ROUTE_ADD` or `REDISTRIBUTE_ROUTE_DEL` triggers an immediate tree write. This sequential model is appropriate because:

1. Route redistribution messages arrive at moderate rates (tens per second during convergence, single-digit per minute steady-state).
2. Tree writes are fast (in-memory map update under the per-model `sync.RWMutex` for `ietf-routing:routing`).
3. Sequential processing preserves route ordering semantics -- a delete followed by an add for the same prefix is applied in the correct order.

The tree write from the ZAPI watcher goroutine and the tree reads from RESTCONF/NETCONF handlers are serialized by the per-model `sync.RWMutex` for the `ietf-routing:routing` key. No additional synchronization is needed.

### 4.1novies D-Bus Monitor Subsystem

The D-Bus Monitor Subsystem provides reactive monitoring of dnsmasq DHCP lease events and firewalld configuration reloads via D-Bus signal subscriptions. Instead of polling the DHCP lease file or periodically querying firewall state, `yangerd` subscribes to D-Bus signals emitted by these services and reacts immediately when state changes occur. This follows the same event-as-trigger pattern used by the netlink and bridge subsystems: the D-Bus signal is the notification mechanism, but the actual data is re-read from the canonical source (lease file and D-Bus method call for DHCP; firewalld D-Bus method calls for firewall).

#### Why D-Bus Instead of inotify/Polling

The previous design used `fswatcher` (inotify) for the dnsmasq lease file and polling for firewall state. D-Bus is superior for both cases:

- **dnsmasq**: While inotify on `/var/lib/misc/dnsmasq.leases` works, dnsmasq explicitly provides D-Bus signals (`DHCPLeaseAdded`, `DHCPLeaseDeleted`, `DHCPLeaseUpdated`) designed for exactly this purpose. Using D-Bus signals rather than watching the file avoids race conditions where inotify fires before dnsmasq has finished writing the file, and provides semantic information (which lease changed) rather than just "file modified."
- **firewalld**: Firewall state is managed by firewalld and accessed via its D-Bus API. The only alternative to D-Bus signals is periodic polling, but firewalld provides no file-based state representation. D-Bus signals (`Reloaded`, plus `NameOwnerChanged` for restart detection) provide instant notification with zero steady-state CPU cost. On each signal, yangerd re-reads the full firewall state via firewalld D-Bus method calls (`getDefaultZone()`, `getActiveZones()`, `getZoneSettings2()`, `getPolicies()`, `getPolicySettings()`, `listServices()`, `getServiceSettings2()`, `getLogDenied()`, `queryPanicMode()`).

#### DBusMonitor Implementation

The following Go code defines the `DBusMonitor` type and its core event loop in `internal/dbusmonitor/dbusmonitor.go`:

```go
package dbusmonitor

import (
    "context"
    "encoding/json"
    "fmt"
    "log/slog"
    "math"
    "os"
    "os/exec"
    "strings"
    "time"

    "github.com/godbus/dbus/v5"
    "github.com/kernelkit/infix/src/yangerd/internal/tree"
)

const (
    // dnsmasq D-Bus constants
    dnsmasqBusName   = "uk.org.thekelleys.dnsmasq"
    dnsmasqInterface = "uk.org.thekelleys.dnsmasq"
    dnsmasqPath      = "/uk/org/thekelleys/dnsmasq"

    // firewalld D-Bus constants
    firewalldBusName   = "org.fedoraproject.FirewallD1"
    firewalldInterface = "org.fedoraproject.FirewallD1"
    firewalldPath      = "/org/fedoraproject/FirewallD1"

    // D-Bus standard interface for service lifecycle
    dbusInterface = "org.freedesktop.DBus"
    dbusPath      = "/org/freedesktop/DBus"

    // Data sources
    dnsmasqLeaseFile = "/var/lib/misc/dnsmasq.leases"

    // Tree keys
    dhcpTreeKey     = "infix-dhcp-server:dhcp-server"
    firewallTreeKey = "infix-firewall:firewall"

    // Reconnection parameters
    reconnectInitial = 100 * time.Millisecond
    reconnectMax     = 30 * time.Second
    reconnectFactor  = 2.0
)

// DBusMonitor subscribes to D-Bus signals from dnsmasq and firewalld,
// using each signal as a trigger to re-read data from canonical sources.
type DBusMonitor struct {
    tree *tree.Tree
    log  *slog.Logger
}

func New(t *tree.Tree, log *slog.Logger) *DBusMonitor {
    return &DBusMonitor{tree: t, log: log}
}
```

#### Signal Subscription

The monitor subscribes to three categories of D-Bus signals using `AddMatchSignal()` match rules:

| Signal | Interface | Source | Trigger Action |
|--------|-----------|--------|----------------|
| `DHCPLeaseAdded` | `uk.org.thekelleys.dnsmasq` | dnsmasq | Re-read lease file + `GetMetrics()` |
| `DHCPLeaseDeleted` | `uk.org.thekelleys.dnsmasq` | dnsmasq | Re-read lease file + `GetMetrics()` |
| `DHCPLeaseUpdated` | `uk.org.thekelleys.dnsmasq` | dnsmasq | Re-read lease file + `GetMetrics()` |
| `Reloaded` | `org.fedoraproject.FirewallD1` | firewalld | Re-read firewall state via firewalld D-Bus method calls |
| `NameOwnerChanged` | `org.freedesktop.DBus` | D-Bus daemon | Detect service restart; trigger full re-read |

```go
func (m *DBusMonitor) subscribe(conn *dbus.Conn) error {
    // Subscribe to dnsmasq DHCP lease signals.
    if err := conn.AddMatchSignal(
        dbus.WithMatchInterface(dnsmasqInterface),
        dbus.WithMatchMember("DHCPLeaseAdded"),
    ); err != nil {
        return fmt.Errorf("dbus: match DHCPLeaseAdded: %w", err)
    }
    if err := conn.AddMatchSignal(
        dbus.WithMatchInterface(dnsmasqInterface),
        dbus.WithMatchMember("DHCPLeaseDeleted"),
    ); err != nil {
        return fmt.Errorf("dbus: match DHCPLeaseDeleted: %w", err)
    }
    if err := conn.AddMatchSignal(
        dbus.WithMatchInterface(dnsmasqInterface),
        dbus.WithMatchMember("DHCPLeaseUpdated"),
    ); err != nil {
        return fmt.Errorf("dbus: match DHCPLeaseUpdated: %w", err)
    }

    // Subscribe to firewalld reload signal.
    if err := conn.AddMatchSignal(
        dbus.WithMatchInterface(firewalldInterface),
        dbus.WithMatchMember("Reloaded"),
    ); err != nil {
        return fmt.Errorf("dbus: match Reloaded: %w", err)
    }

    // Subscribe to NameOwnerChanged for dnsmasq and firewalld restart detection.
    if err := conn.AddMatchSignal(
        dbus.WithMatchInterface(dbusInterface),
        dbus.WithMatchMember("NameOwnerChanged"),
        dbus.WithMatchArg(0, dnsmasqBusName),
    ); err != nil {
        return fmt.Errorf("dbus: match dnsmasq NameOwnerChanged: %w", err)
    }
    if err := conn.AddMatchSignal(
        dbus.WithMatchInterface(dbusInterface),
        dbus.WithMatchMember("NameOwnerChanged"),
        dbus.WithMatchArg(0, firewalldBusName),
    ); err != nil {
        return fmt.Errorf("dbus: match firewalld NameOwnerChanged: %w", err)
    }

    return nil
}
```

#### Main Run Loop with Reconnection

The D-Bus monitor follows the same reconnection pattern as the ZAPI watcher (Section 4.1octies): exponential backoff from 100ms to 30s with a 2x factor. When the D-Bus connection drops, the monitor reconnects and re-subscribes to all signals.

```go
// Run starts the D-Bus monitor. It blocks until ctx is cancelled.
// On disconnect, it reconnects with exponential backoff.
func (m *DBusMonitor) Run(ctx context.Context) error {
    delay := reconnectInitial

    for {
        conn, err := dbus.ConnectSystemBus()
        if err != nil {
            m.log.Warn("dbus: connect failed, retrying",
                "error", err, "delay", delay)
            select {
            case <-ctx.Done():
                return ctx.Err()
            case <-time.After(delay):
            }
            delay = time.Duration(math.Min(
                float64(delay)*reconnectFactor,
                float64(reconnectMax)))
            continue
        }

        // Reset backoff on successful connection.
        delay = reconnectInitial

        if err := m.subscribe(conn); err != nil {
            m.log.Warn("dbus: subscribe failed", "error", err)
            conn.Close()
            continue
        }

        m.log.Info("dbus: connected and subscribed",
            "signals", "dnsmasq(3)+firewalld(1)+nameowner(2)")

        // Perform initial data load for both services.
        m.refreshDHCP(conn)
        m.refreshFirewall(conn)

        // Process signals until disconnect.
        m.processSignals(ctx, conn)

        conn.Close()
        m.log.Warn("dbus: disconnected, reconnecting")
    }
}
```

#### Signal Processing

Incoming D-Bus signals are dispatched based on interface and member name. The `NameOwnerChanged` signal carries three string arguments: the bus name, the old owner, and the new owner. When the new owner is empty, the service has stopped; when the old owner is empty, the service has started.

```go
func (m *DBusMonitor) processSignals(ctx context.Context, conn *dbus.Conn) {
    ch := make(chan *dbus.Signal, 32)
    conn.Signal(ch)
    defer conn.RemoveSignal(ch)

    for {
        select {
        case <-ctx.Done():
            return
        case sig, ok := <-ch:
            if !ok {
                return // D-Bus connection lost
            }
            m.handleSignal(conn, sig)
        }
    }
}

func (m *DBusMonitor) handleSignal(conn *dbus.Conn, sig *dbus.Signal) {
    switch sig.Name {
    case dnsmasqInterface + ".DHCPLeaseAdded",
        dnsmasqInterface + ".DHCPLeaseDeleted",
        dnsmasqInterface + ".DHCPLeaseUpdated":
        m.log.Debug("dbus: dnsmasq lease event", "signal", sig.Name)
        m.refreshDHCP(conn)

    case firewalldInterface + ".Reloaded":
        m.log.Debug("dbus: firewalld reloaded")
        m.refreshFirewall(conn)

    case dbusInterface + ".NameOwnerChanged":
        if len(sig.Body) < 3 {
            return
        }
        name, _ := sig.Body[0].(string)
        oldOwner, _ := sig.Body[1].(string)
        newOwner, _ := sig.Body[2].(string)

        switch name {
        case dnsmasqBusName:
            if oldOwner == "" && newOwner != "" {
                m.log.Info("dbus: dnsmasq started")
                m.refreshDHCP(conn)
            } else if oldOwner != "" && newOwner == "" {
                m.log.Info("dbus: dnsmasq stopped")
                m.tree.Set(dhcpTreeKey, json.RawMessage(`{}`))
            }
        case firewalldBusName:
            if oldOwner == "" && newOwner != "" {
                m.log.Info("dbus: firewalld started")
                m.refreshFirewall(conn)
            } else if oldOwner != "" && newOwner == "" {
                m.log.Info("dbus: firewalld stopped")
                m.tree.Set(firewallTreeKey, json.RawMessage(`{}`))
            }
        }
    }
}
```

#### Data Refresh Functions

Each refresh function re-reads data from the canonical source. For DHCP, this involves two operations: parsing the lease file and querying dnsmasq metrics via a D-Bus method call. For the firewall, this queries firewalld via D-Bus method calls to retrieve zones, policies, services, and global settings.

```go
func (m *DBusMonitor) refreshDHCP(conn *dbus.Conn) {
    // 1. Re-read the lease file.
    leaseData, err := os.ReadFile(dnsmasqLeaseFile)
    if err != nil {
        m.log.Warn("dbus: read lease file", "error", err)
        return
    }
    leases := parseDnsmasqLeases(string(leaseData))

    // 2. Query dnsmasq DHCP metrics via D-Bus method call.
    obj := conn.Object(dnsmasqBusName, dnsmasqPath)
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    var metrics map[string]uint64
    if err := obj.CallWithContext(ctx, dnsmasqInterface+".GetMetrics", 0).Store(&metrics); err != nil {
        m.log.Warn("dbus: GetMetrics call failed", "error", err)
        // Continue with lease data only; metrics are supplementary.
    }

    // 3. Combine leases and metrics into YANG-compatible JSON.
    result := buildDHCPTree(leases, metrics)
    m.tree.Set(dhcpTreeKey, result)
    m.log.Debug("dbus: DHCP tree updated", "leases", len(leases))
}

func (m *DBusMonitor) refreshFirewall(conn *dbus.Conn) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    obj := conn.Object(firewalldBusName, firewalldPath)

    // 1. Query global firewall state.
    var defaultZone string
    if err := obj.CallWithContext(ctx, firewalldInterface+".getDefaultZone", 0).Store(&defaultZone); err != nil {
        m.log.Warn("dbus: firewalld getDefaultZone", "error", err)
        return
    }
    var logDenied string
    obj.CallWithContext(ctx, firewalldInterface+".getLogDenied", 0).Store(&logDenied)
    var panicMode bool
    obj.CallWithContext(ctx, firewalldInterface+".queryPanicMode", 0).Store(&panicMode)

    // 2. Query active zones and per-zone settings.
    zoneObj := conn.Object(firewalldBusName, firewalldPath)
    var activeZones map[string]interface{}
    zoneObj.CallWithContext(ctx, firewalldInterface+".zone.getActiveZones", 0).Store(&activeZones)
    zoneSettings := make(map[string]interface{})
    for name := range activeZones {
        var settings interface{}
        zoneObj.CallWithContext(ctx, firewalldInterface+".zone.getZoneSettings2", 0, name).Store(&settings)
        zoneSettings[name] = settings
    }

    // 3. Query policies.
    var policies []string
    obj.CallWithContext(ctx, firewalldInterface+".policy.getPolicies", 0).Store(&policies)
    policySettings := make(map[string]interface{})
    for _, name := range policies {
        var settings interface{}
        obj.CallWithContext(ctx, firewalldInterface+".policy.getPolicySettings", 0, name).Store(&settings)
        policySettings[name] = settings
    }

    // 4. Query services.
    var services []string
    obj.CallWithContext(ctx, firewalldInterface+".listServices", 0).Store(&services)
    serviceSettings := make(map[string]interface{})
    for _, name := range services {
        var settings interface{}
        obj.CallWithContext(ctx, firewalldInterface+".getServiceSettings2", 0, name).Store(&settings)
        serviceSettings[name] = settings
    }

    // 5. Build YANG-compatible JSON tree from all firewalld data.
    result := buildFirewallTree(defaultZone, logDenied, panicMode,
        zoneSettings, policySettings, serviceSettings)
    m.tree.Set(firewallTreeKey, result)
    m.log.Debug("dbus: firewall tree updated", "zones", len(zoneSettings),
        "policies", len(policySettings), "services", len(serviceSettings))
}
```

#### NameOwnerChanged Handling

The `NameOwnerChanged` signal from the D-Bus daemon provides service lifecycle detection without polling. When dnsmasq or firewalld restarts, the D-Bus daemon emits this signal with the bus name, old owner (empty if service just appeared), and new owner (empty if service just disappeared). This allows yangerd to:

- **Service start**: Perform a full data refresh immediately, ensuring the tree is populated even if signals were missed during the restart window.
- **Service stop**: Clear the relevant tree key, presenting an empty (but valid) subtree to RESTCONF/NETCONF clients rather than stale data.

This is analogous to the ZAPI watcher's `clearAllRoutes()` on zebra disconnect (Section 4.1octies): the tree reflects the actual service state, not cached data from a previous service instance.

#### Differences from Other Reactive Subsystems

| Aspect | NLMonitor (netlink) | ZAPI watcher | D-Bus Monitor |
|--------|-------------------|--------------|---------------|
| Implementation | Native Go netlink channels (`vishvananda/netlink`) | Native Go Unix socket (`osrg/gobgp/v4/pkg/zebra`) | Native Go D-Bus (`godbus/dbus/v5`) |
| Event source | Kernel multicast groups | Zebra redistribution messages | Userspace service signals |
| Signal semantics | Low-level (RTM_NEWLINK, etc.) | Protocol-level (route add/del) | Application-level (lease added, config reloaded) |
| Data re-read | `ip -json -batch -` subprocess | Direct from ZAPI message body | Lease file + D-Bus method call (DHCP); firewalld D-Bus method calls (firewall) |
| Failure mode | Channel close -> re-subscribe | EOF -> clear routes -> reconnect | Connection lost -> reconnect with backoff |
| Service absence | Always available (kernel) | Requires FRR zebra | Requires dnsmasq/firewalld; tree cleared when absent |
| Data exclusivity | Supplements ip batch re-reads | Sole source for routes | Sole source for DHCP leases and firewall state |

#### Concurrency Model

The D-Bus monitor runs as a single goroutine executing the `Run()` event loop. All incoming D-Bus signals are processed sequentially within this loop. The `refreshDHCP()` and `refreshFirewall()` functions are called synchronously from the signal handler. Tree writes are serialized by the per-model `sync.RWMutex` for `infix-dhcp-server:dhcp-server` and `infix-firewall:firewall` respectively. No additional synchronization is needed.

Signal processing is fast (file read + parse, or D-Bus method calls to firewalld), so sequential processing does not introduce meaningful latency. If multiple lease events arrive in rapid succession, each triggers a full re-read; this is acceptable because lease file parsing is inexpensive and the tree converges to the correct state after the final event.

### 4.1decies LLDP Monitor Subsystem

The LLDP monitor provides **reactive** LLDP neighbor updates by running a persistent `lldpcli -f json0 watch` subprocess. This replaces periodic `lldpctl -f json` polling. The monitor follows the same lifecycle pattern as `IWMonitor`: long-lived subprocess, stdout parsing loop, exponential backoff restart, and event-triggered tree replacement.

#### Command and Output Contract

- Command: `lldpcli -f json0 watch` (**`-f json0` before `watch`**)
- Output framing: pretty-printed JSON objects separated by a blank line (`\n\n`)
- Event roots: `lldp-added`, `lldp-updated`, `lldp-deleted`
- Payload: each event object contains full neighbor data (not a delta patch)
- `json0` guarantees stable structure (arrays stay arrays)

Unlike NDJSON, each event is multi-line JSON. Framing must therefore use blank-line delimiters or brace-depth counting; single-line splitting is incorrect.

#### Framing Strategy

`internal/lldpmonitor/monitor.go` reads stdout as a stream and accumulates bytes until an object boundary is detected:

1. Preferred: split on `\n\n` (lldpcli watch object separator)
2. Defensive fallback: brace-depth counter for malformed/partial separators
3. Parse each complete object via `json.Unmarshal`
4. Dispatch by root key (`lldp-added` / `lldp-updated` / `lldp-deleted`)

Each event triggers full in-memory LLDP subtree regeneration for `ieee802-dot1ab-lldp:lldp` from the watch payload, preserving list shape and RFC7951 key structure.

#### Failure and Restart Behavior

If `lldpd` is restarted or the subprocess exits, LLDPMonitor logs WARN, restarts `lldpcli -f json0 watch` with exponential backoff (100ms → 30s, factor 2x), and rebuilds state from subsequent watch events. During restart windows, the previous LLDP subtree remains served as last-known-good data.

### 4.1undecies mDNS Monitor Subsystem

The mDNS monitor provides **reactive** updates for `/infix-services:mdns/neighbors` using Avahi's D-Bus API (`org.freedesktop.Avahi`). This is a migration of `src/statd/avahi.c` behavior into pure Go.

#### Why D-Bus (not libavahi-client)

`yangerd` is pure Go (no CGo), so linking `libavahi-client` is not allowed. Avahi already exposes complete browsing/resolution via D-Bus signals and objects:

- `ServiceTypeBrowser`
- `ServiceBrowser`
- `ServiceResolver`

`internal/mdnsmonitor/` uses `godbus/dbus/v5` (already present for DBusMonitor) to subscribe to Avahi events and resolve service instances.

#### Data Model Mapping

The monitor writes:

- Path: `/infix-services:mdns/neighbors`
- Keys: `neighbor/hostname`, nested `service/name`
- Leaves: `hostname`, `address` (leaf-list), `last-seen`, `service/name`, `service/type`, `service/port`, `service/txt` (leaf-list)

On add/update/remove signals, the affected neighbor/service entries are rebuilt and the `infix-services:mdns` subtree is atomically replaced.

#### Alternative Considered

Pure Go mDNS libraries (`hashicorp/mdns`, `brutella/dnssd`) are possible, but Avahi D-Bus is preferred because Avahi is already running on target systems and is the canonical system mDNS authority.

### 4.1septies Event-Triggered Batch Re-read Pattern (All Netlink Events)

This section documents the unified pattern used by all netlink event handlers for **link, address, and neighbor** events: **every event (both add and remove) triggers a full re-read of the affected state via ip batch**. Events are received as typed Go structs on `vishvananda/netlink` channels (`LinkUpdate`, `AddrUpdate`, `NeighUpdate`). The event itself is used only as a trigger -- its content is not parsed for data. Route data is sourced exclusively from the ZAPI watcher's streaming connection to zebra (Section 4.1octies) and is not part of this pattern -- yangerd does not subscribe to netlink route groups. This design is driven by two observations:

1. **Partial state updates lead to inconsistency.** If the event handler only queries the single attribute that changed (e.g., oper-status from an RTM_NEWLINK, or one address from an RTM_NEWADDR), other attributes of the same entity may be from a different point in time. By re-reading the full state for the affected scope, all data in the tree is coherent.

2. **Delete events require re-reading, not surgical removal.** Parsing RTM_DEL* events to determine exactly which subtree entry to remove is complex and fragile. Instead, re-reading the full state after a delete naturally produces the correct result without the deleted entry.

For link events specifically, there is a third driver:

3. **The ethtool genetlink monitor (`ETHNL_MCGRP_MONITOR`) does NOT fire on link up/down.** When a physical link transitions (cable plugged/unplugged, carrier lost/restored), the kernel negotiates speed, duplex, and auto-negotiation with the link partner. However, this negotiation does not produce `ETHTOOL_MSG_LINKINFO_NTF` or `ETHTOOL_MSG_LINKMODES_NTF` messages. The link handler must explicitly re-query ethtool settings.

When the NLMonitor's select loop receives a netlink event from any subscription channel, the re-read scope depends on the event type:

```
=== Link Event (RTM_NEWLINK / RTM_DELLINK) ===
Step 1: Write three queries to ip batch stdin (full interface re-read)
   link show dev <iface>              -> link state (flags, MTU, operstate, qdisc, ...)
   -s link show dev <iface>           -> link state + hardware counters (rx/tx bytes/packets/errors)
   addr show dev <iface>              -> all IPv4/IPv6 addresses on this interface
Step 2: Read three JSON array responses from ip batch stdout
Step 3: tree.Set("/ietf-interfaces:.../interface[name='<iface>']", linkData)
        tree.Set(".../<iface>/statistics", statsData)
        tree.Set(".../<iface>/addresses", addrData)
Step 4: ethmonitor.RefreshInterface("<iface>") -> re-query speed/duplex/autoneg
Step 5: If oper-status changed: record time.Now() as last-change

=== Address Event (RTM_NEWADDR / RTM_DELADDR) ===
Step 1: Write one query to ip batch stdin
   addr show dev <iface>              -> all addresses on this interface
Step 2: Read one JSON array response
Step 3: tree.Set(".../<iface>/addresses", addrData)

=== Neighbor Event (RTM_NEWNEIGH / RTM_DELNEIGH) ===
Step 1: Write one query to ip batch stdin
   neigh show dev <iface>             -> all neighbors on this interface
Step 2: Read one JSON array response
Step 3: tree.Set(".../<iface>/neighbors", neighData)
```

#### Why Full Re-read Instead of Targeted Queries

The alternative — parsing the event content to extract the changed data and applying it surgically to the tree — has three problems:

1. **Netlink events carry typed Go structs, not raw data.** The `vishvananda/netlink` channels deliver `LinkUpdate`, `AddrUpdate`, `NeighUpdate` structs. While these contain parsed netlink attributes, they do not reliably indicate which fields changed. RTM_NEWLINK fires for many reasons (oper-status, MTU, flags, master, alias). RTM_NEWADDR/RTM_DELADDR carry the affected address, but the full address set may have other concurrent changes (e.g., IPv6 DAD state transitions). A full re-read is more reliable than trying to reconstruct state from individual update structs.

2. **Point-in-time consistency.** A full re-read ensures all data for the affected scope (interface, address set, neighbor table) is from a single coherent point in time.

3. **Simplicity.** Batch queries are cheap (microseconds over a local stdin/stdout pipe). The complexity of transforming each netlink update struct into a partial tree mutation and applying it surgically would be significantly higher and more fragile than a blanket re-read. Delete handling is especially simplified -- no need to construct the exact tree key from event attributes.
#### Event Rate and Debouncing

On a typical Infix system, netlink events arrive at single-digit rates per second under normal operation. During convergence events (e.g., STP topology change, link aggregation failover), rates can spike to hundreds per second. Since events arrive on Go channels rather than subprocess stdout, channel buffer capacity provides implicit backpressure. For link/addr/neigh events, the batch re-read approach generates at most 3 ip batch queries per event (for link events; 1 for addr/neigh), which is well within the capacity of the persistent `ip -json -force -batch -` subprocess.

If event storms are detected (e.g., the same interface generating multiple events of the same type within a 10ms window), per-entity debouncing is applied: only the last event in the window triggers a re-read. Debouncing is per-interface for link, addr, and neigh events.

#### Interaction with Other Subsystems

| Event Type | ip batch Queries | Additional Triggers | Debounce Key |
|------------|-----------------|---------------------|--------------|
| RTM_NEWLINK / RTM_DELLINK | 3 (link + stats + addr) | `ethmonitor.RefreshInterface()` + last-change | per-interface |
| RTM_NEWADDR / RTM_DELADDR | 1 (addr show dev) | None | per-interface |
| RTM_NEWNEIGH / RTM_DELNEIGH | 1 (neigh show dev) | None | per-interface |

Subsystems NOT affected by the NLMonitor's netlink channels:
- **bridge batch** -- bridge state queries use a separate `bridge -json -batch -` subprocess; bridge events arrive on the NLMonitor's existing channels (FDB via `neighCh`, VLAN via `linkCh`, MDB via raw netlink)
- **iw event** -- WiFi events use a separate `iw event` subprocess; unrelated to the NLMonitor's netlink channels
- **fswatcher** -- file events are independent of netlink
- **ethmonitor** -- has its own genetlink subscription; only cross-triggered by link events via `RefreshInterface()`
- **ZAPI watcher** -- route data is sourced from zebra's zserv socket via the ZAPI watcher subsystem (Section 4.1octies); independent of NLMonitor's netlink channels
### 4.2 In-Memory Data Tree

#### 4.2.1 Design Rationale
- **Pre-serialized JSON:** Trading write-time CPU for zero-allocation, zero-copy reads.
- **Subtree Replacement:** Each update replaces only the affected module's JSON blob.
- **Per-Model RWMutex:** Each YANG module key has its own `sync.RWMutex`, so writers for different modules never block each other and readers only contend with writers of the same module. A top-level `sync.RWMutex` protects the models map structure itself (new key insertion).

#### 4.2.2 Core Tree Type
```go
// internal/tree/tree.go

// modelEntry holds a single YANG module's pre-serialized JSON blob
// and its own read-write mutex.
type modelEntry struct {
    mu      sync.RWMutex
    data    json.RawMessage
    updated time.Time
}

// Tree holds the operational YANG data in per-module JSON blobs.
// Each module key has its own sync.RWMutex, so writers for different
// modules never block each other.
// All methods are safe for concurrent use.
type Tree struct {
    mu     sync.RWMutex               // protects the models map itself
    models map[string]*modelEntry
}

func New() *Tree {
    return &Tree{models: make(map[string]*modelEntry)}
}

// Set replaces the entire subtree at the given YANG module key.
// Only the target module's write lock is held; other modules remain
// readable and writable.
func (t *Tree) Set(key string, v json.RawMessage) {
    t.mu.RLock()
    entry, ok := t.models[key]
    t.mu.RUnlock()
    if !ok {
        t.mu.Lock()
        entry, ok = t.models[key]
        if !ok {
            entry = &modelEntry{}
            t.models[key] = entry
        }
        t.mu.Unlock()
    }
    entry.mu.Lock()
    entry.data = v
    entry.updated = time.Now()
    entry.mu.Unlock()
}

// Get returns the raw JSON for the given module key.
// Only the target module's read lock is held.
func (t *Tree) Get(key string) json.RawMessage {
    t.mu.RLock()
    entry, ok := t.models[key]
    t.mu.RUnlock()
    if !ok {
        return nil
    }
    entry.mu.RLock()
    defer entry.mu.RUnlock()
    return entry.data
}

// GetMulti returns the concatenated raw JSON for multiple module keys.
// Each module's read lock is acquired and released individually.
// Lock ordering safety: the top-level RLock is held for the iteration
// (preventing map mutation), then each modelEntry.mu.RLock is acquired
// and released inline. This is deadlock-free because Set() never holds
// both the top-level WLock and a modelEntry.mu.Lock simultaneously
// (it uses double-checked locking with release-then-reacquire).
func (t *Tree) GetMulti(keys []string) []json.RawMessage {
    result := make([]json.RawMessage, 0, len(keys))
    t.mu.RLock()
    defer t.mu.RUnlock()
    for _, key := range keys {
        if entry, ok := t.models[key]; ok {
            entry.mu.RLock()
            result = append(result, entry.data)
            entry.mu.RUnlock()
        }
    }
    return result
}
```

**Consistency note**: `GetMulti()` acquires each model's read lock individually within a single pass. A response spanning multiple modules (e.g., `ietf-interfaces` and `ietf-routing`) may reflect different points in time — this is eventual consistency, not snapshot isolation. This is an explicit design choice: operational data is inherently a best-effort snapshot of continuously changing system state, and the cost of a global read lock across all models would introduce contention between unrelated data sources. For single-model queries (the common case via statd), the response is always self-consistent.


#### 4.2.3 Update Strategy
Each monitor maintains its own in-memory Go struct and re-serializes the entire module subtree to JSON on each update to ensure consistency.

```go
func (m *LinkMonitor) updateTree(link netlink.Link) {
    m.mu.Lock()
    m.ifaces[link.Attrs().Name] = linkToInterface(link)
    raw, _ := json.Marshal(m.buildInterfacesTree())
    m.mu.Unlock()
    m.tree.Set("ietf-interfaces:interfaces", raw)
}
```

#### 4.2.4 Memory Bounds

The in-memory tree has no hard size cap by default — in typical deployments, the total tree size is under 1 MiB. However, to guard against pathological cases (e.g., an extremely large routing table or a runaway collector producing oversized JSON), the following safeguards apply:

- **Per-model size limit**: Each `tree.Set()` call checks the size of the incoming `json.RawMessage`. If it exceeds `YANGERD_MAX_MODEL_BYTES` (default: 16 MiB), the update is rejected, the previous value is retained, and a warning is logged. This prevents a single collector from consuming unbounded memory.
- **Total tree size monitoring**: The health endpoint reports `size_bytes` per model and the aggregate total. Operators can monitor this via `yangerctl health` or automated checks.
- **No backpressure to kernel**: Netlink events are never dropped intentionally by yangerd (the kernel drops on ENOBUFS). Tree writes are fast (mutex + pointer swap), so memory pressure does not create backpressure in the event pipeline.


### 4.3 IPC Protocol Specification

#### 4.3.1 Transport
`AF_UNIX SOCK_STREAM` at `/run/yangerd.sock`. Permissions `0660`, owned by `root:yangerd`.

#### 4.3.2 Framing
1-byte protocol version + 4-byte big-endian length header + JSON body. The version field enables future protocol changes without ambiguity. Version `1` is the initial release.

```
+--------+--------+--------+--------+--------+------- ... -------+
| ver(1) | length (uint32 big-endian, bytes)  |   JSON body       |
+--------+--------+--------+--------+--------+------- ... -------+
```

#### 4.3.3 Request Schema
```json
{
  "method": "get",
  "path": "/ietf-interfaces:interfaces",
  "filter": {"name": "eth0"}
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `method` | string | yes | `"get"` or `"health"` |
| `path` | string | yes (get) | YANG module-qualified path |
| `filter` | object | no | Key-value map selecting a single list entry |

**Subcommand-to-IPC mapping**: The `yangerctl` CLI subcommands map to IPC requests as follows:
- `yangerctl get <path>` → `{"method": "get", "path": "<path>"}`
- `yangerctl health` → `{"method": "health"}`
- `yangerctl dump` → `{"method": "get", "path": "/"}` (root path returns all models)
- `yangerctl watch <path>` → Client-side polling loop: repeated `{"method": "get", "path": "<path>"}` requests at 1-second intervals with client-side diff. There is no server-side subscription or push mechanism.

#### 4.3.4 Response Schema
**Success:**
```json
{"status": "ok", "data": { "module:node": { ... } }}
```

**Error:**
```json
{"status": "error", "code": 404, "message": "..."}
```

#### 4.3.5 Health Response Schema

The `health` method returns per-subsystem status and per-model freshness data:

```json
{
  "status": "ok",
  "subsystems": {
    "nlmonitor":    {"state": "running", "restarts": 0},
    "ipbatch":      {"state": "running", "pid": 1234, "restarts": 0},
    "bridgebatch":  {"state": "restarting", "pid": null, "restarts": 2, "backoff_ms": 400},
    "zapiwatcher":  {"state": "running", "restarts": 0},
    "ethmonitor":   {"state": "running"},
    "fswatcher":    {"state": "running", "watches": 12},
    "dbusmonitor":  {"state": "running"},
    "iwmonitor":    {"state": "disabled"}
  },
  "models": {
    "ietf-interfaces:interfaces":  {"last_updated": "2026-03-04T12:34:56Z", "size_bytes": 8192},
    "ietf-routing:routing":        {"last_updated": "2026-03-04T12:34:55Z", "size_bytes": 2048},
    "ietf-hardware:hardware":      {"last_updated": "2026-03-04T12:34:50Z", "size_bytes": 1024}
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `subsystems` | object | Per-subsystem status. Keys match internal package names. |
| `subsystems.*.state` | string | `"running"`, `"restarting"`, `"failed"`, or `"disabled"` |
| `subsystems.*.restarts` | int | Cumulative restart count since daemon start |
| `subsystems.*.pid` | int/null | PID of managed subprocess (ip batch, bridge batch, iw event); null during restart |
| `subsystems.*.backoff_ms` | int | Current backoff delay in milliseconds (only present during restart) |
| `subsystems.*.watches` | int | Number of active inotify watches (fswatcher only) |
| `models` | object | Per-model tree metadata. Keys are YANG module-qualified names. |
| `models.*.last_updated` | string | ISO 8601 timestamp of the last `tree.Set()` call for this model |
| `models.*.size_bytes` | int | Size in bytes of the stored `json.RawMessage` |


### 4.4 Supplementary Collectors

#### 4.4.1 Interface
```go
type Collector interface {
    Name()     string
    Interval() time.Duration
    Collect(ctx context.Context, tree *tree.Tree) error
}
```

#### 4.4.2 Failure Philosophy
- Never panic.
- Log at WARN.
- Retain stale data.

**Exceptions — intentional clearing**: Two subsystems intentionally clear their tree keys instead of retaining stale data:
- **ZAPI watcher** (routes): On zebra disconnect, the route subtree is cleared immediately. Stale routes from a previous zebra session could direct traffic to non-existent next-hops; serving no routes is safer than serving wrong routes. Routes are repopulated atomically on reconnect via a full RIB dump.
- **D-Bus monitor** (DHCP, firewall): When dnsmasq or firewalld stops (detected via `NameOwnerChanged`), the corresponding tree key is set to `{}`. A stopped service has no active leases or rules; retaining data from the previous instance would misrepresent the system state.

In all other failure modes (collector timeouts, parse errors, subprocess restarts), stale data is retained.

#### 4.4.3 Detailed Collector Specifications

The following collectors handle operational data not exposed via Linux netlink multicast groups and not handled by the bridge reactive subsystem (Section 4.4.3 item 1). Each collector runs in its own goroutine on a fixed polling interval.

##### 1. Bridge Data (Reactive via Netlink + `bridge -json -batch -`)

Bridge data collection is **fully reactive** — there is no polling collector for bridge state. All bridge data updates are driven by kernel netlink events that trigger re-queries via the persistent `bridge -json -batch -` subprocess. This follows the same event-as-trigger pattern used for link, address, and neighbor data.

**FDB (Forwarding Database)**: FDB entries arrive as `NeighUpdate` events on the neighbor channel (entries with `NDA_MASTER` flag are bridge FDB, not ARP/NDP). Each event triggers `fdb show br <bridge>` via bridge batch to re-read the full FDB for the affected bridge.

**VLAN membership**: VLAN changes arrive as `LinkUpdate` events on the link channel (bridge VLAN attributes on link update messages). Each event triggers `vlan show` via bridge batch.

**MDB (Multicast Database)**: MDB events (`RTM_NEWMDB`, `RTM_DELMDB`) arrive via a raw netlink socket subscribed to `RTNLGRP_MDB` (group 26). Each event triggers `mdb show` via bridge batch.

**STP port state**: STP port state changes arrive as `RTM_NEWLINK` events carrying `IFLA_BRPORT_STATE` in `IFLA_PROTINFO`. The link event handler detects bridge port events and triggers a bridge batch re-query. STP root and topology-change data are not proactively notified by the kernel (`br_root_selection()` does not call `br_ifinfo_notify`), so these are re-read from the bridge device via batch whenever a port state change event is received.

**Data source**: `bridge -json -batch -` (persistent subprocess) — commands written to stdin include `fdb show br <bridge>`, `vlan show`, `mdb show`, and per-bridge STP state queries.
**Failure behavior**: Log warning; retain stale bridge data in tree (except on persistent `bridge` subprocess crash, where data is cleared after 3 restart attempts).
**Writes to**: `ietf-interfaces:interfaces`.
##### 2. WiFi Collector (`internal/collector/wifi.go`) — Feature-Gated
**Collects**: SSID, BSSID, channel, frequency (MHz), bitrate (Mbps), signal strength (dBm), RX/TX speed, scan results, and a list of associated stations with per-station TX/RX statistics.
**Sources**:
- `exec iw dev <iface> info` — interface-level parameters (SSID, channel, frequency, interface mode AP/station)
- `exec iw dev <iface> link` (via `iw.py link <ifname>`) — station-mode link info including **signal strength in dBm**, connected SSID, RX/TX speed. This is the **only reliable source** for WiFi signal strength on modern cfg80211/nl80211 drivers; `/proc/net/wireless` is empty on these drivers.
- `exec iw dev <iface> station dump` — per-station statistics (AP mode: connected clients; station mode: single entry with detailed stats)
- `exec wpa_cli -i <iface> scan_result` — available network scan results from wpa_supplicant (station mode only)
**Interval**: 10 seconds for polling path; reactive re-queries on `iw event` triggers (`connected`, `disconnected`, `new station`, `ch_switch_started_notify`).
**Failure behavior**: Log warning; write an empty station list. Common failure causes: interface is down, or the interface is not a wireless interface. Virtual interfaces return `ENODEV` from `iw`; these are silently skipped. (Note: On `iw event` monitor disconnection, the WiFi subtree is NOT cleared — stale link data is retained).
**Writes to**: `ietf-interfaces:interfaces`.
**Feature gate**: `YANGERD_ENABLE_WIFI=true`. When WiFi support is not included in the Infix build, this collector and the IW Event Monitor are not started. When enabled, `iw` and `wpa_cli` are guaranteed present on the target.

##### 3. Ethtool Collector (`internal/collector/ethtool.go`) — Hybrid Reactive/Polling
**Collects**: Link speed (Mbps), duplex mode (`half`/`full`), auto-negotiation state (`enabled`/`disabled`), advertised link modes, and extended per-group hardware statistics (eth-mac, rmon counters).
**Sources**: A hybrid of two mechanisms:
- **Reactive (settings)**: The `internal/ethmonitor/` package subscribes to the kernel's `ETHNL_MCGRP_MONITOR` genetlink multicast group. When the kernel emits `ETHTOOL_MSG_LINKINFO_NTF` or `ETHTOOL_MSG_LINKMODES_NTF` notifications (e.g., after link renegotiation), the ethmonitor re-queries speed, duplex, and auto-negotiation via `ethtool.Client.LinkInfo()` and `ethtool.Client.LinkMode()` and writes the updated values to the tree immediately.
- **Polling (statistics)**: Hardware counters (FramesTransmittedOK, FrameCheckSequenceErrors, OctetsReceivedOK, etc.) have no kernel notification mechanism — there is no `ETHTOOL_MSG_STATS_NTF`. These are polled every 30 seconds via `ethtool.Client.Stats()`.
**Interval**: Polling at 30 seconds for statistics only. Speed, duplex, and auto-negotiation are updated reactively via ethmonitor (no polling needed).
**Failure behavior**: Virtual interfaces, tunnel interfaces, and loopback return `ENOTSUP` from the ethtool generic netlink family. These are silently skipped — no warning is logged for `ENOTSUP`. Unexpected errors (permission denied, kernel bug) are logged at WARN.
**Writes to**: `ietf-interfaces:interfaces` — `infix-ethernet-interface` augment subtrees under each physical Ethernet interface.

##### 4. WireGuard Collector (`internal/collector/wireguard.go`)
**Collects**: Per-peer statistics for all WireGuard interfaces: public key, endpoint IP:port, allowed IPs, time of latest handshake, received bytes, and transmitted bytes.
**Sources**: `golang.zx2c4.com/wireguard/wgctrl` — reads via WireGuard generic netlink (`WG_CMD_GET_DEVICE`) without requiring the `wg` CLI tool.
**Interval**: 30 seconds.
**Failure behavior**: If the WireGuard kernel module is not loaded, `wgctrl.New()` returns an error at daemon startup and the collector is disabled. If the module is loaded but a specific interface has been deleted between polls, log at WARN and skip that interface.
**Writes to**: `ietf-interfaces:interfaces`.

##### 5. Route Table Collector (`internal/zapiwatcher/`) -- Reactive via ZAPI Streaming
**Collects**: Complete IPv4 and IPv6 routing tables (RIBs) from FRRouting, including all route types: kernel, connected, static, OSPF-learned, RIP-learned. Each route includes destination prefix, source protocol, administrative distance, metric, next-hops (with outgoing interface and gateway address), and active/installed flags. Includes routes in zebra's RIB that are NOT installed in the Linux kernel FIB (unresolvable next-hops, routes that lost admin-distance election, ECMP overflow, table-map filtered).
**Sources**:
- ZAPI v6 streaming connection to zebra via `/var/run/frr/zserv.api` Unix domain socket
- `REDISTRIBUTE_ROUTE_ADD` and `REDISTRIBUTE_ROUTE_DEL` messages from zebra for subscribed route types (kernel, connected, static, OSPF, RIP)
**Trigger**: Streaming -- no trigger needed. The ZAPI watcher receives incremental route updates as they occur in zebra's RIB. Upon initial connection, zebra sends a full dump of all routes matching the subscribed redistribution types. This replaces the previous `vtysh`-based approach where netlink route events (RTM_NEWROUTE/RTM_DELROUTE) were used as triggers for `vtysh` re-reads. See Section 4.1octies for the full ZAPI watcher design.
**Initial startup**: The ZAPI watcher connects to zebra and subscribes to redistribution. Zebra responds with a full dump of all matching routes, populating the tree before the NLMonitor's select loop begins processing events.
**Failure behavior**: If zebra is not running (socket absent), the watcher retries with exponential backoff (100ms initial, 30s max). Routes are cleared from the tree immediately upon ZAPI disconnection to prevent serving stale routing data. On reconnect, the full RIB dump repopulates the subtree.
**Writes to**: `ietf-routing:routing/ribs` (shared tree — routes only; ARP/NDP neighbors under `ietf-routing:routing` are written by the NLMonitor's neighbor handler, and forwarding flags are written by the fswatcher).

##### 5b. FRR Protocol Collectors (`internal/collector/ospf.go`, `rip.go`, `bfd.go`)
**Collects**: OSPF neighbor state/adjacency, RIP full route table with metrics, BFD session state/peer address.
**Sources**:
- `exec vtysh -c 'show ip ospf json'` and `vtysh -c 'show ip ospf neighbor json'`
- `exec vtysh -c 'show ip rip json'`
- `exec vtysh -c 'show bfd peers json'`
**Interval**: 10 seconds for all three. Protocol state machines can transition quickly (OSPF adjacency flap, BFD session down); 10 seconds balances responsiveness with `vtysh` execution overhead.
**Failure behavior**: If FRRouting is not running, write empty structures for the relevant subtrees. Log at ERROR on first failure; suppress to DEBUG for subsequent identical failures. (Note: Unlike the ZAPI watcher, protocol-specific state is cleared immediately when `vtysh` returns an error).
**Writes to**: `ietf-routing:routing/control-plane-protocols/control-plane-protocol` (OSPF under `.../ietf-ospf:ospf`, RIP under `.../ietf-rip:rip`, BFD under `.../ietf-bfd:bfd/ietf-bfd-ip-sh:...`).

##### 6. Hardware Collector (`internal/collector/hardware.go`)
**Collects**: Temperature readings, fan speeds, voltage rail readings from kernel hwmon drivers; chassis inventory (manufacturer, model, serial number) from DMI.
**Sources**:
- `/sys/class/hwmon/hwmon*/temp*_input`, `fan*_input`, `in*_input`, `temp*_fault`
- `exec dmidecode -t system` — chassis manufacturer, product name, serial number
**Intervals**: 10 seconds for sensor readings; 300 seconds for DMI inventory.
**Failure behavior**: If a hwmon path does not exist, the path is silently skipped.
**Writes to**: `ietf-hardware:hardware`.

##### 7. System Collector (`internal/collector/system.go`)
**Collects**: Hostname, OS distribution name and version, kernel release string, system uptime, boot timestamp, and current active user sessions.
**Sources**:
- `/proc/uptime`, `/etc/os-release`
- `exec uname -r`, `exec who -H`
- `time.Now()` combined with `/proc/uptime` to compute boot timestamp
**Interval**: 60 seconds. Hostname and OS release are effectively static.
**Failure behavior**: Individual source failures are logged at WARN; the collector writes whatever fields could be collected successfully.
**Writes to**: `ietf-system:system-state`.

##### 8. NTP Collector (`internal/collector/ntp.go`)
**Collects**: Synchronization status, reference server address, clock offset (seconds), stratum, and RMS jitter from chrony.
**Sources**:
- chrony cmdmon protocol v6 over Unix socket (`/var/run/chrony/chronyd.sock`) -- tracking request (synchronization state, stratum, refid, offset, root delay/dispersion, frequency, leap status)
- chrony cmdmon protocol v6 over Unix socket -- sources request (configured NTP source list with mode, state, address, stratum, poll interval, reachability)

Uses `github.com/facebook/time/ntp/chrony` to speak the cmdmon protocol natively in Go, eliminating `exec chronyc` subprocess spawning. The protocol is strictly request-response (no subscription/push mode exists); polling is the only supported monitoring approach.
**Interval**: 60 seconds (configurable via `YANGERD_POLL_INTERVAL_NTP`).
**Failure behavior**: If chrony is not running (Unix socket absent or connection refused), write `synchronized: false` with an empty source list and log at WARN.
**Writes to**: `ietf-ntp:ntp`.

##### 9. LLDP Monitor (`internal/lldpmonitor/`) — Reactive Subprocess
**Collects**: Per-port LLDP neighbor information: chassis ID, port ID, TTL, system name, system capabilities, and management addresses.
**Sources**: Persistent `exec lldpcli -f json0 watch` subprocess. Output consists of pretty-printed JSON objects separated by blank lines, rooted at `lldp-added`, `lldp-updated`, or `lldp-deleted`.
**Trigger**: Event-driven by `lldpd` watch output (no fixed polling interval).
**Framing**: Blank-line split (`\n\n`) with brace-depth fallback; **not** NDJSON line parsing.
**Failure behavior**: If `lldpd`/`lldpcli` is unavailable, monitor restarts with exponential backoff and serves last-known-good LLDP subtree until events resume.
**Writes to**: `ieee802-dot1ab-lldp:lldp`.

##### 10. DHCP Collector (`internal/collector/dhcp.go`) — Removed (D-Bus Reactive)
**Status**: This collector has been removed. DHCP lease data is now collected reactively by the D-Bus Monitor Subsystem (Section 4.1novies).
**Previously**: Polled `/var/lib/misc/dnsmasq.leases` at 30-second intervals.
**Now**: The D-Bus Monitor subscribes to dnsmasq signals (`DHCPLeaseAdded`, `DHCPLeaseDeleted`, `DHCPLeaseUpdated`). On each signal, it re-reads the lease file and calls `GetMetrics()` via D-Bus method call.
**Writes to**: `infix-dhcp-server:dhcp-server` (via D-Bus Monitor, not collector loop).

##### 11. Firewall Collector (`internal/collector/firewall.go`) — Removed (D-Bus Reactive)
**Status**: This collector has been removed. Firewall data is now collected reactively by the D-Bus Monitor Subsystem (Section 4.1novies).
**Previously**: Polled `exec nft list ruleset -j` at 30-second intervals.
**Now**: The D-Bus Monitor subscribes to firewalld signals (`Reloaded`, plus `NameOwnerChanged` for restart detection). On each signal, it re-reads the full firewall state via firewalld D-Bus method calls (`getDefaultZone()`, `getActiveZones()`, `getZoneSettings2()`, `getPolicies()`, `getPolicySettings()`, `listServices()`, `getServiceSettings2()`, `getLogDenied()`, `queryPanicMode()`).
**Writes to**: `infix-firewall:firewall` (via D-Bus Monitor, not collector loop).

##### 12. Container Collector (`internal/collector/containers.go`) — Phase 2, Feature-Gated
**Collects**: Running container names, image references, state, and creation timestamps.
**Sources**: `exec podman ps --format json`, `exec podman inspect --format json`.
**Interval**: 10 seconds.
**Failure behavior**: Log at WARN. Container-internal interface statistics require more complex namespace traversal, deferred to Phase 2.
**Writes to**: `infix-containers:containers`.
**Feature gate**: `YANGERD_ENABLE_CONTAINERS=true`. When container support is not included in the Infix build, the Buildroot recipe sets this to `false` and the container collector is not started. When enabled, `podman` is guaranteed present on the target.

**Phase-2 reactive recommendation**: container lifecycle state (`create`, `start`, `stop`, `die`, `remove`) can be made reactive via a persistent `podman events --format json` subscription. This would eliminate lifecycle polling lag and keep polling only for runtime metrics (CPU/memory), which still require periodic sampling.

**Phase-2 container namespace design**: Collecting per-container network interface statistics requires entering each container's network namespace to read `/sys/class/net/*/statistics/` or query netlink. The planned approach uses `netns.Set()` from `vishvananda/netns` to switch the calling goroutine's network namespace, perform the queries, and switch back. Because Go goroutines can migrate between OS threads, the goroutine must be locked to its OS thread via `runtime.LockOSThread()` before the namespace switch. Each container's statistics are collected in a dedicated goroutine to prevent namespace leaks from affecting other collectors. Container namespace enumeration uses `podman inspect --format '{{.State.Pid}}'` to obtain the container's PID, from which `/proc/<pid>/ns/net` provides the network namespace file descriptor.
### 4.5 statd Integration

`yangerd.c` / `yangerd.h` helper file implementing the IPC client, and (b) a modified
`ly_add_yangerd_data()` function in `statd.c` that calls the helper first and falls back
to the existing `fsystemv()` path when yangerd is unavailable.

### Current Code Path (statd.c)

The function being replaced is `ly_add_yanger_data()` (lines 76–120 of `statd.c` at the
time of writing). It allocates a `memfd`, wraps it in a `FILE *` stream, calls
`fsystemv(yanger_args, NULL, stream, NULL)` to fork-and-exec the yanger Python interpreter
with stdout redirected to the memfd, rewinds with `lseek()`, and parses the result with
`lyd_parse_data_fd()`:

```c
/* Current implementation (abbreviated) */
static int ly_add_yanger_data(const struct ly_ctx *ctx, struct lyd_node **parent,
			      char *yanger_args[])
{
	FILE *stream;
	int err, fd;

	fd = memfd_create("yanger_tmpfile", MFD_CLOEXEC | MFD_NOEXEC_SEAL);
	stream = fdopen(fd, "w+");
	err = fsystemv(yanger_args, NULL, stream, NULL);  /* fork + exec yanger */
	fflush(stream);
	lseek(fd, 0, SEEK_SET);
	err = lyd_parse_data_fd(ctx, fd, LYD_JSON, LYD_PARSE_ONLY, 0, parent);
	fclose(stream);
	return err;
}
```

### New yangerd.c Helper

A new file `src/statd/yangerd.c` (with corresponding `yangerd.h`) implements the IPC
client. It follows the same style as `gpsd.c`: a module-static fd, a `connect` function,
and a `query` function. Unlike `gpsd.c` (which uses non-blocking I/O and `ev_io`),
`yangerd.c` uses blocking I/O with a `SO_RCVTIMEO` timeout because statd calls it
synchronously from within a sysrepo callback.

```c
/* SPDX-License-Identifier: BSD-3-Clause */

/*
 * yangerd.c - yangerd IPC client for statd.
 *
 * Maintains a persistent AF_UNIX SOCK_STREAM connection to /run/yangerd.sock.
 * yangerd_query() returns a malloc'd JSON string on success (caller must free),
 * or NULL if yangerd is unavailable -- statd falls back to fsystemv() / yanger.
 */

#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <errno.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <arpa/inet.h>

#include <srx/common.h>

#include "yangerd.h"

#define YANGERD_SOCK_PATH "/run/yangerd.sock"
#define YANGERD_TIMEOUT_MS 50
#define YANGERD_MAX_RESP   (4 * 1024 * 1024)
#define YANGERD_VERSION    1

static int yangerd_fd = -1;   /* persistent connection fd */

static int yangerd_connect(void)
{
	struct timeval tv = { .tv_sec = 0, .tv_usec = YANGERD_TIMEOUT_MS * 1000 };
	struct sockaddr_un addr = {
		.sun_family = AF_UNIX,
		.sun_path   = YANGERD_SOCK_PATH,
	};
	int fd;

	fd = socket(AF_UNIX, SOCK_STREAM, 0);
	if (fd < 0)
		return -1;

	if (connect(fd, (struct sockaddr *)&addr, sizeof(addr)) < 0) {
		close(fd);
		return -1;
	}

	/* Enforce read timeout so a stalled yangerd doesn't block statd */
	if (setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &tv, sizeof(tv)) < 0) {
		close(fd);
		return -1;
	}

	yangerd_fd = fd;
	DEBUG("yangerd: connected");
	return 0;
}

/*
 * yangerd_query - query yangerd for operational data at @path.
 *
 * Returns malloc'd JSON string (RFC 7951 fragment) on success; caller must
 * free().  Returns NULL if yangerd is unavailable, timed out, or returned an
 * error status -- the caller should return SR_ERR_INTERNAL.
 */
char *yangerd_query(const char *path)
{
	uint32_t len;
	uint8_t ver;
	char req[512], *resp;
	ssize_t n, total;

	if (yangerd_fd < 0 && yangerd_connect() < 0)
		return NULL;

	snprintf(req, sizeof(req),
		 "{\"method\":\"get\",\"path\":\"%s\"}", path);

	ver = YANGERD_VERSION;
	len = htonl((uint32_t)strlen(req));
	if (write(yangerd_fd, &ver, 1) != 1 ||
	    write(yangerd_fd, &len, 4) != 4 ||
	    write(yangerd_fd, req, strlen(req)) != (ssize_t)strlen(req)) {
		DEBUG("yangerd: write failed: %s", strerror(errno));
		close(yangerd_fd);
		yangerd_fd = -1;
		return NULL;
	}

	if (read(yangerd_fd, &ver, 1) != 1 || ver != YANGERD_VERSION) {
		DEBUG("yangerd: read version failed or mismatch (got %u)", ver);
		close(yangerd_fd);
		yangerd_fd = -1;
		return NULL;
	}

	if (read(yangerd_fd, &len, 4) != 4) {
		DEBUG("yangerd: read header failed: %s", strerror(errno));
		close(yangerd_fd);
		yangerd_fd = -1;
		return NULL;
	}
	len = ntohl(len);
	if (len == 0 || len > YANGERD_MAX_RESP) {
		ERROR("yangerd: bad response length %u", len);
		close(yangerd_fd);
		yangerd_fd = -1;
		return NULL;
	}

	resp = malloc(len + 1);
	if (!resp)
		return NULL;

	/* Read body in a loop to handle partial reads on Unix sockets */
	total = 0;
	while (total < (ssize_t)len) {
		n = read(yangerd_fd, resp + total, len - total);
		if (n <= 0) {
			DEBUG("yangerd: read body failed (got %zd/%u): %s",
			      total, len, strerror(errno));
			free(resp);
			close(yangerd_fd);
			yangerd_fd = -1;
			return NULL;
		}
		total += n;
	}
	resp[len] = '\0';
	return resp;
}

void yangerd_close(void)
{
	if (yangerd_fd >= 0) {
		close(yangerd_fd);
		yangerd_fd = -1;
	}
}
```

### Modified ly_add_yangerd_data() in statd.c

`ly_add_yanger_data()` is replaced by `ly_add_yangerd_data()`, which queries yangerd
over the IPC socket. The Python yanger interpreter and `fsystemv()` fork path are removed
entirely -- yangerd is the sole source of operational data:

```c
/*
 * ly_add_yangerd_data - query operational data from yangerd.
 *
 * Queries yangerd over /run/yangerd.sock.  On success, the JSON response body
 * is passed to lyd_parse_data_mem() to integrate the data into the libyang tree.
 * If yangerd is unavailable (not running, timed out, error response), returns
 * SR_ERR_INTERNAL -- there is no fallback path.
 *
 * The @path argument is the YANG module-qualified path yangerd was subscribed
 * to, e.g. "/ietf-interfaces:interfaces".
 */
static int ly_add_yangerd_data(const struct ly_ctx *ctx, struct lyd_node **parent,
			       const char *path)
{
	char *json;
	int err;

	json = yangerd_query(path);
	if (!json) {
		ERROR("yangerd: query failed for %s", path);
		return SR_ERR_INTERNAL;
	}

	err = lyd_parse_data_mem(ctx, json, LYD_JSON, LYD_PARSE_ONLY, 0, parent);
	if (err)
		ERROR("yangerd: lyd_parse_data_mem failed (%d)", err);

	free(json);
	return err;
}
```

Each callback in `statd.c` that previously called `ly_add_yanger_data(ctx, parent, yanger_args)`
is updated to call `ly_add_yangerd_data(ctx, parent, XPATH_BASE)`, passing the
relevant `XPATH_*_BASE` constant as the `path` argument. The `yanger_args` parameter is
removed entirely. For `sr_iface_cb()`, which may
pass a per-interface filter, the path remains
`XPATH_IFACE_BASE` (`"/ietf-interfaces:interfaces"`) -- filter support in yangerd is
handled server-side by the optional `filter` JSON field in the request.

### Build Integration

Two source files are added to `src/statd/Makefile.am`:

```makefile
statd_SOURCES = statd.c yangerd.c yangerd.h gpsd.c gpsd.h shared.h journal.c journal.h
```

No new library dependencies are introduced. `yangerd.c` uses only POSIX headers present
in every Buildroot toolchain: `<sys/socket.h>`, `<sys/un.h>`, `<arpa/inet.h>`, and
`<unistd.h>`. The `SO_RCVTIMEO` socket option is POSIX.1-2008.

### Connection Lifecycle

statd opens one persistent connection to yangerd on first use. `yangerd_fd` is a
module-static `int` initialised to `-1`. `yangerd_query()` checks `yangerd_fd < 0` and
calls `yangerd_connect()` if needed. On any I/O error (`EPIPE`, `ECONNRESET`, short read,
timeout), the fd is closed and `yangerd_fd` is reset to `-1`. The next call will
reconnect. Reconnect failure returns `NULL` immediately — `ly_add_yangerd_data()`
returns `SR_ERR_INTERNAL` without retrying, ensuring a single failed `connect()` does not
add more than one syscall's overhead to the sysrepo callback latency.

`yangerd_close()` is called from `main()` during statd shutdown (after `unsub_to_all()`
and before `sr_disconnect()`) to close the socket cleanly.


### 4.6 yangerctl CLI

`yangerctl` is a statically-linked Go CLI tool (`cmd/yangerctl/main.go`) that connects to the yangerd Unix socket and provides human-readable access to the in-memory YANG tree. It is built from the same Go module as `yangerd` and installed to `/usr/bin/yangerctl` on the Infix target via the same Buildroot package. Because it has no CGo dependency and is statically linked, it can be copied directly to a target device for debugging without any shared library prerequisites.

`yangerctl` is intended for two use cases: interactive debug sessions on production devices (inspecting live operational state without a NETCONF client) and CI test assertions (scripted queries with `jq` to verify that yangerd is populating the correct YANG subtrees).

### Connection

`yangerctl` connects to `/run/yangerd.sock` by default. The socket path can be overridden with `--socket <path>` for local testing against a non-system yangerd instance. There is no authentication — access control is enforced entirely by Unix socket file permissions (`srw-rw---- root:yangerd`).

### Subcommands

```
yangerctl get <yang-path>      Query a YANG subtree from the in-memory tree
yangerctl health               Show daemon health status and per-collector state
yangerctl dump                 Dump the entire in-memory tree as JSON
yangerctl watch <yang-path>    Poll a path every second and print diffs (debug)
```

#### `yangerctl get <yang-path>`

Queries a single YANG subtree. The path must be a module-qualified XPath prefix in the form `/module-name:top-level-node`. An optional `--filter key=value` argument restricts the output to a single list entry.

```bash
# Query all interfaces
$ yangerctl get /ietf-interfaces:interfaces
{
  "ietf-interfaces:interfaces": {
    "interface": [
      { "name": "eth0", "oper-status": "up", "statistics": { "in-octets": 1234567 } },
      { "name": "eth1", "oper-status": "down" }
    ]
  }
}

# Query a specific interface by key filter
$ yangerctl get /ietf-interfaces:interfaces --filter name=eth0
{
  "ietf-interfaces:interfaces": {
    "interface": [
      { "name": "eth0", "oper-status": "up", "phys-address": "52:54:00:ab:cd:ef",
        "statistics": { "in-octets": 1234567, "out-octets": 987654 } }
    ]
  }
}

# Query routing state
$ yangerctl get /ietf-routing:routing
{
  "ietf-routing:routing": {
    "ribs": {
      "rib": [
        { "name": "ipv4-master", "routes": { "route": [ ... ] } }
      ]
    }
  }
}
```

#### `yangerctl health`

Displays the daemon's overall health, per-subsystem status (with restart counts and PIDs), and per-model freshness data (last-updated timestamps and sizes). The output matches the canonical health response schema (Section 4.3.5).

```bash
$ yangerctl health
{
  "status": "ok",
  "subsystems": {
    "nlmonitor":    {"state": "running", "restarts": 0},
    "ipbatch":      {"state": "running", "pid": 1234, "restarts": 0},
    "bridgebatch":  {"state": "running", "pid": 1235, "restarts": 0},
    "zapiwatcher":  {"state": "running", "restarts": 0},
    "ethmonitor":   {"state": "running"},
    "fswatcher":    {"state": "running", "watches": 8},
    "dbusmonitor":  {"state": "running"},
    "iwmonitor":    {"state": "disabled"}
  },
  "models": {
    "ietf-interfaces:interfaces":  {"last_updated": "2026-03-04T12:34:56Z", "size_bytes": 8192},
    "ietf-routing:routing":        {"last_updated": "2026-03-04T12:34:55Z", "size_bytes": 2048},
    "ietf-hardware:hardware":      {"last_updated": "2026-03-04T12:34:50Z", "size_bytes": 1024},
    "ietf-system:system-state":    {"last_updated": "2026-03-04T12:34:48Z", "size_bytes": 512},
    "ietf-ntp:ntp":                {"last_updated": "2026-03-04T12:34:45Z", "size_bytes": 256}
  }
}
```

A collector that has never succeeded (e.g., FRRouting not yet running) is shown as `error` with the failure message:

```bash
  ospf:      error: exec: "vtysh": executable file not found in $PATH
```

#### `yangerctl dump`

Dumps the entire in-memory tree as a single JSON object to stdout. Useful for piping into `jq` for CI assertions or saving a snapshot of daemon state for offline analysis.

```bash
# Dump all tree data and extract interface names with jq
$ yangerctl dump | jq '."ietf-interfaces:interfaces".interface[].name'
"eth0"
"eth1"
"lo"

# Verify OSPF has at least one neighbor in state Full
$ yangerctl dump | jq '."ietf-routing:routing" | .. | objects | select(."ospf-neighbor-state"? == "Full") | ."neighbor-id"'
"192.168.1.2"

# Save a diagnostic snapshot
$ yangerctl dump > /tmp/yangerd-snapshot-$(date +%s).json
```

#### `yangerctl watch <yang-path>`

Polls the specified YANG path every second and prints a diff whenever the returned JSON changes. Intended for interactive debugging of reactive updates — for example, observing that a link state change propagates into the tree within milliseconds of the kernel event.

```bash
# Watch for changes to the routing table
$ yangerctl watch /ietf-routing:routing
[1s] no change
[2s] no change
[3s] changed:
  - "oper-status": "up"
  + "oper-status": "down"
[4s] no change

# Watch WireGuard peer handshake timestamps
$ yangerctl watch /ietf-interfaces:interfaces --filter name=wg0
[1s] no change
[30s] changed:
  - "latest-handshake": "2026-02-23T10:00:00Z"
  + "latest-handshake": "2026-02-23T10:00:30Z"
```

Press `Ctrl-C` to exit; `yangerctl watch` catches `SIGINT` and exits cleanly with exit code 0.

### Global Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--socket <path>` | `/run/yangerd.sock` | Unix socket path for yangerd connection |
| `--timeout <duration>` | `5s` | Per-request connection and read timeout |
| `--json` | false | Force JSON output even for commands that default to human-readable text (e.g., `health`, `watch`) |

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Connection error (socket not present, connection refused, timeout) |
| 2 | Path not found in the in-memory tree |
| 3 | Daemon is starting up — returned when yangerd responds with HTTP 503 equivalent (tree not yet populated) |

### Build and Installation

`yangerctl` is built alongside `yangerd` in the same Go module:

```bash
# Host build
go build -o yangerctl ./cmd/yangerctl

# Cross-compile for AArch64 Infix target (static, no CGo)
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags='-extldflags -static' -o yangerctl ./cmd/yangerctl
```

Being statically linked with no CGo dependency, the resulting binary can be copied directly to a target device via `scp` for one-off debug sessions without requiring package installation:

```bash
scp yangerctl admin@192.168.1.1:/tmp/
ssh admin@192.168.1.1 /tmp/yangerctl health
```

### 4.7 Design Decisions

#### AF_UNIX vs TCP
Using Unix domain sockets for inter-process communication provides the most efficient and secure transport for local daemon interactions. Unlike TCP sockets, AF_UNIX avoids the overhead of the network stack, including checksum calculation, sequence numbering, and acknowledgement packets. This choice ensures that data exchange between statd and yangerd occurs with near-zero latency while also permitting the use of standard filesystem permissions to restrict access to the statd user group. By leveraging a stream-oriented socket, we maintain the ability to handle large JSON payloads that might otherwise exceed the size limits of datagram-based alternatives.

#### Pre-serialization
Storing operational data as pre-serialized JSON blobs in the in-memory tree is a deliberate trade-off that prioritizes read performance over write efficiency. Since the operational data is read far more frequently than it is updated, especially under heavy monitoring from multiple NETCONF or RESTCONF clients, removing the serialization cost from the request path significantly reduces overall response latency. Each update to the tree involves a single serialization of the affected module, whereas every query becomes a simple memory lookup followed by a socket write. This architecture ensures that yangerd remains responsive even when the number of concurrent management sessions increases.

#### Per-Model RWMutex
The in-memory data tree uses per-model read-write mutexes rather than a single global lock. Each YANG module key (`ietf-interfaces:interfaces`, `ietf-routing:routing`, etc.) has its own `sync.RWMutex` inside a `modelEntry` struct, while a separate top-level `sync.RWMutex` protects the models map structure itself (new key insertion only). This design ensures that writers for different YANG modules never block each other -- for example, a netlink link event updating `ietf-interfaces:interfaces` does not block a ZAPI route update to `ietf-routing:routing`. Readers only contend with writers of the same module. On multi-module IPC requests, per-model read locks are acquired individually, data is read and concatenated into the response. The per-model write locks remain extremely short (updating a single map entry to a new JSON blob), preserving the low-contention characteristics of the original design while eliminating cross-module blocking entirely.

#### No CGo
The strict requirement to avoid CGo is driven by the necessity of maintaining a stable and reproducible cross-compilation environment within the Buildroot build system. Using pure Go allows the daemon to be compiled for ARM, AArch64, RISC-V, and x86_64 architectures using only the standard Go toolchain and environment variables, without needing a matching C cross-compiler and target sysroot for each architecture. This significantly reduces the complexity of the CI/CD pipeline and eliminates a common source of binary incompatibility and linking errors in embedded Linux environments. Furthermore, a pure Go binary is easier to audit for memory safety and simplifies the deployment process by producing a single, statically linked executable.
#### ip -batch -json for Data Queries vs vishvananda/netlink for Event Monitoring

yangerd uses a split approach: `vishvananda/netlink` for **event monitoring** and `iproute2` batch mode for **data queries**. Each tool is chosen for what it does best.

**Why vishvananda/netlink for events:**

1. **`ip monitor -json` does not produce JSON.** Investigation of the iproute2 source code (`ip/ipmonitor.c`) confirmed that `do_ipmonitor()` never calls `new_json_obj()`. The `-json` flag is parsed globally, but the JSON writer (`_jw`) is never allocated for the monitor subcommand. Likewise, `bridge/monitor.c` has zero JSON references. This was confirmed by Ubuntu bug #2116779 (2025-07-12). Parsing raw text output from `ip monitor` would be fragile and under-specified.

2. **Typed Go structs eliminate text parsing.** The `vishvananda/netlink` library delivers events as typed Go structs (`LinkUpdate`, `AddrUpdate`, `NeighUpdate`) on dedicated channels. Since yangerd uses events only as triggers (not for data extraction), the library's attribute coverage is sufficient -- we only need the interface name (from `update.Link.Attrs().Name`) or address family to route the event to the correct re-read handler.

3. **Production-proven in Docker, Cilium, Calico, OVN-Kubernetes, Antrea.** All major Go-based container networking projects use `vishvananda/netlink` for netlink event subscriptions. The library's `ErrorCallback` + context cancellation pattern is battle-tested at scale.

4. **Fewer subprocesses.** Replacing `ip monitor -json` and `bridge monitor -json` subprocesses with native Go channels reduces the subprocess count from FIVE to THREE (`ip batch`, `bridge batch`, `iw event`). This simplifies process management, reduces file descriptor usage, and eliminates two text-parsing codepaths.

**Why ip -batch -json for data queries (NOT vishvananda/netlink):**

1. **The problem**: `vishvananda/netlink` handles common attributes well, but the Linux kernel continuously adds new netlink attributes for features like XDP, tc flower offloads, bridge VLAN filtering extensions, and other advanced networking features. The Go library lags behind kernel development, meaning `yangerd` would be unable to report on features that `iproute2` already supports.

2. **The solution**: For state queries, `yangerd` delegates all netlink attribute parsing to `iproute2`'s `ip` command running in persistent batch mode (`ip -json -force -batch -`). `iproute2` is always present on the target system, always compiled against the running kernel's headers, and handles every netlink attribute the kernel exposes -- including obscure ones that no Go library wraps.

3. **How it works**: `yangerd` maintains a persistent `ip -json -force -batch -` subprocess. Commands are written to stdin one per line; each produces a JSON array on stdout. The `-force` flag ensures the process continues past errors. The `-json` flag must precede `-batch` in the argument list.

4. **Benefits**: (a) No dependency on Go netlink library feature parity with kernel for DATA. (b) `iproute2` handles all TLV parsing including vendor-specific and newly-added attributes. (c) No fork/exec overhead per query -- the batch process is persistent. (d) JSON output is directly usable as YANG operational data with minimal transformation.

5. **Trade-offs**: (a) Runtime dependency on `iproute2` (always present on Infix). (b) One extra process per `iproute2` tool family (`ip`, `bridge`). (c) Parsing `iproute2` JSON output instead of typed Go structs requires JSON unmarshalling. (d) Query latency includes IPC to subprocess (negligible for batch mode -- sub-millisecond). (e) `vishvananda/netlink` is an additional Go dependency for events, but this is a well-maintained library with minimal transitive dependencies.

#### inotify/fsnotify for File Watching

Using inotify (via Go's `fsnotify` library) eliminates fixed polling intervals for data sources based on real filesystem entries that change infrequently, such as procfs forwarding flags. This reactive approach reduces CPU wake-ups and provides near-instant detection of changes. However, this choice introduces a dependency on kernel-level inotify limits (`/proc/sys/fs/inotify/max_user_watches`), and requires special handling for the `IN_IGNORED` event to re-establish watches when files are deleted and recreated (a common pattern for atomic file writes). Note that sysfs pseudo-files (`/sys/class/hwmon/*`, `/sys/class/thermal/*`) do not support inotify -- the kernel generates values on `read()` and never calls `fsnotify_modify()` -- so hardware sensors are collected via polling instead (see Section 5, collector #6). DHCP lease files and firewall state, which were previously candidates for inotify/polling, are now handled reactively via D-Bus signals (see Section 4.1novies).

#### D-Bus Signal Subscriptions for Service-Managed Data

Using D-Bus signal subscriptions for dnsmasq DHCP leases and firewalld configuration changes replaces both inotify-based file watching and periodic polling with a semantically richer event source. D-Bus signals are emitted by the application itself at the exact moment state changes, providing both timeliness and context that filesystem-level mechanisms cannot match.

1. **Why not inotify for DHCP leases?** While inotify on `/var/lib/misc/dnsmasq.leases` functionally works, it has limitations. inotify fires on every `write()` syscall, which may arrive before dnsmasq has finished writing all lease data -- creating a race window where a partial file is read. dnsmasq's D-Bus signals (`DHCPLeaseAdded`, `DHCPLeaseDeleted`, `DHCPLeaseUpdated`) are emitted after the lease state is fully committed. Additionally, D-Bus signals carry semantic meaning (which lease changed) rather than just "file modified," enabling more targeted logging and diagnostics.

2. **Why not polling for firewall state?** The firewall configuration is managed by firewalld. While nftables kernel tables hold the runtime state, firewalld's D-Bus API provides the authoritative, structured view of zones, policies, and services. Periodic polling would require either subprocess execution or repeated D-Bus calls on a fixed schedule, with two costs: (a) unnecessary IPC every 30 seconds regardless of whether anything changed, and (b) up to 30 seconds of stale data after a firewall reload. firewalld's `Reloaded` D-Bus signal provides instant notification with zero steady-state CPU cost. On each signal, yangerd re-reads the full firewall state via firewalld D-Bus method calls.

3. **Why `godbus/dbus/v5`?** This is the standard Go D-Bus library, well-maintained and widely used. It provides `AddMatchSignal()` for signal subscription, `Signal()` for channel-based delivery, and `Object.CallWithContext()` for method invocations (used for `GetMetrics()` on dnsmasq and all firewalld data retrieval: `getDefaultZone()`, `getActiveZones()`, `getZoneSettings2()`, `getPolicies()`, `getPolicySettings()`, `listServices()`, `getServiceSettings2()`, `getLogDenied()`, `queryPanicMode()`). The API surface required by yangerd is minimal: connect, subscribe, receive signals, call methods.

**External command timeouts**: All short-lived external commands (`exec.Command`) use `exec.CommandContext(ctx)` with an explicit per-command timeout to prevent indefinite blocking. Timeout values: `vtysh` commands (OSPF/RIP/BFD collectors): 5 seconds; `iw` queries (station list, interface info): 2 seconds; `dmidecode` (hardware collector): 5 seconds. D-Bus method calls (dnsmasq `GetMetrics()`: 2 seconds; firewalld data retrieval: 5 seconds) use `CallWithContext()` with context-based timeouts. If a command or D-Bus call exceeds its timeout, the context cancellation terminates the operation, the monitor logs a warning, and the affected tree key retains its previous value.

4. **Trade-offs**: (a) Runtime dependency on the D-Bus system bus daemon (always present on Infix -- `dbus-daemon` is a core system component). (b) Service absence handling is more complex than file-based approaches: when dnsmasq or firewalld is not running, no signals arrive, and the `NameOwnerChanged` mechanism must be used for lifecycle detection. (c) D-Bus method calls (`GetMetrics()` for dnsmasq, firewalld zone/policy/service queries) have IPC overhead, though this is negligible for the call frequency involved (only on signal receipt, not periodic). (d) The `godbus/dbus/v5` library is an additional Go dependency, but it has minimal transitive dependencies and is already used by many system-level Go programs.

#### bridge -json -batch - for Bridge Data

A separate `bridge` batch subprocess is utilized for bridge-specific netlink queries instead of multiplexing through the existing `ip` batch subprocess. While both tools belong to the `iproute2` family, they utilize distinct command grammars and produce different JSON output structures. By maintaining a dedicated `bridge -json -batch -` process, `yangerd` avoids the complexity of a multiplexing layer while reusing the established subprocess management pattern (persistent stdin/stdout pipes, health monitoring, and exponential backoff). This ensures that VLAN, FDB, MDB, and STP data—which are not exposed via the `ip` command—are collected efficiently using the most authoritative tool available on the system.

#### iw event for 802.11 Wireless Monitoring

The `iw event -t` command from the `iw` tool provides reactive notification of 802.11 wireless events via the Linux kernel's nl80211 netlink family. Unlike the NLMonitor's `vishvananda/netlink` subscriptions (which receive typed Go structs), `iw event` produces human-readable text output rather than JSON, and `iw` has no batch query mode. Despite these differences, `iw event` is the only reliable mechanism for detecting wireless client associations, disconnections, channel switches, and regulatory domain changes without polling.

1. **Why not nl80211 directly in Go?** While Go libraries for generic netlink exist (`mdlayher/genetlink`), the nl80211 family has an exceptionally complex attribute set (over 300 attributes, nested TLVs, vendor-specific extensions). The `iw` tool handles all nl80211 attribute parsing and version compatibility, just as `iproute2` handles RTNL parsing for the ip/bridge subsystems. Delegating to `iw` avoids duplicating a fragile and rapidly-evolving netlink parser.

2. **Why not use a persistent subprocess for queries?** The `iw` tool has no `-batch -` mode. Each query requires a separate `exec.Command` invocation. This is acceptable because WiFi events are infrequent (typically single-digit events per minute), so the overhead of spawning short-lived processes for re-queries is negligible compared to the persistent `ip -json -batch -` subprocess that handles hundreds of queries per second during convergence events.

3. **Why is the subsystem feature-gated?** Not all Infix deployments include wireless hardware. WiFi support is a build-time option in Buildroot. The `YANGERD_ENABLE_WIFI` environment variable (set by the Buildroot recipe in `/etc/default/yangerd`) controls whether the IW Event Monitor and WiFi collector are started. When WiFi is included in the build, the `iw` binary is guaranteed present on the target.

4. **Trade-offs**: (a) When WiFi is enabled, `iw` is a runtime dependency (guaranteed present by the build system). (b) Text parsing is more fragile than JSON parsing—format changes in `iw` output could break the parser. (c) Short-lived subprocesses for re-queries have higher per-query overhead than batch mode, but the low event rate makes this negligible. (d) A single goroutine processes events sequentially, which is sufficient for typical WiFi event rates but could become a bottleneck on systems with many wireless interfaces.

#### Ethtool Genetlink Monitor for Settings Changes

The Linux kernel's ethtool netlink family exposes a `"monitor"` multicast group (`ETHNL_MCGRP_MONITOR`) that delivers notifications when Ethernet link settings change. Infix targets Linux kernel 6.18, where this facility is unconditionally available. This allows yangerd to receive `ETHTOOL_MSG_LINKINFO_NTF` and `ETHTOOL_MSG_LINKMODES_NTF` messages whenever speed, duplex, auto-negotiation, or other link parameters are renegotiated—without polling.

1. **Why not poll for everything?** The original design polled ethtool data every 30 seconds. While acceptable for statistics (which change continuously), speed/duplex/auto-negotiation only change on link renegotiation events—typically seconds to minutes apart. Polling at 30 seconds means up to 30 seconds of stale data after a link renegotiation. The genetlink monitor reduces this to sub-second latency.

2. **Why not use mdlayher/ethtool for monitoring?** The `mdlayher/ethtool` Go library provides typed access to ethtool genetlink queries (LinkInfo, LinkMode, Stats) but does not expose a Monitor or Subscribe API for multicast notifications. However, the lower-level `mdlayher/genetlink` library fully supports `Conn.JoinGroup()` and `Conn.Receive()`, enabling yangerd to subscribe to the ethtool monitor group natively in Go without any subprocess.

3. **Why is this NOT a subprocess?** Unlike the `iw event` subsystem—which shells out to an external tool because `iw` handles complex nl80211 attribute parsing—the ethtool monitor notifications are simple genetlink messages with a command byte that identifies the notification type. The actual data retrieval is then done via the existing `mdlayler/ethtool` typed API. No complex TLV parsing is needed in the notification path, so a native Go genetlink socket is both simpler and more efficient than spawning an external process. The core netlink event monitoring (link, addr, neigh) is also native Go via `vishvananda/netlink`, making the ethtool genetlink monitor consistent with the overall architecture.

4. **Hybrid model**: The ethtool collector becomes a hybrid: reactive for settings (speed, duplex, auto-negotiation via `ETHNL_MCGRP_MONITOR` genetlink subscription) and polling for statistics (hardware counters via `ethtool.Client.Stats()` at 30-second intervals). Statistics have no `_NTF` message type—they must remain polling.

5. **No fallback needed**: Infix targets Linux kernel 6.18, where ethtool netlink is unconditionally available. The ethmonitor is always active in production — there is no polling fallback for settings. If the genetlink subscription fails, it indicates a system misconfiguration, not a kernel capability gap.

6. **Trade-offs**: (a) Dependency on `mdlayher/genetlink` in addition to `mdlayher/ethtool` (both are pure Go, no CGo). (b) The genetlink socket is an additional file descriptor per yangerd instance. (c) Only settings changes are reactive; statistics remain polling.

7. **Ethtool NTF gap on link up/down**: The `ETHNL_MCGRP_MONITOR` multicast group does NOT deliver notifications when a link goes up or down. When a physical link transitions, the kernel negotiates speed/duplex/autoneg with the link partner, but this negotiation is invisible to the ethtool genetlink monitor. To close this gap, the link event handler (`monitor/link.go`) calls `ethmonitor.RefreshInterface()` on every RTM_NEWLINK event, explicitly re-querying ethtool data for the affected interface. This ensures sub-second convergence for ethtool data after link events, matching the latency of the genetlink monitor for explicit settings changes.

8. **Parser version robustness**: At startup, yangerd logs the output of `iw --version` to record the exact `iw` version in use. The text parser handles unknown/unparsed event lines by logging them at DEBUG level and skipping them -- unrecognized lines do not cause errors or stop event processing. This provides forward compatibility with newer `iw` versions that may add new event types. Test fixtures in `testdata/` capture known-good outputs from `iw` 6.9 for regression testing.

#### Event-Triggered Batch Re-read Pattern (All Netlink Events)

All netlink events -- link, address, and neighbor, both add (`RTM_NEW*`) and remove (`RTM_DEL*`) -- use the same core pattern: the event content is used only as a **trigger** to identify which entity changed. The event payload itself is NOT parsed for data. Instead, the event dispatcher issues a full re-read of the affected state and replaces the corresponding subtree in the YANG tree atomically. For link, address, and neighbor events, re-reads go through `ip -json -force -batch -`. Route data is sourced exclusively from the ZAPI watcher's streaming connection to zebra (Section 4.1octies) and is NOT part of the netlink event-triggered re-read pattern.

The per-event-type re-read queries are:

| Event Type | Trigger | ip batch Re-read Queries | Subtree Replaced | Additional Actions |
|------------|---------|--------------------------|------------------|--------------------|
| **Link** (RTM_NEWLINK / RTM_DELLINK) | Interface name | `link show dev <iface>`, `-s link show dev <iface>`, `addr show dev <iface>` (3 queries) | Entire interface subtree (flags, MTU, operstate, stats, addresses) | `ethmonitor.RefreshInterface()` + `last-change` timestamp |
| **Address** (RTM_NEWADDR / RTM_DELADDR) | Interface name | `addr show dev <iface>` (1 query) | Address subtree for that interface | -- |
| **Neighbor** (RTM_NEWNEIGH / RTM_DELNEIGH) | Interface name | `neigh show dev <iface>` (1 query) | Neighbor subtree for that interface | -- |

**Why the same pattern for delete events?** After a `RTM_DEL*` event, the re-read query returns the current state which simply omits the deleted entity. The tree update replaces the entire subtree with this result. No surgical removal logic is needed — the subtree replacement naturally drops the deleted entry. This eliminates an entire class of bugs related to partial tree surgery.

**Design Rationale:**

1. **Why re-read instead of parsing the event?** Netlink events carry only partial state. An RTM_NEWLINK does not include addresses; an RTM_NEWADDR does not include the full address list for the interface. Event payloads vary by kernel version and may omit fields. A full re-read via `ip batch` for link/addr/neigh captures all fields at a single coherent point in time, making the tree self-consistent and kernel-version-independent.

2. **Why include addresses in the link re-read?** An RTM_NEWLINK event itself does not carry address information, but address behavior can change as a consequence of link state (e.g., IPv6 SLAAC addresses are added/removed on link up/down, DAD state may transition). By including `addr show dev` in the link re-read set, the tree always reflects the current address state even if the separate addr monitor event arrives slightly later.

3. **Why trigger ethtool re-query on link events?** As noted in point 7 above, `ETHNL_MCGRP_MONITOR` does not fire on link up/down. The link event handler calls `ethmonitor.RefreshInterface()` to re-query speed/duplex/autoneg after every RTM_NEWLINK, ensuring ethtool data converges within milliseconds of the link event.

4. **Per-entity debouncing**: During convergence storms (STP topology change, ARP storms), the same entity may generate tens of events per second. A per-entity debounce window (10ms) coalesces rapid events into a single full re-read, preventing redundant queries while still converging to the correct final state. Debouncing is keyed by interface name for link/addr/neigh events.

5. **Trade-offs**: (a) Full re-reads for link/addr/neigh issue more ip batch queries per event compared to targeted field updates. This is well within ip batch throughput capacity (microseconds per query over a local pipe). (b) Per-entity debouncing adds a small latency (up to 10ms) in the storm case, but ensures efficiency. (c) The `ethmonitor.RefreshInterface()` call on link events adds two ethtool genetlink queries (LinkInfo + LinkMode), which is negligible.
### 4.8 Monitoring & Observability

Monitoring the internal state of yangerd is critical for ensuring that data collection remains accurate and timely. The daemon exposes a health endpoint over the IPC socket that provides real-time status for all netlink monitors and supplementary collectors.

#### Health Endpoint

The `yangerctl health` command (and the underlying `{"method":"health"}` IPC request) returns the same schema defined in Section 4.3.5 (`subsystems` + `models`):

```json
{
  "status": "ok",
  "subsystems": {
    "nlmonitor":   { "state": "running", "restarts": 0 },
    "ipbatch":     { "state": "running", "pid": 1321, "restarts": 0 },
    "bridgebatch": { "state": "running", "pid": 1322, "restarts": 0 },
    "zapiwatcher": { "state": "running", "restarts": 1 },
    "iwmonitor":   { "state": "disabled" },
    "ethmonitor":  { "state": "running" },
    "lldpmonitor": { "state": "running", "pid": 1323, "restarts": 0 },
    "mdnsmonitor": { "state": "running", "restarts": 0 },
    "dbusmonitor": { "state": "running" },
    "fswatcher":   { "state": "running", "watches": 12 }
  },
  "models": {
    "ietf-routing:routing": { "last_updated": "2026-03-27T10:22:01Z", "size_bytes": 15012 },
    "ieee802-dot1ab-lldp:lldp": { "last_updated": "2026-03-27T10:22:00Z", "size_bytes": 4312 },
    "infix-services:mdns": { "last_updated": "2026-03-27T10:21:58Z", "size_bytes": 2088 }
  }
}
```

#### Metrics Tracked

For each subsystem: state (`running`/`restarting`/`failed`/`disabled`), restart counters, and PID for managed subprocesses. The route health entry is attributed to **`zapiwatcher`** (not NLMonitor), because routes are sourced from zebra via ZAPI. For each model: `last_updated` and `size_bytes`.

#### Log Levels

yangerd uses structured logging (Go `slog` package). The default log level is `info`. Setting `YANGERD_LOG_LEVEL=debug` enables per-event netlink message logging and per-request IPC tracing. The `warn` level logs ENOBUFS recoveries and collector timeouts. The `error` level logs collector failures and IPC protocol violations.

### 4.9 Security Considerations

Security is a primary concern given that yangerd handles sensitive network state and runs with elevated privileges on some platforms.

#### Socket Permissions

The Unix domain socket at `/run/yangerd.sock` is created with mode `0660` and owned by `root:yangerd`. Only processes running as root or in the `yangerd` group can connect. In practice, the only consumer is statd. The socket path is not configurable via the IPC protocol itself — it is set at daemon startup via the `YANGERD_SOCK` environment variable or the compile-time default.

#### Linux Capabilities

yangerd drops all capabilities at startup except those explicitly required:

- `CAP_NET_ADMIN` — required to open netlink sockets and subscribe to multicast groups.
- `CAP_SYS_RAWIO` — required only when the `ietf-hardware` collector invokes `dmidecode` to read SMBIOS data from `/dev/mem`. This capability is granted via the Finit service file (`cap_sys_rawio+ep`) and is only needed on physical hardware; virtual machines can omit it.

No other capabilities are retained. yangerd does not need `CAP_SYS_ADMIN`, `CAP_NET_RAW`, or any filesystem-related capabilities.

#### Runtime User and Process Model

yangerd runs as `root` but drops all capabilities except those listed above via `cap_set_proc()` at startup. Running as root is required because:
- Netlink multicast group subscriptions require `CAP_NET_ADMIN`, which must be in the process's effective set.
- The ZAPI watcher connects to `/var/run/frr/zserv.api`, which is owned by `root:frr` with mode `0660`. yangerd must be in the `frr` group.
- The IPC socket at `/run/yangerd.sock` is created with `root:yangerd` ownership.

The Finit service file grants the minimum required capabilities:

```
# /etc/finit.d/yangerd.conf
service [S12345] env:-/etc/default/yangerd \
        log:prio:daemon.notice,tag:yangerd \
        <!pid/1> yangerd -- yangerd operational data daemon
```

Subprocesses (`ip batch`, `bridge batch`, `iw event`) inherit yangerd's reduced capability set. They do not require additional capabilities beyond what the parent provides.

#### Trust Boundary

The trust boundary is the Unix domain socket. yangerd trusts that any process connecting to the socket is authorized (enforced by filesystem permissions). It does not perform authentication or authorization on individual IPC requests. All IPC payloads are validated for size (maximum 64 KiB request, configurable) and structural correctness (must be valid JSON with a `command` field) before processing, preventing resource exhaustion and malformed-input attacks.

The absence of CGo eliminates an entire class of memory-safety vulnerabilities (buffer overflows, use-after-free, format string attacks) that would be present if yangerd linked against C libraries. The Go runtime's garbage collector and bounds checking provide defense-in-depth for the data processing pipeline.

## 5. Data Source Matrix

Every operational YANG leaf collected by yangerd is listed below with its data source, the collection method (ip batch query via persistent `ip -json -force -batch -` subprocess, or other tool), whether collection is reactive (event-driven via `vishvananda/netlink` subscriptions) or polling-based, and any known gaps or caveats.

### ietf-interfaces — Interface Operational State

| YANG Path | Source | Go Method | Reactive/Polling | Notes |
|-----------|--------|-----------|-----------------|-------|
| `.../interface/oper-status` | RTNLGRP_LINK (IFF_UP \| IFF_RUNNING) | `ip -json -batch` | REACTIVE | Kernel delivers on every link state change |
| `.../interface/phys-address` | RTNLGRP_LINK (IFLA_ADDRESS) | `ip -json -batch` | REACTIVE | MAC address from NLMSG_NEWLINK |
| `.../interface/if-index` | RTNLGRP_LINK (ifi_index) | `ip -json -batch` | REACTIVE | |
| `.../interface/statistics/in-octets` | RTNLGRP_LINK (stats64.rx_bytes) | `ip -json -batch` | REACTIVE | Full stats64 in `ip -json -s link show` output |
| `.../interface/statistics/out-octets` | RTNLGRP_LINK (stats64.tx_bytes) | `ip -json -batch` | REACTIVE | |
| `.../interface/statistics/in-unicast-pkts` | RTNLGRP_LINK (stats64.rx_packets) | `ip -json -batch` | REACTIVE | |
| `.../interface/statistics/out-unicast-pkts` | RTNLGRP_LINK (stats64.tx_packets) | `ip -json -batch` | REACTIVE | |
| `.../interface/statistics/in-errors` | RTNLGRP_LINK (stats64.rx_errors) | `ip -json -batch` | REACTIVE | |
| `.../interface/statistics/out-errors` | RTNLGRP_LINK (stats64.tx_errors) | `ip -json -batch` | REACTIVE | |
| `.../interface/statistics/in-discards` | RTNLGRP_LINK (stats64.rx_dropped) | `ip -json -batch` | REACTIVE | |
| `.../interface/statistics/out-discards` | RTNLGRP_LINK (stats64.tx_dropped) | `ip -json -batch` | REACTIVE | |
| `.../ietf-ip:ipv4/address/ip` | RTNLGRP_IPV4_IFADDR | `ip -json -batch` | REACTIVE | Full address re-read via `addr show dev <iface>` on any RTM_NEWADDR or RTM_DELADDR; event is trigger only, not parsed for data |
| `.../ietf-ip:ipv4/address/prefix-length` | RTNLGRP_IPV4_IFADDR | `ip -json -batch` | REACTIVE | Full address re-read on any addr event (add or remove) |
| `.../ietf-ip:ipv6/address/ip` | RTNLGRP_IPV6_IFADDR | `ip -json -batch` | REACTIVE | Full address re-read via `addr show dev <iface>` on any RTM_NEWADDR or RTM_DELADDR |
| `.../ietf-ip:ipv6/address/prefix-length` | RTNLGRP_IPV6_IFADDR | `ip -json -batch` | REACTIVE | Full address re-read on any addr event (add or remove) |
| `.../ietf-ip:ipv6/address/status` | RTNLGRP_IPV6_IFADDR | `ip -json -batch` | REACTIVE | Full address re-read on any addr event; `ip -json addr show` includes preferred/deprecated status in JSON output |
| `.../ietf-ip:ipv4/neighbor/ip` | RTNLGRP_NEIGH (AF_INET) | `ip -json -batch` | REACTIVE | Full neighbor re-read via `neigh show dev <iface>` on any RTM_NEWNEIGH or RTM_DELNEIGH; event is trigger only |
| `.../ietf-ip:ipv4/neighbor/link-layer-address` | RTNLGRP_NEIGH (AF_INET) | `ip -json -batch` | REACTIVE | Full neighbor re-read on any neigh event (add or remove); NDA_LLADDR attribute |
| `.../ietf-ip:ipv4/neighbor/origin` | RTNLGRP_NEIGH (NUD flags) | `ip -json -batch` | REACTIVE | Full neighbor re-read on any neigh event; dynamic/static from state field in JSON |
| `.../ietf-ip:ipv6/neighbor/ip` | RTNLGRP_NEIGH (AF_INET6) | `ip -json -batch` | REACTIVE | Full neighbor re-read via `neigh show dev <iface>` on any RTM_NEWNEIGH or RTM_DELNEIGH; NDP table |
| `.../ietf-ip:ipv6/neighbor/link-layer-address` | RTNLGRP_NEIGH (AF_INET6) | `ip -json -batch` | REACTIVE | Full neighbor re-read on any neigh event (add or remove) |
| `.../infix-ethernet-interface:ethernet/speed` | ETHNL_MCGRP_MONITOR (ETHTOOL_MSG_LINKMODES_NTF) + RTM_NEWLINK | mdlayher/ethtool + mdlayher/genetlink | REACTIVE (ethtool genetlink monitor) | Reactive via ethmonitor subscription; also re-queried on every RTM_NEWLINK (link up/down) via `ethmonitor.RefreshInterface()` since ETHNL_MCGRP_MONITOR does not fire on link state changes |
| `.../infix-ethernet-interface:ethernet/duplex` | ETHNL_MCGRP_MONITOR (ETHTOOL_MSG_LINKMODES_NTF) + RTM_NEWLINK | mdlayher/ethtool + mdlayher/genetlink | REACTIVE (ethtool genetlink monitor) | Reactive via ethmonitor subscription; also re-queried on RTM_NEWLINK via `ethmonitor.RefreshInterface()` |
| `.../infix-ethernet-interface:ethernet/auto-negotiation` | ETHNL_MCGRP_MONITOR (ETHTOOL_MSG_LINKMODES_NTF) + RTM_NEWLINK | mdlayher/ethtool + mdlayher/genetlink | REACTIVE (ethtool genetlink monitor) | Reactive via ethmonitor subscription; also re-queried on RTM_NEWLINK via `ethmonitor.RefreshInterface()` |
| `.../infix-interfaces:bridge/stp-state` | RTM_NEWLINK (IFLA_BRPORT_STATE) | `bridge -json -batch` | REACTIVE (netlink) | STP port state change arrives via LinkUpdate on bridge port; triggers bridge batch re-query. 0=disabled,1=listening,2=learning,3=forwarding,4=blocking |
| `.../infix-interfaces:bridge/vlan` | RTM_NEWLINK (VLAN attributes on bridge port) | `bridge -json -batch` | REACTIVE (netlink) | VLAN changes arrive via LinkUpdate; trigger `vlan show` via bridge batch |
| `.../infix-interfaces:wifi/ssid` | `iw event -t` + `iw dev <iface> info` | iwmonitor + exec.Command | REACTIVE (iw event) | Re-queried on `connected`, `ch_switch_started_notify` events |
| `.../infix-interfaces:wifi/frequency` | `iw event -t` + `iw dev <iface> info` | iwmonitor + exec.Command | REACTIVE (iw event) | Re-queried on `connected`, `ch_switch_started_notify` events |
| `.../infix-interfaces:wifi/bitrate` | `iw event -t` + `iw dev <iface> station dump` | iwmonitor + exec.Command | REACTIVE (iw event) | Re-queried on `new station`, `connected` events |
| `.../infix-interfaces:wifi/signal-strength` | `iw event -t` + `iw dev <iface> link` (via `iw.py link`) | iwmonitor + exec.Command | REACTIVE (iw event) | Signal strength in dBm; available in station mode only (not AP mode). Re-queried on `connected`, `disconnected`, `signal` events. Source is `iw dev <iface> link`, NOT `/proc/net/wireless` (which is empty on modern cfg80211/nl80211 drivers). |
| `.../infix-interfaces:wifi/station/scan-results` | `wpa_cli -i <iface> scan_result` | exec.Command | POLLING 10 seconds | Available scan results from wpa_supplicant; returns BSSID, frequency, signal, flags, SSID per network. Only populated in station mode. |
| `.../infix-interfaces:wireguard/peer/endpoint` | wgctrl (generic netlink WG_CMD_GET_DEVICE) | wgctrl.Client | POLLING 30 seconds | WireGuard kernel module required |
| `.../infix-interfaces:wireguard/peer/rx-bytes` | wgctrl | wgctrl.Client | POLLING 30 seconds | |
| `.../infix-interfaces:wireguard/peer/tx-bytes` | wgctrl | wgctrl.Client | POLLING 30 seconds | |
| `.../interface/last-change` | RTNLGRP_LINK (RTM_NEWLINK with oper-status change) | `time.Now()` at event receipt | REACTIVE | Timestamp recorded when link event handler detects oper-status transition |

### ietf-routing — Routing State

| YANG Path | Source | Go Method | Reactive/Polling | Notes |
|-----------|--------|-----------|-----------------|-------|
| `.../routing/ribs/rib[name='ipv4-master']/routes/route` | ZAPI `REDISTRIBUTE_ROUTE_ADD` (streaming) | `zapiwatcher.ZAPIWatcher` (gobgp/v4/pkg/zebra) | REACTIVE (ZAPI watcher) | Route data sourced from zebra's zserv socket via ZAPI v6 redistribution subscription. The ZAPI watcher receives incremental route add/delete messages covering ALL route types (kernel, connected, static, OSPF, RIP) with FRR-enriched metadata (protocol, distance, metric, next-hops, active/installed flags) -- including routes in zebra's RIB not installed in the kernel FIB. Replaces previous `vtysh`-based collection. See Section 4.1octies. |
| `.../routing/ribs/rib[name='ipv6-master']/routes/route` | ZAPI `REDISTRIBUTE_ROUTE_ADD` (streaming) | `zapiwatcher.ZAPIWatcher` (gobgp/v4/pkg/zebra) | REACTIVE (ZAPI watcher) | Same as IPv4 but for IPv6 routes. ZAPI subscription covers both address families. See Section 4.1octies. |
| `.../control-plane-protocols/ospf/neighbors` | exec `vtysh -c 'show ip ospf neighbor json'` | exec.Command | POLLING 10 seconds | FRRouting must be running |
| `.../control-plane-protocols/ospf/areas/interfaces` | exec `vtysh -c 'show ip ospf interface json'` | exec.Command | POLLING 10 seconds | FRR exposes this state via request/response CLI only; no streaming API |
| `.../control-plane-protocols/rip/routes` | exec `vtysh -c 'show ip rip json'` | exec.Command | POLLING 10 seconds | FRR exposes this state via request/response CLI only; no streaming API |
| `.../control-plane-protocols/bfd/sessions` | exec `vtysh -c 'show bfd peers json'` | exec.Command | POLLING 10 seconds | FRR exposes this state via request/response CLI only; no streaming API |

### ietf-hardware — Hardware Components

| YANG Path | Source | Go Method | Reactive/Polling | Notes |
|-----------|--------|-----------|-----------------|-------|
| `.../hardware/component[class='sensor']/sensor-data/value` | /sys/class/hwmon/*/temp*_input, fan*_input, in*_input | collector/hardware.go (`os.ReadFile`) | POLLING 10 seconds | Millidegree Celsius, RPM, millivolt raw values. sysfs pseudo-files do not emit inotify events; polling is the only correct method. |
| `.../hardware/component[class='sensor']/sensor-data/oper-status` | /sys/class/hwmon/*/temp*_fault | collector/hardware.go (`os.ReadFile`) | POLLING 10 seconds | Fault flag read alongside sensor values |
| `.../hardware/component[class='chassis']/mfg-name` | exec `dmidecode -s system-manufacturer` | exec.Command | POLLING 300 seconds | Rarely changes; cached after first read |
| `.../hardware/component[class='chassis']/model-name` | exec `dmidecode -s system-product-name` | exec.Command | POLLING 300 seconds | |
| `.../hardware/component[class='chassis']/serial-num` | exec `dmidecode -s system-serial-number` | exec.Command | POLLING 300 seconds | |

### ietf-system — System State

| YANG Path | Source | Go Method | Reactive/Polling | Notes |
|-----------|--------|-----------|-----------------|-------|
| `.../system-state/platform/os-name` | exec `uname -s` | exec.Command | POLLING 300 seconds | |
| `.../system-state/platform/os-release` | exec `uname -r` | exec.Command | POLLING 300 seconds | |
| `.../system-state/platform/machine` | exec `uname -m` | exec.Command | POLLING 300 seconds | |
| `.../system-state/clock/current-datetime` | time.Now() | time package | POLLING 60 seconds | |
| `.../system-state/clock/boot-datetime` | /proc/uptime + time.Now() | os.ReadFile | POLLING 60 seconds | |
| `.../system/users/user/name` | /etc/passwd parsing | os.ReadFile | POLLING 300 seconds | |

### ietf-ntp — NTP State

| YANG Path | Source | Go Method | Reactive/Polling | Notes |
|-----------|--------|-----------|-----------------|-------|
| `.../ntp-state/association/address` | chrony cmdmon protocol (sources request) | `github.com/facebook/time/ntp/chrony` | POLLING 60 seconds | Unix socket `/var/run/chrony/chronyd.sock` |
| `.../ntp-state/association/stratum` | chrony cmdmon protocol (sources request) | `github.com/facebook/time/ntp/chrony` | POLLING 60 seconds | |
| `.../ntp-state/association/offset` | chrony cmdmon protocol (tracking request) | `github.com/facebook/time/ntp/chrony` | POLLING 60 seconds | |
| `.../ntp-state/association/synchronized` | chrony cmdmon protocol (tracking request) | `github.com/facebook/time/ntp/chrony` | POLLING 60 seconds | |

### ieee802-dot1ab-lldp — LLDP Neighbors

| YANG Path | Source | Go Method | Reactive/Polling | Notes |
|-----------|--------|-----------|-----------------|-------|
| `.../lldp/ports/port/neighbors/neighbor/chassis-id` | `lldpcli -f json0 watch` (`lldp-added`/`lldp-updated`/`lldp-deleted`) | persistent subprocess monitor (`internal/lldpmonitor/`) | REACTIVE | Blank-line-delimited pretty JSON objects; parsed by framing-aware stream parser |
| `.../lldp/ports/port/neighbors/neighbor/port-id` | `lldpcli -f json0 watch` | persistent subprocess monitor (`internal/lldpmonitor/`) | REACTIVE | `json0` structural stability (arrays always arrays) |
| `.../lldp/ports/port/neighbors/neighbor/ttl` | `lldpcli -f json0 watch` | persistent subprocess monitor (`internal/lldpmonitor/`) | REACTIVE | Full neighbor payload in each event |
| `.../lldp/ports/port/neighbors/neighbor/system-name` | `lldpcli -f json0 watch` | persistent subprocess monitor (`internal/lldpmonitor/`) | REACTIVE | |
| `.../lldp/ports/port/neighbors/neighbor/system-capabilities` | `lldpcli -f json0 watch` | persistent subprocess monitor (`internal/lldpmonitor/`) | REACTIVE | |

### infix-containers — Container State (Feature-Gated)

**Feature gate**: This data source is only collected when `YANGERD_ENABLE_CONTAINERS=true`. When container support is not included in the Infix build, the container collector is not started and these paths are absent from the tree.

| YANG Path | Source | Go Method | Reactive/Polling | Notes |
|-----------|--------|-----------|-----------------|-------|
| `.../containers/container/name` | exec `podman ps --format json` | exec.Command | POLLING 10 seconds | **Phase 2**: container namespace handling deferred |
| `.../containers/container/state` | exec `podman ps --format json` | exec.Command | POLLING 10 seconds | Phase 2 |
| `.../containers/container/image` | exec `podman ps --format json` | exec.Command | POLLING 10 seconds | Phase 2 |

### infix-dhcp-server — DHCP Leases

| YANG Path | Source | Go Method | Reactive/Polling | Notes |
|-----------|--------|-----------|-----------------|-------|
| `.../dhcp-server/leases/lease/ip-address` | /var/lib/misc/dnsmasq.leases | D-Bus Monitor `refreshDHCP()` | REACTIVE (D-Bus) | dnsmasq `DHCPLeaseAdded`/`Deleted`/`Updated` signals |
| `.../dhcp-server/leases/lease/hw-address` | /var/lib/misc/dnsmasq.leases | D-Bus Monitor `refreshDHCP()` | REACTIVE (D-Bus) | |
| `.../dhcp-server/leases/lease/hostname` | /var/lib/misc/dnsmasq.leases | D-Bus Monitor `refreshDHCP()` | REACTIVE (D-Bus) | |
| `.../dhcp-server/leases/lease/expire` | /var/lib/misc/dnsmasq.leases | D-Bus Monitor `refreshDHCP()` | REACTIVE (D-Bus) | UNIX timestamp in lease file |

### infix-firewall — Firewall State

| YANG Path | Source | Go Method | Reactive/Polling | Notes |
|-----------|--------|-----------|-----------------|-------|
| `.../firewall/default-zone` | firewalld D-Bus `getDefaultZone()` | D-Bus Monitor `refreshFirewall()` | REACTIVE (D-Bus) | firewalld `Reloaded` signal + `NameOwnerChanged` |
| `.../firewall/log-denied` | firewalld D-Bus `getLogDenied()` | D-Bus Monitor `refreshFirewall()` | REACTIVE (D-Bus) | |
| `.../firewall/lockdown` | firewalld D-Bus `queryPanicMode()` | D-Bus Monitor `refreshFirewall()` | REACTIVE (D-Bus) | |
| `.../firewall/zones/zone` | firewalld D-Bus `getActiveZones()` + `getZoneSettings2()` | D-Bus Monitor `refreshFirewall()` | REACTIVE (D-Bus) | Per-zone: interfaces, sources, services, forwards, rich rules |
| `.../firewall/policies/policy` | firewalld D-Bus `getPolicies()` + `getPolicySettings()` | D-Bus Monitor `refreshFirewall()` | REACTIVE (D-Bus) | Per-policy: ingress/egress zones, action, priority, rich rules |
| `.../firewall/services/service` | firewalld D-Bus `listServices()` + `getServiceSettings2()` | D-Bus Monitor `refreshFirewall()` | REACTIVE (D-Bus) | Per-service: port/protocol definitions |

### 5.9bis infix-services — mDNS Neighbors

| YANG Path | Source | Go Method | Reactive/Polling | Notes |
|-----------|--------|-----------|-----------------|-------|
| `/infix-services:mdns/neighbors/neighbor/hostname` | Avahi D-Bus `ServiceBrowser`/`ServiceResolver` signals | `internal/mdnsmonitor/` via `godbus/dbus/v5` | REACTIVE (D-Bus) | Keyed by hostname |
| `/infix-services:mdns/neighbors/neighbor/address` | Avahi D-Bus resolver results | `internal/mdnsmonitor/` | REACTIVE (D-Bus) | Leaf-list of resolved addresses |
| `/infix-services:mdns/neighbors/neighbor/last-seen` | Event timestamp at signal handling | `time.Now()` in mDNS monitor | REACTIVE (D-Bus) | Updated on add/update events |
| `/infix-services:mdns/neighbors/neighbor/service/name` | Avahi service instance metadata | `internal/mdnsmonitor/` | REACTIVE (D-Bus) | Service list key |
| `/infix-services:mdns/neighbors/neighbor/service/type` | Avahi service type | `internal/mdnsmonitor/` | REACTIVE (D-Bus) | e.g. `_ssh._tcp` |
| `/infix-services:mdns/neighbors/neighbor/service/port` | Avahi resolver payload | `internal/mdnsmonitor/` | REACTIVE (D-Bus) | |
| `/infix-services:mdns/neighbors/neighbor/service/txt` | Avahi TXT records | `internal/mdnsmonitor/` | REACTIVE (D-Bus) | Leaf-list |

### Summary

| Category | Leaf Count | Strategy |
|----------|-----------|----------|
| REACTIVE (Monitor/Watcher) | 56 | `vishvananda/netlink` subscriptions (link, addr, neigh channels + bridge FDB/VLAN/MDB/STP events as triggers for `bridge -json -batch -` re-reads), ZAPI watcher (streaming route redistribution from zebra via zserv socket), D-Bus Monitor (dnsmasq DHCP lease signals + firewalld reload signals), LLDP monitor (`lldpcli -f json0 watch`), mDNS monitor (Avahi D-Bus), `iw event -t`, ethtool genetlink monitor (`ETHNL_MCGRP_MONITOR`), and `fswatcher` (inotify for procfs forwarding flags). |
| POLLING 10 seconds | 6 | FRRouting (OSPF/RIP/BFD) via `vtysh` JSON queries |
| POLLING 10 seconds | 6 | Hardware sensors (hwmon temperature, fan, voltage, fault — sysfs files do not support inotify), container state (Phase 2, feature-gated: `YANGERD_ENABLE_CONTAINERS`), WiFi scan results via `wpa_cli` (feature-gated: `YANGERD_ENABLE_WIFI`) |
| POLLING 30 seconds | 3 | Ethtool statistics (counters only -- speed/duplex/autoneg now reactive), WireGuard peer data |
| POLLING 60 seconds | 8 | NTP state (chrony cmdmon protocol), system clock/uptime, users |
| POLLING 300 seconds | 5 | Hardware inventory (DMI chassis data), OS platform info |

### 5.10bis Polling Justification Notes

The remaining polling sources are intentionally polling because no reliable subscription/event interface exists for the required data:

- **WireGuard**: no kernel event stream for peer stats (`last-handshake`, `rx/tx bytes`); these values are available via `WG_CMD_GET_DEVICE` snapshots only.
- **FRR OSPF/RIP/BFD protocol state**: protocol internals are exposed through `vtysh show ...` request/response commands; FRR does not provide a stable streaming API for these views.
- **NTP (chrony)**: cmdmon protocol is strictly request/response (confirmed in revision 0.20); no subscribe mechanism.
- **Hardware sensors**: sysfs pseudo-files do not emit inotify modify events (confirmed in revision 0.14).
- **Ethtool statistics counters**: `ETHNL_MCGRP_MONITOR` emits setting-change notifications, not counter-change notifications.

**Startup note**: All netlink-reactive data paths perform an initial full dump on daemon startup, using the subscribe-first-then-list pattern (subscriptions established BEFORE dump, following Antrea's approach). Link, address, and neighbor data is populated by writing bulk query commands (`ip -s -d -j link show`, `ip -j addr show`, `ip -j neigh show`) to the persistent `ip -json -force -batch -` subprocess. Route data is populated by the ZAPI watcher, which connects to zebra's zserv socket and receives a full dump of all routes matching the subscribed redistribution types (kernel, connected, static, OSPF, RIP) upon initial connection -- see Section 4.1octies. This replaces the previous `vtysh`-based initial route dump. OSPF, RIP, and BFD protocol-specific data is still collected via `vtysh` polling (unchanged). This populates the tree before the NLMonitor's select loop begins processing incremental netlink events. Without this, the tree appears empty until the first kernel event fires for each interface.
## Module-by-Module Mapping

For each existing Python yanger script, this section documents the external commands and data sources it uses, and how those will be reimplemented as Go collector functions in yangerd. Each subsection covers what the Python code does, every external process it spawns (or file it reads), and the equivalent Go approach.

---

### ietf_interfaces — `python/yanger/ietf_interfaces/` → `internal/collector/interfaces.go` + `internal/monitor/`

**Python approach**: The package entry point (`__init__.py`) calls `link.interfaces()` and `container.interfaces()` to build the full `ietf-interfaces:interfaces` list. `link.py` delegates per-interface type handling to `ip.py`, `ethernet.py`, `bridge.py`, `wifi.py`, `wireguard.py`, `vlan.py`, and `lag.py`. Interface and address lists are pre-fetched by `common.py` and cached for the duration of the invocation.

**External commands invoked**:

| Command | Invoked in | Purpose |
|---------|-----------|---------|
| `ip -s -d -j link show [dev <if>]` | `common.py:iplinks()` | JSON dump of all link attributes, stats64, linkinfo (type, slave data), flags, operstate |
| `ip -j addr show [dev <if>]` | `common.py:ipaddrs()` | JSON dump of all interface addresses with family, prefix, protocol origin |
| `ip -j netns exec <ns> ip -s -d -j link show` | `common.py:iplinks(netns=...)` | Same as above but inside a container network namespace |
| `ip -j netns exec <ns> ip -j addr show` | `common.py:ipaddrs(netns=...)` | Address list inside a container network namespace |
| `ip -j netns list` | `container.py:ip_netns_list()` | Enumerate all named network namespaces (for container interfaces) |
| `ls /sys/class/net/<ifname>/wireless/` | `link.py:iplink2yang_type()` | Detect whether an `ether` link is a WiFi interface |

**Reads** `/proc/sys/net/ipv6/conf/<ifname>/mtu` (ip.py) for IPv6 MTU.

**Go replacement**:
- `ip -s -d -j link show` -> `ip -json -force -batch -` query (write `link show -s -d` to stdin); `vishvananda/netlink` `LinkSubscribeWithOptions` for events; stats64, operstate, flags, linkinfo all in JSON output
- `ip -j addr show` -> `ip -json -force -batch -` query (write `addr show` to stdin); `vishvananda/netlink` `AddrSubscribeWithOptions` for events
- `/proc/sys/net/ipv6/conf/<iface>/mtu` → `os.ReadFile()` directly; no process spawn needed
- Wireless detection → `os.Stat("/sys/class/net/<iface>/wireless")` (dir exists check)
- Container namespace traversal → deferred to Phase 2 (requires setns syscall or `ip netns exec` via `exec.Command`)

---

### ietf_interfaces/ethernet.py → `internal/collector/ethernet.go`

**Python approach**: Uses `ethtool --json` (twice per interface) to obtain speed/duplex/auto-negotiation and extended per-group statistics (eth-mac, rmon counters). Both calls emit JSON; the script maps counter names to YANG leaf names.

**External commands invoked**:

| Command | Purpose |
|---------|---------|
| `ethtool --json <ifname>` | Speed (Mbps), duplex, auto-negotiation enable flag |
| `ethtool --json -S <ifname> --all-groups` | Per-group hardware counters: `FramesTransmittedOK`, `FrameCheckSequenceErrors`, `OctetsReceivedOK`, etc. |

**Go replacement**:
- Both `ethtool` calls → `mdlayher/ethtool` Go library (uses generic netlink ETHTOOL_GENL family; no subprocess needed)
- Speed/duplex/autoneg → **REACTIVE** via `internal/ethmonitor/` package: subscribes to `ETHNL_MCGRP_MONITOR` genetlink multicast group; on `ETHTOOL_MSG_LINKINFO_NTF` or `ETHTOOL_MSG_LINKMODES_NTF`, re-queries via `ethtool.Client.LinkInfo()` + `ethtool.Client.LinkMode()` and writes updated values to tree immediately. Additionally, `ethmonitor.RefreshInterface()` is called by the link event handler on every RTM_NEWLINK event, because `ETHNL_MCGRP_MONITOR` does NOT fire on link up/down — only on explicit settings renegotiation. This cross-subsystem trigger ensures speed/duplex/autoneg converge within milliseconds of link state changes.
- Extended stats → `ethtool.Client.Stats()` keyed by counter name string (no kernel notification available for statistics — remains **POLLING** at 30-second interval)
- Strategy: **HYBRID** — reactive for settings (speed, duplex, autoneg) via ethmonitor (unconditionally active on kernel 6.18), polling for statistics (counters have no kernel notification).

---

### ietf_interfaces/bridge.py → `internal/collector/bridge.go`

**Python approach**: Queries bridge VLAN tables, STP state from `mstpctl`, and multicast group membership from `mctl`. STP data is fetched per-bridge and per-port using `mstpctl showtree`, `showbridge`, and `showportdetail`. Multicast data is fetched with `mctl show igmp json` and `bridge mdb show -j dev <br>`. VLAN global state is fetched with `bridge vlan global show dev <br>` and `bridge vlan show -j`.

**External commands invoked**:

| Command | Purpose |
|---------|---------|
| `bridge -j vlan show` | VLAN membership table for all ports (PVID, tagged/untagged flags) |
| `bridge -j vlan global show dev <brname>` | Per-VLAN global bridge settings (vlan list for VID population) |
| `bridge -j mdb show dev <br> [vid <v>]` | Multicast group database (MDB) entries per bridge and VLAN |
| `mstpctl -f json showbridge <brname>` | STP bridge state: force-protocol, hello-time, forward-delay, max-age, tx-hold-count |
| `mstpctl -f json showtree <brname> <msti>` | STP tree state: priority, bridge-id, root-port, topology-change |
| `mstpctl -f json showportdetail <brname> <port>` | Per-port STP state: edge, external-path-cost, BPDU statistics |
| `mstpctl -f json showtreeport <brname> <port> <msti>` | Per-port per-tree STP state: port-id, role, designated bridge/port |
| `mctl -p show igmp json` | IGMP/MLD querier state per bridge/VLAN (mode: off/proxy/auto, query-interval) |

- `bridge vlan show -j`, `bridge mdb show -j`, `bridge fdb show br <br>` -> persistent `bridge -json -batch -` query (write `vlan show`, `mdb show`, or `fdb show` to stdin); bridge events arrive via `vishvananda/netlink` channels (FDB via `NeighSubscribeWithOptions`, VLAN via `LinkSubscribeWithOptions`, MDB via raw netlink `RTNLGRP_MDB`, STP via `LinkSubscribeWithOptions` detecting `IFLA_BRPORT_STATE` in `IFLA_PROTINFO`). All events are triggers only -- full state is re-read via bridge batch.
- `mstpctl` and `mctl` calls → `exec.Command` with JSON parsing
- Strategy: **REACTIVE** (netlink event triggers + `bridge -json -batch -` re-reads); initial full state populates tree on startup via bridge batch queries

---

### ietf_interfaces/wifi.py → `internal/collector/wifi.go`

**Python approach**: Delegates to an on-device helper script `/usr/libexec/infix/iw.py`. For AP mode, gets interface info and connected station list. For station mode, gets link info and scan results from `wpa_cli`. Mode detection is done by calling `iw.py info <ifname>` and reading the `iftype` field.

**External commands invoked**:

| Command | Purpose |
|---------|---------|
| `/usr/libexec/infix/iw.py info <ifname>` | Interface mode (iftype), SSID, channel, TX power — wraps `iw dev <if> info` |
| `/usr/libexec/infix/iw.py station <ifname>` | Connected station list (AP mode) — wraps `iw dev <if> station dump` |
| `/usr/libexec/infix/iw.py link <ifname>` | Link info for station mode: SSID, signal strength, RX/TX speed |
| `wpa_cli -i <ifname> scan_result` | Network scan results (BSSID, SSID, RSSI, encryption flags) |

**Go replacement**:
- All `iw.py` calls → `exec.Command("iw", "dev", ifname, "info")`, `exec.Command("iw", "dev", ifname, "station", "dump")` with custom text parsing
- `wpa_cli scan_result` → `exec.Command("wpa_cli", "-i", ifname, "scan_result")` with text parsing
- Primary method: REACTIVE via `iw event -t` — WiFi events (station association/disassociation, connection, channel switch) trigger re-queries via short-lived `exec.Command("iw", ...)` subprocesses
- `wpa_cli` queries remain polling-based (no event interface available)
- `wpa_cli` queries remain polling-based (no event interface available)

---

### ietf_interfaces/wireguard.py → `internal/collector/wireguard.go`

**Python approach**: Runs `wg show <ifname> dump` and parses the tab-delimited output. The first line is the interface (skipped); subsequent lines are peers with public key, endpoint, allowed IPs, last handshake timestamp, RX/TX bytes, and persistent keepalive.

**External commands invoked**:

| Command | Purpose |
|---------|---------|
| `wg show <ifname> dump` | Peer list with endpoint, handshake time, RX/TX bytes, allowed IPs |

**Go replacement**:
- `wg show dump` → `golang.zx2c4.com/wireguard/wgctrl` library (`wgctrl.Client.Device(name)`)
- Returns `wgtypes.Device` with `Peers []wgtypes.Peer` including `Endpoint`, `LastHandshakeTime`, `ReceiveBytes`, `TransmitBytes`, `AllowedIPs` — no subprocess needed
- Poll interval: 30 seconds

---

### ietf_interfaces/vlan.py → part of `internal/collector/interfaces.go`

**Python approach**: Pure data transformation — maps the `linkinfo.info_data.protocol` string (`802.1Q`, `802.1ad`) and `id` field from the `ip link show -j` JSON into YANG identity values. No external commands are invoked directly; all data comes from the already-fetched `iplinks()` result.

**External commands invoked**: None (data comes from `ip -s -d -j link show` in `common.py:iplinks()`).

**Go replacement**:
- VLAN linkinfo data is present in the `ip -json link show` output (linkinfo.info_data object contains protocol and id fields)
- The `linkinfo.info_data.protocol` (802.1Q/802.1ad) and `id` fields map directly to YANG `tag-type` and `id`
- No separate collector needed; extracted from the ip batch link query response inline in the interfaces handler

---

### ietf_interfaces/container.py → `internal/collector/container_ifaces.go` (Phase 2, Feature-Gated)

**Feature gate**: `YANGERD_ENABLE_CONTAINERS=true`. This collector is only active when container support is included in the Infix build.

**Python approach**: Lists all named network namespaces via `ip -j netns list`, then for each namespace runs `ip -s -d -j link show` and `ip -j addr show` inside the namespace to find container interfaces (identified by `ifalias`). Cross-references with `podman ps` output to map interface names to container names.

**External commands invoked**:

| Command | Purpose |
|---------|---------|
| `ip -j netns list` | Enumerate Linux network namespaces (one per running container) |
| `ip netns exec <ns> ip -s -d -j link show` | Interface list and stats inside the container namespace |
| `ip netns exec <ns> ip -j addr show` | Address list inside the container namespace |
| `podman ps -a --format=json` | Running containers with network/Names for cross-referencing |

**Go replacement (Phase 2)**:
- `ip netns list` → `os.ReadDir("/run/netns")` or `exec.Command("ip", "-j", "netns", "list")`
- Per-namespace link/addr queries → `exec.Command("ip", "netns", "exec", nsName, "ip", "-json", "-s", "-d", "link", "show")` (cannot use the shared ip batch subprocess across namespaces)
- `podman ps` → `exec.Command("podman", "ps", "-a", "--format=json")`
- Deferred to Phase 2 due to namespace traversal complexity

---

### ietf_routing.py -> `internal/zapiwatcher/` + `internal/collector/routing.go`

**Python approach**: Fetches IPv4 and IPv6 route tables from FRRouting via `vtysh`, then lists all interfaces with IPv4/IPv6 forwarding enabled via `sysctl`. The `vtysh` JSON output includes route prefix, protocol, distance, metric, next hops, and active/installed flags.

**External commands invoked**:

| Command | Purpose |
|---------|---------|
| `vtysh -c 'show ip route json'` | Full IPv4 RIB from FRRouting (kernel, connected, static, OSPF, RIP routes) |
| `vtysh -c 'show ipv6 route json'` | Full IPv6 RIB from FRRouting |
| `ip -j link show` | Interface list for forwarding-enabled interface enumeration |
| `sysctl -n net.ipv4.conf.<iface>.forwarding` | IPv4 forwarding state per interface |
| `sysctl -n net.ipv6.conf.<iface>.force_forwarding` | IPv6 forwarding state per interface |

**Go replacement**:
- IPv4/IPv6 RIB -> `internal/zapiwatcher/` -- streaming ZAPI connection to zebra's zserv socket (Section 4.1octies). Replaces the previous netlink-triggered `vtysh` re-read approach. The ZAPI watcher subscribes to route redistribution for kernel, connected, static, OSPF, and RIP route types and receives incremental `REDISTRIBUTE_ROUTE_ADD` / `REDISTRIBUTE_ROUTE_DEL` messages. Upon connection, zebra sends a full dump of matching routes. Reconnects automatically on zebra restart with exponential backoff. Captures routes in zebra's RIB not installed in the kernel FIB.
- `sysctl` forwarding checks -> `os.ReadFile("/proc/sys/net/ipv4/conf/<iface>/forwarding")` and equivalent IPv6 path
- OSPF/RIP/BFD protocol-specific data continues to use dedicated `vtysh` commands via `exec.Command` in separate collectors (`ospf.go`, `rip.go`, `bfd.go`), poll 10 seconds

---

### ietf_ospf.py → `internal/collector/ospf.go`

**Python approach**: Queries two data sources from FRRouting. OSPF area/interface/neighbor state is fetched via an on-device helper `/usr/libexec/statd/ospf-status` (which wraps FRRouting `vtysh` calls into a structured JSON format). OSPF routes are fetched directly via `vtysh -c 'show ip ospf route json'`.

**External commands invoked**:

| Command | Purpose |
|---------|---------|
| `/usr/libexec/statd/ospf-status` | Structured OSPF JSON: router-id, areas, interfaces, neighbors, timers, DR/BDR |
| `vtysh -c 'show ip ospf route json'` | OSPF local RIB: prefixes, route types (intra/inter/external), next hops, metrics |

**Go replacement**:
- `ospf-status` helper → `exec.Command("vtysh", "-c", "show ip ospf json")` + `exec.Command("vtysh", "-c", "show ip ospf neighbor detail json")` + `exec.Command("vtysh", "-c", "show ip ospf interface json")`, parse and merge
- `vtysh show ip ospf route json` → `exec.Command("vtysh", "-c", "show ip ospf route json")`
- All `vtysh` calls go through `exec.Command`; FRRouting must be running (graceful skip if unavailable)
- Poll interval: 10 seconds

---

### ietf_rip.py → `internal/collector/rip.go`

**Python approach**: Combines two FRRouting queries. RIP status (timers, distance, default metric, interface table, neighbor table) is fetched as raw text via `vtysh -c 'show ip rip status'` and parsed with regular expressions. RIP-learned routes are fetched as JSON via `vtysh -c 'show ip route rip json'`.

**External commands invoked**:

| Command | Purpose |
|---------|---------|
| `vtysh -c 'show ip rip status'` | RIP global state: update/invalid/flush intervals, distance, default-metric, interface versions, neighbor last-update |
| `vtysh -c 'show ip route rip json'` | RIP-learned routes: prefix, metric, next-hop IP and interface |

**Go replacement**:
- `vtysh show ip rip status` → `exec.Command("vtysh", "-c", "show ip rip status")`, text output parsed with regexp (same approach; no JSON alternative in FRR for this command)
- `vtysh show ip route rip json` → `exec.Command("vtysh", "-c", "show ip route rip json")`, JSON unmarshal
- Poll interval: 10 seconds; graceful skip if FRRouting not running

---

### ietf_bfd_ip_sh.py → `internal/collector/bfd.go`

**Python approach**: Fetches all BFD peer state from FRRouting via a single `vtysh` command. Filters to single-hop sessions only. Extracts discriminators, session state, timing intervals (in milliseconds, converted to microseconds for YANG), and derives detection time from multiplier × receive-interval.

**External commands invoked**:

| Command | Purpose |
|---------|---------|
| `vtysh -c 'show bfd peers json'` | BFD peer list: peer IP, interface, local/remote discriminator, status, intervals, detect-multiplier |

**Go replacement**:
- `vtysh show bfd peers json` → `exec.Command("vtysh", "-c", "show bfd peers json")`, JSON unmarshal
- Poll interval: 10 seconds; graceful skip if FRRouting not running

---

### ietf_hardware.py → `internal/collector/hardware.go`

**Python approach**: Assembles hardware component list from five sub-sources: (1) `/run/system.json` (board VPD data written by confd at boot), (2) USB port authorization from `/sys/bus/usb/devices/*/authorized_default`, (3) hwmon sensor files under `/sys/class/hwmon/hwmon*/temp*_input`, `fan*_input`, `in*_input`, `curr*_input`, `power*_input`, (4) thermal zones under `/sys/class/thermal/thermal_zone*/temp`, and (5) WiFi radio PHY info via the `iw.py` helper (only when `YANGERD_ENABLE_WIFI=true`) and GPS receiver status from `/run/gps-status.json` (only when `YANGERD_ENABLE_GPS=true`).

**External commands invoked**:

| Command / File | Purpose |
|---------------|---------|
| `/run/system.json` (file read) | Board VPD (vendor, product, serial, MAC, USB port list) — written by confd at startup |
| `ls /sys/class/hwmon` | Enumerate hwmon entries |
| `/sys/class/hwmon/hwmon*/name` (file read) | Device name for sensor component naming |
| `/sys/class/hwmon/hwmon*/{temp,fan,in,curr,power}*_input` (file reads) | Raw sensor values (millidegrees, RPM, millivolts, milliamps, microwatts) |
| `/sys/class/hwmon/hwmon*/{temp,fan,in,curr,power}*_label` (file reads) | Human-readable sensor label |
| `ls /sys/class/thermal` | Enumerate thermal zones |
| `/sys/class/thermal/thermal_zone*/type` (file read) | Thermal zone type name |
| `/sys/class/thermal/thermal_zone*/temp` (file read) | Temperature in millidegrees Celsius |
| `/usr/libexec/infix/iw.py list` | List all WiFi PHY names (only when `YANGERD_ENABLE_WIFI=true`) |
| `/usr/libexec/infix/iw.py dev` | Map PHY numbers to virtual interface names (only when `YANGERD_ENABLE_WIFI=true`) |
| `/usr/libexec/infix/iw.py info <phy>` | Per-PHY capabilities: bands, driver, manufacturer, interface combinations (only when `YANGERD_ENABLE_WIFI=true`) |
| `/usr/libexec/infix/iw.py survey <ifname>` | Per-channel survey data (frequency, noise, active/busy/receive/transmit time) (only when `YANGERD_ENABLE_WIFI=true`) |
| `readlink -f /dev/gps<n>` | Resolve GPS device symlinks to actual device paths (only when `YANGERD_ENABLE_GPS=true`) |
| `/run/gps-status.json` (file read) | Cached GPS/GNSS operational state (driver, fix mode, lat/lon/alt, satellite counts) (only when `YANGERD_ENABLE_GPS=true`) |
| `/sys/bus/usb/devices/*/authorized_default` (file reads) | USB port lock state (1=unlocked, 0=locked) |

- `/run/system.json` → `os.ReadFile()` + JSON unmarshal
- hwmon sensor files (`*_input`, `*_fault`) → `collector/hardware.go` polling every 10 seconds (sysfs pseudo-files do not emit inotify events; the kernel generates values on `read()`, never calling `fsnotify_modify()`)
- thermal zone files (`temp`) → `collector/hardware.go` polling every 10 seconds (same sysfs limitation)
- `iw.py` calls → `exec.Command("iw", "list")`, `exec.Command("iw", "dev")`, `exec.Command("iw", "phy", phyName, "info")`, `exec.Command("iw", "dev", ifname, "survey", "dump")` with custom text or JSON parsing (skipped when `YANGERD_ENABLE_WIFI=false`)
- GPS status → `os.ReadFile("/run/gps-status.json")` + JSON unmarshal (skipped when `YANGERD_ENABLE_GPS=false`)
- USB port state → `os.ReadFile(authorizedDefaultPath)`
- Strategy: **POLLING** 10 seconds for sensors (sysfs inotify impossibility); **POLLING** 300 seconds for static inventory (VPD, chassis data)

---

### ietf_system.py → `internal/collector/system.go`

**Python approach**: Assembles `ietf-system:system` and `ietf-system:system-state` from multiple sub-collectors: hostname, users from `/etc/passwd` and `/etc/shadow` via `getent`, SSH authorized keys, timezone from `realpath /etc/localtime`, NTP sources via `chronyc`, DNS from `/etc/resolv.conf.head` and `resolvconf -l`, RAUC slot status via `rauc status`, init service list via `initctl`, boot-order from `fw_printenv`/`grub-editenv`, and resource usage from `/proc/meminfo`, `/proc/loadavg`, and `df`.

**External commands invoked**:

| Command / File | Purpose |
|---------------|---------|
| `hostname` | System hostname |
| `getent passwd` | User list with UID, shell path |
| `getent shadow` | Password hashes for users with 1000 ≤ UID < 10000 |
| `/var/run/sshd/<user>.keys` (file read) | SSH authorized keys per user |
| `realpath /etc/localtime` | Resolve timezone symlink to zone name |
| `chronyc -c sources` | NTP source list: mode, state, address, stratum, poll, reach, offset |
| `/etc/resolv.conf.head` (file read) | Static DNS nameservers and search domains |
| `/sbin/resolvconf -l` | DHCP-assigned DNS nameservers |
| `rauc status --detailed --output-format=json` | RAUC software slot status: compatible, variant, booted slot, installed/activated timestamps |
| `rauc-installation-status` | In-progress upgrade: operation type, progress percentage, message |
| `initctl -j` | Finit service list: PID, identity, status, description, memory, uptime, restart-count |
| `fw_printenv BOOT_ORDER` | U-Boot boot order (preferred slot ordering) |
| `grub-editenv /mnt/aux/grub/grubenv list` | GRUB boot order (x86 targets) |
| `/etc/os-release` (file read) | OS name, version ID, build ID, architecture |
| `/proc/meminfo` (file read) | MemTotal, MemFree, MemAvailable |
| `/proc/loadavg` (file read) | 1-min, 5-min, 15-min load averages |
| `df -k <mount>` | Filesystem size/used/available for `/`, `/var`, `/cfg` |
| `/proc/uptime` (file read) | System uptime in seconds (for boot-datetime calculation) |

- `hostname` → `os.Hostname()`
- `/etc/passwd`, `/etc/shadow` → `os.ReadFile()` + line parsing
- `realpath /etc/localtime` → `os.Readlink("/etc/localtime")`
- NTP data → handled by `internal/collector/ntp.go` via `github.com/facebook/time/ntp/chrony` cmdmon protocol (not by system.go)
- `/etc/resolv.conf.head` → `os.ReadFile()`
- `resolvconf -l` → `exec.Command("/sbin/resolvconf", "-l")`
- `rauc status` → `exec.Command("rauc", "status", "--detailed", "--output-format=json")`
- `initctl -j` → `exec.Command("initctl", "-j")`
- `fw_printenv` / `grub-editenv` → `exec.Command(...)` with fallback
- `/proc/meminfo`, `/proc/loadavg`, `/proc/uptime`, `/etc/os-release` → `os.ReadFile()`
- `/proc/sys/net/ipv4/conf/*/forwarding` → `fswatcher` inotify (reactive updates)
- `df -k` → `syscall.Statfs()` per mount point
- Strategy: **REACTIVE** for IP forwarding; **POLLING** 60 seconds for clock/NTP; **POLLING** 300 seconds for static data

---
- All `chronyc -c` calls -> native Go cmdmon protocol via `github.com/facebook/time/ntp/chrony` over Unix socket `/var/run/chrony/chronyd.sock`. This eliminates all subprocess spawning for NTP data collection. The library speaks chrony's undocumented cmdmon protocol v6 natively, providing typed Go structs for tracking, sources, and sourcestats responses.
- Poll interval: configured via `YANGERD_POLL_INTERVAL_NTP` (default 60 seconds)

### ietf_ntp.py → `internal/collector/ntp.go`

**Python approach**: Calls `chronyc` three times per collection cycle: once for `sources` (NTP association list), once for `sourcestats` (offset/std-dev per source), and once for `tracking` (clock state, stratum, refid, root delay/dispersion). A fourth call to `serverstats` provides NTP server packet statistics. Additionally queries `ss -ulnp` to determine the listening UDP port.

**External commands invoked**:

| Command | Purpose |
|---------|---------|
| `chronyc -c sources` | NTP source list: mode, state, address, stratum, poll, reach, lastRx, offset, error |
| `chronyc -c sourcestats` | Per-source statistics: estimated offset and standard deviation |
| `chronyc -c tracking` | Global clock state: stratum, refid, system offset, root delay, root dispersion, frequency, leap status |
| `chronyc -c serverstats` | Server statistics: packets received/dropped/sent/failed |
| `ss -ulnp` | UDP listening sockets — identify chronyd's NTP port |

**Go replacement**:
- All `chronyc -c` calls -> native Go cmdmon protocol via `github.com/facebook/time/ntp/chrony` over Unix socket `/var/run/chrony/chronyd.sock`. This eliminates all subprocess spawning for NTP data collection. The library speaks chrony's undocumented cmdmon protocol v6 natively, providing typed Go structs for tracking, sources, and sourcestats responses.
- `ss -ulnp` -> parse `/proc/net/udp` directly for port 123
- Poll interval: 60 seconds
- Investigation confirmed chrony has no D-Bus interface, no event-driven socket protocol, and no subscribe/push mechanism. The cmdmon protocol is strictly request-response (client sends `CMD_Request`, daemon sends `CMD_Reply` -- no server-initiated messages, no subscription opcodes). Polling is the only supported monitoring approach, per upstream chrony design.

---

### infix_lldp.py → `internal/lldpmonitor/`

**Python approach**: Queries lldpd for its LLDP neighbor database in JSON format via `lldpcli`. Parses per-interface chassis-id and port-id (with subtype mapping), constructs `remote-systems-data` entries grouped by local port name.

**External commands invoked**:

| Command | Purpose |
|---------|---------|
| `lldpcli show neighbors -f json` | LLDP neighbor table snapshot: per-interface chassis-id, port-id, age (for time-mark), rid |

**Go replacement**:
- Snapshot polling is replaced with a persistent subprocess monitor: `lldpcli -f json0 watch`
- Stream parser handles pretty JSON objects delimited by blank lines (`\n\n`) and dispatches `lldp-added`, `lldp-updated`, `lldp-deleted` events
- `json0` output is required for structural stability (arrays always arrays)
- Strategy: **REACTIVE** via `internal/lldpmonitor/` (persistent subprocess + framing-aware parser)

---

### infix_containers.py → `internal/collector/containers.go` (Phase 2, Feature-Gated)

**Feature gate**: `YANGERD_ENABLE_CONTAINERS=true`. This collector is only active when container support is included in the Infix build. When `YANGERD_ENABLE_CONTAINERS=false`, the container collector is not started and no container data appears in the tree.

**Python approach**: Lists all containers (including stopped) via `podman ps -a --format=json`. For each container, runs `podman inspect <name>` for network settings and cgroup path, reads cgroup resource limit files directly (`/sys/fs/cgroup<cgroupPath>/memory.max`, `cpu.max`), and runs `podman stats --no-stream --format json <name>` for live CPU/memory/IO/PID usage.

**External commands invoked**:

| Command / File | Purpose |
|---------------|---------|
| `podman ps -a --format=json` | Full container list: name, ID, image, state, status, ports, networks |
| `podman inspect <name>` | NetworkSettings (host mode detection), CgroupPath |
| `/sys/fs/cgroup<path>/memory.max` (file read) | Container memory limit in bytes |
| `/sys/fs/cgroup<path>/cpu.max` (file read) | Container CPU quota and period (for millicores calculation) |
| `podman stats --no-stream --format json --no-reset <name>` | Live resource usage: memory, CPU%, block I/O, network I/O, PIDs |

**Go replacement (Phase 2)**:
- `podman ps -a --format=json` → `exec.Command("podman", "ps", "-a", "--format=json")`
- `podman inspect <name>` → `exec.Command("podman", "inspect", name)`
- cgroup file reads → `os.ReadFile(cgroupBasePath + "/memory.max")` etc.
- `podman stats` → `exec.Command("podman", "stats", "--no-stream", "--format", "json", "--no-reset", name)`
- Poll interval: 10 seconds

---

### infix_dhcp_server.py → `internal/collector/dhcp.go`

**Python approach**: Two data sources. (1) Reads the dnsmasq lease file directly (`/var/lib/misc/dnsmasq.leases`) — a whitespace-delimited flat file with expiry timestamp, MAC, IP, hostname, and client-id per line. (2) Queries dnsmasq DHCP statistics via D-Bus (`uk.org.thekelleys.dnsmasq` interface `GetMetrics()`) for offer/ack/nak/discover/request/release/inform counters.

**External commands invoked**:

| Command / Source | Purpose |
|-----------------|---------|
| `/var/lib/misc/dnsmasq.leases` (file read) | DHCP lease table: expiry epoch, MAC, IP, hostname, client-id |
| D-Bus `uk.org.thekelleys.dnsmasq` `GetMetrics()` | DHCP packet counters: offers, acks, naks, declines, discovers, requests, releases, informs |

- Lease file (`/var/lib/misc/dnsmasq.leases`) → D-Bus Monitor `refreshDHCP()`, triggered by dnsmasq D-Bus signals (`DHCPLeaseAdded`, `DHCPLeaseDeleted`, `DHCPLeaseUpdated`)
- D-Bus metrics → `godbus/dbus/v5` package: `bus.Object("uk.org.thekelleys.dnsmasq", ...).Call("GetMetrics", 0)`, called as part of the same `refreshDHCP()` handler
- Strategy: **REACTIVE** (D-Bus signals for lease events; metrics queried on each lease change)

---

### infix_firewall.py → `internal/collector/firewall.go`

**Python approach**: Queries firewalld entirely via D-Bus (no subprocess calls). Connects to `org.fedoraproject.FirewallD1` on the system bus and calls methods on the zone, policy, and service interfaces to enumerate active zones, policies, and services. Zone settings include interfaces, sources, services, port-forwards, and rich rules for ICMP filters. Policy settings include ingress/egress zones, action, priority, masquerade, and rich rules.

**External commands invoked**:

| Source | Method | Purpose |
|--------|--------|---------|
| D-Bus `org.fedoraproject.FirewallD1` | `getDefaultZone()`, `getLogDenied()`, `queryPanicMode()` | Global firewall state |
| D-Bus `org.fedoraproject.FirewallD1.zone` | `getActiveZones()`, `getZoneSettings2(<name>)` | Active zone list and per-zone settings (interfaces, sources, services, port-forwards) |
| D-Bus `org.fedoraproject.FirewallD1.policy` | `getPolicies()`, `getPolicySettings(<name>)` | Policy list and per-policy settings (ingress/egress zones, action, priority, rich-rules) |
| D-Bus `org.fedoraproject.FirewallD1` | `listServices()`, `getServiceSettings2(<name>)` | Service definitions with port/protocol |

**Go replacement**:
- Data source preserved from Python: firewalld D-Bus method calls (`getDefaultZone()`, `getActiveZones()`, `getZoneSettings2()`, `getPolicies()`, `getPolicySettings()`, `listServices()`, `getServiceSettings2()`, `getLogDenied()`, `queryPanicMode()`)
- The Go implementation uses `godbus/dbus/v5` `Object.CallWithContext()` to invoke the same firewalld D-Bus methods that the Python code uses via `dbus.Interface`
- No subprocess execution (`nft`, `iptables`, etc.) is involved -- all data flows through the firewalld D-Bus API
- Trigger: firewalld D-Bus signals (`Reloaded` + `NameOwnerChanged` for restart detection) via D-Bus Monitor (Section 4.1novies)
- Strategy: **REACTIVE** (D-Bus signal trigger + D-Bus method call data retrieval)

---

### Summary Table

| Python Module | Go File | Primary Method | Phase |
|--------------|---------|----------------|-------|
| `ietf_interfaces/__init__.py` + `link.py` | `internal/monitor/link.go` + `internal/monitor/addr.go` | `vishvananda/netlink` LinkUpdate + AddrUpdate channels (event trigger) + full re-read via ip batch on every event (link: 3 queries + ethmonitor; addr: 1 query) | 1 |
| `ietf_interfaces/ip.py` | `internal/monitor/addr.go` | `vishvananda/netlink` AddrUpdate channel (event trigger) + full address re-read via `ip -json -batch` on every RTM_NEWADDR/RTM_DELADDR + `os.ReadFile` /proc/sys | 1 |
| `ietf_interfaces/ethernet.py` | `internal/collector/ethernet.go` + `internal/ethmonitor/` | `mdlayher/ethtool` + `mdlayher/genetlink` (reactive settings via ETHNL_MCGRP_MONITOR; polling stats) | 1 |
| `ietf_interfaces/bridge.py` | `internal/collector/bridge.go` | Bridge netlink event triggers (FDB via NeighUpdate, VLAN via LinkUpdate, STP via LinkUpdate with IFLA_BRPORT_STATE, MDB via raw RTNLGRP_MDB) + `bridge -json -batch -` re-reads | 1 |
| `ietf_interfaces/wifi.py` | `internal/collector/wifi.go` + `internal/iwmonitor/` | `iw event -t` (reactive) + `exec.Command` iw + wpa_cli | 2 (feature-gated: `YANGERD_ENABLE_WIFI`) |
| `ietf_interfaces/wireguard.py` | `internal/collector/wireguard.go` | `golang.zx2c4.com/wireguard/wgctrl` | 1 |
| `ietf_interfaces/vlan.py` | `internal/monitor/link.go` (inline) | `ip -json -batch` link query (linkinfo.info_data fields) | 1 |
| `ietf_interfaces/container.py` | `internal/collector/container_ifaces.go` | `exec.Command` ip-netns (cannot share ip batch across namespaces) | 2 (feature-gated: `YANGERD_ENABLE_CONTAINERS`) |
| `ietf_routing.py` | `internal/zapiwatcher/` + `internal/collector/routing.go` | ZAPI v6 streaming connection to zebra (`osrg/gobgp/v4/pkg/zebra`) for route redistribution; replaces `vtysh` for route table collection. See Section 4.1octies. | 1 |
| `ietf_ospf.py` | `internal/collector/ospf.go` | `exec.Command` vtysh | 2 |
| `ietf_rip.py` | `internal/collector/rip.go` | `exec.Command` vtysh | 2 |
| `ietf_bfd_ip_sh.py` | `internal/collector/bfd.go` | `exec.Command` vtysh | 2 |
| `ietf_hardware.py` | `internal/collector/hardware.go` | `os.ReadFile` sysfs (sensors, polling 10s) + `exec.Command` dmidecode (inventory) | 2 |
| `ietf_system.py` | `internal/collector/system.go` | `os.ReadFile` /proc/* + `/etc/*` + `exec.Command` rauc/initctl | 2 |
| `ietf_ntp.py` | `internal/collector/ntp.go` | `github.com/facebook/time/ntp/chrony` cmdmon protocol over Unix socket | 2 |
| `infix_lldp.py` | `internal/lldpmonitor/` | Persistent `lldpcli -f json0 watch` subprocess + stream parser (`lldp-added`/`updated`/`deleted`) | 2 |
| `(new — from statd/avahi.c)` | `internal/mdnsmonitor/` | Avahi D-Bus `ServiceTypeBrowser`/`ServiceBrowser`/`ServiceResolver` signals | 1 |
| `infix_containers.py` | `internal/collector/containers.go` | `exec.Command` podman + `os.ReadFile` cgroup | 2 (feature-gated: `YANGERD_ENABLE_CONTAINERS`) |
| `infix_dhcp_server.py` | `internal/dbusmonitor/dbusmonitor.go` | D-Bus Monitor: dnsmasq signals (`DHCPLeaseAdded`/`Deleted`/`Updated`) → `refreshDHCP()` (lease file re-read + `GetMetrics()`) | 1 |
| `infix_firewall.py` | `internal/dbusmonitor/dbusmonitor.go` | D-Bus Monitor: firewalld signals (`Reloaded` + `NameOwnerChanged`) → `refreshFirewall()` (firewalld D-Bus method calls: zones, policies, services, global state) | 1 |
## 6. Project Structure

The yangerd Go module lives at `src/yangerd/` inside the Infix repository, following the existing Infix pattern where each daemon is a self-contained subdirectory under `src/`.

```
src/yangerd/
├── cmd/
│   ├── yangerd/
│   │   └── main.go          # daemon entry point: flag parsing, signal handling, errgroup
│   └── yangerctl/
│       └── main.go          # CLI tool: subcommands get/health/dump/watch
├── internal/
│   ├── tree/
│   │   └── tree.go          # Tree type: per-model sync.RWMutex + map[string]*modelEntry
│   ├── monitor/
│   │   ├── link.go          # RTNLGRP_LINK goroutine: full interface re-read (3 ip batch queries) + ethmonitor.RefreshInterface() + last-change
│   │   ├── addr.go          # RTNLGRP_*IFADDR goroutine: full address re-read via ip batch on any addr event
│   │   ├── neigh.go         # RTNLGRP_NEIGH goroutine: full neighbor re-read via ip batch on any neigh event
│   ├── collector/
│   │   ├── collector.go     # Collector interface + RunAll() loop
│   │   ├── bridge.go        # Bridge STP/VLAN/FDB: exec bridge + /sys/class/net
│   │   ├── wifi.go          # WiFi state: exec iw dev
│   │   ├── wireguard.go     # WireGuard peers: wgctrl
│   │   ├── ethtool.go       # Ethernet speed/duplex/autoneg: mdlayher/ethtool
│   │   ├── ospf.go          # OSPF state: exec vtysh -c 'show ip ospf ...'
│   │   ├── rip.go           # RIP state: exec vtysh
│   │   ├── bfd.go           # BFD sessions: exec vtysh
│   │   ├── hardware.go      # Hardware sensors + inventory: /sys/class/hwmon + dmidecode
│   │   ├── system.go        # System state: /proc, /etc/os-release, uname
│   │   ├── ntp.go           # NTP sync status: chrony cmdmon protocol via facebook/time
│   │   ├── lldp.go          # LLDP transform helpers (fed by LLDP monitor events)
│   │   ├── containers.go    # Container state: exec podman ps (Phase 2, feature-gated: YANGERD_ENABLE_CONTAINERS)
│   │   ├── dhcp.go          # DHCP lease parsing: parseDnsmasqLeases() + buildDHCPTree() (called by D-Bus Monitor)
│   │   └── firewall.go      # Firewall state: buildFirewallTree() from firewalld D-Bus data (called by D-Bus Monitor)
│   ├── ipc/
│   │   ├── server.go        # Unix socket listener + connection handler goroutines
│   │   ├── client.go        # Client dial/query helper (used by yangerctl)
│   │   └── protocol.go      # Request/Response types + marshal/unmarshal
│   ├── ipbatch/
│   │   └── batch.go         # IPBatch subprocess manager: persistent ip -json -force -batch -
│   ├── fswatcher/
│   │   └── fswatcher.go     # inotify/fsnotify goroutine: watches procfs forwarding flags
│   ├── nlmonitor/
│   │   └── nlmonitor.go     # NLMonitor: vishvananda/netlink subscriptions (link, addr, neigh, and bridge FDB/VLAN/MDB). Route data comes from ZAPI watcher -- no netlink route group subscription.
│   ├── iwmonitor/
│   │   ├── monitor.go       # iw event -t subprocess manager + event parser
│   │   └── query.go         # Short-lived iw re-query helpers (info, station dump)
│   ├── lldpmonitor/
│   │   └── monitor.go       # lldpcli -f json0 watch subprocess manager + framed JSON parser
│   ├── ethmonitor/
│   │   └── ethmonitor.go    # Ethtool genetlink monitor: ETHNL_MCGRP_MONITOR subscription
│   ├── zapiwatcher/
│   │   └── zapiwatcher.go  # ZAPI watcher: connects to zebra zserv socket, subscribes to route redistribution, maintains route tree with reconnection
│   ├── dbusmonitor/
│   │   └── dbusmonitor.go   # D-Bus Monitor: dnsmasq lease signals + firewalld reload signals → reactive data refresh
│   ├── mdnsmonitor/
│   │   └── mdnsmonitor.go   # Avahi D-Bus monitor: reactive mDNS service browse/resolve updates
│   ├── scheduler/
│   │   └── scheduler.go     # Runs collectors via time.NewTicker at configured intervals
│   └── config/
│       └── config.go        # Config struct: socket path, polling intervals, log level
├── go.mod                   # module github.com/kernelkit/infix/src/yangerd
├── go.sum
└── Makefile                 # cross-compilation targets for Buildroot integration
```

### Package Descriptions

**`cmd/yangerd/`** — The main daemon entry point. Initializes the in-memory tree, configuration, and all monitor/collector subsystems. Orchestrates the startup sequence (Section 4.2.2) and runs them under a single `errgroup.Group` used purely as a goroutine join point for clean shutdown. All `Run()` methods follow a strict error-swallowing contract: internal failures (subprocess crashes, netlink subscription errors, collector timeouts) are logged and retried internally within each goroutine. A `Run()` method only returns when `ctx.Done()` fires (i.e., on SIGTERM/SIGINT). This ensures that a single collector failure never propagates to cancel unrelated monitors or the IPC server.

**`cmd/yangerctl/`** — A developer and operator CLI tool. Subcommands include `get <path>` (query yangerd and print JSON), `health` (display monitor status and tree size), `dump` (print the full in-memory tree), and `watch <path>` (poll and print changes). Connects to `/run/yangerd.sock` using the same IPC protocol as statd.

**`internal/tree/`** -- The `Tree` type: a `map[string]*modelEntry` where each `modelEntry` holds its own `sync.RWMutex`, its `updated` timestamp, and a `json.RawMessage`. A top-level `sync.RWMutex` protects the map structure. Provides `Set(key, raw)`, `Get(key) json.RawMessage`, `GetMulti(keys) []json.RawMessage`, and `LastUpdated(key)` accessor. This package has no external dependencies -- it only imports `sync` and `encoding/json` from the standard library.

**`internal/monitor/`** — Event dispatcher goroutines that consume native Go netlink channel events via `vishvananda/netlink` subscriptions (LinkUpdate, AddrUpdate, NeighUpdate) and trigger state re-queries. For link, address, and neighbor events, re-queries go through the `ip -json -force -batch -` subprocess. Route data is sourced exclusively from the ZAPI watcher (see `internal/zapiwatcher/`) -- yangerd does not subscribe to netlink route groups. Each monitor follows the `Run(ctx context.Context) error` signature and calls `tree.Set()` after parsing the JSON response.

**`internal/ipbatch/`** — Manages the persistent `ip -json -force -batch -` subprocess for state queries. `batch.go` implements the `IPBatch` type that maintains a persistent `ip -json -force -batch -` process, writing query commands to stdin and reading JSON array responses from stdout. Includes health monitoring, automatic restart with exponential backoff, and canary-query validation after restarts. This package handles data acquisition only — event monitoring is handled by the `internal/nlmonitor/` package via native Go netlink subscriptions.

**`internal/collector/`** — Polling-based supplementary collectors for data not exposed via netlink. Each file implements the `Collector` interface:

```go
type Collector interface {
    Name() string
    Interval() time.Duration
    Collect(ctx context.Context, tree *tree.Tree) error
}
```

`collector.go` provides `RunAll(ctx, collectors, tree)`, which launches one goroutine per collector and ticks it on its configured interval. Failed `Collect()` calls are logged and retried on the next tick — a single collector failure does not affect other collectors or the IPC server.

**`internal/fswatcher/`** — Implements reactive monitoring for filesystem-based data sources that support inotify. It runs a single event loop that subscribes to inotify events via the `fsnotify` library. Paths like `/proc/sys/net/ipv4/conf/*/forwarding` (IP forwarding flags) are added at startup (with glob expansion). Modification events trigger a debounced re-read of the affected file, updating the tree immediately. Handles the `IN_IGNORED` event by automatically re-adding watches after file deletion/recreation. Note: sysfs pseudo-files under `/sys/class/hwmon/` and `/sys/class/thermal/` are NOT watched here — they do not emit inotify events (the kernel generates values on `read()`) and are instead collected by `collector/hardware.go` via polling. Bridge STP state is NOT watched here — it is handled reactively via netlink events and `bridge -json -batch -` re-reads. DHCP leases and firewall state are NOT watched here — they are handled reactively via D-Bus signals in `internal/dbusmonitor/`.

**`internal/nlmonitor/`** — Implements native Go netlink event subscriptions via `vishvananda/netlink`. The `NLMonitor` struct subscribes to LinkUpdate, AddrUpdate, and NeighUpdate channels, plus bridge-specific events (FDB entries via NeighUpdate with NDA_MASTER flag, VLAN changes via LinkUpdate, STP port state changes via LinkUpdate with IFLA_BRPORT_STATE in IFLA_PROTINFO, MDB entries via raw RTNLGRP_MDB subscription). All bridge events are used as triggers only — full state is re-read via the `bridge -json -batch -` subprocess. yangerd does NOT subscribe to netlink route groups (RTNLGRP_IPV4_ROUTE, RTNLGRP_IPV6_ROUTE) — route data is sourced exclusively from the ZAPI watcher. A single `select` loop dispatches events to the appropriate monitor goroutines. Uses context cancellation for clean shutdown and a shared error callback for automatic re-subscription on netlink errors.
After any subscription error, the error callback performs a full-scope re-read of ALL entities for the affected event type — not just the entity that was being processed when the error occurred. For example, a link subscription error triggers `ip -json -force -batch -` queries for every known interface (link show, addr show, neigh show) to resynchronize the entire tree. Events that occurred during the error/resubscribe window are inherently lost (netlink provides no replay), so only a full re-read guarantees consistency.


**`internal/zapiwatcher/`** — Implements a streaming ZAPI client that connects to FRR zebra's zserv unix domain socket (`/var/run/frr/zserv.api`) and subscribes to route redistribution notifications. On startup, the watcher sends `ZEBRA_HELLO`, `ZEBRA_ROUTER_ID_ADD`, and `ZEBRA_REDISTRIBUTE_ADD` messages for each route type (kernel, connected, static, OSPF, RIP), which causes zebra to send a full RIB dump followed by incremental `REDISTRIBUTE_ROUTE_ADD` and `REDISTRIBUTE_ROUTE_DEL` notifications. Routes are parsed from `IPRouteBody` into the in-memory tree keyed by prefix and protocol. This captures routes that exist in zebra's RIB but not in the Linux kernel FIB (unresolvable nexthop, lost admin-distance election, ECMP overflow, table-map filtered). Reconnection is automatic with exponential backoff (100ms initial, 30s max, factor 2x); on reconnect, the full subscription handshake is replayed and the route subtree is replaced atomically to clear stale entries. Uses `github.com/osrg/gobgp/v4/pkg/zebra` for ZAPI v6 message framing. The watcher signals readiness via the same `sync.WaitGroup` mechanism used by the netlink monitors.
On disconnection, the watcher immediately clears the route subtree from the in-memory tree (`tree.Set("ietf-routing:routing", nil)`) rather than serving stale routes. This makes the data gap explicit: during the disconnection window, route queries return empty data. This matches the principle that stale routing data is worse than absent routing data — an operator seeing no routes knows something is wrong, whereas stale routes may silently black-hole traffic. On successful reconnect, the full RIB dump repopulates the subtree atomically.


**`internal/dbusmonitor/`** — Implements reactive monitoring for dnsmasq DHCP lease events and firewalld configuration reloads via D-Bus signal subscriptions. The `DBusMonitor` struct connects to the system D-Bus bus, subscribes to dnsmasq signals (`DHCPLeaseAdded`, `DHCPLeaseDeleted`, `DHCPLeaseUpdated`) and firewalld signals (`Reloaded`), and monitors `NameOwnerChanged` for service lifecycle detection. On dnsmasq signals, it re-reads `/var/lib/misc/dnsmasq.leases` and calls `GetMetrics()` via D-Bus method call, combining lease data and packet counters into the YANG tree. On firewalld signals, it re-reads the full firewall state via firewalld D-Bus method calls (`getDefaultZone()`, `getActiveZones()`, `getZoneSettings2()`, `getPolicies()`, `getPolicySettings()`, `listServices()`, `getServiceSettings2()`, `getLogDenied()`, `queryPanicMode()`) and transforms the results into the YANG tree. Service absence is handled via `NameOwnerChanged`: when a service stops, the corresponding tree key is cleared to empty; when it starts, a full data refresh is performed. Reconnection to the D-Bus bus is automatic with exponential backoff (100ms initial, 30s max, factor 2x). Follows the same `Run(ctx context.Context) error` signature as all other monitors. Uses `github.com/godbus/dbus/v5` for D-Bus connectivity. See Section 4.1novies.

**`internal/iwmonitor/`** — Manages the persistent `iw event -t` subprocess for reactive 802.11 wireless monitoring. The `monitor.go` file contains the `IWMonitor` struct, the event parsing logic (`parseIWEvent`), and the main event loop goroutine. The `query.go` file provides helper functions that spawn short-lived `exec.Command("iw", ...)` subprocesses to re-query WiFi state (interface info, station list) in response to events. The package is initialized only when `YANGERD_ENABLE_WIFI=true`; when WiFi is not included in the build, it is not started.

**`internal/ethmonitor/`** — Implements the native Go genetlink subscription to the kernel's ethtool `ETHNL_MCGRP_MONITOR` multicast group for reactive Ethernet settings monitoring. The `ethmonitor.go` file contains the `EthMonitor` struct, which holds a `genetlink.Conn` (for the multicast subscription) and an `ethtool.Client` (for typed re-queries). On receiving `ETHTOOL_MSG_LINKINFO_NTF` or `ETHTOOL_MSG_LINKMODES_NTF` notifications, the monitor re-queries speed, duplex, and auto-negotiation via the ethtool client and writes updated values to the in-memory tree. Unlike `iwmonitor` and `brmonitor`, this package does not manage a subprocess—it uses a native genetlink socket. The `RefreshInterface(ifname)` public method is called by the link monitor on every RTM_NEWLINK event, since ETHNL_MCGRP_MONITOR does not fire on link state transitions. On Infix's target kernel (6.18), ethtool netlink is unconditionally available — the ethmonitor is always active. If the `ETHNL_MCGRP_MONITOR` subscription fails at startup (e.g., on a developer machine with an older kernel), ethmonitor logs a fatal error and does not start. There is no fallback to polling — developers must test on the target kernel or a kernel version that supports ethtool genetlink (5.6+).

**`internal/ipc/`** — The Unix domain socket server (`AF_UNIX SOCK_STREAM`). `server.go` accepts connections in a loop and dispatches each to a short-lived goroutine that reads the 4-byte big-endian length header, reads the JSON request body, acquires a tree read lock, serializes the response, and writes the framed reply. `protocol.go` defines the `Request` and `Response` structs and their JSON marshalling.
The IPC server always serves whatever data is currently in the in-memory tree, regardless of whether underlying subprocesses (ip batch, bridge batch) are temporarily unavailable due to restarts. During a subprocess restart window, the tree contains the last successfully collected state. This means IPC responses may reflect slightly stale data during the restart gap (typically under 30 seconds), but the server never blocks or returns errors due to subprocess unavailability — only due to protocol-level issues (malformed request, unknown path).


**`internal/config/`** — The `Config` struct read from a TOML or environment-variable source. Fields include socket path (default `/run/yangerd.sock`), per-collector polling intervals, log level, startup timeout before the IPC server begins accepting connections, and three boolean feature flags (`EnableWiFi`, `EnableContainers`, `EnableGPS`) parsed from the `YANGERD_ENABLE_WIFI`, `YANGERD_ENABLE_CONTAINERS`, and `YANGERD_ENABLE_GPS` environment variables. Parsing defaults each flag to `true` when the env var is unset. In production, Buildroot writes `/etc/default/yangerd` explicitly and sets unsupported features to `false`; therefore disabled packages are still disabled. The phrase “missing file enables all features” refers specifically to `/etc/default/yangerd` being absent (all vars unset → parser defaults apply).

### Key Dependencies

| Module | Version | Purpose |
|--------|---------|---------|
| iproute2 (`ip`, `bridge`) | system | Persistent `ip -json -force -batch -` subprocess for link/addr/neigh state queries; `bridge -json -batch -` for VLAN/FDB/STP queries. Event monitoring is handled natively via `vishvananda/netlink`, not via `ip monitor`. |
| github.com/fsnotify/fsnotify | v1.x | Cross-platform inotify for reactive file watching (procfs forwarding flags). Bridge STP state is handled via netlink events, not fsnotify. DHCP leases and firewall state are handled via D-Bus signals, not fsnotify. Note: sysfs pseudo-files (`/sys/class/hwmon/*`, `/sys/class/thermal/*`) are NOT monitored via fsnotify — they do not emit inotify events and are polled by `collector/hardware.go` instead. |
| github.com/vishvananda/netlink | v1.x | Native Go netlink subscriptions for reactive event monitoring: `LinkSubscribeWithOptions`, `AddrSubscribeWithOptions`, `NeighSubscribeWithOptions`. Also provides bridge FDB events (via NDA_MASTER flag on NeighUpdate), bridge VLAN events (via LinkUpdate), bridge STP port state events (via LinkUpdate with IFLA_BRPORT_STATE in IFLA_PROTINFO), and bridge MDB events (via raw RTNLGRP_MDB subscription). All bridge events are triggers only — full state re-read via `bridge -json -batch -`. Route data comes from the ZAPI watcher — no netlink route group subscription. |
| iw (iw tool) | system | Persistent `iw event -t` subprocess for reactive 802.11 wireless events; short-lived `iw dev <iface> info` and `iw dev <iface> station dump` for re-queries. Feature-gated: only active when `YANGERD_ENABLE_WIFI=true`. |
| github.com/mdlayher/ethtool | v0.x | Ethernet speed/duplex/autoneg queries via ethtool genetlink; used by both the polling collector (stats) and the ethmonitor (re-queries after notifications) |
| github.com/mdlayher/genetlink | v1.x | Generic netlink socket for subscribing to ETHNL_MCGRP_MONITOR multicast group; provides `Conn.JoinGroup()` + `Conn.Receive()` for native ethtool notification reception |
| github.com/godbus/dbus/v5 | v5.x | D-Bus system bus connection for the D-Bus Monitor Subsystem (Section 4.1novies): signal subscriptions for dnsmasq DHCP lease events and firewalld reload notifications, plus `GetMetrics()` method calls on dnsmasq for DHCP packet counters |
| github.com/facebook/time/ntp/chrony | latest | Native Go implementation of chrony's cmdmon protocol v6. Provides typed request/response structs for tracking, sources, sourcestats, and activity queries over the Unix socket at `/var/run/chrony/chronyd.sock`. Eliminates `exec chronyc` subprocess spawning for NTP data collection. Used by `internal/collector/ntp.go`. Apache-2.0 license; production-tested in Facebook's ntpcheck and Prometheus chrony_exporter. |
| golang.zx2c4.com/wireguard/wgctrl | v0.x | WireGuard peer statistics |
| golang.org/x/sync/errgroup | latest | Monitor goroutine lifecycle management |
| github.com/osrg/gobgp/v4 | latest | ZAPI client library for connecting to FRR zebra's zserv unix socket. Provides ZAPI v6 message framing, `ZEBRA_REDISTRIBUTE_ADD` subscription, and `IPRouteBody` parsing for route prefix, protocol, distance, metric, nexthops, and flags. Used by `internal/zapiwatcher/`. |

All Go dependencies are pure Go with no CGo requirement. The module graph avoids C bindings entirely, which is a hard constraint for cross-compilation in Buildroot. The `iproute2` runtime dependency is always present on Infix targets as part of the base system.

## 7. Deployment & Operations


The `go.mod` module path `github.com/kernelkit/infix/src/yangerd` matches the Infix directory structure, making the module self-describing with respect to its source location.

Cross-compilation for embedded targets requires only standard Go toolchain invocation — no CGo means no cross-C-compiler complexity:

```bash
GOARCH=arm64 GOOS=linux go build ./cmd/yangerd
GOARCH=arm   GOOS=linux GOARM=7 go build ./cmd/yangerd
```

The canonical target build is via a Buildroot package at `package/yangerd/yangerd.mk`, using the standard `golang-package` infrastructure with `BR2_PACKAGE_YANGERD`. The `Makefile` in `src/yangerd/` is for local host development and static analysis only, mirroring the pattern used by other native daemons under `src/`.

## Deployment
### Finit Service File

yangerd is managed by finit, Infix's init system. The service definition lives at `/etc/finit.d/yangerd.conf`:

```
# yangerd — operational data daemon
service [S12345] env:-/etc/default/yangerd \
        log:prio:daemon.notice,tag:yangerd \
        <!pid/1> yangerd -- yangerd operational data daemon
```

Key attributes:

- **Runlevels `S,1,2,3,4,5`**: yangerd starts during system initialisation (runlevel S) and remains running through all multi-user runlevels.
- **Condition `<pid/1>`**: yangerd starts as soon as PID 1 (finit itself) is fully running. No external network or storage condition is required — yangerd only needs the kernel's netlink subsystem, which is always available.
- **Ordering relative to statd**: statd declares yangerd as a hard dependency via its finit condition. statd will not start until yangerd's service condition is satisfied. The finit condition for statd is:

```
service [2345] env:-/etc/default/statd \
        log:prio:daemon.notice,tag:statd \
        <!svc/yangerd> statd -- statd operational datastore daemon
```

The `<svc/yangerd>` condition is *mandatory*: statd requires yangerd to be running. If yangerd fails to start, statd will not proceed -- there is no Python fallback path. This ensures that operational data always comes from yangerd's in-memory tree.

### Socket

yangerd creates and owns `/run/yangerd.sock` (type `SOCK_STREAM`, Unix domain) at startup. **There is no socket activation**: yangerd creates the socket itself by calling `net.Listen("unix", "/run/yangerd.sock")` before accepting connections. The file is removed on clean shutdown via a `defer os.Remove(...)` registered in `main()`. If a stale socket file exists at startup (e.g. after a crash), yangerd removes it before binding.

Access to the socket is restricted to the `statd` group (`chmod 0660`). statd runs as a member of the `statd` group to permit reads without requiring root.

### Environment Variables

Three feature flags (`YANGERD_ENABLE_WIFI`, `YANGERD_ENABLE_CONTAINERS`, `YANGERD_ENABLE_GPS`) control runtime feature flags for optional subsystems.

| Variable | Default | Description |
|----------|---------|-------------|
| `YANGERD_SOCKET` | `/run/yangerd.sock` | Path to the Unix domain socket yangerd creates and listens on. Override in tests or multi-instance setups. |
| `YANGERD_LOG_LEVEL` | `info` | Log verbosity: `trace`, `debug`, `info`, `warn`, `error`. Parsed at startup; changing the file requires a restart. |
| `YANGERD_TIMEOUT_MS` | `50` | Milliseconds statd waits for a response from yangerd on a single IPC request. Exceeding this timeout causes `ly_add_yangerd_data()` to return `SR_ERR_INTERNAL` for that request. |
| `YANGERD_STARTUP_TIMEOUT` | `5s` | How long yangerd waits, after launching all monitors, for the initial state dump goroutine to complete before marking itself `ready`. A Go `time.Duration` string (e.g. `5s`, `10s`). |
| `YANGERD_POLL_INTERVAL_OSPF` | `10s` | Interval between OSPF collector runs (executes `vtysh -c 'show ip ospf json'`). Longer interval reduces load from FRR queries. |
| `YANGERD_POLL_INTERVAL_NTP` | `60s` | Interval between NTP collector runs (queries chrony via native cmdmon protocol). Default is higher than OSPF because NTP state changes are infrequent. |
| `YANGERD_ENABLE_WIFI` | `true` | Enable WiFi operational data collection (IW Event Monitor + WiFi polling collector). Set to `false` by the Buildroot recipe when WiFi support (`BR2_PACKAGE_IW`) is not included in the build. When `false`, the `iwmonitor` and `collector/wifi.go` subsystems are not started and no WiFi-related data appears in the tree. |
| `YANGERD_ENABLE_CONTAINERS` | `true` | Enable container operational data collection (podman collector). Set to `false` by the Buildroot recipe when container support (`BR2_PACKAGE_PODMAN`) is not included in the build. When `false`, the `collector/containers.go` subsystem is not started and no container data appears in the tree. |
| `YANGERD_ENABLE_GPS` | `true` | Enable GPS/GNSS operational data collection. Set to `false` by the Buildroot recipe when GPS support is not included in the build. When `false`, GPS-related data in the hardware collector is skipped. |

**Configuration reload policy**: All configuration (socket path, log level, polling intervals, feature flags) is read once at startup from environment variables. There is no hot-reload mechanism — changing any setting requires a daemon restart (`initctl restart yangerd`). This simplifies the implementation by avoiding concurrent access to configuration values and ensures that the daemon's behavior is deterministic for its entire lifetime.


### Startup Sequence

The following steps occur in order during yangerd startup:

1. **Parse environment**: read `YANGERD_SOCKET`, `YANGERD_LOG_LEVEL`, all interval variables, and the three feature flags (`YANGERD_ENABLE_WIFI`, `YANGERD_ENABLE_CONTAINERS`, `YANGERD_ENABLE_GPS`).
2. **Create socket**: call `net.Listen("unix", socketPath)` and `chmod 0660` the resulting file. From this point, connection attempts from statd will queue in the kernel backlog.
3. **Initialise tree**: create the `tree.Tree` (empty per-model-locked map).
4. **Launch monitors**: start the NLMonitor (netlink subscriptions for link, addr, neigh, and bridge events), goroutines for `monitor.Link`, `monitor.Addr`, and `monitor.Neigh` (always active), and the ZAPI watcher (`internal/zapiwatcher/`, connects to zebra's zserv socket for route data). Conditionally start feature-gated subsystems:
   - **Always**: NLMonitor (netlink subscriptions), `monitor.Link`, `monitor.Addr`, `monitor.Neigh`, ZAPI watcher (route data via zserv), bridge batch, `ethmonitor`, `fswatcher`
   - **If `YANGERD_ENABLE_WIFI=true`**: start `iwmonitor` (persistent `iw event -t` subprocess) and `collector/wifi.go`
   - **If `YANGERD_ENABLE_CONTAINERS=true`**: start `collector/containers.go` and `collector/container_ifaces.go`
   - **If `YANGERD_ENABLE_GPS=true`**: enable GPS data collection in `collector/hardware.go`
   Each monitor immediately calls its corresponding list API to populate an initial snapshot:
   - `link.LinkList()` — enumerates all existing links
   - `addr.AddrList(nil, netlink.FAMILY_ALL)` — enumerates all addresses on all links
   - `neigh.NeighList(0, netlink.FAMILY_ALL)` — enumerates all ARP/NDP entries
   - ZAPI watcher: sends `ZEBRA_HELLO` + `ZEBRA_ROUTER_ID_ADD` + `ZEBRA_REDISTRIBUTE_ADD` per route type; zebra responds with a full RIB dump
   - fswatcher: calls `InitialRead()` after glob expansion and watch setup — reads every watched procfs file (forwarding flags) and populates the tree (sub-millisecond, synchronous)
   Note: only `LinkSubscribeWithOptions{ListExisting: true}` auto-delivers existing entries via the event channel; the address and neighbour monitors must call their respective list APIs explicitly. Route data is bootstrapped by the ZAPI watcher's redistribution subscription, which triggers a full dump from zebra.
5. **Launch initial dump goroutine**: a separate goroutine waits for the three netlink monitor goroutines and the ZAPI watcher to signal completion of their initial data load (via a `sync.WaitGroup`). Once all four complete, it sets the daemon-wide `ready` flag to `true`.
6. **Start IPC accept loop**: the main goroutine begins accepting connections from `/run/yangerd.sock`.
7. **While `ready == false`**: any incoming IPC request returns immediately with a JSON response `{"status": "starting", "code": 503}`. statd treats `code == 503` as a transient unavailability signal and returns `SR_ERR_INTERNAL` to sysrepo for that request. The management client sees an empty operational subtree during this brief window (typically under one second).

#### Startup Readiness Protocol

The `ready` flag transitions from `false` to `true` only when ALL of the following initial data loads complete:

1. **NLMonitor**: `LinkList()`, `AddrList()`, and `NeighList()` have each returned and populated the tree.
2. **ZAPI watcher**: The initial RIB dump from zebra is complete (all `REDISTRIBUTE_ROUTE_ADD` messages for the initial dump have been received and processed).
3. **BridgeBatch**: Initial `vlan show`, `fdb show`, and `mdb show` queries have completed.
4. **FSWatcher**: `InitialRead()` has completed — all watched procfs files (forwarding flags) have been read and their values stored in the tree. This completes sub-millisecond but is included in the WaitGroup for correctness, ensuring forwarding state is never absent from the tree when the daemon begins serving queries.

Each component signals completion via a shared `sync.WaitGroup`. The `main()` goroutine calls `wg.Wait()` with a timeout of `YANGERD_STARTUP_TIMEOUT` (default `5s`). If the timeout expires before all components signal, the daemon logs a warning identifying which components have not yet completed and sets `ready = true` anyway — serving partial data is preferable to blocking statd indefinitely.

If a required data source (e.g., zebra) is permanently unavailable, the startup timeout ensures the daemon becomes ready within a bounded time. The affected tree keys will be empty until the source becomes available, and the health endpoint will report the specific subsystem as `"failed"`.

#### Graceful Shutdown Sequence

On SIGTERM or SIGINT, yangerd performs an ordered shutdown:

1. **Stop accepting**: Close the `net.Listener` to stop accepting new IPC connections. In-flight handler goroutines continue to completion.
2. **Cancel context**: Cancel the root `context.Context`, which propagates to all monitor and collector goroutines.
3. **Drain monitors**: Wait for all `Run()` methods to return (they return on `ctx.Done()`). The `errgroup.Wait()` call blocks until all goroutines exit.
4. **Drain batch subprocesses**: `IPBatch` and `BridgeBatch` close their stdin pipes, causing the subprocesses to exit. Wait for process exit to avoid zombies.
5. **Remove socket**: `os.Remove("/run/yangerd.sock")` via `defer` in `main()`.

The entire shutdown completes within 5 seconds under normal conditions. If a subprocess hangs, `exec.CommandContext` ensures it is killed when the context is cancelled.

#### Signal Handling

yangerd handles the following signals:
- **SIGTERM**: Initiates graceful shutdown (see Graceful Shutdown Sequence above).
- **SIGINT**: Same as SIGTERM — initiates graceful shutdown. Useful for interactive debugging.

All other signals use their default kernel behavior. Notably:
- **SIGHUP**: Not handled — does not trigger config reload (see Configuration reload policy above).
- **SIGUSR1/SIGUSR2**: Not handled. Use `yangerctl health` or `yangerctl dump` for runtime diagnostics instead of signal-based debug dumps.

8. **Once `ready == true`**: IPC requests are served from the in-memory tree. Monitors and the ZAPI watcher continue running indefinitely, updating the tree on every netlink event or ZAPI route notification.

### Local Development Build

```bash
cd src/yangerd
go build ./cmd/yangerd      # build the daemon binary
go build ./cmd/yangerctl    # build the CLI diagnostic tool
go vet ./...                # static analysis
go test ./...               # run all unit tests
```

For cross-compilation to Infix targets:

```bash
GOARCH=arm64 GOOS=linux go build ./cmd/yangerd
GOARCH=arm   GOOS=linux GOARM=7 go build ./cmd/yangerd
GOARCH=riscv64 GOOS=linux go build ./cmd/yangerd
```

No `CGO_ENABLED=0` flag is needed because yangerd contains no CGo code, but setting it explicitly (`CGO_ENABLED=0`) is recommended in CI to enforce the constraint.

### Buildroot Package

The canonical target build is via `package/yangerd/yangerd.mk` using the standard `golang-package` Buildroot infrastructure:

```makefile
################################################################################
#
# yangerd
#
################################################################################

YANGERD_VERSION = 1.0.0
YANGERD_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/yangerd
YANGERD_SITE_METHOD = local
YANGERD_LICENSE = BSD-2-Clause
YANGERD_LICENSE_FILES = LICENSE

YANGERD_BUILD_TARGETS = cmd/yangerd cmd/yangerctl

define YANGERD_INSTALL_INIT_FINIT
    $(INSTALL) -D -m 0644 $(YANGERD_PKGDIR)/yangerd.conf \
        $(TARGET_DIR)/etc/finit.d/yangerd.conf
endef

# Generate /etc/default/yangerd with build-time feature flags.
# Each flag is derived from the corresponding BR2_PACKAGE_* selection.
# When a feature is not selected in the Buildroot config, its flag is
# set to false and the corresponding collectors are not started at runtime.
define YANGERD_INSTALL_TARGET_CMDS
    $(INSTALL) -d $(TARGET_DIR)/etc/default
    echo '# yangerd build-time feature flags (generated by yangerd.mk)' \
        > $(TARGET_DIR)/etc/default/yangerd
    echo 'YANGERD_ENABLE_WIFI=$(if $(BR2_PACKAGE_IW),true,false)' \
        >> $(TARGET_DIR)/etc/default/yangerd
    echo 'YANGERD_ENABLE_CONTAINERS=$(if $(BR2_PACKAGE_PODMAN),true,false)' \
        >> $(TARGET_DIR)/etc/default/yangerd
    echo 'YANGERD_ENABLE_GPS=$(if $(BR2_PACKAGE_GPSD),true,false)' \
        >> $(TARGET_DIR)/etc/default/yangerd
endef

$(eval $(golang-package))
```

The `golang-package` macro handles `GOARCH`/`GOOS` setting from the Buildroot target tuple, vendor directory management, and stripping of debug symbols from the installed binary.


## 8. Testing Strategy

Testing yangerd spans six levels: testability contracts defining interface boundaries for all external dependencies, unit tests for internal packages, integration tests for the full daemon, regression tests comparing output parity with existing Python yanger scripts, CI enforcement via the race detector, and a concrete verification loop defining the per-module definition of done.

### Unit Tests (`go test ./internal/...`)

Unit tests exercise each internal package in isolation, with no external process invocations and no kernel dependencies.

#### `internal/tree`

| Test | Description |
|------|-------------|
| `TestTreeSetGet` | Call `tree.Set("ietf-interfaces:interfaces", data)`, then `tree.Get("ietf-interfaces:interfaces")`; assert returned bytes are byte-identical to `data`. |
| `TestTreeConcurrentReadWrite` | Spawn 100 goroutines each calling `tree.Set()` with unique module keys, and 100 goroutines each calling `tree.Get()` on the same keys concurrently. Run with `-race`; no data race must be reported. Verify that per-model locks allow concurrent writes to different modules without blocking. |
| `TestTreeGetMissing` | Call `tree.Get("/nonexistent")` on an empty tree; assert the result is `nil` and no panic occurs. |
| `TestTreeSetOverwrite` | Call `tree.Set("/k", data1)`, then `tree.Set("/k", data2)`; assert `tree.Get("/k")` returns `data2`. |
| `TestTreePrefixScan` | Set three keys under `/a/` and two under `/b/`; call `tree.Scan("/a/")` and assert exactly three entries are returned. |

#### `internal/ipc`

| Test | Description |
|------|-------------|
| `TestProtocolMarshal` | Construct a `Request{Method: "get", Path: "/ietf-interfaces:interfaces"}`, marshal to JSON, unmarshal into a new struct, assert field equality. Repeat for `Response`. |
| `TestFraming` | Write a 1-byte version header followed by a 4-byte big-endian length header followed by a JSON payload into a `bytes.Buffer`. Read back using the IPC protocol reader. Assert the recovered payload is byte-exact and the version byte matches `YANGERD_VERSION`. Also test with a payload of exactly 0 bytes and a payload of 65535 bytes. Test that a mismatched version byte causes the reader to return a version-mismatch error. |
| `TestIPCServer` | Start a server on a `tmpdir` socket using `ipc.Listen()`. Connect a client using `ipc.Dial()`. Send a `get /` request. Assert the response is a valid JSON object. Shut down the server and verify the client receives an `io.EOF` or connection-closed error. |
| `TestIPCServerConcurrent` | Start a server, connect 50 clients simultaneously, each sending one request and reading one response. Assert no request is lost, no response is misrouted to the wrong client. |

#### `internal/collector`

| Test | Description |
|------|-------------|
| `TestParseDnsmasqLeases` | Provide a sample `/var/lib/misc/dnsmasq.leases` file (three entries, mixed IPv4/IPv6). Call `collector.ParseDnsmasqLeases(reader)`. Assert the returned slice has length 3 and each entry's MAC, IP, and hostname fields match expectations. |
| `TestParseVtyshOspf` | Provide sample JSON output from `vtysh -c 'show ip ospf json'` (two neighbors, one `Full` and one `ExStart`). Call `collector.ParseVtyshOspfJSON(data)`. Assert neighbor count is 2, one neighbor has state `"Full"`. |
| `TestCollectorRunAll` | Create a mock collector that returns an error on first call and succeeds on second. Call `RunAll([]Collector{mockCollector})` twice. Assert the tree is updated on the second call, and the first error is logged but does not terminate the collector loop or affect other collectors. |
| `TestCollectorTimeout` | Create a mock collector that blocks for 5 seconds. Call it with a context having a 100 milliseconds deadline. Assert the call returns within 200 milliseconds with a context error, and the tree retains the previous value for that key. |

### Integration Tests

Integration tests launch a real yangerd binary (built by `go test -v -run TestIntegration...`) against a controlled environment:

- **Netlink injection (full interface re-read)**: use Go's `net.Pipe()` to create a mock netlink event source and a mock `IPBatch` that records queries written to it. Inject a synthetic `RTM_NEWLINK` message (interface `eth99` transitioning to `UP`). Assert that:
  1. Within 100 milliseconds, the mock `IPBatch` receives exactly three queries: `link show dev eth99`, `-s link show dev eth99`, and `addr show dev eth99` (the full interface re-read set).
  2. `yangerctl get /ietf-interfaces:interfaces/interface[name='eth99']/oper-status` returns `"up"`.
  3. `yangerctl get /ietf-interfaces:interfaces/interface[name='eth99']/statistics` contains rx/tx counters from the `-s link show` response.
  4. A mock `EthMonitor` records that `RefreshInterface("eth99")` was called exactly once (validating the cross-subsystem ethtool re-query trigger).
  5. If oper-status changed, `yangerctl get /ietf-interfaces:interfaces/interface[name='eth99']/last-change` returns a timestamp within 100ms of the injection time.
  This validates the full RTM_NEWLINK path: netlink event -> monitor goroutine -> full re-read (3 ip batch queries) -> tree write -> ethtool re-query -> IPC response.

- **IPC end-to-end**: start yangerd against the host's real netlink (requires root or `CAP_NET_ADMIN`; run as a privileged CI job step). Send `get /ietf-interfaces:interfaces` over the Unix socket. Assert the JSON response contains at least one interface entry with a `name` field.

- **503 during startup**: intercept the `ready` flag using a test hook (a `testing.T`-injected boolean gate). Verify that requests received before the flag is set receive `{"code": 503, "status": "starting"}`. Verify that the first request after the flag transitions receives a normal `{"code": 200}` response.

### Regression Tests

Regression tests verify that yangerd's output is structurally correct according to the YANG schema and matches golden-file reference data captured from a known-good system state. The test matrix is:

| Module | Golden File | yangerd path | Architecture |
|--------|-------------|--------------|-------------|
| ietf-interfaces | `golden/interfaces.json` | /ietf-interfaces:interfaces | x86_64, aarch64 |
| ietf-routing (routes) | `golden/routing-ribs.json` | /ietf-routing:routing/ribs | x86_64, aarch64 |
| ietf-routing (neighbors) | `golden/routing-neigh.json` | /ietf-routing:routing (arp) | x86_64, aarch64 |
| ethtool statistics | `golden/ethtool-stats.json` | /ietf-interfaces:interfaces/.../statistics | x86_64 |

For each cell in the matrix:
1. Boot a Qemu x86_64 or aarch64 Infix image.
2. Call `yangerctl get <path>` and capture the JSON output to `actual.json`.
3. Compare against the golden file using a YANG-aware structural comparator (`jd` or a custom Go tool) that treats list ordering as insignificant and ignores ephemeral counters (byte counts, uptime) that legitimately differ between runs.
4. Pass `actual.json` through `yanglint` (libyang validation) to verify it is accepted as valid YANG instance data.
5. Assert zero structural differences in non-counter fields.

Golden files are generated once from a reference system with a known network configuration and committed to the test suite. They are updated whenever the YANG model changes or yangerd's output format is intentionally modified.

### Race Detector Policy

All unit tests and integration tests run with `-race` enabled in CI:

```yaml
# In .github/workflows/build.yml
- name: yangerd unit tests
  run: |
    cd src/yangerd
    go test -race ./...
```

The per-model `sync.RWMutex` locks in `internal/tree`, the buffered channels in each monitor, and the context-based shutdown in collector goroutines are all required to be race-free under this policy. Any PR introducing a data race (as detected by `-race`) is automatically blocked.

Specific race-sensitive areas tested:
- `tree.Set()` called from a monitor goroutine concurrently with `tree.Get()` from an IPC handler goroutine.
- Monitor goroutine shutdown via `ctx.Done()` while an IPC handler holds a read lock.
- `ready` flag transition from `false` to `true` visible to all IPC handler goroutines without stale reads.

### Testability Contracts (Interface Boundaries)

Every external dependency is abstracted behind a Go interface. This enables unit testing with mock implementations -- no kernel, no D-Bus, no FRR, no hardware. The production binary uses the real implementations; `go test` uses mocks.

| Dependency | Interface | Package | Production Implementation | Mock |
|------------|-----------|---------|--------------------------|------|
| Netlink subscriptions | `NetlinkSubscriber` | `internal/nlmonitor/` | `vishvananda/netlink` channels | Channel-fed fake with injectable events |
| ip batch subprocess | `Executor` | `internal/ipbatch/` | Persistent `ip -json -force -batch -` process | In-memory map returning canned JSON per query |
| bridge batch subprocess | `Executor` | `internal/bridgebatch/` | Persistent `bridge -json -batch -` process | In-memory map returning canned JSON per query |
| D-Bus connection | `DBusConnector` | `internal/dbusmonitor/` | `godbus/dbus/v5` system bus | Fake bus with configurable method returns and injectable signals |
| ZAPI (zebra) socket | `ZAPIDialer` | `internal/zapiwatcher/` | `net.Dial("unix", "/var/run/frr/zserv.api")` | `net.Pipe()` with scripted ZAPI v6 messages |
| Ethtool genetlink | `EthtoolQuerier` | `internal/ethmonitor/` | `mdlayher/ethtool` client | Struct literal returns |
| Chrony cmdmon | `ChronyClient` | `internal/collector/` | `facebook/time/ntp/chrony` | Struct literal returns |
| Command execution | `CommandRunner` | `internal/collector/` | `exec.CommandContext` | Canned stdout/stderr per command |
| File reads | `FileReader` | `internal/collector/` | `os.ReadFile` / `filepath.Glob` | `fstest.MapFS` or in-memory bytes |

Interface definitions:

```go
// internal/ipbatch/batch.go (also used by internal/bridgebatch/)
type Executor interface {
    Query(ctx context.Context, cmd string) (json.RawMessage, error)
    Close() error
}

// internal/nlmonitor/nlmonitor.go
type NetlinkSubscriber interface {
    LinkSubscribe(ch chan<- netlink.LinkUpdate, done <-chan struct{}) error
    AddrSubscribe(ch chan<- netlink.AddrUpdate, done <-chan struct{}) error
    NeighSubscribe(ch chan<- netlink.NeighUpdate, done <-chan struct{}) error
}

// internal/dbusmonitor/dbusmonitor.go
type DBusConnector interface {
    Signal(ch chan<- *dbus.Signal)
    AddMatchSignal(opts ...dbus.MatchOption) error
    Object(dest string, path dbus.ObjectPath) DBusObject
    Close() error
}

type DBusObject interface {
    Call(method string, flags dbus.Flags, args ...interface{}) *dbus.Call
}

// internal/zapiwatcher/zapiwatcher.go
type ZAPIDialer interface {
    Dial(ctx context.Context) (net.Conn, error)
}

// internal/ethmonitor/ethmonitor.go
type EthtoolQuerier interface {
    LinkInfo(ifi int) (*ethtool.LinkInfo, error)
    LinkMode(ifi int) (*ethtool.LinkMode, error)
}

// internal/collector/ntp.go
type ChronyClient interface {
    Tracking(ctx context.Context) (*chrony.ReplyTracking, error)
    Sources(ctx context.Context) ([]chrony.ReplySourceData, error)
}

// internal/collector/runner.go (shared by vtysh, iw, podman, dmidecode)
type CommandRunner interface {
    Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// internal/collector/reader.go (shared by /proc, /sys, lease file readers)
type FileReader interface {
    ReadFile(path string) ([]byte, error)
    Glob(pattern string) ([]string, error)
}
```

**Import restriction rule**: No `internal/` package may import `os/exec`, `os.ReadFile`, `vishvananda/netlink`, `godbus/dbus`, `mdlayher/ethtool`, or `facebook/time/ntp/chrony` directly in production code outside of the interface implementation files. All access goes through the interface. This is enforced by a `go vet` linter check (or `depguard` via `golangci-lint`) in CI.

**Mock location**: Reusable mock implementations live in `internal/testutil/`. Package-specific mocks live in `_test.go` files within their package.

### Verification Loop (Definition of Done)

A module is complete when all of the following pass on a developer workstation with no target hardware, no kernel dependencies, and no running services:

```bash
# 1. Compiles with zero errors
go build ./cmd/yangerd ./cmd/yangerctl

# 2. Static analysis clean
go vet ./...

# 3. All tests pass, no data races
go test -race -count=1 ./...
```

Step 3 implicitly validates golden-file parity: every collector's test function loads canned input from `testdata/`, runs it through the collector with mocked dependencies, and compares the resulting YANG JSON against a `.golden` file committed to the repository. A mismatch fails the test.

**Golden-file capture process** (one-time, from a running Infix system with the current Python yanger scripts):

1. Capture expected output for each module:
   ```bash
   # On target, for each yanger module:
   yangerctl get /ietf-interfaces:interfaces > golden/interfaces.json
   yangerctl get /ietf-routing:routing      > golden/routing.json
   yangerctl get /ietf-hardware:hardware    > golden/hardware.json
   # ... for all 14 modules
   ```
2. Capture the corresponding raw inputs that produced that output:
   ```bash
   ip -json link show    > testdata/interfaces/ip-link.json
   ip -json addr show    > testdata/interfaces/ip-addr.json
   ip -json -s link show > testdata/interfaces/ip-link-stats.json
   vtysh -c 'show ip ospf json' > testdata/ospf/vtysh-ospf.json
   cat /var/lib/misc/dnsmasq.leases > testdata/dhcp/leases.txt
   # ... for all data sources
   ```
3. Commit both `testdata/` (inputs) and `golden/` (expected outputs) to the repository.
4. Each collector's unit test creates a mock with the canned inputs, runs the collector, and asserts the output matches the golden file using a structural JSON diff (key structure must match; volatile fields like counters and timestamps are ignored).

**YANG schema validation** (CI only -- requires `yanglint`):

```bash
# Validate each golden file against the YANG schema
for f in golden/*.json; do
    yanglint --format json -t data -m yang/*.yang "$f"
done
```

This runs in CI but not on every developer `go test` invocation, since `yanglint` requires libyang (a C dependency). The golden-file structural comparison in `go test` catches output regressions; `yanglint` catches schema violations.

**Per-module completion checklist**:

1. Go interface defined in the consuming package
2. Production implementation wired in `cmd/yangerd/main.go`
3. Mock implementation in `internal/testutil/` or `_test.go`
4. Canned inputs captured in `testdata/<module>/`
5. Golden output captured in `golden/<module>.json`
6. Unit test: canned input -> mock -> collector -> assert output == golden
7. `go test -race` passes for the package
8. No direct imports of external libraries outside interface implementation files

When all migration modules and new modules pass this checklist (13 migrated modules + 1 new module = 14 total YANG modules), yangerd is feature-complete and ready for integration testing on target hardware.

## 9. Migration Plan


yangerd ships as a single, complete delivery covering all 14 YANG modules. There is no phased rollout -- yangerd completely replaces the Python yanger scripts in one step. Migration scope is 13 modules (12 existing Python modules plus new `infix-services:mdns` migrated from `statd/avahi.c`).

### Module Inventory
**Text parser test fixtures**: The `iw event` and `vtysh` output parsers process human-readable text that varies across tool versions. Test fixtures capture known-good outputs from specific versions (iw 6.9, vtysh from FRR 10.5.1) including edge cases: truncated output, empty responses, multi-line entries, and malformed lines. Each fixture is stored as a `.txt` file in `testdata/` alongside the expected parsed Go struct as a `.golden` JSON file.


All 14 modules are implemented and delivered together (with additional supporting bridge and WireGuard collectors listed for completeness):

| Module | YANG Path | Data Source | Go File |
|--------|-----------|-------------|---------|
| ietf-interfaces | `/ietf-interfaces:interfaces` | Netlink RTNLGRP_LINK + RTNLGRP_*IFADDR | `internal/monitor/link.go`, `addr.go` |
| ietf-routing (RIBs) | `/ietf-routing:routing/ribs` | ZAPI watcher (streaming from zebra zserv socket) | `internal/zapiwatcher/zapiwatcher.go` |
| ietf-routing (ARP/NDP) | `/ietf-routing:routing` (neighbor tables) | Netlink RTNLGRP_NEIGH | `internal/monitor/neigh.go` |
| Interface statistics | `/ietf-interfaces:interfaces/interface/statistics` | mdlayher/ethtool genetlink | `internal/collector/ethtool.go` |
| ietf-routing (OSPF) | `.../control-plane-protocol/ietf-ospf:ospf` | `vtysh -c 'show ip ospf json'` | `internal/collector/ospf.go` |
| ietf-routing (RIP) | `.../control-plane-protocol/ietf-rip:rip` | `vtysh -c 'show ip rip json'` | `internal/collector/rip.go` |
| ietf-routing (BFD) | `.../control-plane-protocol/ietf-bfd:bfd` | `vtysh -c 'show bfd peers json'` | `internal/collector/bfd.go` |
| ietf-hardware | `/ietf-hardware:hardware` | `/sys/class/hwmon`, `dmidecode` | `internal/collector/hardware.go` |
| ietf-system | `/ietf-system:system-state` | `/proc/uptime`, `/etc/os-release`, `/proc/loadavg` | `internal/collector/system.go` |
| ietf-ntp | `/ietf-ntp:ntp/state` | chrony cmdmon protocol (tracking + sources) | `internal/collector/ntp.go` |
| ieee802-dot1ab-lldp | `/ieee802-dot1ab-lldp:lldp` | `lldpcli -f json0 watch` | `internal/lldpmonitor/monitor.go` |
| infix-containers | `/infix-containers:containers` | `podman ps --format json` | `internal/collector/containers.go` (feature-gated) |
| infix-dhcp-server | `/infix-dhcp-server:dhcp-server` | `/var/lib/misc/dnsmasq.leases` | `internal/collector/dhcp.go` |
| infix-firewall | `/infix-firewall:firewall` | firewalld D-Bus method calls (zones, policies, services, global state) | `internal/collector/firewall.go` |
| infix-services (mDNS) | `/infix-services:mdns/neighbors` | Avahi D-Bus (`org.freedesktop.Avahi`) ServiceBrowser/ServiceResolver signals | `internal/mdnsmonitor/mdnsmonitor.go` |
| bridge STP/VLAN/FDB/MDB | bridge state | Netlink event triggers + `bridge -json -batch -` re-reads | `internal/collector/bridge.go` |
| WireGuard | WireGuard tunnels | `wgctrl.Client.Devices()` | `internal/collector/wireguard.go` |

**Initial state bootstrap** (required because only `LinkSubscribeWithOptions{ListExisting: true}` auto-delivers existing entries; address, neighbour, and route monitors must bootstrap explicitly):

```go
// In monitor/addr.go -- startup bootstrap
existing, err := netlink.AddrList(nil, netlink.FAMILY_ALL)
if err != nil {
    log.Warnf("addr bootstrap failed: %v", err)
} else {
    for _, a := range existing {
        tree.Set(addrToPath(a), marshalAddr(a))
    }
}
```

The same pattern applies to `NeighList()`. Route data is bootstrapped by the ZAPI watcher's redistribution subscription (see Section 4.1octies), not by `RouteListFiltered()`.

All collectors use `context.WithTimeout()` with per-command timeouts to bound each external process invocation: vtysh commands 5s, nft 5s, iw queries 2s, dmidecode 5s (see Section 4.7 Design Rationale for the full timeout table). On timeout, the previous tree value is retained and a warning is logged. Collectors are registered in `cmd/yangerd/main.go` and scheduled by `internal/scheduler/scheduler.go`, which runs each collector at its configured poll interval using `time.NewTicker`.

### Deliverables

- `internal/monitor/{link,addr,neigh}.go`
- `internal/zapiwatcher/zapiwatcher.go`
- `internal/collector/{ethtool,ospf,rip,bfd,hardware,system,ntp,containers,dhcp,firewall,wifi,bridge,wireguard}.go`
- `internal/lldpmonitor/monitor.go`
- `internal/mdnsmonitor/mdnsmonitor.go`
- `internal/tree/tree.go`
- `internal/ipc/{server,client,protocol}.go`
- `cmd/yangerd/main.go`
- `cmd/yangerctl/main.go`
- `package/yangerd/yangerd.mk` + `yangerd.conf`
- Unit tests + integration tests for all modules
- Regression tests on x86_64, aarch64, and armv7
- Removal of Python yanger scripts from Buildroot package
- Updated finit service file with `group frr` for vtysh access

### Milestone Criteria

All 14 modules pass regression tests across x86_64, aarch64, and armv7 in CI. The Python yanger scripts are removed from the Buildroot package. statd's `get_oper_data()` function calls only `ly_add_yangerd_data()` -- there is no Python fallback path.
## 10. Risk Assessment

### 10.1 Detailed Risks

**Risk 1 — ip batch subprocess crash or netlink subscription failure**
The `ip -json -force -batch -` subprocess is a long-lived external process managed by yangerd for state queries. If it crashes unexpectedly (segfault, OOM-killed) or hangs (blocked on a kernel call), yangerd loses its ability to query link, address, and neighbor state until the subprocess is restarted. A hung subprocess could also leave stale file descriptors or pipe buffers that interfere with the replacement process. Additionally, if the ip binary is upgraded on disk while yangerd is running, the replacement subprocess may exhibit different JSON output format or behavior. Separately, the native Go netlink subscriptions (via `vishvananda/netlink`) could fail if the kernel's netlink buffer overflows under heavy load, causing dropped events and temporarily stale data.


To mitigate this risk, the `internal/ipbatch/` package implements health monitoring for the batch subprocess. The subprocess is supervised by a dedicated goroutine that detects unexpected EOF on stdout (indicating process exit) and restarts the subprocess with exponential backoff starting at one hundred milliseconds and capping at thirty seconds. Before accepting the restarted subprocess, yangerd performs a canary query (`link show dev lo`) to verify it produces valid JSON. For the native netlink subscriptions in `internal/nlmonitor/`, the shared error callback triggers context cancellation and full re-subscription. On re-subscription, a full state resynchronization is triggered by writing bulk dump commands to the ip batch subprocess, ensuring the in-memory tree is consistent with the current kernel state. Context cancellation provides clean shutdown of both the subprocess and netlink subscriptions. The health endpoint reports subprocess uptime, restart count, netlink subscription status, and last error for operational visibility.

**Risk 2 — Memory pressure under high-frequency netlink event storms**
On large Layer 2 segments or during periods of network instability such as Address Resolution Protocol storms, the kernel can generate thousands of neighbor events per second. Each event causes a tree write including a mutex lock, JSON serialization, and map insertion, which may trigger frequent garbage collection cycles. Under sustained heavy load, this can produce elevated central processing unit usage and garbage collection pause times that are visible as increased latency for IPC requests from statd, potentially causing management timeouts and service degradation.

We mitigate this risk by debouncing tree writes for each module key using a one hundred millisecond coalescing window, ensuring that only the final value in a burst of events is committed to the shared tree. Additionally, the netlink subscription channels are buffered to hold up to two hundred and fifty-six events; any events beyond this limit are dropped, and a counter is incremented to provide visibility into the loss. A per-monitor event rate gauge is also exposed via the health endpoint to make storm conditions visible to operators and automated monitoring systems, allowing for proactive troubleshooting of network anomalies and preventing cascading failures in the management plane.

**Risk 3 — dbus/external process query timeouts**
Phase 2 collectors that invoke external processes such as vtysh or podman, or query native protocols such as chrony cmdmon and D-Bus APIs, may block if those processes are slow to start, waiting for a file lock, or unresponsive due to extreme system resource contention. A blocked collector goroutine could cause the corresponding tree key to remain in a stale state indefinitely if the collection logic does not account for execution delays. This would result in incorrect or outdated operational data being served to management clients, which could lead to incorrect diagnostic conclusions or automated system failures.

Every collection operation is wrapped with a context that enforces a per-command timeout (2-5 seconds depending on source: e.g., iw 2s; vtysh/dmidecode/podman 5s; D-Bus calls 2-5s). If a collector exceeds this deadline, the operation is aborted, a warning is logged to the system journal, and the last known good value is retained in the in-memory tree to prevent serving empty data. The collector then waits for the next scheduled interval before attempting the operation again. This ensures that a single slow or hung process cannot block other collectors or degrade the responsiveness of the IPC server, maintaining the overall stability of the daemon under various failure modes and ensuring that the system remains manageable even under duress.

**Risk 4 — Incomplete tree state at first statd query (startup race)**
Both statd and yangerd start concurrently during the system initialization process managed by finit. It is highly probable that statd's first operational data request will fire before yangerd has completed its initial state snapshot from the kernel using the bootstrap listing APIs. If yangerd responded with an empty or partial tree in this state, sysrepo might cache incorrect operational data, leading to a misleading view of the system state for the first few seconds after boot and potentially causing monitoring alerts to trigger unnecessarily.

To prevent this, yangerd maintains a ready flag that is only set to true once all initial netlink dumps have successfully completed and populated the tree. While this flag is false, every IPC response is returned with a five hundred and three service unavailable status code. The statd daemon is configured to treat this status code as a transient error and will retry on the next sysrepo callback invocation. During the brief startup window (typically under one second), statd logs a warning indicating that yangerd is still initializing and returns `SR_ERR_INTERNAL` to sysrepo, which causes the management client to see an empty operational subtree for that brief period. Once yangerd signals readiness, all subsequent queries are served from the fully populated in-memory tree.

**Risk 5 — FRR group membership**
The vtysh utility and the zebra zserv socket are used to query state from the FRRouting suite. vtysh connects to per-daemon control sockets, and the ZAPI watcher connects to the zserv unix socket (`/var/run/frr/zserv.api`). Both socket paths are owned by the frr user and group with restricted permissions that prevent unauthorized access. If the yangerd process is not running with the correct group memberships, OSPF/RIP/BFD collector queries via vtysh and the ZAPI watcher's route data stream will both fail with permission denied errors, resulting in empty operational subtrees for routing protocols and routes.

This risk is addressed through the deployment configuration in the finit service file and the Buildroot package definition. The service file explicitly specifies that yangerd should run with membership in the frr group, granting it the necessary permissions to communicate with both vtysh control sockets and the zebra zserv socket. Furthermore, the post-installation script in the Buildroot package ensures that the yangerd system user is correctly added to the frr group on the target filesystem. These configuration steps are verified during the integration testing phase to ensure that both protocol state collection via vtysh and route data collection via the ZAPI watcher are functional across all supported hardware platforms and software configurations.

**Risk 6 — dmidecode privilege**
The dmidecode utility requires elevated privileges to read System Management BIOS data from physical memory or the specialized sysfs interface. On many platforms, this requires the CAP_SYS_RAWIO capability to access low-memory addresses that are not otherwise exposed to unprivileged users. Without this capability, the utility will exit with a permission denied error, causing the hardware inventory collector to fail and leaving the inventory tree empty, which prevents identification of the specific hardware revision or serial number.

We provide two mitigation options that can be selected based on the specific security requirements of the deployment. The first option is to grant the necessary capability to the yangerd service through the finit configuration, allowing it to run the utility directly with the required privileges. The second, more restrictive option is to pre-cache the hardware inventory data during the system build process or at initial boot from a privileged context, saving the output to a file that yangerd can read as an unprivileged user. This avoids the need for elevated privileges at runtime while still providing accurate hardware inventory information to management clients through the YANG models, maintaining a strict security posture.

**Risk 7 — inotify watch limit exhaustion**
The Linux kernel maintains a per-user limit on the number of active inotify watches (`/proc/sys/fs/inotify/max_user_watches`). On systems with many DHCP lease files, it is possible for yangerd to exhaust this limit, especially if other daemons are also using inotify. When the limit is reached, any attempt to add a new watch will fail with `ENOSPC`. Note: hardware sensors are not watched via inotify (sysfs pseudo-files do not emit inotify events), and bridge STP state is not watched via inotify (it uses netlink events), so neither contributes to watch exhaustion.

To mitigate this, the `internal/fswatcher/` package logs a clear warning identifying the specific path that failed to be watched. For every such failure, yangerd automatically falls back to the polling collector for that data source. This ensures that data collection continues at the configured polling interval, maintaining operational visibility at the cost of increased latency and CPU wake-ups, rather than failing entirely.
These paths map to specific YANG leaves: `/proc/sys/net/ipv4/conf/*/forwarding` maps to `ietf-ip:ipv4/forwarding`, `/proc/sys/net/ipv6/conf/*/forwarding` maps to `ietf-ip:ipv6/forwarding`, and `/proc/sys/net/ipv6/conf/*/accept_redirects` maps to neighbor discovery configuration leaves.


**Risk 8 — bridge batch subprocess failure or bridge netlink event loss**
yangerd manages a persistent `bridge -json -batch -` subprocess for bridge-specific state queries (VLANs, MDB, FDB, STP). Bridge events are received natively via `vishvananda/netlink`: FDB entries arrive as NeighUpdate events with the NDA_MASTER flag, VLAN changes arrive as LinkUpdate events, STP port state changes arrive as LinkUpdate events carrying IFLA_BRPORT_STATE in IFLA_PROTINFO, and MDB entries are received via a raw netlink socket subscribed to RTNLGRP_MDB. All events are used as triggers only -- full state is re-read via bridge batch. A crash in the bridge batch subprocess would prevent state re-queries from completing. A failure in the netlink subscriptions would stop reactive updates for bridge state.

The bridge batch subprocess uses the same robust health monitoring as the ip batch subprocess, providing automatic restarts with exponential backoff and canary-query validation. For bridge netlink events, the shared NLMonitor error callback handles subscription failures by triggering re-subscription and a full re-query of bridge state via the batch subprocess. This ensures the in-memory tree remains synchronized with the kernel after any failure. The health endpoint reports bridge batch subprocess status and bridge netlink subscription status separately.

**Risk 9 — iw event subprocess failure (when WiFi enabled)**

The `iw event -t` subprocess may exit unexpectedly due to kernel driver issues or nl80211 subsystem errors. This risk applies only when WiFi support is included in the build (`YANGERD_ENABLE_WIFI=true`). If the subprocess exits during operation, WiFi event notifications stop and the in-memory tree retains stale wireless data until the subprocess is restarted.

The `internal/iwmonitor/` package mitigates this with the same exponential backoff restart pattern used by the NLMonitor re-subscription and batch subprocess restarts (initial delay 100ms, max 30s, factor 2x). Upon restart, a full re-query of all known wireless interfaces is performed. When WiFi is not included in the build (`YANGERD_ENABLE_WIFI=false`), the IW Event Monitor is not started at all and no WiFi data appears in the tree.

**Risk 10 — ethtool genetlink subscription failure**

The `internal/ethmonitor/` package subscribes to the kernel's `ETHNL_MCGRP_MONITOR` genetlink multicast group at startup. Since Infix targets Linux kernel 6.18, ethtool netlink is unconditionally available and the subscription is expected to always succeed. If the `genetlink.Conn` dial or `JoinGroup()` call fails, it indicates a system misconfiguration (e.g., missing kernel module, permission denied) rather than a kernel version issue. Such failures are logged at ERROR.

If the genetlink subscription succeeds initially but the connection is later broken (e.g., due to a kernel module reload or netlink buffer overflow), the ethmonitor logs a warning and attempts to re-establish the subscription with exponential backoff (initial delay 100ms, max 30s, factor 2x). During the reconnection window, the ethtool collector's 30-second polling cycle for statistics continues to provide counter data. Settings data (speed, duplex, autoneg) may be briefly stale until the subscription is restored or until the next RTM_NEWLINK event triggers a RefreshInterface() call.


**Risk 11 — ZAPI watcher failure (zebra unavailability or restart)**
The ZAPI watcher connects to zebra's zserv unix domain socket to receive route redistribution notifications. If zebra is not running at yangerd startup (e.g., delayed start, crash, or intentional restart), the watcher cannot establish its initial connection and the route subtree will be empty until zebra becomes available. If zebra restarts while the watcher is connected, the watcher receives an EOF on its receive channel and must reconnect and re-subscribe. During the reconnection window, no route updates are received and the in-memory tree retains stale route data from the previous session.

The `internal/zapiwatcher/` package mitigates this with exponential backoff reconnection (initial delay 100ms, max 30s, factor 2x). On each successful reconnection, the full ZAPI subscription handshake is replayed (HELLO + ROUTER_ID_ADD + REDISTRIBUTE_ADD per route type), which causes zebra to send a complete RIB dump. The watcher uses a full replacement strategy: it builds a new route map from the dump and atomically replaces the route subtree in the tree, ensuring that stale routes from the previous session are cleared. The health endpoint reports ZAPI watcher connection status (connected, reconnecting, failed) and the timestamp of the last successful route update. ZAPI v6 wire format has been stable across FRR 8.x, 9.x, and 10.x (including the target FRR 10.5.1), reducing the risk of protocol version mismatch after FRR upgrades.
### 10.2 Risk Summary

| # | Risk | Likelihood | Impact | Status |
|---|------|-----------|--------|--------|
| 1 | ip batch subprocess crash or netlink subscription failure | Low–Medium | High | Mitigated — health-monitored subprocess with auto-restart, exponential backoff, canary query; netlink re-subscription with full resync |
| 2 | Netlink event storm (memory/CPU) | Low–Medium | Medium | Mitigated — 100 milliseconds debounce per key; 256-event buffer; health metrics |
| 3 | dbus/process query timeout | Low | Medium | Mitigated — 2-5 seconds `context.WithTimeout` (command-specific); stale value retained; retry on next tick |
| 4 | Startup race (incomplete tree at first query) | High | Low | Mitigated -- `code 503` response; statd retries on next callback; brief empty window during init |
| 5 | FRR group membership (vtysh + zserv socket) | High | Medium | Deployment requirement — finit `group frr`; Buildroot adds user to group |

| 6 | dmidecode privilege (CAP_SYS_RAWIO) | Low | Medium | Mitigated — pre-cache at build time or grant CAP_SYS_RAWIO via finit |
| 7 | inotify watch limit exhaustion | Low | Medium | Mitigated — logs warning and falls back to polling collector |
| 8 | bridge batch subprocess failure or bridge netlink event loss | Low | Medium | Mitigated — health-monitored batch subprocess with auto-restart; netlink re-subscription with full bridge state resync |
| 9 | iw event subprocess failure (WiFi enabled) | Low | Low | Mitigated — feature-gated subsystem (`YANGERD_ENABLE_WIFI`); exponential backoff restart when enabled |
| 10 | ethtool genetlink subscription failure | Low | Low | Mitigated — unconditionally available on kernel 6.18; failure indicates misconfiguration, not kernel gap; exponential backoff reconnection |
| 11 | ZAPI watcher failure (zebra unavailability, reconnection gap, protocol mismatch) | Medium | Medium | Mitigated — exponential backoff reconnection; full RIB re-sync on reconnect; stale route data cleared atomically; ZAPI v6 stable across FRR 8.x–10.x |
| 12 | D-Bus service unavailability (dnsmasq or firewalld not running, D-Bus daemon restart) | Low | Low | Mitigated — `NameOwnerChanged` signal detects service disappearance and reappearance; full data refresh on service (re)start; stale data retained until refresh succeeds; exponential backoff reconnection to D-Bus system bus |
## Appendices

### A.1 Netlink Group Reference

The following table lists all `RTNLGRP_*` multicast groups monitored by yangerd via native Go netlink subscriptions (`vishvananda/netlink`). These groups are subscribed to directly using `LinkSubscribeWithOptions`, `AddrSubscribeWithOptions`, and `NeighSubscribeWithOptions`, plus a raw netlink socket for `RTNLGRP_MDB`. The constant values are from the Linux kernel's `rtnetlink.h` header.

| Group Name | Constant Value | Event Types | Monitor File |
|------------|---------------|-------------|-------------|
| `RTNLGRP_LINK` | 1 | `RTM_NEWLINK`, `RTM_DELLINK` | `monitor/link.go` | On `RTM_NEWLINK`: triggers full interface re-read (3 `ip -json -batch` queries) + `ethmonitor.RefreshInterface()` to re-query ethtool settings, since `ETHNL_MCGRP_MONITOR` does not fire on link state transitions. Updates `last-change` timestamp. |
| `RTNLGRP_NEIGH` | 3 | `RTM_NEWNEIGH`, `RTM_DELNEIGH` | `monitor/neigh.go` | On any neigh event (add or remove): triggers full neighbor re-read via `neigh show dev <iface>` through ip batch. Event is trigger only — not parsed for data. Delete events produce a re-read that omits the removed neighbor. |
| `RTNLGRP_IPV4_IFADDR` | 5 | `RTM_NEWADDR`, `RTM_DELADDR` (IPv4) | `monitor/addr.go` | On any addr event (add or remove): triggers full address re-read via `addr show dev <iface>` through ip batch. Event is trigger only — not parsed for data. |
| `RTNLGRP_IPV6_IFADDR` | 9 | `RTM_NEWADDR`, `RTM_DELADDR` (IPv6) | `monitor/addr.go` | Same re-read pattern as IPv4; both groups dispatched by AF inside `monitor/addr.go`. |
| `RTNLGRP_MDB` | 26 | `RTM_NEWMDB`, `RTM_DELMDB` | `nlmonitor/nlmonitor.go` | On any MDB event: triggers full MDB state re-read via `mdb show` through bridge batch. Event is trigger only — not parsed for data. |

Notes:
- `RTNLGRP_IPV4_IFADDR` and `RTNLGRP_IPV6_IFADDR` are subscribed together in a single `netlink.Subscribe()` call by passing both group constants. The resulting events are dispatched by address family inside `monitor/addr.go`.
- `RTNLGRP_NEIGH` covers both ARP (IPv4) and NDP (IPv6) neighbour events — no separate IPv6 group is needed.
- Kernel buffer overflow (`ENOBUFS`) on any of these subscriptions is handled by logging a warning and performing a full re-list (e.g. `AddrList()`) to recover any dropped events, followed by re-subscription.

### A.2 YANG Module Registry

All 14 YANG modules that yangerd handles, with their canonical YANG path prefix and the corresponding Python predecessor (if any):

| YANG Module | Path Prefix | Replaces |
|-------------|------------|----------|
| `ietf-interfaces` | `/ietf-interfaces:interfaces` | `interface.py` |
| `ietf-routing` (RIBs/routes) | `/ietf-routing:routing/ribs` | `routing.py` |
| `ietf-routing` (ARP/NDP neighbors) | `/ietf-routing:routing` (neighbor tables) | `routing.py` |
| `ietf-routing` (OSPF) | `/ietf-routing:routing/control-plane-protocols/control-plane-protocol/ietf-ospf:ospf` | `ospf.py` |
| `ietf-routing` (RIP) | `/ietf-routing:routing/control-plane-protocols/control-plane-protocol/ietf-rip:rip` | `rip.py` |
| `ietf-routing` (BFD) | `/ietf-routing:routing/control-plane-protocols/control-plane-protocol/ietf-bfd:bfd` | `bfd.py` |
| `ietf-hardware` | `/ietf-hardware:hardware` | `hardware.py` |
| `ietf-system` | `/ietf-system:system-state` | `system.py` |
| `ietf-ntp` | `/ietf-ntp:ntp/state` | `ntp.py` |
| `ieee802-dot1ab-lldp` | `/ieee802-dot1ab-lldp:lldp` | `lldp.py` (served reactively via `lldpcli -f json0 watch`) |
| `infix-containers` | `/infix-containers:containers` | `containers.py` (feature-gated: `YANGERD_ENABLE_CONTAINERS`) |
| `infix-dhcp-server` | `/infix-dhcp-server:dhcp-server` | `dhcp-server.py` |
| `infix-firewall` | `/infix-firewall:firewall` | `firewall.py` |
| `infix-services` (mDNS) | `/infix-services:mdns` | `(new — migrated from statd/avahi.c via Avahi D-Bus)` |
Note: the registry lists 14 rows because `ietf-routing` covers three distinct sub-trees (RIBs, neighbors, and routing protocol instances) that correspond to distinct `sr_oper_get_subscribe()` paths, while `infix-services:mdns` is an additional module entry beyond legacy Python parity.

### A.3 Glossary
**inotify**
A Linux kernel subsystem that provides notifications about filesystem events (creation, modification, deletion) to user-space applications. yangerd uses inotify to implement reactive file watching for procfs forwarding flags. Bridge STP state is handled via netlink events, not inotify. DHCP lease and firewall state changes are handled via D-Bus signal subscriptions (see Section 4.1novies), not inotify. Note: sysfs pseudo-files (hwmon sensors, thermal zones) do not emit inotify events and are polled instead.

**fsnotify**
The cross-platform Go library (`github.com/fsnotify/fsnotify`) that wraps Linux inotify (and other OS-specific equivalents) to provide a high-level API for filesystem events.

**bridge netlink events**
Bridge-specific kernel events received by yangerd via native Go netlink subscriptions. FDB (forwarding database) events arrive as `NeighUpdate` messages with the `NDA_MASTER` flag set, indicating they belong to a bridge master device. VLAN membership changes arrive as `LinkUpdate` messages with bridge VLAN attributes. STP port state changes arrive as `LinkUpdate` messages carrying `IFLA_BRPORT_STATE` in `IFLA_PROTINFO`. MDB (multicast database) events are received via a raw netlink socket subscribed to `RTNLGRP_MDB` (group 26). All bridge events are used as triggers only -- full state is re-read via the `bridge -json -batch -` subprocess.

**D-Bus Monitor**
The yangerd subsystem (`internal/dbusmonitor/`) that subscribes to D-Bus system bus signals for reactive monitoring of service-managed data. It watches `DHCPLeaseAdded`, `DHCPLeaseDeleted`, and `DHCPLeaseUpdated` signals from dnsmasq, and `Reloaded` signals from firewalld. Each signal triggers a full data refresh: DHCP refreshes re-read `/var/lib/misc/dnsmasq.leases` and call dnsmasq's `GetMetrics()` D-Bus method; firewall refreshes query firewalld via D-Bus method calls (`getDefaultZone()`, `getActiveZones()`, `getZoneSettings2()`, `getPolicies()`, `getPolicySettings()`, `listServices()`, `getServiceSettings2()`, `getLogDenied()`, `queryPanicMode()`). The D-Bus Monitor also watches `NameOwnerChanged` on `org.freedesktop.DBus` to detect service restarts and trigger immediate data re-reads. Implemented using `godbus/dbus/v5` with `AddMatchSignal()` for signal subscriptions. See Section 4.1novies.
If the lease file is unreadable or contains malformed data, `refreshDHCP()` logs a warning and leaves the tree unchanged (serving last-known-good data). The `GetMetrics()` D-Bus method call uses a 2-second timeout; on timeout or error, the metrics portion is omitted from the tree update while the lease data (if successfully parsed) is still applied. Similarly, `refreshFirewall()` applies a 5-second timeout to the firewalld D-Bus method calls; on timeout or error, the firewall tree retains its previous state.


**iw event**
The `iw event -t` command from the `iw` tool, which subscribes to the Linux kernel's nl80211 netlink family and emits timestamped, human-readable text lines on stdout for each 802.11 wireless event. Events include station associations/disassociations, connection/disconnection, channel switches, scan activity, and regulatory domain changes. Unlike yangerd's core netlink subscriptions (which use native Go via `vishvananda/netlink`), `iw event` is run as a subprocess because there is no mature Go nl80211 library. `iw event` does not produce JSON output—it requires custom text parsing. yangerd runs this as the only persistent event-monitoring subprocess (all other event monitoring uses native Go netlink channels).

**nl80211**
The Linux kernel's netlink-based interface for 802.11 wireless device configuration and monitoring. It is the successor to the older Wireless Extensions (WEXT) interface. The `iw` tool communicates with the kernel via nl80211 generic netlink messages. nl80211 defines over 300 attributes for wireless device state, including station information, scan results, regulatory domains, and channel configuration. yangerd accesses nl80211 indirectly through the `iw` command-line tool rather than implementing a Go-native nl80211 client, avoiding the complexity of parsing the extensive attribute set.

**ethnl (ethtool netlink)**
The Linux kernel's genetlink family for querying and configuring Ethernet device settings. It provides a structured netlink interface to ethtool functionality that was previously only accessible via ioctl. The family name is `"ethtool"` and it exposes commands for link info, link modes, features, WOL, rings, channels, coalesce, pause, EEE, FEC, module parameters, and more. Unconditionally available on Infix's target kernel (6.18). yangerd uses the ethtool netlink family both for typed queries (via `mdlayher/ethtool`) and for reactive monitoring (via `mdlayher/genetlink` subscription to the monitor multicast group).

**ETHTOOL_MSG_*_NTF**
Notification message types emitted by the kernel's ethtool netlink family when Ethernet device settings change. Each corresponds to a specific settings domain: `ETHTOOL_MSG_LINKINFO_NTF` (command 28) for link info changes (speed, PHY type, transceiver), `ETHTOOL_MSG_LINKMODES_NTF` (command 29) for link mode changes (advertised speeds, autoneg, duplex), `ETHTOOL_MSG_FEATURES_NTF` for offload feature changes, etc. Statistics and counters do not have NTF message types—they must be polled.

**genetlink multicast**
A mechanism in the Linux generic netlink subsystem that allows user-space processes to subscribe to named multicast groups and receive asynchronous notifications from the kernel. Each genetlink family can define one or more multicast groups. The ethtool family defines a single group named `"monitor"` (constant `ETHNL_MCGRP_MONITOR`) that delivers all `_NTF` notification messages. yangerd subscribes to this group via `genetlink.Conn.JoinGroup()` to receive ethtool setting change notifications.

**ETHNL_MCGRP_MONITOR**
The single multicast group defined by the kernel's ethtool genetlink family, named `"monitor"`. Subscribing to this group via `genetlink.Conn.JoinGroup(groupID)` delivers all ethtool notification messages (`ETHTOOL_MSG_LINKINFO_NTF`, `ETHTOOL_MSG_LINKMODES_NTF`, `ETHTOOL_MSG_FEATURES_NTF`, etc.) to the subscriber. The group ID is obtained at runtime by looking up the `"monitor"` group in the ethtool family's multicast group list via `genetlink.Family.Groups`.


**Full Interface Re-read**
The pattern where the link event handler (`monitor/link.go`) responds to an RTM_NEWLINK event by writing three queries to the persistent `ip -json -force -batch -` subprocess: `link show dev <iface>` (link state), `-s link show dev <iface>` (link state + hardware counters), and `addr show dev <iface>` (IP addresses). This captures the complete interface state at a single coherent point in time and updates the entire YANG subtree for that interface atomically. The full re-read also triggers a cross-subsystem ethtool re-query via `ethmonitor.RefreshInterface()`, since `ETHNL_MCGRP_MONITOR` does not fire on link up/down events.

**IPC Indirection**
The architecture pattern where statd does not directly collect operational data but delegates to a separate long-running daemon (yangerd) via a Unix socket. This decouples data collection timing from sysrepo callback timing: collection is reactive (driven by kernel events) or periodic (driven by a scheduler), while sysrepo callbacks are pull-on-demand. The indirection boundary is the socket — statd knows only the request/response protocol, not the collection mechanism.

**Reactive**
Data that is updated in response to asynchronous events rather than on a fixed timer. Reactive event sources include kernel netlink multicast messages, ethtool genetlink notifications, ZAPI route redistribution messages, bridge netlink triggers, and D-Bus signals (for service-managed data such as DHCP leases and firewall rules). A reactive update path has event-driven latency: the tree entry is updated within microseconds of the event, making the data current without polling. Contrast with *polling*.

**Polling**
Data collected on a fixed interval by querying a native protocol (e.g. chrony cmdmon), running an external process (e.g. `vtysh`), or reading a file. Polling is necessary for data sources that do not emit asynchronous events (neither kernel events nor D-Bus signals). yangerd uses polling only for data that cannot be obtained reactively (Phase 2 collectors). NTP data is polled via the chrony cmdmon protocol (native Go, no subprocess) because chrony has no event/subscription mechanism -- the protocol is strictly request-response. Polling interval is configurable per-collector via environment variables.

**RTNLGRP**
Routing Netlink Group — a numbered multicast group in the Linux netlink subsystem. Processes subscribe to one or more groups when opening a `NETLINK_ROUTE` socket. The kernel sends a copy of each matching event to every subscribed socket. yangerd uses four RTNLGRP groups (LINK, NEIGH, IPV4_IFADDR, IPV6_IFADDR) to receive notifications about changes to link state, addresses, and neighbours.

**ip -json -force -batch -**
A persistent `iproute2` subprocess that reads commands from stdin and produces JSON arrays on stdout. yangerd uses this as its primary mechanism for querying kernel network state for links, addresses, and neighbors, replacing direct netlink socket access via Go libraries. The `-json` flag enables JSON output, `-force` continues past errors (reporting them on stderr), and `-batch -` reads from stdin. Each command written to stdin produces exactly one JSON array on stdout (one per line). This approach delegates all netlink TLV attribute parsing to iproute2, which is always compiled against the running kernel's headers and supports every netlink attribute the kernel exposes. Note: route data is NOT queried via ip batch — route data comes from the ZAPI watcher's streaming connection to zebra's zserv socket (see Section 4.1octies).

**ip monitor -json (historical)**
The `iproute2` command `ip monitor -json` was originally considered for event monitoring but was found to NOT produce JSON output (confirmed by iproute2 source code analysis: `ip/ipmonitor.c` never calls `new_json_obj()`; see also Ubuntu bug #2116779). yangerd uses native Go netlink subscriptions via `vishvananda/netlink` instead. The `ip` binary is still used for state queries via `ip -json -force -batch -`, where the `-json` flag works correctly.

**wgctrl**
A pure-Go library (`golang.zx2c4.com/wireguard/wgctrl`) for querying WireGuard interface state via the WireGuard netlink family (`WireGuard genl family`). It returns typed Go structs for each WireGuard interface, its peers, allowed IPs, and handshake timestamps. Used by yangerd's Phase 2 WireGuard collector.

**vtysh**
FRRouting's integrated virtual shell — a CLI that connects to the control sockets of FRR daemons (ospfd, ripd, bfdd, bgpd) and forwards commands. yangerd's Phase 2 routing protocol collectors invoke `vtysh -c 'show ... json'` to obtain JSON-formatted protocol state. Requires membership in the `frr` Unix group.

**IPC frame**
The wire unit of the yangerd IPC protocol: a 1-byte protocol version (currently `1`), followed by a 4-byte big-endian unsigned integer encoding the payload length in bytes, followed immediately by that many bytes of JSON-encoded payload. The version byte enables future protocol changes to be detected unambiguously; a receiver that encounters an unknown version must close the connection. The maximum payload size enforced by yangerd is **4 MiB** (4 × 1024 × 1024 bytes); this is a software limit, not a protocol limit. Both request and response use the same framing. Partial reads (TCP-style) are handled by reading in a loop until exactly `length` bytes are accumulated before parsing.

**Operational datastore**
The sysrepo datastore holding current runtime state, as opposed to the `running`, `candidate`, and `startup` configuration datastores. The operational datastore is read-only from the management protocol perspective (NETCONF `<get>`, RESTCONF GET) and is populated by `sr_oper_get_subscribe()` callbacks registered by statd. yangerd's data ultimately reaches operators via this datastore after statd parses it with libyang and pushes it into sysrepo.

**sr_oper_get_subscribe**
The sysrepo C API function that registers a callback for operational data subtree queries. Signature: `sr_error_t sr_oper_get_subscribe(sr_session_ctx_t *session, const char *module_name, const char *path, sr_oper_get_items_cb callback, void *private_data, uint32_t opts, sr_subscription_ctx_t **subscription)`. Current legacy statd code calls this 13 times in `subscribe_to_all()`. The target yangerd design covers 14 YANG modules total (13 migration modules + `infix-services:mdns`). Each registered callback calls `ly_add_yangerd_data()` to populate the operational tree from yangerd's IPC response.

## Troubleshooting Guide

### IPC Connection Issues
If statd is unable to connect to yangerd, first verify that the daemon is running using the initctl status yangerd command. If the daemon is active, check the permissions on /run/yangerd.sock; it should be owned by root:yangerd with 0660 permissions. If the socket file is missing, check the system logs for any startup errors that might have caused the daemon to exit prematurely. You can also attempt to connect manually using the yangerctl health command to verify that the IPC server is responding to requests. Network namespace isolation can also interfere with socket communication if not correctly configured.

### Stale Data in the Tree
When a collector fails to update its designated module in the in-memory tree, yangerd retains the last known good value to prevent serving empty data. If you suspect that the data for a particular module like OSPF or LLDP is stale, use yangerctl health to check the timestamp of the last successful collection for that specific collector. A failure in a collector is usually accompanied by a warning message in the system log. Common causes for stale data include incorrect group memberships, unresponsive background services that the collectors depend on (e.g., FRRouting not yet running for OSPF/RIP/BFD), or feature-gated subsystems that are disabled in the build. Verification of kernel module status for protocols like WireGuard is also recommended.

### Performance Bottlenecks
Although yangerd is designed for high performance, extreme conditions can lead to increased latency. Use the top or htop utility to monitor the central processing unit and memory usage of the yangerd process. If memory usage is unexpectedly high, it may indicate a leak in a collector or an exceptionally large routing table that exceeds typical deployment scales. High central processing unit usage during event storms is mitigated by debouncing, but sustained storms may still impact responsiveness. Monitoring the drop counters in the health endpoint will indicate if the netlink event buffer is being exceeded, suggesting that the system is under more load than it can handle reactively.

## Detailed IPC Examples

### Example 1: Full Interface List Query
A client wishing to retrieve the entire operational state for all interfaces sends the following framed request. The length header would be forty-seven bytes to account for the JSON payload.
```json
{"method": "get", "path": "/ietf-interfaces:interfaces"}
```
The server responds with a success message containing the list of all interfaces and their associated statistics, carrier status, and assigned addresses. The response is encapsulated in the same length-prefixed framing format.

### Example 2: Routing Table Query
To retrieve only the IPv4 routing table, the path should be specified as follows in the request body.
```json
{"method": "get", "path": "/ietf-routing:routing/ribs/rib[name='ipv4-master']"}
```
The response will contain a structured representation of all IPv4 routes currently installed in the kernel's routing table, including destination prefixes, next-hop addresses, and outgoing interface names.
