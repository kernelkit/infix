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

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interface gre0</b>
admin@example:/config/interface/gre0/> <b>set gre local 192.168.3.1 remote 192.168.3.2</b>
admin@example:/config/interface/gre0/> <b>set ipv4 address 10.255.0.1 prefix-length 30</b>
admin@example:/config/interface/gre0/> <b>leave</b>
admin@example:/>
</code></pre>

This creates a Layer 3 tunnel between 192.168.3.1 and 192.168.3.2 using
the outer IP addresses, with the tunnel itself using 10.255.0.0/30 for
the inner IP addressing.

### GRETAP Configuration

GRETAP tunnels operate at Layer 2, allowing bridging across the tunnel:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interface gretap0</b>
admin@example:/config/interface/gretap0/> <b>set type gretap</b>
admin@example:/config/interface/gretap0/> <b>set gre local 192.168.3.1 remote 192.168.3.2</b>
admin@example:/config/interface/gretap0/> <b>leave</b>
admin@example:/>
</code></pre>

GRETAP interfaces can be added to a bridge, bridging local and remote Ethernet
segments.  See the [Bridge Configuration](bridging.md)
for more on bridges.

### OSPF over GRE

GRE tunnels are commonly used to carry dynamic routing protocols like
OSPF across networks that don't support multicast or where you want to
create a virtual topology different from the physical network.

Example topology: Two sites connected via a GRE tunnel, running OSPF to
exchange routes.

**Site A configuration:**

<pre class="cli"><code>admin@siteA:/> <b>configure</b>
admin@siteA:/config/> <b>edit interface gre0</b>
admin@siteA:/config/interface/gre0/> <b>set gre local 203.0.113.1 remote 203.0.113.2</b>
admin@siteA:/config/interface/gre0/> <b>set ipv4 address 10.255.0.1 prefix-length 30</b>
admin@siteA:/config/interface/gre0/> <b>set ipv4 forwarding</b>
admin@siteA:/config/interface/gre0/> <b>end</b>
admin@siteA:/config/> <b>edit routing control-plane-protocol ospfv2 name default ospf</b>
admin@siteA:/config/routing/…/ospf/> <b>set area 0.0.0.0 interface gre0</b>
admin@siteA:/config/routing/…/ospf/> <b>leave</b>
admin@siteA:/>
</code></pre>

**Site B configuration:**

<pre class="cli"><code>admin@siteB:/> <b>configure</b>
admin@siteB:/config/> <b>edit interface gre0</b>
admin@siteB:/config/interface/gre0/> <b>set gre local 203.0.113.2 remote 203.0.113.1</b>
admin@siteB:/config/interface/gre0/> <b>set ipv4 address 10.255.0.2 prefix-length 30</b>
admin@siteB:/config/interface/gre0/> <b>set ipv4 forwarding</b>
admin@siteB:/config/interface/gre0/> <b>end</b>
admin@siteB:/config/> <b>edit routing control-plane-protocol ospfv2 name default ospf</b>
admin@siteB:/config/routing/…/ospf/> <b>set area 0.0.0.0 interface gre0</b>
admin@siteB:/config/routing/…/ospf/> <b>leave</b>
admin@siteB:/>
</code></pre>

Once configured, OSPF will establish a neighbor relationship through the
tunnel and exchange routes between the sites.  For more info on OSPF
configuration, see [OSPFv2 Routing](routing.md#ospfv2-routing).

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

<pre class="cli"><code>admin@example:/config/> <b>edit interface gre0</b>
admin@example:/config/interface/gre0/> <b>set gre ttl 255</b>
admin@example:/config/interface/gre0/> <b>leave</b>
</code></pre>

Valid values are 1-255, or the special value `inherit` which copies the TTL
from the encapsulated packet.

> [!IMPORTANT]
> The `inherit` mode can cause problems with routing protocols like OSPF
> that use TTL=1 for their packets.  For tunnels carrying routing protocols,
> always use a fixed TTL value (typically 64 or 255).

#### Type of Service (ToS)

The ToS setting controls QoS marking for tunnel traffic:

<pre class="cli"><code>admin@example:/config/> <b>edit interface gre0</b>
admin@example:/config/interface/gre0/> <b>set gre tos 0x10</b>
admin@example:/config/interface/gre0/> <b>leave</b>
</code></pre>

Valid values are 0-255 for fixed ToS/DSCP marking, or `inherit` (default)
to copy the ToS value from the encapsulated packet.

#### Path MTU Discovery (GRE only)

The `pmtu-discovery` setting can be used to control the Path MTU Discovery on
GRE tunnels.  When enabled (default), the tunnel respects the Don't Fragment
(DF) bit and performs PMTU discovery:

<pre class="cli"><code>admin@example:/config/> <b>edit interface gre0</b>
admin@example:/config/interface/gre0/> <b>set gre pmtudisc false</b>
admin@example:/config/interface/gre0/> <b>leave</b>
</code></pre>

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

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interface vxlan100</b>
admin@example:/config/interface/vxlan100/> <b>set vxlan local 192.168.3.1</b>
admin@example:/config/interface/vxlan100/> <b>set vxlan remote 192.168.3.2</b>
admin@example:/config/interface/vxlan100/> <b>set vxlan vni 100</b>
admin@example:/config/interface/vxlan100/> <b>leave</b>
admin@example:/>
</code></pre>

The VNI uniquely identifies the VXLAN segment and must match on both
tunnel endpoints.

### VXLAN with Custom UDP Port

The default VXLAN UDP destination port is 4789 (IANA assigned).  In some
cases you may need to use a different port:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interface vxlan100</b>
admin@example:/config/interface/vxlan100/> <b>set vxlan local 192.168.3.1</b>
admin@example:/config/interface/vxlan100/> <b>set vxlan remote 192.168.3.2</b>
admin@example:/config/interface/vxlan100/> <b>set vxlan vni 100</b>
admin@example:/config/interface/vxlan100/> <b>set vxlan remote-port 8472</b>
admin@example:/config/interface/vxlan100/> <b>leave</b>
admin@example:/>
</code></pre>

The remote-port setting allows interoperability with systems using
non-standard VXLAN ports.

> [!NOTE]
> VXLAN tunnels also support the `ttl` and `tos` settings described in
> the [Advanced Tunnel Settings](#advanced-tunnel-settings) section above.
