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

#### Examples

![Switch example (eth0 and lo)](img/ip-address-example-switch.svg)

    root@example:/> show interfaces 
    INTERFACE       STATE          PROTOCOL/ADDRESS          SOURCE                 
    lo              up             00:00:00:00:00:00         unknown
                                   127.0.0.1/8
    
    eth0            up             02:00:00:00:00:00         unknown
    root@example:/>

##### Static IP and link-local IP addresses

![Setting static IPv4 (and link-local IPv4)](img/ip-address-example-ipv4-static.svg)

    root@example:/> configure
    root@example:/config/> edit interfaces interface eth0 ipv4
    root@example:/config/interfaces/interface/eth0/ipv4/> set address 10.0.1.1 prefix-length 24
    root@example:/config/interfaces/interface/eth0/ipv4/> set autoconf enabled true 
    root@example:/config/interfaces/interface/eth0/ipv4/> leave
    root@example:/> show interfaces 
    INTERFACE       STATE          PROTOCOL/ADDRESS          SOURCE                 
    lo              up             00:00:00:00:00:00         unknown
                                   127.0.0.1/8
    
    eth0            up             02:00:00:00:00:00         unknown
                                   169.254.1.3/16
                                   10.0.1.1/24
    
    root@example:/>

##### Use of DHCP for address assignment

![Using DHCP for address assignment](img/ip-address-example-ipv4-dhcp.svg)

    root@example:/> configure 
    root@example:/config/> edit dhcp-client 
    root@example:/config/dhcp-client/> set client-if eth0
    root@example:/config/dhcp-client/> set enabled true 
    root@example:/config/dhcp-client/> leave
    root@example:/> show interfaces 
    INTERFACE       STATE          PROTOCOL/ADDRESS          SOURCE                 
    lo              up             00:00:00:00:00:00         unknown
                                   127.0.0.1/8
    
    eth0            up             02:00:00:00:00:00         unknown
                                   10.1.2.100/24
    
    root@example:/>

### IPv6 Address Assignment

Multiple address assignment methods are available:

| **Type**         | **Yang Model** | **Description**                                                                                                                                   |
|:---------------- |:-------------- |:------------------------------------------------------------------------------------------------------------------------------------------------- |
| static           | ietf-ip        | Static assignment of IPv6 address, e.g., *2001:db8:0:1::1/64*                                                                                     |
| link-local       | ietf-ip[^2]    | (RFC4862) Auto-configured link-local IPv6 address (*fe80::0* prefix + interface identifier, e.g., *fe80::ccd2:82ff:fe52:728b/64*)                 |
| global auto-conf | ietf-ip        | (RFC4862) Auto-configured (stateless) global IPv6 address (prefix from router + interface identifier, e.g., *2001:db8:0:1:ccd2:82ff:fe52:728b/64* |

#### Examples

![Switch example (eth0 and lo)](img/ip-address-example-switch.svg)

    root@example:/> show ip
    1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UP group iface qlen 1000
        link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
        inet 127.0.0.1/8 scope host lo
           valid_lft forever preferred_lft forever
        inet6 ::1/128 scope host 
           valid_lft forever preferred_lft forever
    2: eth0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc pfifo_fast state UP group default qlen 1000
        link/ether 02:00:00:00:00:00 brd ff:ff:ff:ff:ff:ff
        inet6 fe80::ff:fe00:0/64 scope link 
           valid_lft forever preferred_lft forever
    root@example:/>

##### Disabling IPv6 link-local address(es)

The only way to disable IPv6 link-local addresses is by disabling IPv6 on the interface.

```(disabling
root@example:/> configure 
root@example:/config/> edit interfaces interface eth0 ipv6
root@example:/config/interfaces/interface/eth0/ipv6/> set enabled false
root@example:/config/interfaces/interface/eth0/ipv6/> leave
root@example:/> show ip
1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UP group iface qlen 1000
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
    inet 127.0.0.1/8 scope host lo
       valid_lft forever preferred_lft forever
    inet6 ::1/128 scope host 
       valid_lft forever preferred_lft forever
2: eth0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc pfifo_fast state UP group default qlen 1000
    link/ether 02:00:00:00:00:00 brd ff:ff:ff:ff:ff:ff
root@example:/>
```

##### Static IPv6 address

![Setting static IPv6](img/ip-address-example-ipv6-static.svg)

    root@example:/> configure 
    root@example:/config/> edit interfaces interface eth0 ipv6
    root@example:/config/interfaces/interface/eth0/ipv6/> set address 2001:db8::1 prefix-length 64
    root@example:/config/interfaces/interface/eth0/ipv6/> leave
    root@example:/> show ip
    1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UP group iface qlen 1000
        link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
        inet 127.0.0.1/8 scope host lo
           valid_lft forever preferred_lft forever
        inet6 ::1/128 scope host 
           valid_lft forever preferred_lft forever
    2: eth0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc pfifo_fast state UP group default qlen 1000
        link/ether 02:00:00:00:00:00 brd ff:ff:ff:ff:ff:ff
        inet6 2001:db8::1/64 scope global 
           valid_lft forever preferred_lft forever
        inet6 fe80::ff:fe00:0/64 scope link 
           valid_lft forever preferred_lft forever
    root@example:/>

##### Stateless Autoconfiguration of Global IPv6 Address

![Auto-configuration of global IPv6](img/ip-address-example-ipv6-auto-global.svg)

Concatenation of prefix advertised by router (here 2001:db8:0:1::0/64)
and interface identifier.

    root@example:/> configure 
    root@example:/config/> edit interfaces interface eth0 ipv6
    root@example:/config/interfaces/interface/eth0/ipv6/> set address 2001:db8::1 prefix-length 64
    root@example:/config/interfaces/interface/eth0/ipv6/> leave
    root@example:/> show ip
    1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UP group iface qlen 1000
        link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
        inet 127.0.0.1/8 scope host lo
           valid_lft forever preferred_lft forever
        inet6 ::1/128 scope host 
           valid_lft forever preferred_lft forever
    2: eth0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc pfifo_fast state UP group default qlen 1000
        link/ether 02:00:00:00:00:00 brd ff:ff:ff:ff:ff:ff
        inet6 2001:db8:0:1:0:ff:fe00:0/64 scope global dynamic mngtmpaddr 
           valid_lft 86398sec preferred_lft 14398sec
        inet6 fe80::ff:fe00:0/64 scope link 
           valid_lft forever preferred_lft forever
    root@example:/>

[^1]: Please note, link aggregates are not yet supported in Infix.
Link-local IPv6 addresses are implicitly enabled when enabling IPv6. IPv6 can be enabled/disabled per interface in *ietf-ip* YANG model.
