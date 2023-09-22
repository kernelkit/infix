# Linux Networking

## Interface LEGO®

![Linux Networking Blocks](img/lego.svg)

| **Type** | **Yang Model**    | **Description**                                               |
| -------- | ----------------- | ------------------------------------------------------------- |
| bridge   | infix-if-bridge   | SW implementation of an IEEE 802.1Q bridge                    |
| ip       | ietf-ip, infix-ip | IP address to the subordinate interface                       |
| vlan     | ietf-vlan-encap   | Capture all traffic belonging to a specific 802.1Q VID        |
| lag[^1]  | infix-if-lag      | Bonds multiple interfaces into one, creating a link aggregate |
| lo       | ietf-interfaces   | Software loopback interface                                   |
| eth      | ietf-interfaces   | Physical Ethernet device/port                                 |
| veth     | infix-if-veth     | Virtual Ethernet pair, typically one end is in a container    |

## Data Plane

The blocks you ctose, and how you connect them, defines your data plane.  Here we see an example of how to bridge a virtual port with a physical LAN.

![Example of a 4-port switch with a link aggregate and a VETH pair to a container](img/dataplane.svg)

Depending on the (optional) VLAN filtering of the bridge, the container may have full or limited connectivity with outside ports, as well as the internal CPU.

In fact the virtual port connected to the bridge can be member of several VLANs, with each VLAN being an interface with an IP address inside the container.

Thanks to Linux, and technologies like switchdev, that allow you to split a switching fabric into unique (isolated) ports, the full separation and virtualization of all Ethernet layer properties are possible to share with a container.  Meaning, all the building blocks used on the left hand side can also be used freely on the right hand side as well.

### Bridging

This is the most central part of the system.  A bridge is a switch, and a switch is a bridge.  In Linux, setting up a bridge with ports connected to physical switch fabric, means you manage the actual switch fabric!

In Infix ports are by default not switch ports, unless the customer specific factory config sets it up this way.  To enable switching between ports you create a bridge and then add ports to that bridge. That's it.

```
admin@example:/> configure
admin@example:/config/> edit interfaces interface br0
admin@example:/config/interfaces/interface/br0/> up
admin@example:/config/interfaces/> set interface eth0 bridge-port bridge br0
admin@example:/config/interfaces/> set interface eth1 bridge-port bridge br0
admin@example:/config/interfaces/> leave
```

Here we add two ports to bridge `br0`: `eth0` and `eth1`. 

> **Note:** Infix has many built-in helpers controlled by convention. E.g., if you name your bridge `brN`, where `N` is a number, Infix will set the interface type automatically for you, and unlock all bridge features for you.

#### VLAN Filtering Bridge

By default bridges in Linux do not filter based on VLAN tags.   It can be enabled in Infix when creating a bridge by adding a port to a VLAN as a tagged or untagged member:

```
admin@example:/config/> edit interfaces interface br0 
admin@example:/config/interfaces/interface/br0/> up
admin@example:/config/interfaces/> set interface eth0 bridge-port bridge br0
admin@example:/config/interfaces/> set interface eth1 bridge-port bridge br0
admin@example:/config/interfaces/> edit interface br0
admin@example:/config/interfaces/interface/br0/> set bridge vlans vlan 10 untagged eth0
admin@example:/config/interfaces/interface/br0/> set bridge vlans vlan 20 untagged eth1
```

This sets `eth0` as an untagged member of VLAN 10 and `eth1` as an
untagged member of VLAN 20.  Switching between these ports is thus
prohibited.

### VLAN Interfaces

Creating a VLAN can be done in many ways. This section assumes VLAN interfaces created atop another Linux interface.  E.g., the VLAN interfaces created on top of the bridge in the picture above.

A VLAN interface is basically a filtering abstraction. When you run `tcpdump` on a VLAN interface you will only see the frames matching the VLAN ID of the interface, compared to *all* the VLAN IDs if you run `tcpdump` on the parent interface.

