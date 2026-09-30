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
what the last event left without forking anything.

**Poll only what has no events.**  FRR's OSPF, RIP and BFD state, the
service list and gpsd don't announce changes, so they are polled.  The intervals are in the table below and can be
changed through the environment.

**On demand when a value changes all the time, or costs nothing to
read.**  Sensor readings, radio channel surveys, NTP source selection,
uptime, memory and load are computed at GET time by a tree provider, and
so is the hardware inventory, which is a read of `/run/system.json` and
sysfs.  chronyd is asked over its local command socket, so a GET sees a
source the moment chrony selects it.
Polling a temperature every ten seconds that nobody reads is wasted
work.  Providers run on the request path, so they must be cheap.

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

**Long-lived helpers instead of forks.**  `ip`, `ip -s -d` and `bridge`
run as `-json -force -batch -` subprocesses (`internal/ipbatch`), fed one
command per event.  chronyd is queried over cmdmon, ptp4l over its
management socket, FRR over the daemons' vty sockets, all in-process.

**Restart, don't die.**  Every monitor runs under `backoff.Retry`: when
its source goes away (FRR restart, dnsmasq exit, `lldpcli watch` ending),
it reconnects with exponential backoff and rebuilds its state from a full
read.  A monitor's failure never takes the daemon down.

**Readiness is explicit.**  yangerd writes `/run/yangerd.pid` after the
first netlink dump, which is when `notify:pid` in finit marks it ready.
Until then the IPC answers "starting", which statd turns into no data
plus a warning.

**No CGo, Go 1.23, vendored.**  Buildroot ships Go 1.23, so no newer
language features.  Dependencies are vendored (`GOFLAGS=-mod=vendor`).


## What is reactive, polled and on demand

| Tree key | Content | How | Source |
|---|---|---|---|
| `ietf-interfaces:interfaces` | links, addresses, neighbours, bridge FDB/MDB, VLANs, LAG | event | rtnetlink, re-read with `ip`/`bridge` batch |
| | Ethernet speed, duplex, PMD | event | ethtool genetlink monitor, sweep at start |
| | WiFi station, AP, mesh point | event | nl80211 and wpa_supplicant/hostapd control sockets |
| | STP port and bridge state | poll 5 s | mstpd |
| | WireGuard peers | poll 10 s | `wg` netlink |
| | `last-change` | on demand | oper-state transitions seen since start |
| `ietf-routing:routing` | `ribs` | event | zebra ZAPI redistribution |
| | `control-plane-protocols` | poll 10 s | ospfd, ripd, bfdd vty sockets |
| | forwarding per interface | event | inotify on `/proc/sys/net/*/conf/*/forwarding` |
| `ietf-system:system` | hostname, timezone, users, SSH keys | event | inotify |
| `ietf-system:system-state` | services | poll 60 s | `initctl -j` |
| | software slots, boot order | event | RAUC D-Bus signal, bootloader env files |
| | platform | once | at start |
| | clock, memory, load, filesystems, installer | on demand | procfs, statfs, RAUC D-Bus |
| | NTP sources | on demand | chronyd cmdmon |
| `ietf-hardware:hardware` | mainboard, VPD, USB ports | on demand | `/run/system.json`, sysfs |
| | radio capabilities | event | nl80211 phy, regulatory and interface events |
| | radio channel survey | on demand | nl80211 survey dump |
| | sensors | on demand | hwmon, thermal zones |
| | GPS receivers | poll 10 s | gpsd |
| `ietf-ntp:ntp` | associations, clock state, server stats | on demand | chronyd cmdmon |
| | presence, listening port | poll 60 s | chronyd cmdmon, `ss` |
| `ieee1588-ptp-tt:ptp` | port state, time status, parent | event | ptp4l subscription, near-static sets refreshed every 30 s |
| `ieee802-dot1ab-lldp:lldp` | neighbours | event | `lldpcli watch` |
| `infix-containers:containers` | containers | event | `podman events`, re-read with `podman ps` |
| `infix-dhcp-server:dhcp-server` | leases | event | dnsmasq D-Bus signals |
| `infix-firewall:firewall` | zones, policies, services | event | firewalld D-Bus signals |
| | address-set entries | on demand | nft, only sets with timeouts |
| `infix-services:tftp` | files served | event | inotify on the root, mount table changes |

A SIGHUP pokes every polled collector once.  confd sends it after a
commit, best effort, so polled data catches up with new config.


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

`/etc/default/yangerd`, written by `package/yangerd/yangerd.mk` from the
Buildroot selection.

| Variable | Default |
|---|---|
| `YANGERD_SOCKET` | `/run/yangerd.sock` |
| `YANGERD_LOG_LEVEL` | `info` |
| `YANGERD_POLL_INTERVAL_SYSTEM` | `60s` |
| `YANGERD_POLL_INTERVAL_ROUTING` | `10s` |
| `YANGERD_POLL_INTERVAL_NTP` | `60s`, presence and port only |
| `YANGERD_POLL_INTERVAL_HARDWARE` | `10s`, GPS only |
| `YANGERD_POLL_INTERVAL_STP` | `5s` |
| `YANGERD_ENABLE_WIFI` | `false` |
| `YANGERD_ENABLE_LLDP` | `true` |
| `YANGERD_ENABLE_FIREWALL` | `true` |
| `YANGERD_ENABLE_DHCP` | `true` |
| `YANGERD_ENABLE_CONTAINERS` | `false` |
| `YANGERD_ENABLE_GPS` | `false` |

A disabled feature's monitor is not started at all.


## Adding a data source

1. **Find the event.**  If the source can tell you when it changes, write
   a monitor with a `Run(ctx) error` method and start it with `spawn()` in
   `cmd/yangerd/main.go`.  Put the reconnect loop in `backoff.Retry`, and
   rebuild the full state after each reconnect, not just what later events
   report.
2. **No event, value moves slowly:** implement `collector.Collector` and add
   it to the polled list.  Give the interval an environment variable.
3. **No event, value moves all the time:** register a tree provider.  It
   runs on every GET, so no forks and no network calls.
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
  deleted and recreated, `dev wifi0` points at the dead index.  Address,
  neighbour and FDB queries therefore name devices `if<ifindex>`.
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
- **nl80211 delete events.**  On `DEL_INTERFACE` the ifindex no longer
  resolves, so take the name from the message.
- **FRR daemons differ.**  bfdd installs its show commands in the enable
  node only, so the vty client sends `enable` first, as vtysh does.
- **Operational lags the commit.**  Data follows events, so a test that
  reads back a value right after a commit has to poll with `until()`.


## Building and testing

```
go vet -mod=vendor ./...
go test -race -mod=vendor ./...
make yangerd-rebuild all        # from the repo root, rebuilds the image
```

On a DUT, `YANGERD_LOG_LEVEL=debug` in `/etc/default/yangerd` followed by
`initctl restart yangerd` logs every batch command that fails and every
event that is dropped.
