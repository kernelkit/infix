# SNMP

An SNMP agent is available for monitoring the system from an existing network
management system.  It is read-only: the agent answers GET and GETNEXT, and
there is no way to configure write access.  Changing configuration is done
over NETCONF, RESTCONF or the CLI, where NACM controls who may do what.

The agent is disabled by default and has no community configured, so enabling
it is a deliberate act.

> [!IMPORTANT]
> SNMPv1 and v2c carry the community string in clear text on the wire, and
> anyone who can read it can read everything the community's view allows.
> Restrict access by source address, and prefer a management VLAN or firewall
> zone that is not reachable from the outside.  Another alternative is to set
> up a [WireGuard](vpn-wireguard.md) tunnel for management until SNMPv3
> support is added.

## What is served

| MIB                | Contents                                                                       |
|--------------------|--------------------------------------------------------------------------------|
| SNMPv2-MIB         | `sysDescr`, `sysObjectID`, `sysUpTime`, `sysName`, `sysContact`, `sysLocation` |
| IF-MIB             | `ifTable` and `ifXTable` for every interface                                   |
| HOST-RESOURCES-MIB | storage, processors, running software                                          |
| UCD-SNMP-MIB       | load average, memory, disk usage                                               |
| LLDP-MIB           | neighbors, when LLDP is enabled                                                |

Every port is an interface, including ports offloaded to a switch fabric, so
they all appear in the `ifTable`.  The `ifIndex` an NMS sees is the same as
the `if-index` reported under `/interfaces/interface[name='eth0']/if-index`,
which makes it easy to correlate SNMP data with the operational datastore.

We recommend using `ifName` from `ifXTable` as the stable key for a port, not
`ifIndex`.  Interface indices are assigned by the kernel and may shift when
configuration creates or removes VLANs, bridges and other virtual interfaces.

> [!NOTE]
> The counters are the ones the interface itself reports, the same values
> `show interface` shows.  On a port whose traffic is forwarded by the switch
> fabric, how much of that traffic the counters account for depends on the
> switch and its driver.

`sysContact` and `sysLocation` come from `/system/contact` and
`/system/location`, and `sysName` follows `/system/hostname`.  See
[Contact and Location][contact] for what to put in them.

## Configuration

The data model is [RFC 7407][], `ietf-snmp`.  Two things are needed: the
engine enabled, and a community to answer for.

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>set snmp engine enabled</b>
admin@example:/config/> <b>set snmp community monitor security-name public</b>
admin@example:/config/> <b>leave</b>
</code></pre>

That is the whole thing.  `monitor` is just a name for the list entry; the
community string on the wire is `public`, taken from the security name because
no separate `text-name` was given.  Set `text-name` when the two should
differ:

<pre class="cli"><code>admin@example:/> <b>set snmp community monitor text-name s3cret</b>
</code></pre>

<pre class="cli"><code>admin@example:/> <b>show snmp</b>
Enabled         : yes
Versions        : v1, v2c (default)
Listen          : 0.0.0.0:161, [::]:161 (default)

COMMUNITY  SECURITY NAME  SOURCE
public     public         any
</code></pre>

Leaving `version` or `listen` unset means the agent serves every version it
supports, on every address, port 161.  Narrow them when you want something
tighter:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>set snmp engine version v2c</b>
admin@example:/config/> <b>edit snmp engine listen mgmt udp</b>
admin@example:/config/snmp/engine/listen/mgmt/udp/> <b>set ip 192.168.1.10</b>
admin@example:/config/snmp/engine/listen/mgmt/udp/> <b>leave</b>
</code></pre>

Every community reads the whole MIB tree, and nothing can write.  There is no
view or group to configure.  Limit *who* may ask instead, with a source
restriction.

### Restricting by source address

A community with no restriction answers anyone who knows the string.  To limit
it, define a target holding the permitted prefix, tag it, and point the
community at that tag:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit snmp target nms</b>
admin@example:/config/snmp/target/nms/> <b>set udp ip 192.168.1.0</b>
admin@example:/config/snmp/target/nms/> <b>set udp prefix-length 24</b>
admin@example:/config/snmp/target/nms/> <b>set tag nms</b>
admin@example:/config/snmp/target/nms/> <b>set target-params nms</b>
admin@example:/config/snmp/target/nms/> <b>end</b>
admin@example:/config/> <b>set snmp community monitor target-tag nms</b>
admin@example:/config/> <b>leave</b>
</code></pre>

> [!IMPORTANT]
> If no target carries the tag, the community is dropped rather than left
> answering any source.  Check `show snmp` after setting it: the community
> disappears from the table if the tag matched nothing.

### Firewall

The agent listens on UDP port 161.  Permit the `snmp` service in the zone
facing the management network:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit firewall zone mgmt</b>
admin@example:/config/firewall/zone/mgmt/> <b>set service snmp</b>
admin@example:/config/firewall/zone/mgmt/> <b>leave</b>
</code></pre>

## Reading the data

From a management host:

<pre class="cli"><code>linux-pc:# <b>snmpwalk -v2c -c public 192.168.1.10 IF-MIB::ifXTable</b>
IF-MIB::ifName.1 = STRING: lo
IF-MIB::ifName.2 = STRING: eth0
IF-MIB::ifName.3 = STRING: eth1
IF-MIB::ifHCInOctets.2 = Counter64: 20748536
IF-MIB::ifHCOutOctets.2 = Counter64: 1332378
</code></pre>

> [!TIP]
> Poll over SNMPv2c, not v1.  SNMPv1 has no `Counter64`, so the whole of
> `ifXTable` is unreadable over v1 and only the 32-bit counters in `ifTable`
> remain.  Those wrap in well under an hour on a busy gigabit port, which is
> not enough for useful throughput graphs.  Set `snmp engine version v2c` to
> refuse v1 outright.

The device itself carries no MIB text files, so the `snmpwalk` and `snmpget`
on it work with numeric OIDs only.  That is enough for a quick check from the
console:

<pre class="cli"><code>admin@example:~$ <b>snmpwalk -v2c -c public 127.0.0.1 .1.3.6.1.2.1.31.1.1.1.1</b>
.1.3.6.1.2.1.31.1.1.1.1.1 = STRING: lo
.1.3.6.1.2.1.31.1.1.1.1.2 = STRING: eth0
</code></pre>

## Community strings are secrets

`ietf-snmp` marks the community name and the security name
`nacm:default-deny-all`, which sysrepo honours, so they are unreadable by
default.  An operator or guest sees that a community exists but not what it is
called.

## Not yet supported

SNMPv3 (USM), notifications (traps and informs), view-based access control,
the BRIDGE-MIB and the ENTITY-MIB are not implemented.

[contact]: system.md#contact-and-location
[RFC 7407]: https://www.rfc-editor.org/rfc/rfc7407