```
admin@example:/> configure 
admin@example:/config/> edit interfaces interface eth0.20
admin@example:/config/interfaces/interface/eth0.20/> set encapsulation dot1q-vlan outer-tag tag-type c-vlan vlan-id 20
admin@example:/config/interfaces/interface/eth0.20/> set parent-interface eth0
admin@example:/config/interfaces/interface/eth0.20/> leave
```

> **Note:** If you name your VLAN interface `foo0.N`, where `N` is a number, Infix will set the interface type automatically for you.

## Management Plane

This section details IP Addresses And Other Per-Interface IP settings.

Infix support several network interface types, each can be assigned one or more IP addresses, both IPv4 and IPv6 are supported.

![IP on top of network interface examples](img/ip-iface-examples.svg)

### IPv4 Address Assignment

Multiple address assignment methods are available:

| **Type**   | **Yang Model**    | **Description**                                                |
|:---------- |:----------------- |:-------------------------------------------------------------- |
| static     | ietf-ip           | Static assignment of IPv4 address, e.g., *10.0.1.1/24*         |
| link-local | infix-ip          | Auto-assignment of IPv4 address in 169.254.x.x/16 range        |
| dhcp       | infix-dhcp-client | Assignment of IPv4 address by DHCP server, e.g., *10.0.1.1/24* |

DHCP address method is only available for *LAN* interfaces (ethernet, virtual ethernet (veth), bridge, etc.)

### IPv6 Address Assignment

Multiple address assignment methods are available:

