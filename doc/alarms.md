Alarms
======

An alarm is a condition that needs attention and lasts until something
is done about it: a startup configuration that failed to apply, a
software slot the bootloader gave up on, a backup that did not run.  The
device raises an alarm when the condition appears and clears it when the
condition goes away.  A cleared alarm stays in the list, so what
happened while nobody was watching can still be seen, until it is
purged.

The alarm list follows [RFC 8632][] and is served under `/ietf-alarms:alarms`.
A monitoring system reads it over NETCONF or RESTCONF, the WebUI has an
Alarms page and shows the active ones on its dashboard, and the fault
LED of boards that have one follows it.  Alarm changes are logged to
syslog.

> [!NOTE]
> The alarm list is kept in memory and starts empty at boot.  Alarms,
> and what operators have done with them, do not survive a reboot; a
> condition that persists raises its alarm again, as new and
> unacknowledged.  Shelves and severity profiles are configuration and
> stay.

## Alarm Types

| Alarm type              | Severity | Raised when                                              |
|-------------------------|----------|----------------------------------------------------------|
| `startup-config-failed` | critical | The startup configuration could not be applied, the device runs its failure configuration |
| `boot-fallback`         | major    | A software slot failed to boot and is marked bad, the device runs the other slot |
| `partition-out-of-date` | warning  | The other software partition holds an older release than the running one, or none; `upgrade` refreshes the fallback |
| `link-down`             | major    | An enabled port, Wi-Fi interface or veth has had no link for ten seconds; disabling it in the configuration clears the alarm, so shelve the ports that have nothing connected |
| `update-available`      | warning  | The scheduled update check found a newer release; cleared when it is installed or no longer offered |
| `update-check-failed`   | minor    | The scheduled update check could not read the update source; cleared by the next check that succeeds |
| `update-install-failed` | major    | An unattended update failed to install; cleared by the next one that succeeds |
| `external`              | any      | A script or an external system raised it, see below      |

The full list the device can raise, with the severity of each, is in
the alarm inventory:

```
curl -k -u admin:admin https://example.local/restconf/data/ietf-alarms:alarms/alarm-inventory
```

## Reading Alarms

The alarm list has one entry per alarm, keyed by what it is about, the
`resource`, and its type.  An interface is named by its path in the
data model, `/ietf-interfaces:interfaces/interface[name='eth7']`, so a
client can take the alarm straight to the interface; the CLI and the
WebUI show the name.  Each entry says when it was first raised,
when it last changed, its severity, whether it is cleared, and what
operators have done with it.  In the CLI:

<pre class="cli"><code>admin@example:/> <b>show alarms</b>
LAST CHANGE       SEVERITY  STATE    RESOURCE   TYPE                TEXT
2026-11-03 06:12  critical  active   backup     external (nightly)  Nightly backup failed
2026-11-03 15:40  minor     closed   sensor3    external            Humidity sensor unreachable
2026-11-03 08:30  major     cleared  disk-full  external            Disk 95% full
1 shelved, see 'show alarms shelved'
</code></pre>

A resource shows its alarms in full, text, every timestamp and the
operator notes:

<pre class="cli"><code>admin@example:/> <b>show alarms sensor3</b>
sensor3  external
  Severity    : minor
  State       : closed
  Created     : 2026-11-03T15:40:00+00:00
  Last raised : 2026-11-03T15:40:00+00:00
  Last change : 2026-11-03T15:40:00+00:00
  Text        : Humidity sensor unreachable
  Operator history:
    2026-11-03 15:42  ack     admin
    2026-11-03 15:45  closed  admin  Sensor replaced
</code></pre>

The same over RESTCONF:

```
curl -k -u admin:admin https://example.local/restconf/data/ietf-alarms:alarms/alarm-list
```

```json
{
  "ietf-alarms:alarm-list": {
    "number-of-alarms": 1,
    "last-changed": "2026-11-03T06:12:40Z",
    "alarm": [
      {
        "resource": "backup",
        "alarm-type-id": "infix-alarms:external",
        "alarm-type-qualifier": "",
        "time-created": "2026-11-03T06:12:40Z",
        "is-cleared": false,
        "last-raised": "2026-11-03T06:12:40Z",
        "last-changed": "2026-11-03T06:12:40Z",
        "perceived-severity": "critical",
        "alarm-text": "Nightly backup failed"
      }
    ]
  }
}
```

The summary counts alarms per severity, for a dashboard or a quick
check:

```
curl -k -u admin:admin https://example.local/restconf/data/ietf-alarms:alarms/summary
```

## Acknowledging and Closing

The `set-operator-state` action on an alarm records that someone has
seen it, `ack`, or dealt with it, `closed`, with an optional note.  The
history of such changes is kept with the alarm, and goes with it when
the alarm is purged or the device reboots.  In the CLI, an alarm is
named by its resource; the type is only needed when the same resource
has alarms of several types:

<pre class="cli"><code>admin@example:/> <b>alarm ack backup text "Cable ordered"</b>
Alarm backup (external) acknowledged.
admin@example:/> <b>alarm close eth7</b>
Alarm eth7 (link-down) closed.
</code></pre>

Over RESTCONF:

```
curl -k -u admin:admin -X POST \
  -H 'Content-Type: application/yang-data+json' \
  -d '{"ietf-alarms:input": {"state": "ack", "text": "Cable ordered"}}' \
  "https://example.local/restconf/data/ietf-alarms:alarms/alarm-list/alarm=backup,infix-alarms:external,/set-operator-state"
```

