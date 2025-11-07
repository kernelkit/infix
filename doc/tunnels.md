# Tunnel Configuration

Infix supports multiple tunnel encapsulation protocols for connecting
remote networks or devices across an IP backbone.  Tunnels encapsulate
packets within IP datagrams, allowing traffic to traverse intermediate
networks transparently.

> [!IMPORTANT]
> When issuing `leave` to activate your changes, remember to also save
> your settings, `copy running-config startup-config`.  See the [CLI
> Introduction](cli/introduction.md) for a background.

## Generic Routing Encapsulation (GRE)

GRE tunnels provide a simple and efficient method to encapsulate various
network layer protocols over IP networks.  Infix supports both IPv4 and
IPv6 tunnels in two modes:

- **GRE (Layer 3):** Point-to-point IP tunnel for routing protocols and
  routed traffic
- **GRETAP (Layer 2):** Ethernet tunnel for bridging Layer 2 networks

> [!TIP]
> If you name your tunnel interface `greN` or `gretapN`, where `N` is a
> number, the CLI infers the interface type automatically.

### Basic GRE Configuration

A basic GRE tunnel for routing between two sites:

```
admin@example:/> configure
admin@example:/config/> edit interface gre0
admin@example:/config/interface/gre0/> set gre local 192.168.3.1 remote 192.168.3.2
admin@example:/config/interface/gre0/> set ipv4 address 10.255.0.1 prefix-length 30
admin@example:/config/interface/gre0/> leave
admin@example:/>
```

This creates a Layer 3 tunnel between 192.168.3.1 and 192.168.3.2 using
the outer IP addresses, with the tunnel itself using 10.255.0.0/30 for
the inner IP addressing.

### GRETAP Configuration

GRETAP tunnels operate at Layer 2, allowing bridging across the tunnel:

```
admin@example:/> configure
admin@example:/config/> edit interface gretap0
admin@example:/config/interface/gretap0/> set type gretap
admin@example:/config/interface/gretap0/> set gre local 192.168.3.1 remote 192.168.3.2
admin@example:/config/interface/gretap0/> leave
admin@example:/>
```

