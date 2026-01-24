# Network Configuration

Infix aims to support all Linux Networking constructs.  The YANG models
used to describe the system are chosen to fit well and leverage the
underlying Linux kernel's capabilities.  The [ietf-interfaces.yang][1]
model forms the base, extended with [ietf-ip.yang][2] and other layer-3
IETF models.  The layer-2 bridge and aggregate models are defined by
Infix to exploit the unique features not available in IEEE models.

> [!IMPORTANT]
> When issuing `leave` to activate your changes, remember to also save
> your settings, `copy running-config startup-config`.  See the [CLI
> Introduction](cli/introduction.md) for a background.


## Interface LEGO®

The network building blocks available in Linux are akin to the popular
LEGO® bricks.

![Linux Networking Blocks](img/lego.svg)

There are two types of relationships that can link two blocks together:

  1. **Lower-to-upper**: Visually represented by an extruding square
     connected upwards to a square socket.  An interface _can only have
     a single_ lower-to-upper relationship, i.e., it can be attached to
     a single upper interface like a bridge or a LAG.  In `iproute2`
     parlance, this corresponds to the interface's `master` setting
  2. **Upper-to-lower**: Visually represented by an extruding semicircle
     connected downwards to a semicircle socket.  The lower interface in
     these relationships _accepts multiple_ upper-to-lower relationships
     from different upper blocks.  E.g., multiple VLANs and IP address
     blocks can be connected to the same lower interface

![Stacking order dependencies](img/lego-relations.svg)

An interface may simultaneously have a _lower-to-upper_ relation to some
other interface, and be the target of one or more _upper-to-lower_
relationships.  It is valid, for example, for a physical port to be
attached to a bridge, but also have a VLAN interface stacked on top of
it.  In this example, traffic assigned to the VLAN in question would be
diverted to the VLAN interface before entering the bridge, while all
other traffic would be bridged as usual.

| **Type** | **Yang Model**             | **Description**                                              |
|----------|----------------------------|--------------------------------------------------------------|
| bridge   | infix-if-bridge            | SW implementation of an IEEE 802.1Q bridge                   |
| ip       | ietf-ip, infix-ip          | IP address to the subordinate interface                      |
| vlan     | infix-if-vlan              | Capture all traffic belonging to a specific 802.1Q VID       |
| lag      | infix-if-lag               | Link aggregation, static and IEEE 802.3ad (LACP)             |
| lo       | ietf-interfaces            | Software loopback interface                                  |
| eth      | ieee802-ethernet-interface | Physical Ethernet device/port.                               |
|          | infix-ethernet-interface   |                                                              |
| veth     | infix-if-veth              | Virtual Ethernet pair, typically one end is in a container   |
| *common* | ietf-interfaces,           | Properties common to all interface types                     |
|          | infix-interfaces           |                                                              |


## Data Plane

The blocks you choose, and how you connect them, defines your data plane.
Here we see an example of how to bridge a virtual port with a physical
LAN.

![Example of a 4-port switch with a link aggregate and a VETH pair to a container](img/dataplane.svg)

Depending on the (optional) VLAN filtering of the bridge, the container
may have full or limited connectivity with outside ports, as well as the
internal CPU.

In fact the virtual port connected to the bridge can be member of
several VLANs, with each VLAN being an interface with an IP address
inside the container.

Thanks to Linux, and technologies like switchdev, that allow you to
split a switching fabric into unique (isolated) ports, the full
separation and virtualization of all Ethernet layer properties are
possible to share with a container.  Meaning, all the building blocks
used on the left hand side can also be used freely on the right hand
side as well.


### General

General interface settings include `type`, `enable`, custom MAC address,
and text `description`.  Other settings have their own sections, below.

The `type` is important to set when configuring devices remotely because
unlike the CLI, a NETCONF or RESTCONF session cannot guess the interface
type for you.  The operating system provides an override of the
available interface types.

An `enabled` interface can be inspected using the operational datastore,
nodes `admin-state` and `oper-state` show the status, .  Possible values
are listed in the YANG model.

The `custom-phys-address` can be used to set an interface's MAC address.
This is an extension to the ietf-interfaces YANG model, which defines
`phys-address` as read-only[^4].  The following shows the different
configuration options.

The `description` is saved as Linux `ifalias` on an interface.  It is a
free-form string, useful for describing purpose or just adding comments
for remote debugging, e.g., using the operational datastore.

> [!CAUTION]
> There is no validation or safety checks performed by the system when
> using `custom-phys-address`.  In particular the `offset` variant can
> be dangerous to use -- pay attention to the meaning of bits in the
> upper-most octet: local bit, multicast/group, etc.

#### Fixed custom MAC

Use a fixed custom MAC address when the interface must present a
specific, deterministic identity on the network.  This option bypasses
any chassis-derived logic and applies the configured address verbatim.

<pre class="cli"><code>admin@example:/config/> <b>edit interface veth0a</b>
admin@example:/config/interface/veth0a/> <b>set custom-phys-address static 00:ab:00:11:22:33</b>

=> 00:ab:00:11:22:33
</code></pre>

#### Chassis MAC

Chassis MAC, sometimes also referred to as base MAC.  In these two
examples it is `00:53:00:c0:ff:ee`.

<pre class="cli"><code>admin@example:/config/> <b>edit interface veth0a</b>
admin@example:/config/interface/veth0a/> <b>set custom-phys-address chassis</b>

=> 00:53:00:c0:ff:ee
</code></pre>

#### Chassis MAC, with offset

When constructing a derived address it is recommended to set the locally
administered bit.  Same chassis MAC as before.

<pre class="cli"><code>admin@example:/config/> <b>edit interface veth0a</b>
admin@example:/config/interface/veth0a/> <b>set custom-phys-address chassis offset 02:00:00:00:00:02</b>

=> 02:53:00:c0:ff:f0
</code></pre>

### Bridging

This is the most central part of the system.  A bridge is a switch, and
a switch is a bridge.  In Linux, setting up a bridge with ports
connected to physical switch fabric, means you manage the actual switch
fabric!

#### MAC Bridge

In Infix ports are by default not switch ports, unless the customer
specific factory config sets it up this way.  To enable switching, with
offloading if you have a switch chipset, between ports you create a
bridge and then add ports to that bridge.  Like this:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interface br0</b>
admin@example:/config/interface/br0/> <b>up</b>
admin@example:/config/> <b>set interface eth0 bridge-port bridge br0</b>
admin@example:/config/> <b>set interface eth1 bridge-port bridge br0</b>
admin@example:/config/> <b>leave</b>
</code></pre>

Here we add two ports to bridge `br0`: `eth0` and `eth1`.

> [!TIP]
> The CLI has several built-in helpers governed by convention.  E.g.,
> naming bridges `brN`, where `N` is a number, the type is *inferred*
> automatically and unlocks all bridge features.  Other conventions are
> `vethNA`, where `N` is a number and `A` is a letter ('a' for access
> port and 'b' for bridge side is common), and `ethN.M` for VLAN M on
> top of `ethN`, or `dockerN` for a IP masquerading container bridge.
>
> Note, this inference only works with the CLI, configuring networking
> over NETCONF or RESTCONF requires setting the type explicitly.

![A MAC bridge with two ports](img/mac-bridge.svg)

It is possible to create multiple MAC bridges, however, it is
currently[^5] _not recommended_ to use more than one MAC bridge on
products with Marvell LinkStreet switching ASICs. A VLAN filtering
bridge should be used instead.

#### VLAN Filtering Bridge

By default bridges in Linux do not filter based on VLAN tags.  This can
be enabled when creating a bridge by adding a port to a VLAN as a tagged
or untagged member.  Use the port default VID (PVID) setting to control
VLAN association for traffic ingressing a port untagged (default PVID:
1).

<pre class="cli"><code>admin@example:/config/> <b>edit interface br0</b>
admin@example:/config/interface/br0/> <b>up</b>
admin@example:/config/> <b>set interface eth0 bridge-port bridge br0</b>
admin@example:/config/> <b>set interface eth0 bridge-port pvid 10</b>
admin@example:/config/> <b>set interface eth1 bridge-port bridge br0</b>
admin@example:/config/> <b>set interface eth1 bridge-port pvid 20</b>
admin@example:/config/> <b>edit interface br0</b>
admin@example:/config/interface/br0/> <b>set bridge vlans vlan 10 untagged eth0</b>
admin@example:/config/interface/br0/> <b>set bridge vlans vlan 20 untagged eth1</b>
</code></pre>

This sets `eth0` as an untagged member of VLAN 10 and `eth1` as an
untagged member of VLAN 20.  Switching between these ports is thus
prohibited.

![A VLAN bridge with two VLANs](img/vlan-bridge.svg)

To terminate a VLAN in the switch itself, either for switch management
or for routing, the bridge must become a (tagged) member of the VLAN.

