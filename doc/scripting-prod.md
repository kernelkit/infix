# Scripting for Production Tests

This document shows how to set up and remotely script devices with a
focus on production testing.

## VLAN Snake

As part of production tests, verification of Ethernet ports is usually
expected.  A common way for devices with multiple bridged Ethernet ports
is to connect a test PC to two ports and send a *ping* traversing all
ports.  This can be achieved by using VLANs, on the switch, as described
in this section.  The resulting configuration file can be applied to the
running configuration of the produced unit, e.g, use config file restore
as [described previously][2].

In this example we assume a 10 port switch, with ports e1-e10.  The
following VLAN configuration and cable connections will be used:

| VLAN & Ports      | Connect   |
|:------------------|:----------|
| VLAN 10: e1 & e2  | e2 <=> e3 |
| VLAN 20: e3 & e4  | e4 <=> e5 |
| VLAN 30: e5 & e6  | e6 <=> e7 |
| VLAN 40: e7 & e8  | e8 <=> e9 |
| VLAN 50: e9 & e10 |           |

The test PC is connected to e1 and e10 via different interfaces
(alternatively, two different PCs are used).

> [!TIP]
> Configuration here is done via console. When configuring remotely
> over SSH, remember to keep one IP address (the one used for the SSH
> connection)! I.e., set a static IP address first, then perform the
> VLAN configuration step.

## Configuration at Start

Starting out, we assume a configuration where all ports are network
interfaces (possibly with IPv6 enabled).

```
admin@example:/> show interfaces
lo              ethernet   UP          00:00:00:00:00:00
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
e1              ethernet   LOWER-DOWN  00:53:00:06:11:01
e2              ethernet   LOWER-DOWN  00:53:00:06:11:02
e3              ethernet   LOWER-DOWN  00:53:00:06:11:03
e4              ethernet   LOWER-DOWN  00:53:00:06:11:04
e5              ethernet   LOWER-DOWN  00:53:00:06:11:05
e6              ethernet   LOWER-DOWN  00:53:00:06:11:06
e7              ethernet   LOWER-DOWN  00:53:00:06:11:07
e8              ethernet   LOWER-DOWN  00:53:00:06:11:08
e9              ethernet   LOWER-DOWN  00:53:00:06:11:09
e10             ethernet   UP          00:53:00:06:11:0a
                ipv6                   fe80::0053:00ff:fe06:110a/64 (link-layer)
admin@example:/>
```

## Creating Bridge and Adding Ports

The following example [creates a bridge][8] and adds all Ethernet ports
to it.  On a device with layer-2 offloading (switch fabric), this sets
all ports in the same VLAN.  The next section sets up VLAN isolation.

```
admin@example:/> configure
admin@example:/config/> edit interface br0
admin@example:/config/interface/br0/> end
admin@example:/config/> set interface e1 bridge-port bridge br0
admin@example:/config/> set interface e2 bridge-port bridge br0
admin@example:/config/> set interface e3 bridge-port bridge br0
admin@example:/config/> set interface e4 bridge-port bridge br0
admin@example:/config/> set interface e5 bridge-port bridge br0
admin@example:/config/> set interface e6 bridge-port bridge br0
admin@example:/config/> set interface e7 bridge-port bridge br0
admin@example:/config/> set interface e8 bridge-port bridge br0
admin@example:/config/> set interface e9 bridge-port bridge br0
admin@example:/config/> set interface e10 bridge-port bridge br0
admin@example:/config/>
```

The interface status can be viewed using `show interfaces` after leaving
configuration context.  When configuring via SSH, first assign an IP
address to `br0` *before leaving* configuration context, e.g.

```
admin@example:/config/> set interface br0 ipv6 enabled
```

