# Firewall Documentation

## Introduction

The Infix firewall aims to simplify network security.  Instead of complex
per-interface rules, you work with *zones*.  A zone defines a level of trust
between all interfaces assigned to it and security policies regulate traffic
flow between zones.

![Firewall](img/firewall.svg){ align=left width="180" }

This approach is intuitive and maintainable — you think in terms of trust
relationships ("internal networks can access the Internet", "Internet cannot
access my internal network") rather than individual interface rules.  When you
add new interfaces to existing zones, they automatically inherit the
established security policies.

The firewall controls three distinct traffic flows: traffic destined for the
host itself, traffic between interfaces within the same zone, and traffic
between different zones.

> [!TIP] Impatient and ready to get going?
> [Fast forward to the Examples: End Device, Home/Office Router, Enterprise Gateway](#examples)

## Zones

Zones are logical groupings of network interfaces that share the same trust
level.  Each zone has a default *action* that determines what happens to
traffic destined for the host itself (INPUT chain).  A LAN zone may have this
set to *accept*, while a DMZ zone may be set to *reject* by default and only
allow a subset of *services*, e.g., DHCP, DNS, and SSH, on input.

### Intra-Zone Traffic

For a LAN zone, the trust level can be set to allow IP routing between all
interfaces and networks.  This is often referred to as *intra-zone* traffic
and is controlled by the zone's `forwarding` setting, it works independently
of the zone action.  When enabled, devices on different interfaces within the
same zone can communicate directly with each other.

> [!NOTE] Remember IP forwarding!
> Allowing forwarding between interfaces in the zone is not enough, this only
> prevents the firewall from actively blocking the traffic flows, you also
> need to enable [IP forwarding](network.md#ipv4-forwarding).

### Port Forwarding

Each zone can have port forwarding rules (DNAT) that redirect incoming traffic
from one port to a different internal address and port.  This is effectively a
security policy that allows external access to internal services.

The *Firewall Matrix* shows a ⚠ conditional warning flag, coloring the zone
yellow, when policy exceptions like port forwarding are active.

![Firewall Matrix](img/fw-matrix.png)

### Default Zone Concept

Infix requires you to specify a default zone.  Any interface not explicitly
assigned to a zone automatically belongs to the default zone.  This ensures
that no interface is left without firewall protection.

Choose your default zone carefully — it should be the most restrictive zone
appropriate for unmanaged interfaces.  For routers, this is typically the
`wan` zone.

## Policies

![Zone based firewall](img/fw-zones.svg){ align=right width="420" }

Policy rules control traffic **between** zones.  By default all inter-zone
traffic is rejected.  Meaning you must explicitly allow the traffic flows
you intend.

IP masquerading (SNAT) is a policy setting that applies to traffic egressing
a target zone.  (Essential for Internet access from private networks.)

A policy, like zones, have a default action.  If it is *not* set to `accept`
you must specify which services are allowed.

See the [examples below](#enterprise-gateway) for how to set up a policy.  The
built-in help system can also be useful:

```
admin@example:/config/firewall/policy/lan-to-dmz/> help masquerade
NAME
        masquerade <true/false>

DESCRIPTION
        Enable masquerading (SNAT) for traffic matching this policy.

admin@example:/config/firewall/policy/lan-to-dmz/>
```

## Services

Several pre-defined services exist, that cover most use-cases, but you can
also define custom services for applications not covered by the built-in ones.

### Built-in services

Infix provides predefined services for common protocols:

- **`ssh`**: Secure Shell (port 22/tcp)
- **`http`**: Web traffic (port 80/tcp)
- **`https`**: Secure web traffic (port 443/tcp)
- **`dns`**: Domain Name System (port 53/tcp and 53/udp)
- **`dhcp`**: DHCP server (port 67/udp)
- **`dhcpv6-client`**: DHCPv6 client traffic
- **`netconf`**: Network Configuration Protocol (port 830/tcp)
- **`restconf`**: REST-based Network Configuration Protocol (port 443/tcp)

... and more, see `infix-firewall-services.yang` for details

## Examples

### End Device Protection

For devices on untrusted networks like public Wi-Fi or other open Internet
connections.  Provides maximum protection while allowing essential
connectivity.

```
admin@example:/> configure
admin@example:/config/> edit firewall
admin@example:/config/firewall/> set default public
admin@example:/config/firewall/> edit zone public
admin@example:/config/firewall/zone/public/> set description "Public untrusted network - end device protection"
admin@example:/config/firewall/zone/public/> set action drop
admin@example:/config/firewall/zone/public/> set interface eth0
admin@example:/config/firewall/zone/public/> set service ssh
admin@example:/config/firewall/zone/public/> set service dhcpv6-client
admin@example:/config/firewall/zone/public/> leave
```

### Home/Office Router

For typical routers that need to protect internal devices while providing
internet access.  The LAN zone trusts internal devices, while the WAN zone
blocks external threats.

```
admin@example:/> configure
admin@example:/config/> edit firewall
admin@example:/config/firewall/> set default wan
admin@example:/config/firewall/> edit zone lan
admin@example:/config/firewall/zone/lan/> set description "Internal LAN network - trusted"
admin@example:/config/firewall/zone/lan/> set action accept
admin@example:/config/firewall/zone/lan/> set interface eth1
admin@example:/config/firewall/zone/lan/> set service ssh
admin@example:/config/firewall/zone/lan/> set service dhcp
admin@example:/config/firewall/zone/lan/> set service dns
admin@example:/config/firewall/zone/lan/> end
admin@example:/config/firewall/> edit zone wan
admin@example:/config/firewall/zone/wan/> set description "External WAN interface - untrusted"
admin@example:/config/firewall/zone/wan/> set action drop
admin@example:/config/firewall/zone/wan/> set interface eth0
admin@example:/config/firewall/zone/wan/> end
admin@example:/config/firewall/> edit policy loc-to-wan
admin@example:/config/firewall/policy/loc-to-wan/> set description "Allow local LAN/DMZ traffic to WAN with SNAT"
admin@example:/config/firewall/policy/loc-to-wan/> set ingress lan
admin@example:/config/firewall/policy/loc-to-wan/> set egress wan
admin@example:/config/firewall/policy/loc-to-wan/> set action accept
admin@example:/config/firewall/policy/loc-to-wan/> set masquerade
admin@example:/config/firewall/policy/loc-to-wan/> leave
```

> [!NOTE]
> Policy rules apply in a stateful, unidirectional manner.  Meaning, you only
> consider one direction of the traffic.  The return traffic (established,
> related) is implicitly allowed.

### Enterprise Gateway

For businesses that need to host public services while protecting internal
resources.  We can build upon the Home/Office Router example above and add
a DMZ zone with additional policies for controlled access.

```
admin@example:/> configure
admin@example:/config/> edit firewall zone dmz
admin@example:/config/firewall/zone/dmz/> set description "Semi-trusted public services"
admin@example:/config/firewall/zone/dmz/> set action drop
admin@example:/config/firewall/zone/dmz/> set interface eth1
admin@example:/config/firewall/zone/dmz/> set service ssh
admin@example:/config/firewall/zone/dmz/> end
admin@example:/config/firewall/> edit policy lan-to-wan
admin@example:/config/firewall/policy/lan-to-wan/> set ingress dmz
admin@example:/config/firewall/policy/lan-to-wan/> end
admin@example:/config/firewall/> edit policy lan-to-dmz
admin@example:/config/firewall/policy/lan-to-dmz/> set description "Allow LAN to manage DMZ services"
admin@example:/config/firewall/policy/lan-to-dmz/> set ingress lan
admin@example:/config/firewall/policy/lan-to-dmz/> set egress dmz
admin@example:/config/firewall/policy/lan-to-dmz/> set action accept
admin@example:/config/firewall/policy/lan-to-dmz/> end
admin@example:/config/firewall/> edit zone wan port-forward 8080 tcp
admin@example:/config/firewall/zone/wan/port-forward/8080/tcp/> set to addr 192.168.2.10
admin@example:/config/firewall/zone/wan/port-forward/8080/tcp/> set to port 80
admin@example:/config/firewall/zone/wan/port-forward/8080/tcp/> leave
```

This adds a DMZ zone for public services, updates the internet access policy
to include DMZ traffic, allows LAN management of DMZ services, and forwards
external web traffic to the DMZ server.

## Logging and Monitoring

Different log levels are available to monitor and debug firewall behavior.
Configure logging using the CLI:

```
admin@example:/> configure
admin@example:/config/> edit firewall
admin@example:/config/firewall/> set logging all
admin@example:/config/firewall/> leave
```

Firewall logs help you understand traffic patterns and security events.  The
CLI admin-exec command `show firewall` shows the last 10 log messages in the
overview:

![Firewall logs](img/fw-logs.png)

Use the command `show log firewall.log` to display the full logfile (remember,
the syslog daemon rotates and zips too big log files).  You can also use the
`follow firewall.log` command to continuously monitor firewall log messages.
