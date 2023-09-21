# Linux Networking

## Interface LEGO®

![Linux Networking Blocks](img/lego.svg)

| **Type** | **Yang Model**    | **Description**                                               |
|----------|-------------------|---------------------------------------------------------------|
| bridge   | infix-if-bridge   | SW implementation of an IEEE 802.1Q bridge                    |
| ip       | ietf-ip, infix-ip | IP address to the subordinate interface                       |
| vlan     | ietf-vlan-encap   | Capture all traffic belonging to a specific 802.1Q VID        |
| lag[^1]  | infix-if-lag      | Bonds multiple interfaces into one, creating a link aggregate |
| lo       | ietf-interfaces   | Software loopback interface                                   |
| eth      | ietf-interfaces   | Physical Ethernet device/port                                 |
| veth     | infix-if-veth     | Virtual Ethernet pair, typically one end is in a container    |

[^1]: Please note, link aggregates are not yet supported in Infix.

## Dataplane

The blocks you choose, and how you connect them, defines your dataplane.
Here we see an example of how to bridge a virtual port with a physical
LAN.

![Example of a 4-port switch with a link aggregate and a VETH pair to a container](img/dataplane.svg)

Depending on the (optional) VLAN filtering of the bridge, the container
may have full or limited connectivity with outside ports, as well as the
internal CPU.

In fact the virtual port connected to the bridge can be member of
several VLANs, with each VLAN being an interface with an IP address
inside the container.

Thanks to Linux, and technologies like switchdev that allow you to split
a switching fabric into unique (isolated) ports, the full separation and
virtualization of all Ethernet layer properties are possible to share
with a container.  Meaning, all the building blocks used on the left
hand side can also be used freely on the right hand side as well.
