# yangerd

yangerd collects Infix operational data and keeps it ready as YANG-shaped
JSON.  statd asks it for a module's subtree when sysrepo asks statd, and
hands the answer to libyang.  yangerd replaces the Python yanger scripts,
which forked a collector per GET.

```
 NETCONF / RESTCONF client
            |
         sysrepo  <-- oper-get subscriptions --  statd (C)
                                                   |
                                         /run/yangerd.sock (IPC)
                                                   |
                                              yangerd (Go)
                                                   |
        netlink, nl80211, ethtool, ZAPI, D-Bus, wpa_supplicant/hostapd,
        ptp4l, chronyd, lldpcli, podman, FRR vty, inotify, sysfs/procfs
```

statd still owns everything sysrepo-facing: subscriptions, libyang
parsing and error codes.  yangerd has no sysrepo or libyang dependency.
It only knows the JSON its consumers expect.


## Design decisions

**Reactive first.**  A value with an event source is updated when the
event fires, never on a timer.  Netlink, nl80211, ZAPI, D-Bus signals,
wpa_supplicant and hostapd attach events, ptp4l subscriptions, inotify
and `podman events` all push.  Between events a monitor does nothing, and a GET returns
what the last event left.  An event names what changed, so the handler
refreshes that row, not the whole table; the full read is for start and
reconnect.

**Poll only what has no events.**  FRR's OSPF, RIP and BFD state and
gpsd don't announce changes, so they are polled.  The intervals are in
the table below and can be changed through the environment.  Data
without events that is only read on request, like the bridge FDB, is
not operational data but a YANG action in confd.

**On demand when a value changes all the time, or costs nothing to
read.**  Sensor readings, radio channel surveys, NTP source selection,
uptime, memory and load are computed at GET time by a tree provider, and
so is the hardware inventory, which is a read of `/run/system.json` and
sysfs.  chronyd is asked over its local command socket, so a GET sees a
source the moment chrony selects it.  Polling a temperature every ten
seconds that nobody reads is wasted work.  Providers run on the request
path, so they must be cheap and must not fork.  The one that does, the
service list forking `initctl -j`, is debt to be replaced when finit
announces state changes; do not copy it.

**One JSON blob per module.**  The tree (`internal/tree`) stores one
document per top-level YANG node, keyed like `ietf-interfaces:interfaces`,
each behind its own lock.  Writers replace (`Set`) or shallow-merge
(`Merge`) their part.  A key several sources share, such as
`ietf-routing:routing`, has each source owning distinct top-level members
and merging.  Providers overlay their output on the cached blob at read
time without changing it.

**Absent means no data.**  A feature that isn't active has no key.
yangerd deletes the key rather than storing an empty container, because
libyang would instantiate a presence container from `{}`.  statd treats
an empty answer as "nothing to add", not as an error.

**Long-lived helpers instead of forks.**  `ip`, `ip -s -d`, `bridge`
and `tc` run as `-json -force -batch -` subprocesses
(`internal/ipbatch`), fed one command per event.  chronyd is queried over cmdmon, ptp4l over its
management socket, FRR over the daemons' vty sockets, the DCB tables
over rtnetlink, all in-process.

**Restart, don't die.**  Every monitor runs under `backoff.Retry`: when
its source goes away (FRR restart, dnsmasq exit, `lldpcli watch` ending),
it reconnects with exponential backoff and rebuilds its state from a full
read.  A monitor's failure never takes the daemon down.

**Readiness is explicit.**  yangerd writes `/run/yangerd.pid` after the
first netlink dump, which is when `notify:pid` in finit marks it ready.
Until then the IPC answers "starting", which statd turns into no data
plus a warning.

**No CGo, Buildroot's Go.**  Use only the Go version Buildroot ships,
1.26 at the time of writing.  Dependencies are not kept in git, the
build vendors them from `go.sum`.


## What is reactive, polled and on demand

