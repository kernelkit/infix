## Usage

```
netcalc <ADDRESS/LEN | NETWORK NETMASK> [split <1-32 | 64-128>]
```

## Description

`netcalc` is a network calculator that takes an IP address and a subnet
mask, or an IP address and prefix mask in CIDR notation, and then returns
information about the subnet.  Both IPv4 and IPv6 is supported.

A subnet can be entered in two ways:

 - `192.168.2.0 255.255.255.0`: traditional IPv4 'address netmask' style
 - `192.168.2.0/24`: modern prefix length, same also for IPv6

An optional `split LEN` can be given as argument, the new length value
must be bigger than the current prefix length.  See example below.


## Examples

Its most commonly used features are to understand how many addresses an
IP subnet has, what the broadcast address is, first and last *usable*
address.

```
admin@example:/> netcalc 192.168.2.0/24
Address  : 192.168.2.0          11000000.10101000.00000010. 00000000
Netmask  : 255.255.255.0 = 24   11111111.11111111.11111111. 00000000
Wildcard : 0.0.0.255            00000000.00000000.00000000. 11111111
=>
Network  : 192.168.2.0/24       11000000.10101000.00000010. 00000000
HostMin  : 192.168.2.1          11000000.10101000.00000010. 00000001
HostMax  : 192.168.2.254        11000000.10101000.00000010. 11111110
Broadcast: 192.168.2.255        11000000.10101000.00000010. 11111111
Hosts/Net: 254                   Class C, Private network (RFC1918)
```

Another common use-case is for IP subnetting, i.e., using only as many
addresses for an IP subnet as needed.  Example, to split the above /24
in four:

```
admin@example:/> netcalc 192.168.2.0/24 split 26
Address  : 192.168.2.0          11000000.10101000.00000010. 00000000
Netmask  : 255.255.255.0 = 24   11111111.11111111.11111111. 00000000
Wildcard : 0.0.0.255            00000000.00000000.00000000. 11111111
=>
Network  : 192.168.2.0/24       11000000.10101000.00000010. 00000000
HostMin  : 192.168.2.1          11000000.10101000.00000010. 00000001
HostMax  : 192.168.2.254        11000000.10101000.00000010. 11111110
Broadcast: 192.168.2.255        11000000.10101000.00000010. 11111111
Hosts/Net: 254                   Class C, Private network (RFC1918)

[Split network/26]
Network  : 192.168.2.0   - 192.168.2.63     Netmask  : 255.255.255.192
Network  : 192.168.2.64  - 192.168.2.127    Netmask  : 255.255.255.192
Network  : 192.168.2.128 - 192.168.2.191    Netmask  : 255.255.255.192
Network  : 192.168.2.192 - 192.168.2.255    Netmask  : 255.255.255.192
```
