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