The alarm list has a `set-operator-state` action of its own for many
alarms in one call.  It takes the same filter as purging below, the
state and an optional note, and answers with how many alarms changed.
Shelved alarms and alarms already in that state are left alone.  The
WebUI's *Close all* uses it:

```
curl -k -u admin:admin -X POST \
  -H 'Content-Type: application/yang-data+json' \
  -d '{"infix-alarms:input": {"alarm-clearance-status": "not-cleared", "state": "closed", "text": "Site visit done"}}' \
  https://example.local/restconf/data/ietf-alarms:alarms/alarm-list/infix-alarms:set-operator-state
```

## Purging

Cleared alarms stay in the list until purged.  In the CLI, `alarm
purge` removes every cleared alarm, `alarm purge all` the active ones
too:

<pre class="cli"><code>admin@example:/> <b>alarm purge</b>
Purged 2 alarm(s)
</code></pre>

The `purge-alarms` action behind it also takes a filter: only alarms
older than a given age, of a given severity, or in a given operator
state.  Without a filter it removes every cleared alarm and returns how
many went:

```
curl -k -u admin:admin -X POST \
  -H 'Content-Type: application/yang-data+json' \
  -d '{"ietf-alarms:input": {"alarm-clearance-status": "cleared", "older-than": {"days": 7}}}' \
  https://example.local/restconf/data/ietf-alarms:alarms/alarm-list/purge-alarms
```

Purging an alarm that is not cleared only forgets it; the condition is
not touched, and the alarm returns the next time it changes.

## Shelving

A shelf hides alarms that are known and expected, for a resource under
repair or a port with nothing connected, without disabling the alarm
type.  Shelved alarms are kept apart in `shelved-alarms`, do not count
in the summary and do not light the LED.  Removing the shelf brings
them back.

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit alarms control shelf spare-ports</b>
admin@example:/config/alarms/control/shelf/spare-ports/> <b>set resource eth[5-8]</b>
admin@example:/config/alarms/control/shelf/spare-ports/> <b>set description "Not patched yet"</b>
admin@example:/config/alarms/control/shelf/spare-ports/> <b>leave</b>
</code></pre>

Shelved alarms are listed separately:

<pre class="cli"><code>admin@example:/> <b>show alarms shelved</b>
LAST CHANGE       SEVERITY  STATE   RESOURCE  TYPE       SHELF        TEXT
2026-11-03 05:00  major     active  eth7      link-down  spare-ports  An enabled interface has no link.
</code></pre>

A resource is matched by a regular expression against its whole name,
`eth7` for an interface, or against the full path.  A shelf may also
name alarm types, in which case an alarm must match both the resource
and the type.  The qualifier match is a regular expression too, `.*`
takes every qualifier, including none:

<pre class="cli"><code>admin@example:/config/alarms/control/shelf/spare-ports/> <b>set alarm-type link-down alarm-type-qualifier-match .*</b>
</code></pre>

A shelved alarm takes no operator state.  `alarm ack` and `alarm close`
refuse it until the shelf is removed.

## Severity Assignment

The severity of an alarm type can be changed per resource with an alarm
profile.  A profile is keyed by the alarm type, a qualifier match and a
resource match, both regular expressions as in a shelf, and needs a
description.  To treat a lost link on uplinks as critical:

<pre class="cli"><code>admin@example:/config/> <b>edit alarms alarm-profile link-down alarm-type-qualifier-match .* resource "eth0|eth1"</b>
admin@example:/config/alarms/alarm-profile/.../> <b>set description "Uplinks"</b>
admin@example:/config/alarms/alarm-profile/.../> <b>set alarm-severity-assignment-profile severity-level critical</b>
admin@example:/config/alarms/alarm-profile/.../> <b>leave</b>
</code></pre>

The first profile that matches an alarm applies.  A profile with one
severity level gives every matching alarm that level.  External alarms
can be raised at any severity, and a profile with several levels maps
them in order, lowest to highest, so a script's alarms can be held to a
range of your own.  The assigned level shows as the alarm's
`perceived-severity`, and removing the profile puts the original back.

## Alarms from Scripts

A script, a container health check, or an external monitoring system
can raise alarms of the `external` type.  Over the management
protocols, the `raise-alarm` and `clear-alarm` operations do it, and in
the CLI `alarm clear` clears one:

<pre class="cli"><code>admin@example:/> <b>alarm clear backup</b>
</code></pre>

```
curl -k -u admin:admin -X POST \
  -H 'Content-Type: application/yang-data+json' \
  -d '{"infix-alarms:input": {"resource": "backup", "severity": "critical", "text": "Nightly backup failed"}}' \
  https://example.local/restconf/operations/infix-alarms:raise-alarm
```

On the device itself, an alarm is a file.  A script raises one by
creating `/run/alarm/active/external/<resource>` and clears it by
removing the file.  An empty file is a complete alarm with severity
`indeterminate`; to say more, write a JSON object with `severity` and
`text`:

```sh
echo '{"severity": "major", "text": "Disk 95% full"}' > /run/alarm/active/external/disk-full
...
rm /run/alarm/active/external/disk-full
```

The resource is one word: letters, digits, `.`, `_`, `:` and `-`.  A
qualifier that tells several alarms on the same resource apart is given
as `<resource>@<qualifier>` in the file name, or as the
`alarm-type-qualifier` input of the operations.  The directory is
writable by members of the `wheel` group.

[RFC 8632]: https://www.rfc-editor.org/rfc/rfc8632