| **Type**         | **Yang Model** | **Description**                                                                                                                                   |
|:---------------- |:-------------- |:------------------------------------------------------------------------------------------------------------------------------------------------- |
| static           | ietf-ip        | Static assignment of IPv6 address, e.g., *2001:db8:0:1::1/64*                                                                                     |
| link-local       | ietf-ip[^2]    | (RFC4862) Auto-configured link-local IPv6 address (*fe80::0* prefix + interface identifier, e.g., *fe80::ccd2:82ff:fe52:728b/64*)                 |
| global auto-conf | ietf-ip        | (RFC4862) Auto-configured (stateless) global IPv6 address (prefix from router + interface identifier, e.g., *2001:db8:0:1:ccd2:82ff:fe52:728b/64* |

Both for *link-local* and *global auto-configuration*, it is possible
to auto-configure using a random suffix instead of the interface
identifier. 


### Examples

![Switch example (eth0 and lo)](img/ip-address-example-switch.svg)

    root@infix-00-00-00:/> show interfaces 
    INTERFACE       PROTOCOL   STATE       DATA                                     
    eth0            ethernet   UP          02:00:00:00:00:00                        
                    ipv6                   fe80::ff:fe00:0/64 (link-layer)
    lo              ethernet   UP          00:00:00:00:00:00                        
                    ipv4                   127.0.0.1/8 (static)
                    ipv6                   ::1/128 (static)
    root@infix-00-00-00:/>

To illustrate IP address configuration, the examples below uses a
switch with a single Ethernet interface (eth0) and a loopback
interface (lo). As shown above, these examples assume *eth0* has an
IPv6 link-local address and *lo* has static IPv4 and IPv6 addresses by
default.

#### Static and link-local IPv4 addresses

![Setting static IPv4 (and link-local IPv4)](img/ip-address-example-ipv4-static.svg)

    root@example:/> configure
    root@example:/config/> edit interfaces interface eth0 ipv4
    root@example:/config/interfaces/interface/eth0/ipv4/> set address 10.0.1.1 prefix-length 24
    root@example:/config/interfaces/interface/eth0/ipv4/> set autoconf enabled true 
	root@infix-example:/config/interfaces/interface/eth0/ipv4/> diff
    +interfaces {
    +  interface eth0 {
    +    ipv4 {
    +      address 10.0.1.1 {
    +        prefix-length 24;
    +      }
    +      autoconf {
    +        enabled true;
    +      }
    +    }
    +  }
    +}
    root@infix-example:/config/interfaces/interface/eth0/ipv4/> leave
    root@infix-example:/> show interfaces 
    INTERFACE       PROTOCOL   STATE       DATA                                     
    eth0            ethernet   UP          02:00:00:00:00:00                        
                    ipv4                   10.0.1.1/24 (static)
                    ipv6                   fe80::ff:fe00:0/64 (link-layer)
    lo              ethernet   UP          00:00:00:00:00:00                        
                    ipv4                   127.0.0.1/8 (static)
                    ipv6                   ::1/128 (static)
    root@infix-example:/> show interfaces 
    INTERFACE       PROTOCOL   STATE       DATA                                     
    eth0            ethernet   UP          02:00:00:00:00:00                        
                    ipv4                   169.254.1.3/16 (random)
                    ipv4                   10.0.1.1/24 (static)
                    ipv6                   fe80::ff:fe00:0/64 (link-layer)
    lo              ethernet   UP          00:00:00:00:00:00                        
                    ipv4                   127.0.0.1/8 (static)
                    ipv6                   ::1/128 (static)
    root@infix-example:/>

As shown, the link-local IPv4 address is configured with `set autconf
enabled true`.  The resulting address (169.254.1.3/16) is of type
*random* ([IETF ip-yang][ietf-ip-yang]).

#### Use of DHCP for IPv4 address assignment

![Using DHCP for IPv4 address assignment](img/ip-address-example-ipv4-dhcp.svg)

    root@example:/> configure 
    root@example:/config/> edit dhcp-client 
    root@example:/config/dhcp-client/> set client-if eth0
    root@example:/config/dhcp-client/> set enabled true 
    root@example:/config/dhcp-client/> leave
    root@example:/> show interfaces 
    INTERFACE       PROTOCOL   STATE       DATA                                     
    eth0            ethernet   UP          02:00:00:00:00:00                        
                    ipv4                   10.1.2.100/24 (dhcp)
                    ipv6                   fe80::ff:fe00:0/64 (link-layer)
    lo              ethernet   UP          00:00:00:00:00:00                        
                    ipv4                   127.0.0.1/8 (static)
                    ipv6                   ::1/128 (static)

    root@example:/>

The resulting address (10.1.2.100/24) is of type *dhcp*.


#### Disabling IPv6 link-local address(es)

The (only) way to disable IPv6 link-local addresses is by disabling IPv6 on the interface.

```(disabling
root@example:/> configure 
root@example:/config/> edit interfaces interface eth0 ipv6
root@example:/config/interfaces/interface/eth0/ipv6/> set enabled false
root@example:/config/interfaces/interface/eth0/ipv6/> leave
root@example:/> show interfaces 
INTERFACE       PROTOCOL   STATE       DATA                                     
eth0            ethernet   UP          02:00:00:00:00:00                        
lo              ethernet   UP          00:00:00:00:00:00                        
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
root@example:/>
```

#### Static IPv6 address

![Setting static IPv6](img/ip-address-example-ipv6-static.svg)

    root@example:/> configure 
    root@example:/config/> edit interfaces interface eth0 ipv6
    root@example:/config/interfaces/interface/eth0/ipv6/> set address 2001:db8::1 prefix-length 64
    root@example:/config/interfaces/interface/eth0/ipv6/> leave
    root@example:/> show interfaces 
    INTERFACE       PROTOCOL   STATE       DATA                                     
    eth0            ethernet   UP          02:00:00:00:00:00                        
                    ipv6                   2001:db8::1/64 (static)
                    ipv6                   fe80::ff:fe00:0/64 (link-layer)
    lo              ethernet   UP          00:00:00:00:00:00                        
                    ipv4                   127.0.0.1/8 (static)
                    ipv6                   ::1/128 (static)
    root@example:/>

#### Stateless Auto-configuration of Global IPv6 Address

![Auto-configuration of global IPv6](img/ip-address-example-ipv6-auto-global.svg)

Stateless address auto-configuration of global addresses is enabled by
default. The address is formed by concatenating the network prefix
advertised by the router (here 2001:db8:0:1::0/64) and the interface
identifier. The resulting address is of type *other*.

    root@infix-example:/> show interfaces 
    INTERFACE       PROTOCOL   STATE       DATA                                     
    eth0            ethernet   UP          02:00:00:00:00:00                        
                    ipv6                   2001:db8:0:1:0:ff:fe00:0/64 (other)
                    ipv6                   fe80::ff:fe00:0/64 (link-layer)
    lo              ethernet   UP          00:00:00:00:00:00                        
                    ipv4                   127.0.0.1/8 (static)
                    ipv6                   ::1/128 (static)
    root@infix-example:/>

Disabling auto-configuration of global IPv6 addresses can be done as shown
below.

    root@infix-00-00-00:/> configure
    root@infix-00-00-00:/config/> edit interfaces interface eth0 ipv6
    root@infix-00-00-00:/config/interfaces/interface/eth0/ipv6/> set autoconf create-global-addresses false 
    root@infix-00-00-00:/config/interfaces/interface/eth0/ipv6/> leave
    root@infix-00-00-00:/> show interfaces 
    INTERFACE       PROTOCOL   STATE       DATA                                     
    eth0            ethernet   UP          02:00:00:00:00:00                        
                    ipv6                   fe80::ff:fe00:0/64 (link-layer)
    lo              ethernet   UP          00:00:00:00:00:00                        
                    ipv4                   127.0.0.1/8 (static)
                    ipv6                   ::1/128 (static)
    root@infix-00-00-00:/>

#### Random Link Identifiers for IPv6 Stateless Autoconfiguration

![Auto-configuration of global IPv6](img/ip-address-example-ipv6-auto-global.svg)

By default, the auto-configured link-local and global IPv6 addresses
are formed from a link-identifier based on the MAC address.

    root@infix-example:/> show interfaces 
    INTERFACE       PROTOCOL   STATE       DATA                                     
    eth0            ethernet   UP          02:00:00:00:00:00                        
                    ipv6                   2001:db8:0:1:0:ff:fe00:0/64 (other)
                    ipv6                   fe80::ff:fe00:0/64 (link-layer)
    lo              ethernet   UP          00:00:00:00:00:00                        
                    ipv4                   127.0.0.1/8 (static)
                    ipv6                   ::1/128 (static)
    root@infix-example:/>

To avoid revealing identity information in the IPv6 address, it is
possible to specify use of a random identifier ([ietf-ip][ietf-ip-yang] YANG and [RFC8981][ietf-ipv6-privacy]).

    root@infix-example:/> configure 
    root@infix-example:/config/> edit interfaces interface eth0 ipv6
    root@infix-example:/config/interfaces/interface/eth0/ipv6/> set autoconf create-temporary-addresses true 
    root@infix-example:/config/interfaces/interface/eth0/ipv6/> leave
    root@infix-example:/> show interfaces 
    INTERFACE       PROTOCOL   STATE       DATA                                     
    eth0            ethernet   UP          02:00:00:00:00:00                        
                    ipv6                   2001:db8:0:1:fba2:f413:dd22:13ad/64 (other)
                    ipv6                   fe80::b886:6849:18dc:19ef/64 (random)
    lo              ethernet   UP          00:00:00:00:00:00                        
                    ipv4                   127.0.0.1/8 (static)
                    ipv6                   ::1/128 (static)
    root@infix-example:/>

The link-local address has changed type to *random*.

[ietf-ip-yang]:         https://www.rfc-editor.org/rfc/rfc8344.html
[ietf-ipv6-privacy]:    https://www.rfc-editor.org/rfc/rfc8981.html

[^1]: Please note, link aggregates are not yet supported in Infix.
[^2]: Link-local IPv6 addresses are implicitly enabled when enabling IPv6. IPv6 can be enabled/disabled per interface in [ietf-ip][ietf-ip-yang] YANG model.