| Tree key | Content | How | Source |
|---|---|---|---|
| `ietf-interfaces:interfaces` | links, addresses, neighbours, bridge MDB, VLANs, LAG | event | rtnetlink, re-read with `ip`/`bridge` batch |
| | Ethernet speed, duplex, PMD | event | ethtool genetlink monitor, sweep at start; the frame counters are the `get-statistics` action in confd |
| | QoS capabilities | event | rtnetlink TC and DCB, re-read with `tc` batch and DCB over rtnetlink |
| | Wi-Fi station, AP, mesh point | event | nl80211 and wpa_supplicant/hostapd control sockets |
| | STP port and bridge state | poll 5 s | mstpd |
| | WireGuard peers | poll 10 s | `wg` netlink |
| | `last-change` | on demand | oper-state transitions seen since start |
| | `description` | on demand | sysfs `ifalias`, the link row is only current for interfaces that are up |
| | modem bearer | event | ModemManager D-Bus signals |
| | bridge IGMP/MLD querier | on demand | mcd IPC socket, `show interfaces json` |
| | interfaces in container network namespaces | event | podman events and netlink in each container netns, re-read with `podman ps` and `ip -n` |
| `ietf-routing:routing` | `ribs` | event | zebra ZAPI redistribution, or rtnetlink route events without FRR |
| | `control-plane-protocols` | poll 10 s | ospfd, ripd, bfdd vty sockets; the route tables are the `get-routes` actions in confd |
| | forwarding per interface | event | inotify on `/proc/sys/net/*/conf/*/forwarding` |
| `ietf-system:system` | hostname, timezone, users, SSH keys | event | inotify |
| `ietf-system:system-state` | services | on demand | `initctl -j`, last answer while finit is busy |
| | software slots, boot order | event | RAUC D-Bus signal, bootloader env files |
| | platform | once | at start |
| | clock, memory, load, filesystems, installer | on demand | procfs, statfs, RAUC D-Bus |
| | NTP sources | on demand | chronyd cmdmon |
| `ietf-hardware:hardware` | mainboard, VPD, USB ports | on demand | `/run/system.json`, sysfs |
| | radio capabilities | event | nl80211 phy, regulatory and interface events |
| | radio channel survey | on demand | nl80211 survey dump |
| | sensors | on demand | hwmon, thermal zones |
| | GPS receivers | poll 10 s | gpsd |
| | modems, SIM cards | event | ModemManager D-Bus signals, `/run/modemd` |
| `ietf-ntp:ntp` | associations, clock state, server stats | on demand | chronyd cmdmon |
| | presence, listening port | poll 60 s | chronyd cmdmon, `ss` |
| `ieee1588-ptp-tt:ptp` | port state, time status, parent | event | ptp4l subscription, near-static sets refreshed every 30 s |
| | the set of ptp4l instances | poll 10 s | configuration files |
| `ieee802-dot1ab-lldp:lldp` | neighbours | event | `lldpcli watch` |
| `infix-containers:containers` | containers | event | `podman events`, re-read with `podman ps` |
| `infix-dhcp-server:dhcp-server` | leases | event | dnsmasq D-Bus signals |
| `infix-firewall:firewall` | zones, policies, services | event | firewalld D-Bus signals |
| | address-set entries | on demand | nft, only sets with timeouts |
| `infix-services:tftp` | files served | event | inotify on the root, mount table changes |

A SIGHUP pokes the routing, NTP and hardware collectors once.  confd
sends it after a commit, best effort, so polled data catches up with
new config, and the routing collector reads the OSPF auto-cost from the
daemon's running-config again, which only a commit changes.  The STP
and WireGuard polls run on their timer only.

The bridge FDB is not collected.  A switch learns addresses without
telling anyone, so it could only be polled; the `get-forwarding-database`
action in confd reads it when asked.


## IPC

AF_UNIX stream socket, `/run/yangerd.sock`, root only (0660, root:root).
One request per connection, every connection with a 5 s deadline.

```
| ver (1) | length (4, big endian) | JSON |
```

Version is 2.  A request is `{"method": "get", "path": "<tree key>"}`.
The response header is `{"status": "ok", "raw": true}`, and the data
follows in a second frame, so statd can pass it to
`lyd_parse_data_mem()` without decoding the envelope.  `dump` returns all
keys, and `health` returns each key's size and last update time.  The
maximum frame is 4 MiB.

`yangerctl` speaks the same protocol for debugging:

```
yangerctl get /ietf-interfaces:interfaces
yangerctl dump
yangerctl health
```


## The statd side

statd subscribes once per module and derives the yangerd key from the
subscription xpath, not from the request.  When you add a module, keep
in mind:

- **Operational replaces running.**  Subscriptions don't pass
  `SR_SUBSCR_OPER_MERGE`, so what yangerd returns is all a client sees
  under that path.  Config-only leaves are absent.  The exception is
  `ietf-routing:routing/control-plane-protocols`, which is merged,
  because the BFD instance exists only in operational and static route
  instances only in running.
- **Nested subscriptions graft.**  For a nested path such as
  `/infix-services:tftp/files`, statd parses the whole module answer and
  moves only the requested node under sysrepo's parent.
- **One bad value costs one entry.**  If the answer doesn't parse, statd
  parses each list entry alone, drops the ones libyang rejects, and logs
  them as `yangerd: dropping <key> <list>[<name>]: <reason>`.  If you see
  that line, fix the value in yangerd.


## Configuration

Environment from `/etc/default/yangerd`, installed from
`package/yangerd/yangerd.default`.

