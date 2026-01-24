# Bridging

This is the most central part of the system.  A bridge is a switch, and
a switch is a bridge.  In Linux, setting up a bridge with ports
connected to physical switch fabric, means you manage the actual switch
fabric!

## MAC Bridge

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
currently[^1] _not recommended_ to use more than one MAC bridge on
products with Marvell LinkStreet switching ASICs. A VLAN filtering
bridge should be used instead.

## VLAN Filtering Bridge

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
on top of the bridge, see section [VLAN Interfaces](ethernet.md#vlan-interfaces)
for more on this topic.

> [!NOTE]
> In some use-cases only a single management VLAN on the bridge is used.
> For the example above, if the bridge itself is an untagged member only
> in VLAN 10, IP addresses can be set directly on the bridge without the
> need for dedicated VLAN interfaces on top of the bridge.


## Multicast Filtering and Snooping

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

### Terminology & Abbreviations

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
   also only support filtering on the MAC level[^2]
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

## Forwarding of IEEE Reserved Group Addresses

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

[^1]: MAC bridges on Marvell Linkstreet devices are currently limited to
    a single MAC database, this may be a problem if the same MAC address
    appears in different MAC bridges.
[^2]: For example, IPv4 groups are mapped to MAC multicast addresses by
    mapping the low-order 23-bits of the IP address in the low-order 23
    bits of the Ethernet address 01:00:5E:00:00:00.  Meaning, more than
    one IP multicast group maps to the same MAC multicast group.
