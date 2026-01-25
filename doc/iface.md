# Common Interface Settings

Common interface settings include `name`, `type`, `enabled`, `description`,
and custom MAC address.  Type-specific settings are covered in the dedicated
sections for [Bridging](bridging.md), [Link Aggregation](lag.md),
[Ethernet](ethernet.md), [IP Addressing](ip.md), and [Routing](routing.md).


## Interface Name

The interface name is limited to 1-15 characters due to Linux kernel
constraints.  Physical interfaces use their system-assigned names (e.g.,
`eth0`, `eth1`), while user-created interfaces can be named freely within
this limit.

> [!TIP]
> Naming conventions like `br0`, `lag0`, `vlan10`, or `eth0.20` allow
> the CLI to automatically infer the interface type.


## Interface Type

The `type` setting defines what kind of interface this is: `bridge`, `lag`,
`vlan`, `veth`, etc.  When configuring via the CLI, the type is often
inferred from the interface name.  However, when configuring remotely via
NETCONF or RESTCONF, the type *must* be set explicitly.

<pre class="cli"><code>admin@example:/config/> <b>edit interface br0</b>
admin@example:/config/interface/br0/> <b>set type bridge</b>
</code></pre>

Available types can be listed from the CLI:

<pre class="cli"><code>admin@example:/config/interface/br0/> <b>set type ?</b>
  bridge     IEEE bridge interface.
  dummy      Linux dummy interface.  Useful mostly for testing.
  ethernet   Any Ethernet interfaces, regardless of speed, RFC 3635.
  gre        GRE tunnel interface.
  gretap     GRETAP (Ethernet over GRE) tunnel interface.
  lag        IEEE link aggregate interface.
  loopback   Linux loopback interface.
  other      Other interface, i.e., unknown.
  veth       Linux virtual Ethernet pair.
  vlan       Layer 2 Virtual LAN using 802.1Q.
  vxlan      Virtual eXtensible LAN tunnel interface.
  wifi       WiFi (802.11) interface
  wireguard  WireGuard VPN tunnel interface.
</code></pre>


## Enable/Disable

An interface can be administratively disabled using the `enabled` setting.
By default, interfaces are enabled (`true`).

<pre class="cli"><code>admin@example:/config/> <b>edit interface eth0</b>
admin@example:/config/interface/eth0/> <b>set enabled false</b>
admin@example:/config/interface/eth0/> <b>leave</b>
</code></pre>

The operational status can be inspected to see both administrative and
actual link state:

<pre class="cli"><code>admin@example:/> <b>show interfaces</b>
INTERFACE       PROTOCOL   STATE       DATA
eth0            ethernet   <b>DISABLED</b>    02:00:00:00:00:00
eth1            ethernet   UP          02:00:00:00:00:01
...
</code></pre>


## Description

The `description` is a free-form text string (max 64 characters) saved
as the Linux interface alias (`ifalias`).  Use it to document an interface's
purpose or add notes for remote debugging.

<pre class="cli"><code>admin@example:/config/> <b>edit interface eth0</b>
admin@example:/config/interface/eth0/> <b>set description "Uplink to core switch"</b>
admin@example:/config/interface/eth0/> <b>leave</b>
</code></pre>

The description is visible in the operational datastore and in `show`
commands:

<pre class="cli"><code>admin@example:/> <b>show interface eth0</b>
name                : eth0
description         : Uplink to core switch
index               : 2
...
</code></pre>


## Custom MAC Address

The `custom-phys-address` can be used to set an interface's MAC address.
This is an extension to the ietf-interfaces YANG model, which defines
`phys-address` as read-only[^1].

> [!CAUTION]
> There is no validation or safety checks performed by the system when
> using `custom-phys-address`.  In particular the `offset` variant can
> be dangerous to use -- pay attention to the meaning of bits in the
> upper-most octet: local bit, multicast/group, etc.

### Fixed custom MAC

Use a fixed custom MAC address when the interface must present a
specific, deterministic identity on the network.  This option bypasses
any chassis-derived logic and applies the configured address verbatim.

<pre class="cli"><code>admin@example:/config/> <b>edit interface veth0a</b>
admin@example:/config/interface/veth0a/> <b>set custom-phys-address static 00:ab:00:11:22:33</b>

=> 00:ab:00:11:22:33
</code></pre>

### Chassis MAC

Chassis MAC, sometimes also referred to as base MAC.  In these two
examples it is `00:53:00:c0:ff:ee`.

<pre class="cli"><code>admin@example:/config/> <b>edit interface veth0a</b>
admin@example:/config/interface/veth0a/> <b>set custom-phys-address chassis</b>

=> 00:53:00:c0:ff:ee
</code></pre>

### Chassis MAC, with offset

When constructing a derived address it is recommended to set the locally
administered bit.  Same chassis MAC as before.

<pre class="cli"><code>admin@example:/config/> <b>edit interface veth0a</b>
admin@example:/config/interface/veth0a/> <b>set custom-phys-address chassis offset 02:00:00:00:00:02</b>

=> 02:53:00:c0:ff:f0
</code></pre>


[^1]: A YANG deviation was previously used to make it possible to set
    `phys-address`, but this has been replaced with the more flexible
    `custom-phys-address`.
