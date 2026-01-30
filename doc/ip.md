# IP Address Configuration

This section details IP Addresses And Other Per-Interface IP settings.

Infix support several network interface types, each can be assigned one
or more IP addresses, both IPv4 and IPv6 are supported.  (There is no
concept of a "primary" address.)

![IP on top of network interface examples](img/ip-iface-examples.svg)

## IPv4 Address Assignment

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
> Per [RFC3442][1], if the DHCP server returns both a Classless Static
> Routes option (121) and Router option (3), the DHCP client *must*
> ignore the latter.


## IPv6 Address Assignment

Multiple address assignment methods are available:

| **Type**         | **Yang Model**       | **Description**                                                                                                                                   |
|:---------------- |:-------------------- |:------------------------------------------------------------------------------------------------------------------------------------------------- |
| static           | ietf-ip              | Static assignment of IPv6 address, e.g., *2001:db8:0:1::1/64*                                                                                     |
| link-local       | ietf-ip[^1]          | (RFC4862) Auto-configured link-local IPv6 address (*fe80::0* prefix + interface identifier, e.g., *fe80::ccd2:82ff:fe52:728b/64*)                 |
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

## Examples

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

### Static and link-local IPv4 addresses

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


### Use of DHCP for IPv4 address assignment

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

### Use of DHCPv6 for IPv6 address assignment

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

### Disabling IPv6 link-local address(es)

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

### Static IPv6 address

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


### Stateless Auto-configuration of Global IPv6 Address

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


### Random Link Identifiers for IPv6 Stateless Autoconfiguration

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


## IPv4 forwarding

To be able to route (static or dynamic) on the interface it is
required to enable forwarding. This setting controls if packets
received on this interface can be forwarded.

<pre class="cli"><code>admin@example:/config/> <b>edit interface eth0</b>
admin@example:/config/interface/eth0/> <b>set ipv4 forwarding</b>
admin@example:/config/interface/eth0/> <b>leave</b>
admin@example:/>
</code></pre>


## IPv6 forwarding

Forwarding must be enabled on an interface for it to route IPv6
traffic (static or dynamic).  The setting is per-interface and works
the same way as IPv4 forwarding.

The following table shows the IPv6 features that the `forwarding`
setting controls when it is *Enabled* or *Disabled*:

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

[1]: https://www.rfc-editor.org/rfc/rfc3442
[2]: https://www.rfc-editor.org/rfc/rfc8344
[3]: https://www.rfc-editor.org/rfc/rfc8981

[^1]: Link-local IPv6 addresses are implicitly enabled when enabling
    IPv6.  IPv6 can be enabled/disabled per interface in the
    [ietf-ip][2] YANG model.