GRETAP interfaces can be added to a bridge, bridging local and remote Ethernet
segments.  See the [Bridge Configuration](networking.md#bridge-configuration)
for more on bridges.

### OSPF over GRE

GRE tunnels are commonly used to carry dynamic routing protocols like
OSPF across networks that don't support multicast or where you want to
create a virtual topology different from the physical network.

Example topology: Two sites connected via a GRE tunnel, running OSPF to
exchange routes.

**Site A configuration:**

```
admin@siteA:/> configure
admin@siteA:/config/> edit interface gre0
admin@siteA:/config/interface/gre0/> set gre local 203.0.113.1 remote 203.0.113.2
admin@siteA:/config/interface/gre0/> set ipv4 address 10.255.0.1 prefix-length 30
admin@siteA:/config/interface/gre0/> set ipv4 forwarding
admin@siteA:/config/interface/gre0/> end
admin@siteA:/config/> edit routing control-plane-protocol ospfv2 name default ospf
admin@siteA:/config/routing/…/ospf/> set area 0.0.0.0 interface gre0
admin@siteA:/config/routing/…/ospf/> leave
admin@siteA:/>
```

**Site B configuration:**

```
admin@siteB:/> configure
admin@siteB:/config/> edit interface gre0
admin@siteB:/config/interface/gre0/> set gre local 203.0.113.2 remote 203.0.113.1
admin@siteB:/config/interface/gre0/> set ipv4 address 10.255.0.2 prefix-length 30
admin@siteB:/config/interface/gre0/> set ipv4 forwarding
admin@siteB:/config/interface/gre0/> end
admin@siteB:/config/> edit routing control-plane-protocol ospfv2 name default ospf
admin@siteB:/config/routing/…/ospf/> set area 0.0.0.0 interface gre0
admin@siteB:/config/routing/…/ospf/> leave
admin@siteB:/>
```

Once configured, OSPF will establish a neighbor relationship through the
tunnel and exchange routes between the sites.  For more on OSPF
configuration, see [Routing Configuration](routing.md).

> [!NOTE]
> Consider adjusting MTU on the tunnel interface to account for GRE
> overhead (typically 24 bytes for IPv4, 44 bytes for IPv6) to avoid
> fragmentation issues.


### Advanced Tunnel Settings

All tunnel types support common parameters for controlling tunnel behavior
and performance.

#### Time To Live (TTL)

The TTL setting controls the Time To Live value for the outer tunnel packets.
By default, tunnels use a fixed TTL of 64, which allows packets to traverse
multiple hops between tunnel endpoints.

```
admin@example:/config/> edit interface gre0
admin@example:/config/interface/gre0/> set gre ttl 255
admin@example:/config/interface/gre0/> leave
```

Valid values are 1-255, or the special value `inherit` which copies the TTL
from the encapsulated packet.

> [!IMPORTANT]
> The `inherit` mode can cause problems with routing protocols like OSPF
> that use TTL=1 for their packets.  For tunnels carrying routing protocols,
> always use a fixed TTL value (typically 64 or 255).

#### Type of Service (ToS)

The ToS setting controls QoS marking for tunnel traffic:

```
admin@example:/config/> edit interface gre0
admin@example:/config/interface/gre0/> set gre tos 0x10
admin@example:/config/interface/gre0/> leave
```

Valid values are 0-255 for fixed ToS/DSCP marking, or `inherit` (default)
to copy the ToS value from the encapsulated packet.

#### Path MTU Discovery (GRE only)

The `pmtu-discovery` setting can be used to control the Path MTU Discovery on
GRE tunnels.  When enabled (default), the tunnel respects the Don't Fragment
(DF) bit and performs PMTU discovery:

```
admin@example:/config/> edit interface gre0
admin@example:/config/interface/gre0/> set gre pmtudisc false
admin@example:/config/interface/gre0/> leave
```

Disabling PMTU discovery may be necessary in networks with broken ICMP
filtering but can lead to suboptimal performance and fragmentation.

## Virtual eXtensible Local Area Network (VXLAN)

VXLAN is a network virtualization technology that encapsulates Layer 2
Ethernet frames within Layer 4 UDP datagrams.  It uses a 24-bit segment
ID, termed VXLAN Network Identifier (VNI), allowing up to 16 million
isolated networks.

Infix supports both IPv4 and IPv6 for VXLAN tunnel endpoints.

### Basic VXLAN Configuration

> [!TIP]
> If you name your VXLAN interface `vxlanN`, where `N` is a number, the
> CLI infers the interface type automatically.

```
admin@example:/> configure
admin@example:/config/> edit interface vxlan100
admin@example:/config/interface/vxlan100/> set vxlan local 192.168.3.1
admin@example:/config/interface/vxlan100/> set vxlan remote 192.168.3.2
admin@example:/config/interface/vxlan100/> set vxlan vni 100
admin@example:/config/interface/vxlan100/> leave
admin@example:/>
```

The VNI uniquely identifies the VXLAN segment and must match on both
tunnel endpoints.

### VXLAN with Custom UDP Port

The default VXLAN UDP destination port is 4789 (IANA assigned).  In some
cases you may need to use a different port:

```
admin@example:/> configure
admin@example:/config/> edit interface vxlan100
admin@example:/config/interface/vxlan100/> set vxlan local 192.168.3.1
admin@example:/config/interface/vxlan100/> set vxlan remote 192.168.3.2
admin@example:/config/interface/vxlan100/> set vxlan vni 100
admin@example:/config/interface/vxlan100/> set vxlan remote-port 8472
admin@example:/config/interface/vxlan100/> leave
admin@example:/>
```

The remote-port setting allows interoperability with systems using
non-standard VXLAN ports.

> [!NOTE]
> VXLAN tunnels also support the `ttl` and `tos` settings described in
> the [Advanced Tunnel Settings](#advanced-tunnel-settings) section above.
