# Device Discovery

Infix advertises itself via the [mDNS-SD](#mdns-sd) and [LLDP](#lldp)
discovery protocols.  mDNS-SD has good client support in Windows, macOS
and on Linux systems.  More on these protocols later.

An even simpler method is available when directly attached to an Infix
device:

```
.----.       Ethernet      .--------.
| PC +---------------------+ Device |
'----' if1              e1 '--------'
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

### Windows Compatibility

> [!NOTE]
> **Testing Note (2023-2024):** The information below is based on user testing
> performed on Windows 10 and early Windows 11 builds. This has **not** been
> independently verified with recent Windows versions or through external research.
> 
> **We strongly encourage you to:**
> 1. Test IPv6 multicast ping on your current Windows version first
> 2. Report your findings (working/not working, Windows version, build number)
> 3. Share any workarounds you discover
>
> Newer Windows updates may have resolved these issues entirely.

On some Windows systems (based on 2023-2024 reports), IPv6 multicast ping
(e.g., `ping ff02::1%interface`) has had issues where the Infix device
responds correctly but Windows does not display the response at the command
line.

While the ping packets are transmitted and the Infix device responds as
expected, Windows firewall or network stack settings may prevent the response
from being visible in the command prompt. The responses can be verified using
network packet capture tools like Wireshark.

#### Tested Workarounds (Historical)

The following approaches were tested by users on Windows 10 (2023-2024) but
**reportedly did not** resolve the issue:

- Enabling network discovery on Public and Domain profiles
- Disabling all network interfaces except the connection to the Infix device
- Changing the network profile from Public to Private
- Enabling network discovery and file sharing for the Public profile

These limitations appeared to be related to Windows IPv6 implementation and
firewall behavior when handling link-local multicast responses.

> [!TIP]
> **Try IPv6 ping first!** If you're on a recent Windows 11 update or
> newer Windows 10 build, IPv6 multicast ping may work correctly on your
> system. Test with `ping ff02::1%interface` before assuming it won't work.

#### Recommended Alternative: Use mDNS

**If IPv6 multicast ping doesn't work on your Windows system, use mDNS as a
reliable alternative** (described in detail in the [mDNS-SD](#mdns-sd)
section below). Windows 10 (build 1709+) and Windows 11 have native mDNS-SD
support, making this a reliable discovery method:

```cmd
C:\> ping infix-c0-ff-ee.local
C:\> ping infix.local
C:\> ssh admin@infix-c0-ff-ee.local
```

For networks with a single Infix device, the convenient `infix.local` alias
works seamlessly. On networks with multiple devices, use the full hostname
like `infix-c0-ff-ee.local` or the MAC-based name like `switch-c0-ff-ee.local`.

#### Diagnostic Verification

> [!TIP]
> If IPv6 multicast ping doesn't show responses at the command line but
> you want to verify that the Infix device is responding, use Wireshark
> or another packet capture tool while running the ping command. This
> shows whether the issue is with Windows display or actual connectivity.
> As shown in the image below, you can see MDNS, LLMNR, and ICMPv6 echo
> (ping) traffic being exchanged even when Windows doesn't display it.

![Wireshark showing IPv6 ping responses](https://github.com/addiva-elektronik/alder/assets/122900029/c45d7726-448f-4c30-878e-bcf976dff531)

#### Community Feedback Needed

> [!WARNING]
> **This information needs verification!** The details above are based on
> user reports from 2023-2024 and have **not** been independently researched
> or verified with recent Windows versions.
>
> **Please help us improve this documentation:**
> - If you're using Windows 10 (specify build) or Windows 11 (specify version)
> - Test `ping -6 ff02::1%<your-interface>` and report if it works or not
> - Share your Windows version: run `winver` or `systeminfo | findstr /B /C:"OS"`
> - Create an issue or PR to update this section with your findings
>
> Your feedback helps keep this documentation accurate and useful!

For historical context on Windows IPv6 behavior, see these resources:

- [Windows doesn't respond to IPv6 multicast ping](https://superuser.com/questions/490092/windows-doesnt-respond-to-ipv6-multicast-ping)
- [Find device IPv6 link-local](https://serverless.industries/2019/05/30/find-device-ipv6-link-local.en.html)
- [IPv6 Ping Scan from Windows](https://samsclass.info/ipv6/proj/proj-PingScan-Win.html)
- [Hacking IPv6 from Windows](https://medium.com/@netscylla/hacking-ipv6-from-windows-ca23a9602ce7)
- [Link-Local Multicast Name Resolution (LLMNR)](https://rakhesh.com/windows/resolving-names-using-link-local-multicast-name-resolution-llmnr/)


## LLDP

Infix supports LLDP (IEEE 802.1AB). For a device with factory default
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

The following capabilities are available via NETCONF/RESTCONF or the Infix CLI.

### LLDP Enable/Disable

The LLDP service can be disabled using the following commands.

```
admin@infix-c0-ff-ee:/> configure
admin@infix-c0-ff-ee:/config/> no lldp 
admin@infix-c0-ff-ee:/config/> leave
admin@infix-c0-ff-ee:/> 
```

To reenable it from the CLI config mode:

```
admin@test-00-01-00:/config/> set lldp enabled 
admin@test-00-01-00:/config/> leave
```

### LLDP Message Transmission Interval

By default, LLDP uses a `message-tx-interval` of 30 seconds, as defined
by the IEEE standard. Infix allows this value to be customized.
To change it using the CLI:

```
admin@test-00-01-00:/config/> set lldp message-tx-interval 1
admin@test-00-01-00:/config/> leave
```

### LLDP Administrative Status per Interface

Infix supports configuring the LLDP administrative status on a per-port
basis. The default mode is `tx-and-rx`, but the following options are 
also supported:

- `rx-only` – Receive LLDP packets only
- `tx-only` – Transmit LLDP packets only
- `disabled` – Disable LLDP on the interface

Example configuration:

```
admin@test-00-01-00:/config/> set lldp port e8 dest-mac-address 01:80:C2:00:00:0E admin-status disabled
admin@test-00-01-00:/config/> set lldp port e5 dest-mac-address 01:80:C2:00:00:0E admin-status rx-only 
admin@test-00-01-00:/config/> set lldp port e6 dest-mac-address 01:80:C2:00:00:0E admin-status tx-only
admin@test-00-01-00:/config/> leave
```

> [!NOTE]
> The destination MAC address must be the standard LLDP multicast 
> address: `01:80:C2:00:00:0E`.

###  Displaying LLDP Neighbor Information

In CLI mode, Infix also provides a convenient `show lldp` command to
list LLDP neighbors detected on each interface:

```
admin@test-00-01-00:/> show lldp 
INTERFACE       REM-IDX   TIME        CHASSIS-ID          PORT-ID             
e5              1         902         00:a0:85:00:04:01   00:a0:85:00:04:07   
e6              3         897         00:a0:85:00:03:01   00:a0:85:00:03:07   
e8              2         901         00:a0:85:00:02:01   00:a0:85:00:02:05
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
|  . .  | Infix OS — Immutable.Friendly.Secure
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
|  . .  | Infix OS — Immutable.Friendly.Secure
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