| Variable | Default |
|---|---|
| `YANGERD_SOCKET` | `/run/yangerd.sock` |
| `YANGERD_LOG_LEVEL` | `info` |
| `YANGERD_POLL_INTERVAL_ROUTING` | `10s` |
| `YANGERD_POLL_INTERVAL_NTP` | `60s`, presence and port only |
| `YANGERD_POLL_INTERVAL_HARDWARE` | `10s`, GPS only |
| `YANGERD_POLL_INTERVAL_STP` | `5s` |

Which optional parts run is not configured, it follows the YANG model:
Wi-Fi, containers, GPS and modems are features of `infix-interfaces` and
`infix-hardware`, and the routing protocol modules are installed only
with FRR.  confd enables them at build time from the same Buildroot
options that build the daemons, and `ietf-yang-library` reports them
to clients, so yangerd reads the list sysrepo keeps at
`/etc/sysrepo/data/sysrepo.startup` once at start (`src/features`, shared with the WebUI)
and does not start the monitors of what is not there.  The WebUI keys
its optional pages on the same features.  Without the file every
monitor runs; each copes with an absent daemon on its own.


## Adding a data source

1. **Find the event.**  If the source can tell you when it changes, write
   a monitor with a `Run(ctx) error` method and start it with `spawn()` in
   `cmd/yangerd/main.go`.  Put the reconnect loop in `backoff.Retry`, and
   rebuild the full state after each reconnect, not just what later events
   report.
2. **No event:** register a tree provider.  It runs on every GET, so it
   may only do cheap reads: a procfs or sysfs file, a value cached by an
   event, a local socket that answers at once.  No forks, no network
   calls.  If the only way to the value is running a command, find the
   event that makes the command's answer change and cache it from there.
3. **A poll is not an option.**  The polled collectors that exist are
   debt, not a pattern; do not add to them without asking.  A poll hides
   a change for up to its interval, costs CPU on an idle box and races
   tests that read right after a change.  If a change seems to need a
   poll, say which event source is missing and stop.  First ask whether
   the data has to be operational at all: a value read only when someone
   asks, such as the bridge FDB or a Wi-Fi scan, is better as a YANG
   action or RPC in confd, which reads it on request.  A poll that is
   agreed on goes in the table under *What is reactive, polled and on
   demand*, with its interval variable in the environment table, and
   in the SIGHUP paragraph if SIGHUP pokes it.
4. Emit JSON in the shape statd's libyang expects: module-prefixed top
   node, RFC 7951 encoding (64-bit integers as strings, numeric union
   members as numbers), no empty containers.
5. Delete the key when the feature is inactive.
6. Add the subscription in `src/statd/statd.c` if the module is new.
7. Unit test with `testutil.MockRunner` and `testutil.MockFileReader`,
   or with a fake subprocess, see `internal/ipbatch`.


## Gotchas

- **iproute2 caches interface names.**  A long-lived `ip -batch` resolves
  a name through a cache that never expires, so after an interface is
  deleted and recreated, `dev wifi0` points at the dead index.  Address
  and neighbour queries therefore name devices `if<ifindex>`.
  `link show` doesn't accept that form, so a failed link query for an
  interface the kernel still has restarts the `ip` process and asks again.
- **`ip` can answer `[{}]`.**  It opens the JSON object before checking
  the netlink message, so a link racing a delete comes back empty.  Only
  stage a link answer with the queried ifindex and a name.
- **Link and address events use separate sockets.**  They can be handled
  out of order: a delete for an old interface may arrive after a new one
  took its name.  Staging is keyed by ifindex, and name-keyed state is only
  dropped when no interface carries the name any more.
- **A dead batch means one re-dump.**  While `ip` restarts, every event
  fails.  Failures ask for a re-dump, coalesced into one that waits until
  the batches are back.
- **`bridge` show filters outlive the command.**  The `dev`, `br` and
  `vlan` filters of a show are statics, so in a batch they apply to the
  next show of the same kind too.  Our iproute2 resets them for `vlan
  show`, the only show the bridge batch gives a device.
- **nl80211 delete events.**  On `DEL_INTERFACE` the ifindex no longer
  resolves, so take the name from the message.
- **FRR daemons differ.**  bfdd installs its show commands in the enable
  node only, so the vty client sends `enable` first, as vtysh does.
- **No link event for an alias on a down interface.**  The kernel only
  notifies an alias change through `netif_state_change()`, which is a
  no-op unless the interface is up.  A description set on a PPPoE
  interface before pppd raises it, or on a disabled port, never reaches
  the staged link row.  The interfaces provider re-reads
  `/sys/class/net/<if>/ifalias` on every GET instead.
- **Operational lags the commit.**  Data follows events, so a test that
  reads back a value right after a commit has to poll with `until()`.


## Building and testing

```
go vet ./...
go test -race ./...
make yangerd-rebuild all        # from the repo root, rebuilds the image
```

On a DUT, `YANGERD_LOG_LEVEL=debug` in `/etc/default/yangerd` followed by
`initctl restart yangerd` logs every batch command that fails and every
event that is dropped.