This enables IPv6 SLAAC, auto-configured address.  Or skip `leave` and
stay in configuration context until you have completed all the device
setup, including [setting IP address](#set-ip-address).

```
admin@example:/config/> leave
admin@example:/>
admin@example:/> show interfaces
INTERFACE       PROTOCOL   STATE       DATA
br0             bridge
│               ethernet   UP          00:53:00:06:11:01
├ e1            bridge     LOWER-DOWN
├ e2            bridge     LOWER-DOWN
├ e3            bridge     LOWER-DOWN
├ e4            bridge     LOWER-DOWN
├ e5            bridge     LOWER-DOWN
├ e6            bridge     LOWER-DOWN
├ e7            bridge     LOWER-DOWN
├ e8            bridge     LOWER-DOWN
├ e9            bridge     LOWER-DOWN
└ e10           bridge     FORWARDING
lo              ethernet   UP          00:00:00:00:00:00
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
admin@example:/>
```

## Assign VLANs to Ports

Now, configure VLANs as outlined [previously](#vlan-snake): default VID
for ingress (PVID), which is done per port, and egress mode (untagged),
which is done at the bridge level. See the [VLAN bridges][9] section for
more information.

```
admin@example:/>
admin@example:/> configure
admin@example:/config/> set interface e1 bridge-port pvid 10
admin@example:/config/> set interface e2 bridge-port pvid 10
admin@example:/config/> set interface e3 bridge-port pvid 20
admin@example:/config/> set interface e4 bridge-port pvid 20
admin@example:/config/> set interface e5 bridge-port pvid 30
admin@example:/config/> set interface e6 bridge-port pvid 30
admin@example:/config/> set interface e7 bridge-port pvid 40
admin@example:/config/> set interface e8 bridge-port pvid 40
admin@example:/config/> set interface e9 bridge-port pvid 50
admin@example:/config/> set interface e10 bridge-port pvid 50
admin@example:/config/> edit interface br0
admin@example:/config/interface/br0/> edit bridge vlans
admin@example:/config/interface/br0/bridge/vlans/> set vlan 10 untagged e1
admin@example:/config/interface/br0/bridge/vlans/> set vlan 10 untagged e2
admin@example:/config/interface/br0/bridge/vlans/> set vlan 20 untagged e3
admin@example:/config/interface/br0/bridge/vlans/> set vlan 20 untagged e4
admin@example:/config/interface/br0/bridge/vlans/> set vlan 30 untagged e5
admin@example:/config/interface/br0/bridge/vlans/> set vlan 30 untagged e6
admin@example:/config/interface/br0/bridge/vlans/> set vlan 40 untagged e7
admin@example:/config/interface/br0/bridge/vlans/> set vlan 40 untagged e8
admin@example:/config/interface/br0/bridge/vlans/> set vlan 50 untagged e9
admin@example:/config/interface/br0/bridge/vlans/> set vlan 50 untagged e10
admin@example:/config/interface/br0/bridge/vlans/> leave
admin@example:/>
```

Interface status would now should something like the following

```
admin@example:/> show interfaces
INTERFACE       PROTOCOL   STATE       DATA
br0             bridge
│               ethernet   UP          00:53:00:06:11:01
├ e1            bridge     LOWER-DOWN  vlan:10u pvid:10
├ e2            bridge     LOWER-DOWN  vlan:10u pvid:10
├ e3            bridge     LOWER-DOWN  vlan:20u pvid:20
├ e4            bridge     LOWER-DOWN  vlan:20u pvid:20
├ e5            bridge     LOWER-DOWN  vlan:30u pvid:30
├ e6            bridge     LOWER-DOWN  vlan:30u pvid:30
├ e7            bridge     LOWER-DOWN  vlan:40u pvid:40
├ e8            bridge     LOWER-DOWN  vlan:40u pvid:40
├ e9            bridge     LOWER-DOWN  vlan:50u pvid:50
└ e10           bridge     FORWARDING  vlan:50u pvid:50
lo              ethernet   UP          00:00:00:00:00:00
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
admin@example:/>
```

## Connect Cables and Test

We can now connect the PC to e1 and e10, while the other ports are
patched according to [above](#vlan-snake).  We should see link up and
*FORWARDING* on all ports in the bridge.

```
admin@example:/> show interfaces
INTERFACE       PROTOCOL   STATE       DATA
br0             bridge
│               ethernet   UP          00:53:00:06:11:01
├ e1            bridge     FORWARDING  vlan:10u pvid:10
├ e2            bridge     FORWARDING  vlan:10u pvid:10
├ e3            bridge     FORWARDING  vlan:20u pvid:20
├ e4            bridge     FORWARDING  vlan:20u pvid:20
├ e5            bridge     FORWARDING  vlan:30u pvid:30
├ e6            bridge     FORWARDING  vlan:30u pvid:30
├ e7            bridge     FORWARDING  vlan:40u pvid:40
├ e8            bridge     FORWARDING  vlan:40u pvid:40
├ e9            bridge     FORWARDING  vlan:50u pvid:50
└ e10           bridge     FORWARDING  vlan:50u pvid:50
lo              ethernet   UP          00:00:00:00:00:00
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
admin@example:/>
```

Here we use IPv6 ping all hosts (ff02::1) on PC interface eth1 to
check reachability to the other interface of the PC.

> [!TIP]
> We recommend using network namespaces (Linux only) on the PC to ensure
> that traffic is actually sent out to the switch, rather than being
> looped back internally. Alternatively, use two separate PCs.

```
~ $ ping -L ff02::1%eth1
PING ff02::1%eth1(ff02::1%eth1) 56 data bytes
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=1 ttl=64 time=0.496 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=2 ttl=64 time=0.514 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=3 ttl=64 time=0.473 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=4 ttl=64 time=0.736 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=5 ttl=64 time=0.563 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=6 ttl=64 time=0.507 ms
^C
--- ff02::1%eth1 ping statistics ---
6 packets transmitted, 6 received, 0% packet loss, time 5108ms
rtt min/avg/max/mdev = 0.473/0.548/0.736/0.088 ms
~ $
```

We can verify that traffic goes through the switch by disconnecting
one of the patch cables, e.g., between e4 and e5

```
~ $ ping -L ff02::1%eth1
PING ff02::1%eth1(ff02::1%eth1) 56 data bytes
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=1 ttl=64 time=0.510 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=2 ttl=64 time=0.448 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=3 ttl=64 time=0.583 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=4 ttl=64 time=0.515 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=5 ttl=64 time=0.521 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=6 ttl=64 time=0.495 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=7 ttl=64 time=0.743 ms
... Disconnecting patch cable, thus losing packets
... and reconnecting again. Connectivity resumes.
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=16 ttl=64 time=0.961 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=17 ttl=64 time=0.513 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=18 ttl=64 time=0.794 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=19 ttl=64 time=0.755 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=20 ttl=64 time=0.779 ms
^C
--- ff02::1%eth1 ping statistics ---
20 packets transmitted, 12 received, 40% packet loss, time 19432ms
rtt min/avg/max/mdev = 0.448/0.634/0.961/0.156 ms
~ $
```

## Set IP Address

The configuration so far does not provide a means to connect to the
switch management via SSH or NETCONF, as the switch has no IP address.
The example below shows how to add the switch to VLAN 10 (as used for
ports e1 and e2) and enables IPv6.

```
admin@example:/config/> edit interface vlan10
admin@example:/config/interface/vlan10/> set vlan lower-layer-if br0
admin@example:/config/interface/vlan10/> set ipv6 enabled
admin@example:/config/interface/vlan10/> show
type vlan;
ipv6 {
  enabled true;
}
vlan {
  tag-type c-vlan;
  id 10;
  lower-layer-if br0;
}
admin@example:/config/interface/vlan10/>
admin@example:/config/interface/vlan10/> end
admin@example:/config/> edit interface br0 bridge vlans
admin@example:/config/interface/br0/bridge/vlans/> set vlan 10 tagged br0
admin@example:/config/interface/br0/bridge/vlans/> leave
admin@example:/>
```

Interface *vlan10* with an auto-configured IPv6 address should appear.

```
admin@example:/> show interfaces
INTERFACE       PROTOCOL   STATE       DATA
br0             bridge                 vlan:10t
│               ethernet   UP          00:53:00:06:11:01
├ e1            bridge     FORWARDING  vlan:10u pvid:10
├ e2            bridge     FORWARDING  vlan:10u pvid:10
├ e3            bridge     FORWARDING  vlan:20u pvid:20
├ e4            bridge     FORWARDING  vlan:20u pvid:20
├ e5            bridge     FORWARDING  vlan:30u pvid:30
├ e6            bridge     FORWARDING  vlan:30u pvid:30
├ e7            bridge     FORWARDING  vlan:40u pvid:40
├ e8            bridge     FORWARDING  vlan:40u pvid:40
├ e9            bridge     FORWARDING  vlan:50u pvid:50
└ e10           bridge     FORWARDING  vlan:50u pvid:50
lo              ethernet   UP          00:00:00:00:00:00
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
vlan10          ethernet   UP          00:53:00:06:11:01
│               ipv6                   fe80::0053:00ff:fe06:1101/64 (link-layer)
└ br0           ethernet   UP          00:53:00:06:11:01
admin@example:/>
```

When pinging "IPv6 all hosts" from the PC, there should be two
responses for every ping, one from the switch and one from the PC
attached to e10.

```
~ $ ping -L ff02::1%eth1
PING ff02::1%eth1(ff02::1%eth1) 56 data bytes
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=1 ttl=64 time=0.508 ms
64 bytes from fe80::0053:00ff:fe06:1101%eth1: icmp_seq=1 ttl=64 time=0.968 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=2 ttl=64 time=0.866 ms
64 bytes from fe80::0053:00ff:fe06:1101%eth1: icmp_seq=2 ttl=64 time=0.867 ms
64 bytes from fe80::0053:00ff:fe06:1101%eth1: icmp_seq=3 ttl=64 time=0.467 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=3 ttl=64 time=0.469 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=4 ttl=64 time=0.452 ms
64 bytes from fe80::0053:00ff:fe06:1101%eth1: icmp_seq=4 ttl=64 time=0.453 ms
^C
--- ff02::1%eth1 ping statistics ---
4 packets transmitted, 4 received, +4 duplicates, 0% packet loss, time 3031ms
rtt min/avg/max/mdev = 0.452/0.631/0.968/0.211 ms
~ $
```

It should now be possible to access the switch from the PC via SSH (or NETCONF).

```
~ $ ssh admin@fe80::0053:00ff:fe06:1101%eth1
admin@fe80::0053:00ff:fe06:1101%eth1's password:
.-------.
|  . .  | Infix OS — Immutable.Friendly.Secure
|-. v .-| https://kernelkit.org
'-'---'-'

Run the command 'cli' for interactive OAM

admin@example:~$ exit
~ $
```

See previous sections on [backup][1] and [restore][2] of
the created configuration.

[1]: scripting-sysrepocfg.md#backup-configuration
[2]: scripting-sysrepocfg.md#restore-configuration
[8]: networking.md#bridging
[9]: networking.md#vlan-filtering-bridge
