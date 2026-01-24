# Network Configuration

Infix aims to support all Linux Networking constructs.  The YANG models
used to describe the system are chosen to fit well and leverage the
underlying Linux kernel's capabilities.  The [ietf-interfaces.yang][1]
model forms the base, extended with [ietf-ip.yang][2] and other layer-3
IETF models.  The layer-2 bridge and aggregate models are defined by
Infix to exploit the unique features not available in IEEE models.

> [!IMPORTANT]
> When issuing `leave` to activate your changes, remember to also save
> your settings, `copy running-config startup-config`.  See the [CLI
> Introduction](cli/introduction.md) for a background.


## Interface LEGO®

The network building blocks available in Linux are akin to the popular
LEGO® bricks.

![Linux Networking Blocks](img/lego.svg)

There are two types of relationships that can link two blocks together:

  1. **Lower-to-upper**: Visually represented by an extruding square
     connected upwards to a square socket.  An interface _can only have
     a single_ lower-to-upper relationship, i.e., it can be attached to
     a single upper interface like a bridge or a LAG.  In `iproute2`
     parlance, this corresponds to the interface's `master` setting
  2. **Upper-to-lower**: Visually represented by an extruding semicircle
     connected downwards to a semicircle socket.  The lower interface in
     these relationships _accepts multiple_ upper-to-lower relationships
     from different upper blocks.  E.g., multiple VLANs and IP address
     blocks can be connected to the same lower interface

![Stacking order dependencies](img/lego-relations.svg)

An interface may simultaneously have a _lower-to-upper_ relation to some
other interface, and be the target of one or more _upper-to-lower_
relationships.  It is valid, for example, for a physical port to be
attached to a bridge, but also have a VLAN interface stacked on top of
it.  In this example, traffic assigned to the VLAN in question would be
diverted to the VLAN interface before entering the bridge, while all
other traffic would be bridged as usual.

| **Type** | **Yang Model**             | **Description**                                              |
|----------|----------------------------|--------------------------------------------------------------|
| [bridge](bridging.md)   | infix-if-bridge            | SW implementation of an IEEE 802.1Q bridge          |
| [ip](ip.md)             | ietf-ip, infix-ip          | IP address to the subordinate interface             |
| [vlan](ethernet.md#vlan-interfaces) | infix-if-vlan  | Capture all traffic belonging to a specific 802.1Q VID |
| [lag](lag.md)           | infix-if-lag               | Link aggregation, static and IEEE 802.3ad (LACP)    |
| lo                      | ietf-interfaces            | Software loopback interface                         |
| [eth](ethernet.md#physical-ethernet-interfaces) | ieee802-ethernet-interface | Physical Ethernet device/port |
|                         | infix-ethernet-interface   |                                                     |
| [veth](ethernet.md#veth-pairs) | infix-if-veth       | Virtual Ethernet pair, typically one end is in a container |
| [*common*](iface.md)    | ietf-interfaces,           | Properties common to all interface types            |
|                         | infix-interfaces           |                                                     |


## Data Plane

The blocks you choose, and how you connect them, defines your data plane.
Here we see an example of how to bridge a virtual port with a physical
LAN.

![Example of a 4-port switch with a link aggregate and a VETH pair to a container](img/dataplane.svg)

Depending on the (optional) VLAN filtering of the bridge, the container
may have full or limited connectivity with outside ports, as well as the
internal CPU.

In fact the virtual port connected to the bridge can be member of
several VLANs, with each VLAN being an interface with an IP address
inside the container.

Thanks to Linux, and technologies like switchdev, that allow you to
split a switching fabric into unique (isolated) ports, the full
separation and virtualization of all Ethernet layer properties are
possible to share with a container.  Meaning, all the building blocks
used on the left hand side can also be used freely on the right hand
side as well.


[1]: https://www.rfc-editor.org/rfc/rfc8343
[2]: https://www.rfc-editor.org/rfc/rfc8344
