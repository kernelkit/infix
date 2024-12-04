# Discover Devices

Infix advertises itself via the [mDNS-SD](#mdns-sd) and [LLDP](#lldp)
discovery protocols.  mDNS-SD has good client support in Windows, macOS
and on Linux systems.  More on these protocols later.

An even simpler method is available when directly attached to an Infix
device:

```
       .----.      Ethernet       .-------.
       | PC +---------------------+ Infix |
       '----' if1              e1 '-------'
```

With IPv6 you can *ping the all-hosts* address (ff02::1), the device's
link-local IPv6 address is then seen in the response.  In the following
example, the PC here uses *tap0* as *if1*, Infix responds with address
*fe80::ff:fec0:ffed*.

```
linux-pc:# ping -6 -L -c 3 ff02::1%tap0
PING ff02::1%tap0(ff02::1%tap0) 56 data bytes
64 bytes from fe80::ff:fec0:ffed%tap0: icmp_seq=1 ttl=64 time=0.558 ms
64 bytes from fe80::ff:fec0:ffed%tap0: icmp_seq=2 ttl=64 time=0.419 ms
64 bytes from fe80::ff:fec0:ffed%tap0: icmp_seq=3 ttl=64 time=0.389 ms

--- ff02::1%tap0 ping statistics ---
3 packets transmitted, 3 received, 0% packet loss, time 2043ms
rtt min/avg/max/mdev = 0.389/0.455/0.558/0.073 ms
linux-pc:# 
```

> [!TIP]
> The `-L` option ignores local responses from the PC.

This address can then be used to connect to the device, e.g., using SSH.
Notice the syntax `username@address%interface`:

```
linux-pc:# ssh admin@fe80::ff:fec0:ffed%tap0
admin@fe80::ff:fec0:ffed%tap0's password: admin
admin@infix-c0-ff-ee:~$ 
```


## LLDP

Infix supports LLDP (IEEE 802.1AB).  For a device with factory default
settings, the link-local IPv6 address can be read from the Management
Address TLV using *tcpdump* or other sniffing tools[^1]:

```
linux-pc:# tcpdump -i tap0 -Qin -v ether proto 0x88cc
tcpdump: listening on tap0, link-type EN10MB (Ethernet), snapshot length 262144 bytes
15:51:52.061071 LLDP, length 193
    Chassis ID TLV (1), length 7
      Subtype MAC address (4): 02:00:00:c0:ff:ee (oui Unknown)
    Port ID TLV (2), length 7
      Subtype MAC address (3): 02:00:00:c0:ff:ee (oui Unknown)
    Time to Live TLV (3), length 2: TTL 120s
    System Name TLV (5), length 14: infix-c0-ff-ee
    System Description TLV (6), length 91
      Infix by KernelKit Linux 5.19.17 #1 SMP PREEMPT_DYNAMIC Wed Jun 7 08:47:23 CEST 2023 x86_64
    System Capabilities TLV (7), length 4
      System  Capabilities [Bridge, WLAN AP, Router, Station Only] (0x009c)
      Enabled Capabilities [Station Only] (0x0080)
    Management Address TLV (8), length 24
      Management Address length 17, AFI IPv6 (2): fe80::ff:fec0:ffed
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

If the device has an IPv4 address assigned, it is shown in an additional
Management Address TLV.

> [!NOTE]
> The Management Addresses shown by LLDP are not necessarily associated
> with the port transmitting the LLDP message.

In the example below, the IPv4 address (10.0.1.1) happens to be
assigned to *eth0*, while the IPv6 address (2001:db8::1) is not.

```
linux-pc:# sudo tcpdump -i tap0 -Qin -v ether proto 0x88cc
tcpdump: listening on tap0, link-type EN10MB (Ethernet), snapshot length 262144 bytes
15:46:07.908665 LLDP, length 207
    Chassis ID TLV (1), length 7
      Subtype MAC address (4): 02:00:00:c0:ff:ee (oui Unknown)
    Port ID TLV (2), length 7
      Subtype MAC address (3): 02:00:00:c0:ff:ee (oui Unknown)
    Time to Live TLV (3), length 2: TTL 120s
    System Name TLV (5), length 14: infix-c0-ff-ee
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

The LLDP service can be disabled using the following commands.

```
admin@infix-c0-ff-ee:/> configure
admin@infix-c0-ff-ee:/config/> no lldp 
admin@infix-c0-ff-ee:/config/> leave
admin@infix-c0-ff-ee:/> 
```


## mDNS-SD

DNS-SD/mDNS-SD can be used to discover Infix devices and services.  By
default, Infix use the `.local` domain for advertising services.  Some
networks use `.lan` instead, so this configurable:

```
admin@infix-c0-ff-ee:/> configure
admin@infix-c0-ff-ee:/config/> edit mdns
admin@infix-c0-ff-ee:/config/mdns/> set domain lan
```

Other available settings include limiting the interfaces mDNS responder
acts on:

```
admin@infix-c0-ff-ee:/config/> set interfaces allow e1
```

or

```
admin@infix-c0-ff-ee:/config/> set interfaces deny wan
```

The `allow` and `deny` settings are complementary, `deny` always wins.

----

In Linux, tools such as *avahi-browse* or *mdns-scan*[^2] can be used to
search for devices advertising their services via mDNS.

```
linux-pc:# avahi-browse -ar
+   tap0 IPv6 infix-c0-ff-ee                                SFTP File Transfer   local
+   tap0 IPv4 infix-c0-ff-ee                                SFTP File Transfer   local
+   tap0 IPv6 infix-c0-ff-ee                                SSH Remote Terminal  local
+   tap0 IPv4 infix-c0-ff-ee                                SSH Remote Terminal  local
=   tap0 IPv4 infix-c0-ff-ee                                SFTP File Transfer   local
   hostname = [infix-c0-ff-ee.local]
   address = [10.0.1.1]
   port = [22]
   txt = []
=   tap0 IPv4 infix-c0-ff-ee                                SSH Remote Terminal  local
   hostname = [infix-c0-ff-ee.local]
   address = [10.0.1.1]
   port = [22]
   txt = []
=   tap0 IPv6 infix-c0-ff-ee                                SFTP File Transfer   local
   hostname = [infix-c0-ff-ee.local]
   address = [fe80::ff:fec0:ffed]
   port = [22]
   txt = []
=   tap0 IPv6 infix-c0-ff-ee                                SSH Remote Terminal  local
   hostname = [infix-c0-ff-ee.local]
   address = [fe80::ff:fec0:ffed]
   port = [22]
   txt = []
^C
linux-pc:#
```

> [!TIP]
> The `-t` option is also very useful, it stops browsing automatically
> when a "more or less complete list" has been printed.  However, some
> devices on the LAN may be in deep sleep so run the command again if
> you cannot find the device you are looking for.

Additionally, *avahi-resolve-host-name* can be used to verify domain
name mappings for IP addresses.  By default, it translates from IPv4
addresses.  This function allows users to confirm that addresses are
mapped correctly.

```
linux-pc:# avahi-resolve-host-name infix-c0-ff-ee.local
infix-c0-ff-ee.local	10.0.1.1
linux-pc:#
```

Thanks to mDNS we can use the advertised name instead of the IP
address for operations like `ping` and `ssh` as shown below:

```
linux-pc:# ping infix-c0-ff-ee.local -c 3
PING infix-c0-ff-ee.local (10.0.1.1) 56(84) bytes of data.
64 bytes from 10.0.1.1: icmp_seq=1 ttl=64 time=0.852 ms
64 bytes from 10.0.1.1: icmp_seq=2 ttl=64 time=1.12 ms
64 bytes from 10.0.1.1: icmp_seq=3 ttl=64 time=1.35 ms

--- infix-c0-ff-ee.local ping statistics ---
3 packets transmitted, 3 received, 0% packet loss, time 2003ms
rtt min/avg/max/mdev = 0.852/1.105/1.348/0.202 ms

linux-pc:# ssh admin@infix-c0-ff-ee.local
(admin@infix-c0-ff-ee.local) Password: 
.-------.
|  . .  | Infix -- a Network Operating System
|-. v .-| https://kernelkit.org
'-'---'-

Run the command 'cli' for interactive OAM

linux-pc:#
```

To disable mDNS/mDNS-SD, type the commands:

```
admin@infix-c0-ff-ee:/> configure 
admin@infix-c0-ff-ee:/config/> no mdns
admin@infix-c0-ff-ee:/config/> leave
```

### Human-Friendly Hostname Alias

Each Infix deviuce advertise itself as *infix.local*, in addition to its
full hostname (e.g., *infix-c0-ff-ee.local* or *foo.local*).  This alias
works seamlessly on a network with a single Infix device, and makes it
easy to connect when the exact hostname is not known in advance.  The
examples below show how the alias can be used for actions such as
pinging or establishing an SSH connection:

```
linux-pc:# ping infix.local -c 3
PING infix.local (10.0.1.1) 56(84) bytes of data.
64 bytes from 10.0.1.1: icmp_seq=1 ttl=64 time=0.751 ms
64 bytes from 10.0.1.1: icmp_seq=2 ttl=64 time=2.28 ms
64 bytes from 10.0.1.1: icmp_seq=3 ttl=64 time=1.42 ms

--- infix.local ping statistics ---
3 packets transmitted, 3 received, 0% packet loss, time 2003ms
rtt min/avg/max/mdev = 0.751/1.482/2.281/0.626 ms

linux-pc:# ssh admin@infix.local
(admin@infix.local) Password: 
.-------.
|  . .  | Infix -- a Network Operating System
|-. v .-| https://kernelkit.org
'-'---'-

Run the command 'cli' for interactive OAM

admin@infix-c0-ff-ee:~$
```

When multiple Infix devices are present on the LAN the alias will not
uniquely identify a device; *infix.local* will refer to any of the
Infix devices, likely the one that first appeared.

> [!NOTE]
> When multiple Infix devices are present on the LAN, use the full name,
> e.g., *infix-c0-ff-ee.local* or *foo.local* rather than the alias
> *infix.local* to deterministically connect to the device.


### Browse Network Using *network.local*

Another mDNS alias that all Infix devices advertise is *network.local*.
This is a web service which basically runs `avahi-browse` and displays a
table of other Infix devices and their services.

![Netbrowse Service - network.local](img/network-local.png)

With multiple Infix devices on the LAN, one will take the role of your
portal to access all others, if it goes down another takes its place.

To disable the netbrowse service, and the *network.local* alias, the
following commands can be used:

```
admin@infix-c0-ff-ee:/> configure 
admin@infix-c0-ff-ee:/config/> edit web
admin@infix-c0-ff-ee:/config/web/> no netbrowse
admin@infix-c0-ff-ee:/config/web/> leave
```


[^1]: E.g., [lldpd](https://github.com/lldp/lldpd) which includes the
    *lldpcli* too, handy to sniff and display LLDP packets.
[^2]: [mdns-scan](http://0pointer.de/lennart/projects/mdns-scan/): a
    tool for scanning for mDNS/DNS-SD services on the local network.
