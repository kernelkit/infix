# Tunnel configuration

Tunnel traffic from point A to point B


## Generic Routing Encapsulation (GRE)

The support for GRE tunnels includes IPv4 and IPv6 tunnels both in GRE
(IP) and GRETAP (MAC) modes.
```
admin@example:/config/> edit interface gre1
admin@example:/config/interface/gre1/> set type gretap
admin@example:/config/interface/gre1/> set gre local 192.168.3.1 remote 192.168.3.2
admin@example:/config/interface/gre1/> leave
admin@example:/>
```

##  Virtual eXtensible Local Area Network (VXLAN)

The support for VXLAN tunnels includes IPv4 and IPv6.

```
admin@example:/config/> edit interface vxlan100
admin@example:/config/interface/vxlan100/> set vxlan local 192.168.3.1
admin@example:/config/interface/vxlan100/> set vxlan remote 192.168.3.2
admin@example:/config/interface/vxlan100/> set vxlan vni 100
admin@example:/config/interface/vxlan100/> leave
```
