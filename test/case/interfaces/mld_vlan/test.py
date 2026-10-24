#!/usr/bin/env python3
r"""MLD VLAN

Verify MLD querier election and group learning per VLAN on a VLAN
filtering bridge, and that VLAN separation is respected for IPv6
multicast.

....
 vlan55 2001:db8:55::1                     vlan55 2001:db8:55::2
 vlan77 2001:db8:77::1                     vlan77 2001:db8:77::2
           .--------------.  VLAN 55,77 T  .---------------.
           | DUT1    link +----------------+ link     DUT2 |
           '-+----------+-'                '-+-----------+-'
       data1 | 55U  77U | data2        data1 | 55U   77U | data2
             |          |                    |           |
        .----+--.  .----+-----.         .----+-----.  .--+----.
        | msend |  | !memb    |         | mreceive |  | msend |
        '-------'  '----------'         '----------'  '-------'
          ::11       ::11                  ::22          ::22
....

Both DUTs have IPv6 enabled on their VLAN interfaces, so in each VLAN
they must agree on one MLD querier, the one with the lowest link-local
address.  The other DUT receives the queries on its link port, and
learns it as a multicast router port in that VLAN.

A sender in VLAN 55 on DUT1 sends to ff2e::55 and a sender
in VLAN 77 on DUT2 sends to ff2e::77.  Each group is flooded within its
own VLAN, and never leaks into the other.  Once the receiver in VLAN 55
on DUT2 joins ff2e::55, DUT2 learns the group on that port.
"""

import infamy
import infamy.iface as iface
import infamy.multicast as mcast
from infamy.util import until, parallel

GROUP55 = "ff2e::55"
GROUP77 = "ff2e::77"
query_interval = 4


def bridge(data1, data2, link, last):
    def vlan(vid, port):
        return {
            "vid": vid,
            "untagged": [port],
            "tagged": [link, "br0"],
            "multicast": {
                "snooping": True,
                "query-interval": query_interval
            }
        }

    def upper(vid):
        return {
            "name": f"vlan{vid}",
            "enabled": True,
            "type": "infix-if-type:vlan",
            "ipv6": {
                "enabled": True,
                "address": [{"ip": f"2001:db8:{vid}::{last}", "prefix-length": 64}]
            },
            "vlan": {"id": vid, "lower-layer-if": "br0"}
        }

    return {
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    {"name": data1, "enabled": True, "bridge-port": {"bridge": "br0", "pvid": 55}},
                    {"name": data2, "enabled": True, "bridge-port": {"bridge": "br0", "pvid": 77}},
                    {"name": link, "enabled": True, "bridge-port": {"bridge": "br0"}},
                    upper(55),
                    upper(77),
                    {
                        "name": "br0",
                        "enabled": True,
                        "type": "infix-if-type:bridge",
                        "bridge": {
                            "vlans": {
                                "vlan": [vlan(55, data1), vlan(77, data2)]
                            }
                        }
                    }
                ]
            }
        }
    }


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        env = infamy.Env()
        dut1, dut2 = parallel(lambda: env.attach("dut1", "mgmt"),
                              lambda: env.attach("dut2", "mgmt"))

        hsend55 = env["host"]["data11"]
        hnojoin77 = env["host"]["data12"]
        hreceive55 = env["host"]["data21"]
        hsend77 = env["host"]["data22"]

    with test.step("Configure VLAN 55 and 77 with MLD snooping, and IPv6 on the VLAN interfaces"):
        parallel(lambda: dut1.put_config_dicts(bridge(dut1["data1"], dut1["data2"], dut1["link"], 1)),
                 lambda: dut2.put_config_dicts(bridge(dut2["data1"], dut2["data2"], dut2["link"], 2)))

    with test.step("Verify the DUTs agree on one MLD querier in VLAN 55 and in VLAN 77"):
        until(lambda: iface.get_bridge_querier_election([dut1, dut2], "ipv6", vid=55) and
                      iface.get_bridge_querier_election([dut1, dut2], "ipv6", vid=77),
              attempts=30)

    with test.step("Verify the DUT that is not querier has learned its link port as router port in VLAN 55 and 77"):
        def router_port_learned(vid):
            querier, _ = iface.get_bridge_querier_election([dut1, dut2], "ipv6", vid=vid)
            other = [dut1, dut2][1 - querier]
            return other["link"] in iface.get_bridge_router_ports(other, "br0", vid)

        until(lambda: router_port_learned(55) and router_port_learned(77), attempts=10)

    with infamy.IsolatedMacVlan(hsend55) as send55_ns, \
         infamy.IsolatedMacVlan(hnojoin77) as nojoin77_ns, \
         infamy.IsolatedMacVlan(hreceive55) as receive55_ns, \
         infamy.IsolatedMacVlan(hsend77) as send77_ns:

        with test.step("Verify the hosts can reach the DUTs in their VLAN"):
            send55_ns.addip("2001:db8:55::11", prefix_length=64, proto="ipv6")
            nojoin77_ns.addip("2001:db8:77::11", prefix_length=64, proto="ipv6")
            receive55_ns.addip("2001:db8:55::22", prefix_length=64, proto="ipv6")
            send77_ns.addip("2001:db8:77::22", prefix_length=64, proto="ipv6")
            parallel(lambda: send55_ns.must_reach("2001:db8:55::2"),
                     lambda: nojoin77_ns.must_reach("2001:db8:77::2"),
                     lambda: receive55_ns.must_reach("2001:db8:55::1"),
                     lambda: send77_ns.must_reach("2001:db8:77::1"))

        with test.step("Start multicast senders on host:data11, group ff2e::55, and host:data22, group ff2e::77"):
            with mcast.MCastSender(send55_ns, GROUP55), mcast.MCastSender(send77_ns, GROUP77):
                with test.step("Verify group ff2e::55 is flooded to host:data21 and ff2e::77 to host:data12"):
                    parallel(lambda: receive55_ns.must_receive(f"ip6 dst {GROUP55}"),
                             lambda: nojoin77_ns.must_receive(f"ip6 dst {GROUP77}"))

                with test.step("Verify ff2e::55 does not leak into VLAN 77, nor ff2e::77 into VLAN 55"):
                    parallel(lambda: nojoin77_ns.must_not_receive(f"ip6 dst {GROUP55}"),
                             lambda: send77_ns.must_not_receive(f"ip6 dst {GROUP55}"),
                             lambda: send55_ns.must_not_receive(f"ip6 dst {GROUP77}"),
                             lambda: receive55_ns.must_not_receive(f"ip6 dst {GROUP77}"))

                with test.step("Join multicast group ff2e::55 on host:data21"):
                    with mcast.MCastReceiver(receive55_ns, GROUP55):
                        with test.step("Verify dut2 has learned group ff2e::55 on dut2:data1 in VLAN 55"):
                            until(lambda: iface.exist_bridge_multicast_filter(dut2, GROUP55, dut2["data1"], "br0", 55),
                                  attempts=10)

                        with test.step("Verify group ff2e::55 is still received on host:data21, and not in VLAN 77"):
                            parallel(lambda: receive55_ns.must_receive(f"ip6 dst {GROUP55}"),
                                     lambda: nojoin77_ns.must_not_receive(f"ip6 dst {GROUP55}"),
                                     lambda: send77_ns.must_not_receive(f"ip6 dst {GROUP55}"))

    test.succeed()
