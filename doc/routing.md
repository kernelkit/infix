# Routing

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


## IPv4 Static routes

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
> Remember to enable [IPv4 forwarding](ip.md#ipv4-forwarding) for the
> interfaces you want to route between.


## IPv6 Static routes

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit routing control-plane-protocol static name default ipv6</b>
admin@example:/config/routing/…/ipv6/> <b>set route 2001:db8:3c4d:200::/64 next-hop next-hop-address 2001:db8:3c4d:1::1</b>
admin@example:/config/routing/…/ipv6/> <b>leave</b>
admin@example:/>
</code></pre>


## OSPFv2 Routing

The system supports OSPF dynamic routing for IPv4, i.e., OSPFv2.  To
enable OSPF and set one active interface in area 0:

<pre class="cli"><code>admin@example:/config/> <b>edit routing control-plane-protocol ospfv2 name default ospf</b>
admin@example:/config/routing/…/ospf/> <b>set area 0.0.0.0 interface e0 enabled</b>
admin@example:/config/routing/…/ospf/> <b>leave</b>
admin@example:/>
</code></pre>

> [!TIP]
> Remember to enable [IPv4 forwarding](ip.md#ipv4-forwarding) for all the
> interfaces you want to route between.


### OSPF area types

In addition to *regular* OSPF areas, area types *NSSA* and *Stub* are
also supported.  To configure an NSSA area with summary routes:

<pre class="cli"><code>admin@example:/config/> <b>edit routing control-plane-protocol ospfv2 name default ospf</b>
admin@example:/config/routing/…/ospf/> <b>set area 0.0.0.1 area-type nssa-area</b>
admin@example:/config/routing/…/ospf/> <b>set area 0.0.0.1 summary true</b>
admin@example:/config/routing/…/ospf/> <b>leave</b>
admin@example:/>
</code></pre>


### Bidirectional Forwarding Detection (BFD)

It is possible to enable BFD per OSPF interface to speed up detection of
link loss.

<pre class="cli"><code>admin@example:/config/> <b>edit routing control-plane-protocol ospfv2 name default ospf</b>
admin@example:/config/routing/…/ospf/> <b>set area 0.0.0.0 interface e0 bfd enabled true</b>
admin@example:/config/routing/…/ospf/> <b>leave</b>
admin@example:/>
</code></pre>


### OSPF interface settings

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

### OSPF global settings

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


### Debug OSPFv2

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


## RIP Routing

The system supports RIP dynamic routing for IPv4, i.e., RIPv2.  To enable
RIP and set active interfaces:

<pre class="cli"><code>admin@example:/config/> <b>edit routing control-plane-protocol ripv2 name default rip</b>
admin@example:/config/routing/…/rip/> <b>set interfaces interface e0</b>
admin@example:/config/routing/…/rip/> <b>set interfaces interface e1</b>
admin@example:/config/routing/…/rip/> <b>leave</b>
admin@example:/>
</code></pre>

> [!TIP]
> Remember to enable [IPv4 forwarding](ip.md#ipv4-forwarding) for all the
> interfaces you want to route between.


### RIP interface settings

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


### RIP global settings

RIP supports redistribution of connected and static routes:

<pre class="cli"><code>admin@example:/config/routing/…/rip/> <b>set redistribute connected</b>
admin@example:/config/routing/…/rip/> <b>set redistribute static</b>
admin@example:/config/routing/…/rip/> <b>leave</b>
admin@example:/>
</code></pre>


### Debug RIPv2

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


## View routing table

The routing table can be inspected from the operational datastore, XPath
`/routing/ribs`, using sysrepocfg, NETCONF/RESTCONF, or using the CLI.


### IPv4 routing table

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


### IPv6 routing table

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


### Route Preference

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


### Source protocol

The source protocol describes the origin of the route.

| **Protocol** | **Description**                                     |
|:-------------|:----------------------------------------------------|
| kernel       | Added when setting a subnet address on an interface |
| static       | User created, learned from DHCP, or IPv4LL          |
| ospfv2       | Routes learned from OSPFv2                          |

The YANG model *ietf-routing* support multiple ribs but only two are
currently supported, namely `ipv4` and `ipv6`.

[0]: https://frrouting.org/
