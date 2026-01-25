# Ethernet Interfaces

This document covers VLAN interfaces, physical Ethernet interfaces,
and virtual Ethernet (VETH) pairs.


## VLAN Interfaces

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
Filtering Bridge](bridging.md#vlan-filtering-bridge).

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


## Physical Ethernet Interfaces

### Ethernet Settings and Status

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

### Configuring fixed speed and duplex

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

### Ethernet statistics

Ethernet packet statistics[^1] can be listed as shown below.

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


## VETH Pairs

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

[^1]: Ethernet counters are described in *ieee802-ethernet-interface.yang*
    and *infix-ethernet-interface.yang*.  There is a dedicated document on
    [Ethernet Counters](eth-counters.md) that provide additional details
    on the statistics support.
