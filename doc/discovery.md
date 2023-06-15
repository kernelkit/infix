Discover Infix Units
====================

```
 .----.      Ethernet       .-------.
 | PC +---------------------+ Infix |
 '----' if1            eth0 '-------'
```
Figure 1: PC directly connected over Ethernet to Infix unit (here eth0).


When you wish to discover the IP address of an Infix switch, the simplest
way is probably to *ping the IPv6 all-hosts* address (ff02::1) over a
directly connected Ethernet cable. The unit's link-local IPv6 address is
seen in the response.

In the example below, the PC is connected to Infix via interface *tap0*
(*tap0* is *if1* in Figure 1) and Infix responds with address
*fe80::ff:fe00:0*.

```
linux-pc:# ping -6 -L -c 3 ff02::1%tap0
PING ff02::1%tap0(ff02::1%tap0) 56 data bytes
64 bytes from fe80::ff:fe00:0%tap0: icmp_seq=1 ttl=64 time=0.558 ms
64 bytes from fe80::ff:fe00:0%tap0: icmp_seq=2 ttl=64 time=0.419 ms
64 bytes from fe80::ff:fe00:0%tap0: icmp_seq=3 ttl=64 time=0.389 ms

--- ff02::1%tap0 ping statistics ---
3 packets transmitted, 3 received, 0% packet loss, time 2043ms
rtt min/avg/max/mdev = 0.389/0.455/0.558/0.073 ms
linux-pc:# 
```

The PC could connect then connect to Infix, e.g., using SSH.

```
linux-pc:# ssh admin@fe80::ff:fe00:0%tap0
admin@fe80::ff:fe00:0%tap0's password: admin
admin@infix-00-00-00:~$ 
```

## Discovery mechanisms available in Infix

