DHCP Server
===========

The DHCPv4 server provides automatic IP address assignment and network
configuration for clients.  It supports address pools, static host
assignments, and customizable DHCP options.  It also serves as a DNS
proxy for local subnets and can even forward queries to upstream DNS
servers[^1].

> [!NOTE]
> When using the CLI, the system automatically enables essential options
> like DNS servers and default gateway based on the system's network
> configuration.  These options can be disabled, changed or overridden,
> at any level: global, subnet, or per-host.


## Basic Configuration

The following example configures a DHCP server for subnet 192.168.2.0/24
with an address pool:

```
admin@example:/> configure
admin@example:/config/> edit dhcp-server
admin@example:/config/dhcp-server/> edit subnet 192.168.2.0/24
admin@example:/config/dhcp-server/subnet/192.168.2.0/24/> set pool start-address 192.168.2.100 end-address 192.168.2.200
admin@example:/config/dhcp-server/subnet/192.168.2.0/24/> leave
```

When setting up the server from the CLI, the system automatically adds a
few default DHCP options that will be sent to clients: both DNS server
and default gateway will use the system address on the matching
interface.

```
admin@example:/> show running-config
  "infix-dhcp-server:dhcp-server": {
    "subnet": [
      {
        "subnet": "192.168.2.0/24",
        "option": [
          {
            "id": "dns-server",
            "address": "auto"
          },
          {
            "id": "router",
            "address": "auto"
          }
        ],
        "pool": {
          "start-address": "192.168.2.100",
          "end-address": "192.168.2.200"
        }
      }
    ]
  }
```

> [!IMPORTANT]
> Remember to set up an interface in this subnet, avoid using addresses
> in the DHCP pool, or reserved for static hosts.  In Class C networks
> the router usually has address `.1`.  Depending on the use-case, you
> may also want to set up routing.


## Static Host Assignment

To reserve specific IP addresses for clients based on their MAC address,
hostname, or client ID:

```
admin@example:/config/dhcp-server/subnet/192.168.2.0/24/> edit host 192.168.2.10
admin@example:/config/dhcp-server/subnet/192.168.2.0/24/host/192.168.2.10/> set match mac-address 00:11:22:33:44:55
admin@example:/config/dhcp-server/subnet/192.168.2.0/24/host/192.168.2.10/> set hostname printer
admin@example:/config/dhcp-server/subnet/192.168.2.0/24/host/192.168.2.10/> leave
```

Match hosts using a client identifier instead of MAC address:

```
admin@example:/config/dhcp-server/subnet/192.168.1.0/24/> edit host 192.168.1.50
admin@example:/config/dhcp-server/subnet/192.168.1.0/24/host/192.168.1.50/> edit match
admin@example:/config/dhcp-server/subnet/192.168.1.0/24/host/192.168.1.50/match/> set client-id hex c0:ff:ee
admin@example:/config/dhcp-server/subnet/192.168.1.0/24/host/192.168.1.50/match/> leave
admin@example:/config/dhcp-server/subnet/192.168.1.0/24/host/192.168.1.50/> set lease-time infinite
admin@example:/config/dhcp-server/subnet/192.168.1.0/24/host/192.168.1.50/> leave
```

The `hex` prefix here ensures matching of client ID is done using the
hexadecimal octets `c0:ff:ee`, three bytes.  Without the prefix the
ASCII string "c0:ff:ee", eight bytes, is used.

> [!NOTE]
> The DHCP server is fully RFC conformant, in the case of option 61 this
> means that using the `hex` prefix will require the client to set the
> `htype` field of the option to `00`.  See RFC 2132 for details.


## Custom DHCP Options

Configure additional DHCP options globally, per subnet, or per host:

```
admin@example:/config/dhcp-server/> edit subnet 192.168.2.0/24
admin@example:/config/dhcp-server/subnet/192.168.2.0/24/> edit option dns-server
admin@example:/config/dhcp-server/subnet/192.168.2.0/24/option/dns-server/> set address 8.8.8.8
admin@example:/config/dhcp-server/subnet/192.168.2.0/24/option/dns-server/> leave
admin@example:/config/dhcp-server/subnet/192.168.2.0/24/> edit option ntp-server
admin@example:/config/dhcp-server/subnet/192.168.2.0/24/option/ntp-server/> set address 192.168.2.1
admin@example:/config/dhcp-server/subnet/192.168.2.0/24/option/ntp-server/> leave
```

When configuring, e.g., `dns-server`, or `router` options with the value
`auto`, the system uses the IP address from the interface matching the
subnet.  For example:

```
admin@example:/> show interfaces brief
Interface    Status  Address
eth0         UP      192.168.1.1/24
eth1         UP      192.168.2.1/24

admin@example:/config/dhcp-server/subnet/192.168.1.0/24/> edit option dns-server
admin@example:/config/dhcp-server/subnet/192.168.1.0/24/option/dns-server/> set address auto
admin@example:/config/dhcp-server/subnet/192.168.1.0/24/option/dns-server/> leave
```

In this case, clients in subnet 192.168.1.0/24 will receive 192.168.1.1
as their DNS server address.


## Multiple Subnets

Configure DHCP for multiple networks:

```
admin@example:/> configure
admin@example:/config/> edit dhcp-server
admin@example:/config/dhcp-server/> edit subnet 192.168.1.0/24
admin@example:/config/dhcp-server/subnet/192.168.1.0/24/> set pool start-address 192.168.1.100 end-address 192.168.1.200
admin@example:/config/dhcp-server/subnet/192.168.1.0/24/> leave
admin@example:/config/dhcp-server/> edit subnet 192.168.2.0/24
admin@example:/config/dhcp-server/subnet/192.168.2.0/24/> set pool start-address 192.168.2.100 end-address 192.168.2.200
admin@example:/config/dhcp-server/subnet/192.168.2.0/24/> leave
```


## Monitoring

View active leases and server statistics:

```
admin@example:/> show dhcp-server
IP ADDRESS       MAC                HOSTNAME            CLIENT ID             EXPIRES
192.168.2.22     00:a0:85:00:02:05                      00:c0:ff:ee           3591s
192.168.1.11     00:a0:85:00:04:06  foo                 01:00:a0:85:00:04:06  3591s

admin@example:/> show dhcp-server statistics
DHCP offers sent                : 6
DHCP ACK messages sent          : 5
DHCP NAK messages sent          : 0
DHCP decline messages received  : 0
DHCP discover messages received : 6
DHCP request messages received  : 5
DHCP release messages received  : 6
DHCP inform messages received   : 6
```


[^1]: This requires the system DNS resolver to be configured.