<pre class="cli"><code>admin@example:/config/interface/br0/> <b>set bridge vlans vlan 10 tagged br0</b>
admin@example:/config/interface/br0/> <b>set bridge vlans vlan 20 tagged br0</b>
</code></pre>

To route or to manage via a VLAN, a VLAN interface needs to be created
on top of the bridge, see section [VLAN Interfaces](#vlan-interfaces)
below for more on this topic.

> [!NOTE]
> In some use-cases only a single management VLAN on the bridge is used.
> For the example above, if the bridge itself is an untagged member only
> in VLAN 10, IP addresses can be set directly on the bridge without the
> need for dedicated VLAN interfaces on top of the bridge.


#### Multicast Filtering and Snooping

Multicast filtering in the bridge is handled by the bridge itself.  It
can filter both IP multicast and MAC multicast.  For IP multicast it
also supports "snooping", i.e., IGMP and MLD, to automatically reduce
the broadcast effects of multicast.  See the next section for a summary
of the [terminology used](#terminology-abbreviations).

> [!IMPORTANT]
> Currently there is no way to just enable multicast filtering without
> also enabling snooping.  This may change in the future, in which case
> a `filtering` enabled setting will be made available along with the
> existing `snooping` setting.

When creating your bridge you must decide if you need a VLAN filtering
bridge or a plain bridge (see previous section).  Multicast filtering is
supported for either, but take note that it must be enabled and set up
per VLAN when VLAN filtering is enabled -- there are no global multicast
settings in this operating mode.

In the following example we have a regular 8-port bridge without VLAN
filtering.  We focus on the multicast specific settings:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interface br0</b>
admin@example:/config/interface/br0/> <b>set bridge multicast snooping</b>
admin@example:/config/interface/br0/> <b>set ipv4 address 192.168.2.1 prefix-length 24</b>
admin@example:/config/interface/br0/> <b>leave</b>
admin@example:/> <b>copy running-config startup-config</b>
</code></pre>

Here we enable snooping and set a static IPv4 address so that the switch
can take part in IGMP querier elections.  (MLD querier election
currently not supported.)  We can inspect the current state:

<pre class="cli"><code>admin@example:/> <b>show ip multicast</b>
Multicast Overview
Query Interval (default): 125 sec
Router Timeout          : 255
Fast Leave Ports        :
Router Ports            :
Flood Ports             : e0, e1, e2, e3, e4, e5, e6, e7

<span class="header">Interface       VID  Querier                     State  Interval  Timeout  Ver</span>
br0                  192.168.2.1                    Up       125     None    3

<span class="header">Bridge          VID  Multicast Group       Ports                              </span>
br0                  224.1.1.1             e3, e2
br0                  ff02::6a              br0
</code></pre>

This is a rather small LAN, so our bridge has already become the elected
IGMP querier.  We see it is ours because the timeout is `None`, and we
recognize the IP address the system has detected, as ours.  We can also
see two ports that have joined the same IPv4 multicast group, 224.1.1.1,
and one join from the system itself for the IPv6 group ff02::6a.

Now, let us see what happens when we add another bridge, this time with
VLAN filtering enabled.  We skip the boring parts about how to move
ports e4-e7 to `br1` and assign them to VLANs, and again, focus on the
multicast bits only:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interface br1</b>
admin@example:/config/interface/br1/> <b>set bridge vlans vlan 1 multicast snooping</b>
admin@example:/config/interface/br1/> <b>set bridge vlans vlan 2 multicast snooping</b>
admin@example:/config/interface/br1/> <b>leave</b>
admin@example:/> <b>copy running-config startup-config</b>
</code></pre>

Let us see what we get:

<pre class="cli"><code>admin@example:/> <b>show ip multicast</b>
Multicast Overview
Query Interval (default): 125 sec
Router Timeout          : 255
Fast Leave Ports        : e5
Router Ports            : e1, e2, e5, e6, e7
Flood Ports             : e1, e2, e3, e4, e5, e6, e7, e8

<span class="header">Interface       VID  Querier                     State  Interval  Timeout  Ver</span>
br0                  192.168.2.1                    Up       125     None    3
br1               1  0.0.0.0                        Up       125     None    3
br1               2  0.0.0.0                        Up       125     None    3

<span class="header">Bridge          VID  Multicast Group       Ports                              </span>
br0                  224.1.1.1             e2
br0                  ff02::fb              br0
br0                  ff02::6a              br0
br0                  ff02::1:ff00:0        br0
br1               1  224.1.1.1             e5
br1               2  224.1.1.1             e7
br1               1  ff02::fb              br1
br1               1  ff02::1:ff00:0        br1
</code></pre>

In this setup we have a lot more going on.  Multiple multicast router
ports have been detected, and behind the scenes someone has also added
an IGMP/MLD fast-leave port.

##### Terminology & Abbreviations

 - **IGMP**: Internet Group Membership Protocol, multicast subscription
   for IPv4, for details see [RFC3376][]
 - **MLD**: Multicast Listener Discovery (Protocol), multicast
   subscription for IPv6, for details see [RFC3810][]
 - **Unknown/Unregistered multicast**: multicast groups that are *not*
   in the multicast forwarding database (MDB)
 - **Known/Registered multicast**: multicast groups that *are* in the
   multicast forwarding database (MDB)
 - **MDB**: the multicast forwarding database, consists of filters for
   multicast groups, directing where multicast is allowed to egress.  A
   filter entry consists of a group and a port list.  The bridge filters
   with a unique database per VLAN, in the same was as the unicast FDB
 - **Join/Leave**: the terminology used in earlier versions of the two
   protocols to subscribe and unsubscribe to a multicast group.  For
   more information, see *Membership Report*
 - **Membership Report** A membership report is sent by end-devices and
   forwarded by switches to the elected querier on the LAN.  They
   consist of multiple "join" and "leave" operations on groups.  They
   can also, per group, list which senders to allow or block.  Switches
   usually only support the group subscription, and even more common
   also only support filtering on the MAC level[^3]
 - **Querier election**: the process of determining who is the elected
   IGMP/MLD querier on a LAN.  Lowest numerical IP address wins, the
   special address 0.0.0.0 (proxy querier) never wins
 - **Proxy querier**: when no better querier exists on a LAN, one or
   more devices can send proxy queries with source address 0.0.0.0 (or
   :: for IPv6).  See **Query Interval**, below, why this is a good
   thing
 - **Query interval**: the time in seconds between two queries from an
   IGMP/MLD querier.  It is not uncommon that end-devices do not send
   their membership reports unless they first hear a query
 - **Fast Leave**: set on a bridge port to ensure multicast is pruned as
   quickly as possible when a "leave" membership report is received.  In
   effect, this option marks the port as directly connected to an
   end-device.  When not set (default), a query with timeout is first
   sent to ensure no unintentional loss of multicast is incurred
 - **Router port**: can be both configured statically and detected at
   runtime based on connected devices, usually multicast routers.  On
   a router port *all* multicast is forwarded, both known and unknown
 - **Flood port**: set on a bridge port (default: enabled) to ensure
   all *unknown* multicast is forwarded
 - **Router timeout**: the time in seconds until a querier is deemed to
   have been lost and another device (switch/router) takes over.  In the
   tables shown above, a *None* timeout is declared when the current
   device is the active querier

> [!TIP]
> The reason why multicast flooding is enabled by default is to ensure
> safe co-existence with MAC multicast, which is common in industrial
> networks.  It also allows end devices that do not know of IGMP/MLD to
> communicate over multicast as long as the group they have chosen is
> not used by other IGMP/MLD aware devices on the LAN.
>
> As soon as an IGMP/MLD membership report to "join" a group is received
> the group is added to the kernel MDB and forwarding to other ports
> stop.  The only exception to this rule is multicast router ports.
>
> If your MAC multicast forwarding is not working properly, it may be
> because an IP multicast group maps to the same MAC address.  Please
> see [RFC 1112][RFC1112] for details.  Use static multicast router
> ports, or static multicast MAC filters, to mitigate.

[RFC1112]: https://www.rfc-editor.org/rfc/rfc1112.html
[RFC3376]: https://www.rfc-editor.org/rfc/rfc3376.html
[RFC3810]: https://www.rfc-editor.org/rfc/rfc3810.html

#### Forwarding of IEEE Reserved Group Addresses

Addresses in the range `01:80:C2:00:00:0X` are used by various bridge
signaling protocols, and are not forwarded by default.  Still, it is
sometimes useful to let the bridge forward such packets, this can be
done by specifying protocol names or the last address *nibble* as
decimal value `0..15`:

<pre class="cli"><code>admin@example:/config/> <b>edit interface br0 bridge</b>
admin@example:/config/interface/br0/bridge/> <b>set ieee-group-forward</b>     # Tap the ? key for alternatives
  [0..15]  List of IEEE link-local protocols to forward, e.g., STP, LLDP
  dot1x    802.1X Port-Based Network Access Control.
  lacp     802.3 Slow Protocols, e.g., LACP.
  lldp     802.1AB Link Layer Discovery Protocol (LLDP).
  stp      Spanning Tree (STP/RSPT/MSTP).
admin@example:/config/interface/br0/bridge/> <b>set ieee-group-forward</b>
</code></pre>

The following example configures bridge *br0* to forward LLDP packets.

<pre class="cli"><code>admin@example:/config/interface/br0/bridge/> <b>set ieee-group-forward lldp</b>
admin@example:/config/interface/br0/bridge/>
</code></pre>


### Link Aggregation

A link aggregate, or *lag*, allows multiple physical interfaces to be
combined into a single logical interface, providing increased bandwidth
(in some cases) and redundancy (primarily).  Two modes of qualifying lag
member ports are available:

 1. **static**: Active members selected based on link status (carrier)
 2. **lacp:** IEEE 802.3ad Link Aggregation Control Protocol

In LACP mode, LACPDUs are exchanged by the link partners to qualify each
lag member, while in static mode only carrier is used.  This additional
exchange in LACP ensures traffic can be forwarded in both directions.

Traffic distribution, for both modes, across the active lag member ports
is determined by the hash policy[^1].  It uses an XOR of the source,
destination MAC addresses and the EtherType field.  This, IEEE
802.3ad-compliant, algorithm will place all traffic to a particular
network peer on the same link.  Meaning there is no increased bandwidth
for communication between two specific devices.

> [!TIP]
> Similar to other interface types, naming your interface `lagN`, where
> `N` is a number, allows the CLI to automatically infer the interface
> type as LAG.


#### Basic Configuration

Creating a link aggregate interface and adding member ports:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interface lag0</b>
admin@example:/config/interface/lag0/> <b>set lag mode static</b>
admin@example:/config/interface/lag0/> <b>end</b>
admin@example:/config/> <b>set interface eth7 lag-port lag lag0</b>
admin@example:/config/> <b>set interface eth8 lag-port lag lag0</b>
admin@example:/config/> <b>leave</b>
</code></pre>

A static lag responds only to link (carrier) changes of member ports.
E.g., in this example egressing traffic is continuously distributed over
the two links until link down on one link is detected, triggering all
traffic to be steered to the sole remaining link.


#### LACP Configuration

LACP mode provides dynamic negotiation of the link aggregate.  Key
settings include:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interface lag0</b>
admin@example:/config/interface/lag0/> <b>set lag mode lacp</b>
admin@example:/config/interface/lag0/> <b>set lag lacp mode passive</b>
admin@example:/config/interface/lag0/> <b>set lag lacp rate fast</b>
admin@example:/config/interface/lag0/> <b>set lag lacp system-priority 100</b>
</code></pre>

LACP mode supports two operational modes:

 - **active:** Initiates negotiation by sending LACPDUs (default)
 - **passive:** Waits for peer to initiate negotiation

> [!NOTE]
> At least one end of the link must be in active mode for negotiation to occur.

The LACP rate setting controls protocol timing:

 - **slow:** LACPDUs sent every 30 seconds, with 90 second timeout (default)
 - **fast:** LACPDUs sent every second, with 3 second timeout


#### Link Flapping

To protect against link flapping, debounce timers can be configured to
delay link qualification.  Usually only the `up` delay is needed:

<pre class="cli"><code>admin@example:/config/interface/lag0/lag/link-monitor/> <b>edit debounce</b>
admin@example:/config/interface/lag0/lag/link-monitor/debounce/> <b>set up 500</b>
admin@example:/config/interface/lag0/lag/link-monitor/debounce/> <b>set down 200</b>
</code></pre>

#### Operational Status, Overview

Like other interfaces, link aggregates are also available in the general
interfaces overview in the CLI admin-exec context.  Here is the above
static mode aggregate:

<pre class="cli"><code>admin@example:/> <b>show interfaces</b>
<span class="header">INTERFACE       PROTOCOL   STATE       DATA                                    </span>
lo              ethernet   UP          00:00:00:00:00:00
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
.
.
.
lag0            lag        UP          static: balance-xor, hash: layer2
│               ethernet   UP          00:a0:85:00:02:00
├ eth7          lag        ACTIVE
└ eth8          lag        ACTIVE
</code></pre>

Same aggregate, but in LACP mode:

<pre class="cli"><code>admin@example:/> <b>show interfaces</b>
<span class="header">INTERFACE       PROTOCOL   STATE       DATA                                    </span>
lo              ethernet   UP          00:00:00:00:00:00
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
.
.
.
lag0            lag        UP          lacp: active, rate: fast (1s), hash: layer2
│               ethernet   UP          00:a0:85:00:02:00
├ eth7          lag        ACTIVE      active, short_timeout, aggregating, in_sync, collecting, distributing
└ eth8          lag        ACTIVE      active, short_timeout, aggregating, in_sync, collecting, distributing
</code></pre>


#### Operational Status, Detail

In addition to basic status shown in the interface overview, detailed
LAG status can be inspected:

<pre class="cli"><code>admin@example:/> <b>show interface lag0</b>
name                : lag0
index               : 25
mtu                 : 1500
operational status  : up
physical address    : 00:a0:85:00:02:00
lag mode            : static
lag type            : balance-xor
lag hash            : layer2
link debounce up    : 0 msec
link debounce down  : 0 msec
ipv4 addresses      :
ipv6 addresses      :
in-octets           : 0
out-octets          : 2142
</code></pre>

Same aggregate, but in LACP mode:

<pre class="cli"><code>admin@example:/> <b>show interface lag0</b>
name                : lag0
index               : 24
mtu                 : 1500
operational status  : up
physical address    : 00:a0:85:00:02:00
lag mode            : lacp
lag hash            : layer2
lacp mode           : active
lacp rate           : fast (1s)
lacp aggregate id   : 1
lacp system priority: 65535
lacp actor key      : 9
lacp partner key    : 9
lacp partner mac    : 00:a0:85:00:03:00
link debounce up    : 0 msec
link debounce down  : 0 msec
ipv4 addresses      :
ipv6 addresses      :
in-octets           : 100892
out-octets          : 111776
</code></pre>

Member ports provide additional status information:

 - Link failure counter: number of detected link failures
 - LACP state flags: various states of LACP negotiation:
   - `active`: port is actively sending LACPDUs
   - `short_timeout`: using fast rate (1s) vs. slow rate (30s)
   - `aggregating`: port is allowed to aggregate in this LAG
   - `in_sync`: port is synchronized with partner
   - `collecting`: port is allowed to receive traffic
   - `distributing`: port is allowed to send traffic
   - `defaulted`: using default partner info (partner not responding)
   - `expired`: partner info has expired (no LACPDUs received)
 - Aggregator ID: unique identifier for this LAG group
 - Actor state: LACP state flags for this port (local)
 - Partner state: LACP state flags from the remote port

Example member port status:

<pre class="cli"><code>admin@example:/> <b>show interface eth7</b>
name                : eth7
index               : 8
mtu                 : 1500
operational status  : up
physical address    : 00:a0:85:00:02:00
lag member          : lag0
lag member state    : active
lacp aggregate id   : 1
lacp actor state    : active, short_timeout, aggregating, in_sync, collecting, distributing
lacp partner state  : active, short_timeout, aggregating, in_sync, collecting, distributing
link failure count  : 0
ipv4 addresses      :
ipv6 addresses      :
in-octets           : 473244
out-octets          : 499037
</code></pre>


#### Example: Switch Uplink with LACP

LACP mode provides the most robust operation, automatically negotiating
the link aggregate and detecting configuration mismatches.

A common use case is connecting a switch to an upstream device:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interface lag0</b>
admin@example:/config/interface/lag0/> <b>set lag mode lacp</b>
</code></pre>

Enable fast LACP for quicker fail-over:

<pre class="cli"><code>admin@example:/config/interface/lag0/> <b>set lag lacp rate fast</b>
</code></pre>

Add uplink ports

<pre class="cli"><code>admin@example:/config/interface/lag0/> <b>end</b>
admin@example:/config/> <b>set interface eth7 lag-port lag lag0</b>
admin@example:/config/> <b>set interface eth8 lag-port lag lag0</b>
</code></pre>

Enable protection against "link flapping".

<pre class="cli"><code>admin@example:/config/interface/lag0/> <b>edit lag link-monitor</b>
admin@example:/config/interface/lag0/lag/link-monitor/> <b>edit debounce</b>
admin@example:/config/interface/lag0/lag/link-monitor/debounce/> <b>set up 500</b>
admin@example:/config/interface/lag0/lag/link-monitor/debounce/> <b>set down 200</b>
admin@example:/config/interface/lag0/lag/link-monitor/debounce/> <b>top</b>
</code></pre>

Add to bridge for switching

<pre class="cli"><code>admin@example:/config/interface/lag0/lag/link-monitor/debounce/> <b>end</b>
admin@example:/config/> <b>set interface lag0 bridge-port bridge br0</b>
admin@example:/config/> <b>leave</b>
</code></pre>


### VLAN Interfaces

Creating a VLAN can be done in many ways.  This section assumes VLAN
interfaces created atop another Linux interface.  E.g., the VLAN
interfaces created on top of the Ethernet interface or bridge in the
picture below.

![VLAN interface on top of Ethernet or Bridge interfaces](img/interface-vlan-variants.svg)

A VLAN interface is basically a filtering abstraction. When you run
`tcpdump` on a VLAN interface you will only see the frames matching the
VLAN ID of the interface, compared to *all* the VLAN IDs if you run
`tcpdump` on the lower-layer interface.

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interface eth0.20</b>
admin@example:/config/interface/eth0.20/> <b>show</b>
type vlan;
vlan {
  tag-type c-vlan;
  id 20;
  lower-layer-if eth0;
}
admin@example:/config/interface/eth0.20/> <b>leave</b>
</code></pre>

The example below assumes bridge br0 is already created, see [VLAN
Filtering Bridge](#vlan-filtering-bridge).

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interface vlan10</b>
admin@example:/config/interface/vlan10/> <b>set vlan id 10</b>
admin@example:/config/interface/vlan10/> <b>set vlan lower-layer-if br0</b>
admin@example:/config/interface/vlan10/> <b>leave</b>
</code></pre>

As conventions, a VLAN interface for VID 20 on top of an Ethernet
interface *eth0* is named *eth0.20*, and a VLAN interface for VID 10 on
top of a bridge interface *br0* is named *vlan10*.

> [!NOTE]
> If you name your VLAN interface `foo0.N` or `vlanN`, where `N` is a
> number, the CLI infers the interface type automatically.


### Physical Ethernet Interfaces

#### Ethernet Settings and Status

Physical Ethernet interfaces provide low-level settings for speed/duplex as
well as packet status and [statistics](#ethernet-statistics).

By default, Ethernet interfaces defaults to auto-negotiating
speed/duplex modes, advertising all speed and duplex modes available.
In the example below, the switch would by default auto-negotiate speed
1 Gbit/s on port eth1 and 100 Mbit/s on port eth4, as those are the
highest speeds supported by H1 and H2 respectively.

![4-port Gbit/s switch connected to Gbit and Fast Ethernet Hosts](img/ethernet-autoneg.svg)

The speed and duplex status for the links can be listed as shown
below, assuming the link operational status is 'up'. 

<pre class="cli"><code>admin@example:/> <b>show interface eth1</b>
name                : eth1
index               : 2
mtu                 : 1500
operational status  : up
auto-negotiation    : on
duplex              : full
speed               : 1000
physical address    : 00:53:00:06:11:01
ipv4 addresses      :
ipv6 addresses      :
in-octets           : 75581
out-octets          : 43130
...
admin@example:/> <b>show interface eth4</b>
name                : eth4
index               : 5
mtu                 : 1500
operational status  : up
auto-negotiation    : on
duplex              : full
speed               : 100
physical address    : 00:53:00:06:11:04
ipv4 addresses      :
ipv6 addresses      :
in-octets           : 75439
out-octets          : 550704
...
admin@example:/>
</code></pre>

#### Configuring fixed speed and duplex

Auto-negotiation of speed/duplex mode is desired in almost all
use-cases, but it is possible to disable auto-negotiation and specify
a fixed speed and duplex mode.

> [!IMPORTANT]
> When setting a fixed speed and duplex mode, ensure both sides of the
> link have matching configuration.  If speed does not match, the link
> will not come up.  If duplex mode does not match, the result is
> reported collisions and/or bad throughput.

The example below configures port eth3 to fixed speed 100 Mbit/s
half-duplex mode. 

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interface eth3 ethernet</b>
admin@example:/config/interface/eth3/ethernet/> <b>set speed 0.1</b>
admin@example:/config/interface/eth3/ethernet/> <b>set duplex half</b>
admin@example:/config/interface/eth3/ethernet/> <b>set auto-negotiation enable false</b>
admin@example:/config/interface/eth3/ethernet/> <b>show</b>
auto-negotiation {
  enable false;
}
duplex half;
speed 0.1;
admin@example:/config/interface/eth3/ethernet/> <b>leave</b>
admin@example:/>
</code></pre>

Speed metric is in Gbit/s.  Auto-negotiation needs to be disabled in
order for fixed speed/duplex to apply. Only speeds `0.1`(100 Mbit/s)
and `0.01` (10 Mbit/s) can be specified. 1 Gbit/s and higher speeds
require auto-negotiation to be enabled.

#### Ethernet statistics

Ethernet packet statistics[^6] can be listed as shown below.

<pre class="cli"><code>admin@example:/> <b>show interface eth1</b>
name                : eth1
index               : 2
mtu                 : 1500
operational status  : up
auto-negotiation    : on
duplex              : full
speed               : 1000
physical address    : 00:53:00:06:11:0a
ipv4 addresses      :
ipv6 addresses      :
in-octets           : 75581
out-octets          : 43130

eth-in-frames                : 434
eth-in-multicast-frames      : 296
eth-in-broadcast-frames      : 138
eth-in-error-fcs-frames      : 0
eth-in-error-oversize-frames : 0
eth-out-frames               : 310
eth-out-multicast-frames     : 310
eth-out-broadcast-frames     : 0
eth-out-good-octets          : 76821
eth-in-good-octets           : 60598
admin@example:/>
</code></pre>


### VETH Pairs

A Virtual Ethernet (VETH) pair is basically a virtual Ethernet cable.  A
cable can be "plugged in" to a bridge and the other end can be given to
a [container](container.md), or plugged into another bridge.

The latter example is useful if you have multiple bridges in the system
with different properties (VLAN filtering, IEEE group forwarding, etc.),
but still want some way of communicating between these domains.

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interface veth0a</b>
admin@example:/config/interface/veth0a/> <b>set veth peer veth0b</b>
admin@example:/config/interface/veth0a/> <b>end</b>
admin@example:/config/> <b>diff</b>
interfaces {
+  interface veth0a {
+    type veth;
+    veth {
+      peer veth0b;
+    }
+  }
+  interface veth0b {
+    type veth;
+    veth {
+      peer veth0a;
+    }
+  }
}
admin@example:/config/>
</code></pre>

> [!TIP]
> This is another example of the automatic inference of the interface
> type from the name.  Any name can be used, but then you have to set
> the interface type to `veth` manually.


## Management Plane

This section details IP Addresses And Other Per-Interface IP settings.

Infix support several network interface types, each can be assigned one
or more IP addresses, both IPv4 and IPv6 are supported.  (There is no
concept of a "primary" address.)

![IP on top of network interface examples](img/ip-iface-examples.svg)

### IPv4 Address Assignment

Multiple address assignment methods are available:

| **Type**   | **Yang Model**    | **Description**                                                |
|:---------- |:----------------- |:-------------------------------------------------------------- |
| static     | ietf-ip           | Static assignment of IPv4 address, e.g., *10.0.1.1/24*         |
| link-local | infix-ip          | Auto-assignment of IPv4 address in 169.254.x.x/16 range        |
| dhcp       | infix-dhcp-client | Assignment of IPv4 address by DHCP server, e.g., *10.0.1.1/24* |

> [!NOTE]
> The DHCP address method is only available for *LAN* interfaces
> (Ethernet, virtual Ethernet (veth), bridge, link aggregates, etc.)

Supported DHCP (request) options, configurability (Cfg) and defaults,
are listed below.  Configurable options can be disabled on a per client
interface basis, some options, like `clientid` and option 81, are
possible to set the value of as well.

| **Opt** | **Name**                    | **Cfg** | **Description**                                     |
|---------|-----------------------------|---------|-----------------------------------------------------|
| 1       | `netmask`                   | No      | Request IP address and netmask                      |
| 3       | `router`                    | Yes     | Default route(s), see also option 121 and 249       |
| 6       | `dns-server`                | Yes     | DNS server(s), static ones take precedence          |
| 12      | `hostname`                  | Yes     | DHCP cannot set hostname, only for informing server |
| 15      | `domain`                    | Yes     | Default domain name, for name resolution            |
| 28      | `broadcast`                 | Yes     | Broadcast address, calculated if disabled           |
| 42      | `ntp-server`                | Yes     | NTP server(s), static ones take precedence          |
| 50      | `address`                   | Yes     | Request (previously cached) address                 |
| 61      | `client-id`                 | Yes     | Default MAC address (and option 12)                 |
| 81      | `fqdn`                      | Yes     | Similar to option 12, request FQDN update in DNS    |
| 119     | `search`                    | Yes     | Request domain search list                          |
| 121     | `classless-static-route`    | Yes     | Classless static routes                             |
| 249     | `ms-classless-static-route` | Yes     | Microsoft static route, same as option 121          |
|         |                             |         |                                                     |

**Default:** `router`, `dns-server`, `domain`, `broadcast`, `ntp-server`, `search`,
             `address`, `classless-static-route`, `ms-classless-static-route`

When configuring a DHCP client, ensure that the NTP client is enabled
for the `ntp-server` DHCP option to be processed correctly.  If the NTP
client is not enabled, any NTP servers provided by the DHCP server will
be ignored. For details on how to enable the NTP client, see the [NTP
Client Configuration](system.md#ntp-client-configuration) section.

> [!IMPORTANT]
> Per [RFC3442][4], if the DHCP server returns both a Classless Static
> Routes option (121) and Router option (3), the DHCP client *must*
> ignore the latter.


### IPv6 Address Assignment

Multiple address assignment methods are available:

| **Type**         | **Yang Model**       | **Description**                                                                                                                                   |
|:---------------- |:-------------------- |:------------------------------------------------------------------------------------------------------------------------------------------------- |
| static           | ietf-ip              | Static assignment of IPv6 address, e.g., *2001:db8:0:1::1/64*                                                                                     |
| link-local       | ietf-ip[^2]          | (RFC4862) Auto-configured link-local IPv6 address (*fe80::0* prefix + interface identifier, e.g., *fe80::ccd2:82ff:fe52:728b/64*)                 |
| global auto-conf | ietf-ip              | (RFC4862) Auto-configured (stateless) global IPv6 address (prefix from router + interface identifier, e.g., *2001:db8:0:1:ccd2:82ff:fe52:728b/64* |
| dhcp             | infix-dhcpv6-client  | Assignment of IPv6 address by DHCPv6 server, e.g., *2001:db8::42/128*                                                                             |

Both for *link-local* and *global auto-configuration*, it is possible
to auto-configure using a random suffix instead of the interface
identifier.

> [!NOTE]
> The DHCPv6 address method is only available for *LAN* interfaces
> (Ethernet, virtual Ethernet (veth), bridge, link aggregates, etc.)

Supported DHCPv6 (request) options, configurability (Cfg) and defaults,
are listed below.  Configurable options can be disabled on a per client
interface basis, some options, like `client-id` and `client-fqdn`, are
possible to set the value of as well.

| **Opt** | **Name**                   | **Cfg** | **Description**                                        |
|---------|----------------------------|---------|--------------------------------------------------------|
| 1       | `client-id`                | Yes     | Client identifier (DUID), auto-generated by default    |
| 2       | `server-id`                | Yes     | Server identifier (DUID)                               |
| 23      | `dns-server`               | Yes     | DNS recursive name servers, static ones take precedence|
| 24      | `domain-search`            | Yes     | Domain search list                                     |
| 25      | `ia-pd`                    | Yes     | Prefix delegation for downstream networks              |
| 31      | `sntp-server`              | Yes     | Simple Network Time Protocol servers                   |
| 32      | `information-refresh-time` | Yes     | Refresh time for stateless DHCPv6                      |
| 39      | `client-fqdn`              | Yes     | Client FQDN, request DNS update from server            |
| 56      | `ntp-server`               | Yes     | NTP time servers, static ones take precedence          |
|         |                            |         |                                                        |

**Default:** `dns-server`, `domain-search`, `ntp-server`

DHCPv6 supports both **stateful** (address assignment) and **stateless**
(information-only) modes:

- **Stateful DHCPv6**: The server assigns IPv6 addresses to clients. This is
  the default mode when enabling the DHCPv6 client.
- **Stateless DHCPv6**: Used with SLAAC (Stateless Address Autoconfiguration)
  when only configuration information (DNS, NTP, etc.) is needed. Enable with
  the `information-only` setting.

When configuring a DHCPv6 client, ensure that the NTP client is enabled
for the `ntp-server` DHCPv6 option to be processed correctly.  If the
NTP client is not enabled, any NTP servers provided by the DHCPv6 server
will be ignored. For details on how to enable the NTP client, see the
[NTP Client Configuration](system.md#ntp-client-configuration) section.

### Examples

![Switch example (eth0 and lo)](img/ip-address-example-switch.svg)

<pre class="cli"><code>admin@example:/> <b>show interfaces</b>
<span class="header">INTERFACE       PROTOCOL   STATE       DATA                                    </span>
eth0            ethernet   UP          02:00:00:00:00:00
                ipv6                   fe80::ff:fe00:0/64 (link-layer)
lo              ethernet   UP          00:00:00:00:00:00
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
admin@example:/>
</code></pre>

To illustrate IP address configuration, the examples below uses a
switch with a single Ethernet interface (eth0) and a loopback
interface (lo). As shown above, these examples assume *eth0* has an
IPv6 link-local address and *lo* has static IPv4 and IPv6 addresses by
default.

#### Static and link-local IPv4 addresses

![Setting static IPv4 (and link-local IPv4)](img/ip-address-example-ipv4-static.svg)

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interface eth0 ipv4</b>
admin@example:/config/interface/eth0/ipv4/> <b>set address 10.0.1.1 prefix-length 24</b>
admin@example:/config/interface/eth0/ipv4/> <b>set autoconf</b>
admin@example:/config/interface/eth0/ipv4/> <b>diff</b>
+interfaces {
+  interface eth0 {
+    ipv4 {
+      address 10.0.1.1 {
+        prefix-length 24;
+      }
+      autoconf;
+    }
+  }
+}
admin@example:/config/interface/eth0/ipv4/> <b>leave</b>
admin@example:/> <b>show interfaces</b>
<span class="header">INTERFACE       PROTOCOL   STATE       DATA                                    </span>
eth0            ethernet   UP          02:00:00:00:00:00
                ipv4                   169.254.1.3/16 (random)
                ipv4                   10.0.1.1/24 (static)
                ipv6                   fe80::ff:fe00:0/64 (link-layer)
lo              ethernet   UP          00:00:00:00:00:00
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
admin@example:/>
</code></pre>

As shown, the link-local IPv4 address is configured with `set autoconf`.
The presence of the `autoconf` container enables IPv4 link-local address
assignment. The resulting address (169.254.1.3/16) is of type *random*
([ietf-ip.yang][2]).

The IPv4LL client also supports a `request-address` setting which can be
used to "seed" the client's starting address.  If the address is free it
will be used, otherwise it falls back to the default algorithm.

<pre class="cli"><code>admin@example:/config/interface/eth0/ipv4/> <b>edit autoconf</b>
admin@example:/config/interface/eth0/ipv4/autoconf/> <b>set request-address 169.254.1.2</b>
admin@example:/config/interface/eth0/ipv4/autoconf/> <b>leave</b>
</code></pre>


#### Use of DHCP for IPv4 address assignment

![Using DHCP for IPv4 address assignment](img/ip-address-example-ipv4-dhcp.svg)

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interface eth0 ipv4</b>
admin@example:/config/interface/eth0/ipv4/> <b>set dhcp</b>
admin@example:/config/interface/eth0/ipv4/> <b>leave</b>
admin@example:/> <b>show interfaces</b>
<span class="header">INTERFACE       PROTOCOL   STATE       DATA                                    </span>
eth0            ethernet   UP          02:00:00:00:00:00
                ipv4                   10.1.2.100/24 (dhcp)
                ipv6                   fe80::ff:fe00:0/64 (link-layer)
lo              ethernet   UP          00:00:00:00:00:00
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
admin@example:/>
</code></pre>

The resulting address (10.1.2.100/24) is of type *dhcp*.

To configure DHCP client options, such as sending a specific hostname to the
server, you can specify options with values:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interface eth0 ipv4 dhcp</b>
admin@example:/config/interface/eth0/ipv4/dhcp/> <b>set option hostname value myhost</b>
admin@example:/config/interface/eth0/ipv4/dhcp/> <b>show</b>
option hostname {
  value myhost;
}
admin@example:/config/interface/eth0/ipv4/dhcp/> <b>leave</b>
admin@example:/>
</code></pre>

> [!TIP]
> The special value `auto` can be used with the hostname option to
> automatically use the configured system hostname.

Other useful DHCP options include:

- `client-id` - Send a specific client identifier to the server
- `route-preference` - Set the administrative distance for DHCP-learned routes (default: 5)

For advanced usage with vendor-specific options, see the YANG model.

#### Use of DHCPv6 for IPv6 address assignment

![Using DHCPv6 for IPv6 address assignment](img/ip-address-example-ipv6-dhcp.svg)

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interface eth0 ipv6</b>
admin@example:/config/interface/eth0/ipv6/> <b>set dhcp</b>
admin@example:/config/interface/eth0/ipv6/> <b>leave</b>
admin@example:/> <b>show interface</b>
<span class="header">INTERFACE       PROTOCOL   STATE       DATA                                    </span>
eth0            ethernet   UP          02:00:00:00:00:00
                ipv6                   2001:db8::42/128 (dhcp)
                ipv6                   fe80::ff:fe00:0/64 (link-layer)
lo              ethernet   UP          00:00:00:00:00:00
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
admin@example:/>
</code></pre>

The resulting address (2001:db8::42/128) is of type *dhcp*.

To configure DHCPv6 client options, such as requesting prefix delegation
for downstream networks, you can specify options:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interface eth0 ipv6 dhcp</b>
admin@example:/config/interface/eth0/ipv6/dhcp/> <b>set option ia-pd</b>
admin@example:/config/interface/eth0/ipv6/dhcp/> <b>set option dns-server</b>
admin@example:/config/interface/eth0/ipv6/dhcp/> <b>show</b>
option dns-server;
option ia-pd;
admin@example:/config/interface/eth0/ipv6/dhcp/> <b>leave</b>
admin@example:/>
</code></pre>

For stateless DHCPv6 (used with SLAAC to get only configuration information):

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interface eth0 ipv6 dhcp</b>
admin@example:/config/interface/eth0/ipv6/dhcp/> <b>set information-only true</b>
admin@example:/config/interface/eth0/ipv6/dhcp/> <b>show</b>
information-only true;
option dns-server;
option domain-search;
admin@example:/config/interface/eth0/ipv6/dhcp/> <b>leave</b>
admin@example:/>
</code></pre>

Other useful DHCPv6 options include:

- `duid` - Set a specific DHCPv6 Unique Identifier (auto-generated by default)
- `client-fqdn` - Request the server to update DNS records with client's FQDN
- `route-preference` - Set the administrative distance for DHCPv6-learned routes (default: 5)

For advanced usage with vendor-specific options, see the YANG model.

#### Disabling IPv6 link-local address(es)

The (only) way to disable IPv6 link-local addresses is by disabling IPv6
on the interface.

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interface eth0 ipv6</b>
admin@example:/config/interface/eth0/ipv6/> <b>set enabled false</b>
admin@example:/config/interface/eth0/ipv6/> <b>leave</b>
admin@example:/> <b>show interfaces</b>
<span class="header">INTERFACE       PROTOCOL   STATE       DATA                                    </span>
eth0            ethernet   UP          02:00:00:00:00:00
lo              ethernet   UP          00:00:00:00:00:00
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
admin@example:/>
</code></pre>

#### Static IPv6 address

![Setting static IPv6](img/ip-address-example-ipv6-static.svg)

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interface eth0 ipv6</b>
admin@example:/config/interface/eth0/ipv6/> <b>set address 2001:db8::1 prefix-length 64</b>
admin@example:/config/interface/eth0/ipv6/> <b>leave</b>
admin@example:/> <b>show interfaces</b>
<span class="header">INTERFACE       PROTOCOL   STATE       DATA                                    </span>
eth0            ethernet   UP          02:00:00:00:00:00
                ipv6                   2001:db8::1/64 (static)
                ipv6                   fe80::ff:fe00:0/64 (link-layer)
lo              ethernet   UP          00:00:00:00:00:00
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
admin@example:/>
</code></pre>


#### Stateless Auto-configuration of Global IPv6 Address

![Auto-configuration of global IPv6](img/ip-address-example-ipv6-auto-global.svg)

Stateless address auto-configuration of global addresses is enabled by
default. The address is formed by concatenating the network prefix
advertised by the router (here 2001:db8:0:1::0/64) and the interface
identifier.  The resulting address is of type *link-layer*, as it is
formed based on the interface identifier ([ietf-ip.yang][2]).

<pre class="cli"><code>admin@example:/> <b>show interfaces</b>
<span class="header">INTERFACE       PROTOCOL   STATE       DATA                                    </span>
eth0            ethernet   UP          02:00:00:00:00:00
                ipv6                   2001:db8:0:1:0:ff:fe00:0/64 (link-layer)
                ipv6                   fe80::ff:fe00:0/64 (link-layer)
lo              ethernet   UP          00:00:00:00:00:00
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
admin@example:/>
</code></pre>

Disabling auto-configuration of global IPv6 addresses can be done as shown
below.

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interface eth0 ipv6</b>
admin@example:/config/interface/eth0/ipv6/> <b>set autoconf create-global-addresses false</b>
admin@example:/config/interface/eth0/ipv6/> <b>leave</b>
admin@example:/> <b>show interfaces</b>
<span class="header">INTERFACE       PROTOCOL   STATE       DATA                                    </span>
eth0            ethernet   UP          02:00:00:00:00:00
                ipv6                   fe80::ff:fe00:0/64 (link-layer)
lo              ethernet   UP          00:00:00:00:00:00
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
admin@example:/>
</code></pre>


#### Random Link Identifiers for IPv6 Stateless Autoconfiguration

![Auto-configuration of global IPv6](img/ip-address-example-ipv6-auto-global.svg)

By default, the auto-configured link-local and global IPv6 addresses
are formed from a link-identifier based on the MAC address.

<pre class="cli"><code>admin@example:/> <b>show interfaces</b>
<span class="header">INTERFACE       PROTOCOL   STATE       DATA                                    </span>
eth0            ethernet   UP          02:00:00:00:00:00
                ipv6                   2001:db8:0:1:0:ff:fe00:0/64 (link-layer)
                ipv6                   fe80::ff:fe00:0/64 (link-layer)
lo              ethernet   UP          00:00:00:00:00:00
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
admin@example:/>
</code></pre>

To avoid revealing identity information in the IPv6 address, it is
possible to specify use of a random identifier ([ietf-ip.yang][2] and
[RFC8981][3]).

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interface eth0 ipv6</b>
admin@example:/config/interface/eth0/ipv6/> <b>set autoconf create-temporary-addresses true</b>
admin@example:/config/interface/eth0/ipv6/> <b>leave</b>
admin@example:/> <b>show interfaces</b>
<span class="header">INTERFACE       PROTOCOL   STATE       DATA                                    </span>
eth0            ethernet   UP          02:00:00:00:00:00
                ipv6                   2001:db8:0:1:b705:8374:638e:74a8/64 (random)
                ipv6                   fe80::ad3d:b274:885a:9ffb/64 (random)
lo              ethernet   UP          00:00:00:00:00:00
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
admin@example:/>
</code></pre>

Both the link-local address (fe80::) and the global address (2001:)
have changed type to *random*.


### IPv4 forwarding

To be able to route (static or dynamic) on the interface it is
required to enable forwarding. This setting controls if packets
received on this interface can be forwarded.

<pre class="cli"><code>admin@example:/config/> <b>edit interface eth0</b>
admin@example:/config/interface/eth0/> <b>set ipv4 forwarding</b>
admin@example:/config/interface/eth0/> <b>leave</b>
admin@example:/>
</code></pre>


### IPv6 forwarding

Due to how the Linux kernel manages IPv6 forwarding, we can not fully
control it per interface via this setting like how IPv4 works.  Instead,
IPv6 forwarding is globally enabled when at least one interface enable
forwarding, otherwise it is disabled.

The following table shows the system IPv6 features that the `forwarding`
setting control when it is *Enabled* or *Disabled:

| **IPv6 Feature**                         | **Enabled** | **Disabled** |
|:-----------------------------------------|:------------|:-------------|
| IsRouter set in Neighbour Advertisements | Yes         | No           |
| Transmit Router Solicitations            | No          | Yes          |
| Router Advertisements are ignored        | Yes         | Yes          |
| Accept Redirects                         | No          | Yes          |

<pre class="cli"><code>admin@example:/config/> <b>edit interface eth0</b>
admin@example:/config/interface/eth0/> <b>set ipv6 forwarding</b>
admin@example:/config/interface/eth0/> <b>leave</b>
admin@example:/>
</code></pre>


## Routing support

Currently supported YANG models:

| **YANG Model**            | **Description**                 |
|:--------------------------|:--------------------------------|
| ietf-routing              | Base model for all other models |
| ietf-ipv4-unicast-routing | Static IPv4 unicast routing     |
| ietf-ipv6-unicast-routing | Static IPv6 unicast routing     |
| ietf-ospf                 | OSPF routing                    |
| ietf-rip                  | RIP routing                     |
| infix-routing             | Infix deviations and extensions |

The base model, ietf-routing, is where all the other models hook in.  It
is used to set configuration and read operational status (RIB tables) in
the other models.

> [!NOTE]
> The standard IETF routing models allows multiple instances, but Infix
> currently *only support one instance* per routing protocol!  In the
> examples presented here, the instance name `default` is used.


### IPv4 Static routes

The standard IETF model for static routes reside under the `static`
control plane protocol.  For our examples we use the instance name
`default`, you can use any name.

For a route with destination 192.168.200.0/24 via 192.168.1.1:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit routing control-plane-protocol static name default ipv4</b>
admin@example:/config/routing/…/ipv4/> <b>set route 192.168.200.0/24 next-hop next-hop-address 192.168.1.1</b>
admin@example:/config/routing/…/ipv4/> <b>leave</b>
admin@example:/>
</code></pre>

For a "floating" static route with destination 10.0.0.0/16 via a backup
router 192.168.1.1, using the highest possible distance:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit routing control-plane-protocol static name default ipv4</b>
admin@example:/config/routing/…/ipv4/> <b>set route 10.0.0.0/16 next-hop next-hop-address 192.168.1.1 route-preference 254</b>
admin@example:/config/routing/…/ipv4/> <b>leave</b>
admin@example:/>
</code></pre>

> [!TIP]
> Remember to enable [IPv4 forwarding](#ipv4-forwarding) for the
> interfaces you want to route between.


### IPv6 Static routes

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit routing control-plane-protocol static name default ipv6</b>
admin@example:/config/routing/…/ipv6/> <b>set route 2001:db8:3c4d:200::/64 next-hop next-hop-address 2001:db8:3c4d:1::1</b>
admin@example:/config/routing/…/ipv6/> <b>leave</b>
admin@example:/>
</code></pre>


### OSPFv2 Routing

The system supports OSPF dynamic routing for IPv4, i.e., OSPFv2.  To
enable OSPF and set one active interface in area 0:

<pre class="cli"><code>admin@example:/config/> <b>edit routing control-plane-protocol ospfv2 name default ospf</b>
admin@example:/config/routing/…/ospf/> <b>set area 0.0.0.0 interface e0 enabled</b>
admin@example:/config/routing/…/ospf/> <b>leave</b>
admin@example:/>
</code></pre>

> [!TIP]
> Remember to enable [IPv4 forwarding](#ipv4-forwarding) for all the
> interfaces you want to route between.


#### OSPF area types

In addition to *regular* OSPF areas, area types *NSSA* and *Stub* are
also supported.  To configure an NSSA area with summary routes:

<pre class="cli"><code>admin@example:/config/> <b>edit routing control-plane-protocol ospfv2 name default ospf</b>
admin@example:/config/routing/…/ospf/> <b>set area 0.0.0.1 area-type nssa-area</b>
admin@example:/config/routing/…/ospf/> <b>set area 0.0.0.1 summary true</b>
admin@example:/config/routing/…/ospf/> <b>leave</b>
admin@example:/>
</code></pre>


#### Bidirectional Forwarding Detection (BFD)

It is possible to enable BFD per OSPF interface to speed up detection of
link loss.

<pre class="cli"><code>admin@example:/config/> <b>edit routing control-plane-protocol ospfv2 name default ospf</b>
admin@example:/config/routing/…/ospf/> <b>set area 0.0.0.0 interface e0 bfd enabled true</b>
admin@example:/config/routing/…/ospf/> <b>leave</b>
admin@example:/>
</code></pre>


#### OSPF interface settings

We have already seen how to enable OSPF per interface (*enabled true*)
and BFD for OSPF per interface (*bfd enabled true*).  These and other
OSPF interface settings are done in context of an OSFP area, e.g., *area
0.0.0.0*.  Available commands can be listed using the `?` mark.

<pre class="cli"><code>admin@example:/config/routing/…/> <b>edit ospf area 0.0.0.0</b>
admin@example:/config/routing/…/ospf/area/0.0.0.0/> <b>edit interface e0</b>
admin@example:/config/routing/…/ospf/area/0.0.0.0/interface/e0/> <b>set ?</b>
  bfd                  BFD interface configuration.
  cost                 Interface's cost.
  dead-interval        Interval after which a neighbor is declared down
  enabled              Enables/disables the OSPF protocol on the interface.
  hello-interval       Interval between Hello packets (seconds).  It must
  interface-type       Interface type.
  passive              Enables/disables a passive interface.  A passive
  retransmit-interval  Interval between retransmitting unacknowledged Link
  transmit-delay       Estimated time needed to transmit Link State Update
admin@example:/config/routing/…/ospf/area/0.0.0.0/interface/e0/> set
</code></pre>

For example, setting the OSPF *interface type* to *point-to-point* for
an Ethernet interface can be done as follows.

<pre class="cli"><code>admin@example:/config/routing/…/ospf/area/0.0.0.0/interface/e0/> <b>set interface-type point-to-point</b>
admin@example:/config/routing/…/ospf/area/0.0.0.0/interface/e0/>
</code></pre>

#### OSPF global settings

In addition to *area* and *interface* specific settings, OSPF provides
global settings for route redistribution and OSPF router identifier.

<pre class="cli"><code>admin@example:/config/> <b>edit routing control-plane-protocol ospfv2 name default ospf</b>
admin@example:/config/routing/…/ospf/> <b>set ?</b>
  area                     List of OSPF areas.
  default-route-advertise  Distribute default route to network
  explicit-router-id       Defined in RFC 2328.  A 32-bit number
  redistribute             Redistribute protocols into OSPF
admin@example:/config/routing/…/ospf/> set
</code></pre>

- Explicit router ID: By default the router will pick an IP address
  from one of its OSPF interfaces as OSPF router ID. An explicit ID is
  used to get a deterministic behavior, e.g., `set explicit-router-id
  1.1.1.1`.
- Redistribution: `set redistribute static` and `set redistribute connected`
  can be used to include static or connected routes into the OSPF routing
  domain. These routes are redistributed as *external type-2* (E2)
  routes.
- Advertising default route: An OSPF router can be made to distribute
  a default route into the OSPF domain by command `set
  default-route-advertise enabled`. This route is distributed as long
  as the router itself has an *active* default route in its routing
  table. By adding command `set default-route-advertise always` the
  router will distribute a default route even when it lacks a default
  route. The default route will be distributed as an *external type-2*
  (E2) route.


#### Debug OSPFv2

Using NETCONF and the YANG model *ietf-routing* it is possible to read
the OSPF routing table, neighbors and more, that may be useful for
debugging the OSPFv2 setup. The CLI has various OSPF status commands
such as `show ospf neighbor`, `show ospf interface` and `show ospf
routes`.

<pre class="cli"><code>admin@example:/> <b>show ospf neighbor</b>
<span class="header">Neighbor ID     Pri State           Up Time         Dead Time Address         Interface                        RXmtL RqstL DBsmL</span>
10.1.1.2          1 Full/-          3h46m59s          30.177s 10.1.1.2        e0:10.1.1.1                          0     0     0
10.1.1.3          1 Full/-          3h46m55s          34.665s 10.1.1.3        e1:10.1.1.1                          0     0     0
admin@example:/>
</code></pre>

For more detailed troubleshooting, OSPF debug logging can be enabled to
capture specific protocol events. Debug messages are written to the
routing log file (`/var/log/routing`).

> [!CAUTION]
> Debug logging significantly increases log output and may impact
> performance. Only enable debug categories needed for troubleshooting,
> and disable them when done.

To enable specific OSPF debug categories:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit routing control-plane-protocol ospfv2 name default ospf debug</b>
admin@example:/config/routing/…/ospf/debug/> <b>set bfd true</b>
admin@example:/config/routing/…/ospf/debug/> <b>set nsm true</b>
admin@example:/config/routing/…/ospf/debug/> <b>leave</b>
admin@example:/>
</code></pre>

Available debug categories include:

- `bfd`: BFD (Bidirectional Forwarding Detection) events
- `packet`: Detailed packet debugging (all OSPF packets)
- `ism`: Interface State Machine events
- `nsm`: Neighbor State Machine events
- `default-information`: Default route origination
- `nssa`: Not-So-Stubby Area events

All debug options are disabled by default. Refer to the `infix-routing`
YANG model for the complete list of available debug options.

To view current debug settings:

<pre class="cli"><code>admin@example:/> <b>show running-config routing control-plane-protocol</b>
</code></pre>

To disable all debug logging, simply delete the debug settings or set
all options back to `false`:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>delete routing control-plane-protocol ospfv2 name default ospf debug</b>
admin@example:/config/> <b>leave</b>
admin@example:/>
</code></pre>


### RIP Routing

The system supports RIP dynamic routing for IPv4, i.e., RIPv2.  To enable
RIP and set active interfaces:

<pre class="cli"><code>admin@example:/config/> <b>edit routing control-plane-protocol ripv2 name default rip</b>
admin@example:/config/routing/…/rip/> <b>set interfaces interface e0</b>
admin@example:/config/routing/…/rip/> <b>set interfaces interface e1</b>
admin@example:/config/routing/…/rip/> <b>leave</b>
admin@example:/>
</code></pre>

> [!TIP]
> Remember to enable [IPv4 forwarding](#ipv4-forwarding) for all the
> interfaces you want to route between.


#### RIP interface settings

By default, interfaces send and receive RIPv2 packets.  To control the
RIP version per interface:

<pre class="cli"><code>admin@example:/config/routing/…/rip/> <b>edit interfaces interface e0</b>
admin@example:/config/routing/…/rip/interfaces/interface/e0/> <b>set send-version 1</b>
admin@example:/config/routing/…/rip/interfaces/interface/e0/> <b>set receive-version 1-2</b>
admin@example:/config/routing/…/rip/interfaces/interface/e0/> <b>leave</b>
admin@example:/>
</code></pre>

Valid version values are `1`, `2`, or `1-2` (both versions).

To configure a passive interface (advertise network but don't send/receive
RIP updates):

<pre class="cli"><code>admin@example:/config/routing/…/rip/> <b>edit interfaces interface e0</b>
admin@example:/config/routing/…/rip/interfaces/interface/e0/> <b>set passive</b>
admin@example:/config/routing/…/rip/interfaces/interface/e0/> <b>leave</b>
admin@example:/>
</code></pre>


#### RIP global settings

RIP supports redistribution of connected and static routes:

<pre class="cli"><code>admin@example:/config/routing/…/rip/> <b>set redistribute connected</b>
admin@example:/config/routing/…/rip/> <b>set redistribute static</b>
admin@example:/config/routing/…/rip/> <b>leave</b>
admin@example:/>
</code></pre>


#### Debug RIPv2

The CLI provides various RIP status commands:

<pre class="cli"><code>admin@example:/> <b>show ip rip</b>
Default version control: send version 2, receive version 2
<span class="header">  Interface        Send  Recv   Key-chain                    </span>
  e0               2     2
  e1               2     2

Routing for Networks:
  e0
  e1

Routing Information Sources:
<span class="header">  Gateway          BadPackets BadRoutes  Distance Last Update</span>
  10.0.1.2                  0         0       120    00:00:16
Distance: (default is 120)

admin@example:/> <b>show ip rip neighbor</b>
<span class="header">ADDRESS          BAD-PACKETS    BAD-ROUTES                   </span>
10.0.1.2         0              0
admin@example:/>
</code></pre>

For more detailed troubleshooting, RIP debug logging can be enabled to
capture specific protocol events. Debug messages are written to the
routing log file (`/var/log/routing`).

> [!CAUTION]
> Debug logging significantly increases log output and may impact
> performance. Only enable debug categories needed for troubleshooting,
> and disable them when done.

To enable specific RIP debug categories:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit routing control-plane-protocol ripv2 name default rip debug</b>
admin@example:/config/routing/…/rip/debug/> <b>set events true</b>
admin@example:/config/routing/…/rip/debug/> <b>set packet true</b>
admin@example:/config/routing/…/rip/debug/> <b>leave</b>
admin@example:/>
</code></pre>

Available debug categories include:

- `events`: RIP events (sending/receiving packets, timers, interface changes)
- `packet`: Detailed packet debugging (packet dumps with origin and port)
- `kernel`: Kernel routing table updates (route add/delete, interface updates)

All debug options are disabled by default. Refer to the `infix-routing`
YANG model for the complete list of available debug options.

To view current debug settings:

<pre class="cli"><code>admin@example:/> <b>show running-config routing control-plane-protocol</b>
</code></pre>

To disable all debug logging, simply delete the debug settings or set
all options back to `false`:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>delete routing control-plane-protocol ripv2 name default rip debug</b>
admin@example:/config/> <b>leave</b>
admin@example:/>
</code></pre>


### View routing table

The routing table can be inspected from the operational datastore, XPath
`/routing/ribs`, using sysrepocfg, NETCONF/RESTCONF, or using the CLI.


#### IPv4 routing table

This CLI example shows the IPv4 routing table with a few connected
routes and some routes learned from OSPF.  See the next section for
an explanation of route preferences (PREF).

The `>` at the start of a line marks a selected route (in the IETF YANG
model referred to as *active*), if there are more than one route with
the same destination the `*` marks the next-hop used and installed in
the kernel FIB (the YANG model refers to this as *installed*).

<pre class="cli"><code>admin@example:/> <b>show ip route</b>
<span class="header">   DESTINATION            PREF NEXT-HOP         PROTO     UPTIME</span>
>* 0.0.0.0/0             110/2 10.0.23.1        ospfv2   4h2m43s
>* 10.0.0.1/32        110/4000 10.0.13.1        ospfv2   4h2m43s
   10.0.0.3/32           110/0 lo               ospfv2   4h2m57s
>* 10.0.0.3/32             0/0 lo               direct   4h2m58s
   10.0.13.0/30       110/2000 e5               ospfv2   4h2m57s
>* 10.0.13.0/30            0/0 e5               direct   4h2m58s
   10.0.23.0/30          110/1 e6               ospfv2   4h2m57s
>* 10.0.23.0/30            0/0 e6               direct   4h2m58s
   192.168.3.0/24        110/1 e2               ospfv2   4h2m57s
>* 192.168.3.0/24          0/0 e2               direct   4h2m58s
admin@example:/>
</code></pre>


#### IPv6 routing table

This CLI example show the IPv6 routing table.

<pre class="cli"><code>admin@example:/> <b>show ipv6 route</b>
<span class="header">   DESTINATION                      PREF NEXT-HOP              PROTO     UPTIME</span>
>* ::/0                              1/0 2001:db8:3c4d:50::1   static   0h1m20s
>* 2001:db8:3c4d:50::/64             0/0 e6                    direct   0h1m20s
>* 2001:db8:3c4d:200::1/128          0/0 lo                    direct   0h1m20s
 * fe80::/64                         0/0 e7                    direct   0h1m20s
 * fe80::/64                         0/0 e6                    direct   0h1m20s
 * fe80::/64                         0/0 e5                    direct   0h1m20s
 * fe80::/64                         0/0 e4                    direct   0h1m20s
 * fe80::/64                         0/0 e3                    direct   0h1m20s
 * fe80::/64                         0/0 e2                    direct   0h1m20s
>* fe80::/64                         0/0 e1                    direct   0h1m20s
admin@example:/>
</code></pre>


#### Route Preference

The operating system leverages FRRouting ([Frr][0]) as routing engine
for both static and dynamic routing.  Even routes injected from a DHCP
client, and IPv4 link-local (IPv4) routes, are injected into Frr to let
it weigh all routes before installing them into the kernel routing table
(sometimes referred to as FIB).

Routes have different weights made up from a *distance* and a *metric*.
The kernel routing table only talks about *metric*, which unfortunately
is **not the same** -- this is one of the reasons why the term *route
preference* is used instead.  It is recommended to use the CLI, or any
of the other previously mentioned YANG based front-ends, to inspect the
routing table.

Default distances used (lower numeric value wins):

| **Distance** | **Protocol**                            |
|--------------|-----------------------------------------|
| 0            | Kernel routes, i.e., connected routes   |
| 1            | Static routes                           |
| 5            | DHCP routes                             |
| 110          | OSPF                                    |
| 254          | IPv4LL (ZeroConf) device routes         |
| 255          | Route will not be used or redistributed |

Hence, a route learned from OSPF may be overridden by a static route set
locally.  By default, even a route to the same destination, but with a
different next-hop, learned from a DHCP server wins over an OSPF route.

The distance used for static routes and DHCP routes can be changed by
setting a different *routing preference* value.

> [!NOTE]
> The kernel metric is an unsigned 32-bit value, which is read by Frr as
> (upper) 8 bits distance and 24 bits metric.  But it does not write it
> back to the kernel FIB this way, only selected routes are candidates
> to be installed in the FIB by Frr.


#### Source protocol

The source protocol describes the origin of the route.

| **Protocol** | **Description**                                     |
|:-------------|:----------------------------------------------------|
| kernel       | Added when setting a subnet address on an interface |
| static       | User created, learned from DHCP, or IPv4LL          |
| ospfv2       | Routes learned from OSPFv2                          |

The YANG model *ietf-routing* support multiple ribs but only two are
currently supported, namely `ipv4` and `ipv6`.


[1]: https://www.rfc-editor.org/rfc/rfc8343
[2]: https://www.rfc-editor.org/rfc/rfc8344
[3]: https://www.rfc-editor.org/rfc/rfc8981
[4]: https://www.rfc-editor.org/rfc/rfc3442
[0]: https://frrouting.org/

[^1]: `(source MAC XOR destination MAC XOR EtherType) MODULO num_links`
[^2]: Link-local IPv6 addresses are implicitly enabled when enabling
    IPv6.  IPv6 can be enabled/disabled per interface in the
    [ietf-ip][2] YANG model.
[^3]: For example, IPv4 groups are mapped to MAC multicast addresses by
    mapping the low-order 23-bits of the IP address in the low-order 23
    bits of the Ethernet address 01:00:5E:00:00:00.  Meaning, more than
    one IP multicast group maps to the same MAC multicast group.
[^4]: A YANG deviation was previously used to make it possible to set
    `phys-address`, but this has been replaced with the more flexible
    `custom-phys-address`.
[^5]: MAC bridges on Marvell Linkstreet devices are currently limited to
    a single MAC database, this may be a problem if the same MAC address
    appears in different MAC bridges.
[^6]: Ethernet counters are described in *ieee802-ethernet-interface.yang*
    and *infix-ethernet-interface.yang*.  There is a dedicated document on
    [Ethernet Counters](eth-counters.md) that provide additional details
    on the statistics support.