Infix advertises its presence via the [mDNS](#mdns) and [SSDP](#ssdp) discovery
protocols in addition to [LLDP](#lldp).

### LLDP

Infix supports LLDP (IEEE 802.1AB). For a unit with factory default
settings, the PC can readout the link-local IPv6 address from the
Management Address TLV using *tcpdump* or other sniffing tools[^1].
```
linux-pc:# tcpdump -i tap0 -Qin -v ether proto 0x88cc
tcpdump: listening on tap0, link-type EN10MB (Ethernet), snapshot length 262144 bytes
15:51:52.061071 LLDP, length 193
    Chassis ID TLV (1), length 7
      Subtype MAC address (4): 02:00:00:00:00:00 (oui Unknown)
    Port ID TLV (2), length 7
      Subtype MAC address (3): 02:00:00:00:00:00 (oui Unknown)
    Time to Live TLV (3), length 2: TTL 120s
    System Name TLV (5), length 14: infix-00-00-00
    System Description TLV (6), length 91
      Infix by KernelKit Linux 5.19.17 #1 SMP PREEMPT_DYNAMIC Wed Jun 7 08:47:23 CEST 2023 x86_64
    System Capabilities TLV (7), length 4
      System  Capabilities [Bridge, WLAN AP, Router, Station Only] (0x009c)
      Enabled Capabilities [Station Only] (0x0080)
    Management Address TLV (8), length 24
      Management Address length 17, AFI IPv6 (2): fe80::ff:fe00:0
      Interface Index Interface Numbering (2): 2
    Port Description TLV (4), length 4: eth0
    Organization specific TLV (127), length 9: OUI IEEE 802.3 Private (0x00120f)
      Link aggregation Subtype (3)
        aggregation status [supported], aggregation port ID 0
    Organization specific TLV (127), length 9: OUI IEEE 802.3 Private (0x00120f)
      MAC/PHY configuration/status Subtype (1)
        autonegotiation [none] (0x00)
        PMD autoneg capability [unknown] (0x8000)
        MAU type Unknown (0x0000)
    End TLV (0), length 0
^C
1 packet captured
linux-pc:# 
```

If the unit has an IPv4 address assigned, it is shown in an additional
Management Address TLV.

> **Note** The Management Addresses shown by LLDP are not
> necessarily associated with the port transmitting the LLDP message. 

In the example below, the IPv4 address (10.0.1.1) happens to be
assigned to *eth0*, while the IPv6 address (2001:db8::1) is not.

```
linux-pc:# sudo tcpdump -i tap0 -Qin -v ether proto 0x88cc
tcpdump: listening on tap0, link-type EN10MB (Ethernet), snapshot length 262144 bytes
15:46:07.908665 LLDP, length 207
    Chassis ID TLV (1), length 7
      Subtype MAC address (4): 02:00:00:00:00:00 (oui Unknown)
    Port ID TLV (2), length 7
      Subtype MAC address (3): 02:00:00:00:00:00 (oui Unknown)
    Time to Live TLV (3), length 2: TTL 120s
    System Name TLV (5), length 14: infix-00-00-00
    System Description TLV (6), length 91
      Infix by KernelKit Linux 5.19.17 #1 SMP PREEMPT_DYNAMIC Wed Jun 7 08:47:23 CEST 2023 x86_64
    System Capabilities TLV (7), length 4
      System  Capabilities [Bridge, WLAN AP, Router, Station Only] (0x009c)
      Enabled Capabilities [Station Only] (0x0080)
    Management Address TLV (8), length 12
      Management Address length 5, AFI IPv4 (1): 10.0.1.1
      Interface Index Interface Numbering (2): 2
    Management Address TLV (8), length 24
      Management Address length 17, AFI IPv6 (2): 2001:db8::1
      Interface Index Interface Numbering (2): 3
    Port Description TLV (4), length 4: eth0
    Organization specific TLV (127), length 9: OUI IEEE 802.3 Private (0x00120f)
      Link aggregation Subtype (3)
        aggregation status [supported], aggregation port ID 0
    Organization specific TLV (127), length 9: OUI IEEE 802.3 Private (0x00120f)
      MAC/PHY configuration/status Subtype (1)
        autonegotiation [none] (0x00)
        PMD autoneg capability [unknown] (0x8000)
        MAU type Unknown (0x0000)
    End TLV (0), length 0
^C
1 packet captured
2 packets received by filter
0 packets dropped by kernel
linux-pc:#
```

[^1]: [lldpd: implementation of IEEE 802.1ab
    (LLDP)](https://github.com/lldp/lldpd) includes *lldpcli*, which
    is handy to sniff and display LLDP packets.

### mDNS

DNS-SD/mDNS can be used to discover Infix units and services. Infix
units present their IP addresses, services and hostname within the
.local domain. This method has good client support in Apple and Linux
systems. On Linux, tools such as *avahi-browse* or *mdns-scan*[^2] can
be used to search for devices advertising their services via mDNS.

```
linux-pc:# avahi-browse -ar
+   tap0 IPv6 infix-00-00-00                                SFTP File Transfer   local
+   tap0 IPv4 infix-00-00-00                                SFTP File Transfer   local
+   tap0 IPv6 infix-00-00-00                                SSH Remote Terminal  local
+   tap0 IPv4 infix-00-00-00                                SSH Remote Terminal  local
=   tap0 IPv4 infix-00-00-00                                SFTP File Transfer   local
   hostname = [infix-00-00-00.local]
   address = [10.0.1.1]
   port = [22]
   txt = []
=   tap0 IPv4 infix-00-00-00                                SSH Remote Terminal  local
   hostname = [infix-00-00-00.local]
   address = [10.0.1.1]
   port = [22]
   txt = []
=   tap0 IPv6 infix-00-00-00                                SFTP File Transfer   local
   hostname = [infix-00-00-00.local]
   address = [fe80::ff:fe00:0]
   port = [22]
   txt = []
=   tap0 IPv6 infix-00-00-00                                SSH Remote Terminal  local
   hostname = [infix-00-00-00.local]
   address = [fe80::ff:fe00:0]
   port = [22]
   txt = []
^C
linux-pc:#
```
[^2]: [mdns-scan](http://0pointer.de/lennart/projects/mdns-scan/): a
    tool for scanning for mDNS/DNS-SD published services on the local
    network

### SSDP

For Windows clients, Infix advertises itself via the SSDP
protocol. The Infix unit will appear as a *Network Infrastructure* 
equipment icon in the *Network* tab o Windows Explorer.

In Linux, the *ssdp-scan*[^3] tool be used to find Infix units via
SSDP.

```
linux-pc:# ssdp-scan tap0
+ infix-00-00-00                            http://10.0.1.1
linux-pc:# 
```

> Note 1: Infix presents itself with a HTTP URL, however, currently no
> Web server is running. Still, the IP address 10.0.1.1 is discovered
> and can be used for SSH access, etc.

> Note 2: SSDP is limited to IPv4. Thus, it is only valid as discovery
> mechanism when Infix as well as the client PC has an IPv4 address
> assigned.

[^3]: [SSDP Responder for
    Linux/UNIX](https://github.com/troglobit/ssdp-responder)
