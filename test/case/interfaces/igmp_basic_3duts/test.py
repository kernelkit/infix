#!/usr/bin/env python3
"""IGMP basic with three DUTs

The same check as IGMP basic, spread over three bridges in a chain so
that each DUT only needs a single port facing the host.  A multicast sender sits
behind dut1, a non-member behind dut2, and the receiver behind dut3.
The membership report from the receiver must propagate through dut3
and dut2 up to the querier on dut1 before dut2 prunes the group from
its own host port.

....
   10.0.0.1
 .-----------.       .----------------.       .-----------.
 |   dut1    |       |      dut2      |       |   dut3    |
 '-data-link-'-------'-west-data-east-'-------'-link-data-'
    |                       |                       |      10.0.0.0/24
 .-data1-.               .-data2-.               .-data3-.
 | msend |               | !memb |               | mrecv |
 '-------'               '-------'               '-------'
    .2                      .3                      .4
                          HOST
....

With no membership the group 224.1.1.1 is flooded to both `!memb` and
`mrecv`.  Once `mrecv` joins, dut2 and dut1 learn the group on their
links toward dut3, and `!memb` no longer receives it.
"""

import infamy
import infamy.iface as iface
import infamy.multicast as mcast
from infamy.util import until, parallel

query_interval = 4


def bridge(ports, address=None):
    br0 = {
        "name": "br0",
        "enabled": True,
        "type": "infix-if-type:bridge",
        "bridge": {
            "multicast": {
                "snooping": True,
                "query-interval": query_interval
            }
        }
    }
    if address:
        br0["ipv4"] = {"address": [{"ip": address, "prefix-length": 24}]}

    return {
        "ietf-interfaces": {
            "interfaces": {
                "interface": [br0] + [{
                    "name": port,
                    "enabled": True,
                    "bridge-port": {"bridge": "br0"}
                } for port in ports]
            }
        }
    }


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        env = infamy.Env()
        dut1, dut2, dut3 = parallel(lambda: env.attach("dut1", "mgmt"),
                                    lambda: env.attach("dut2", "mgmt"),
                                    lambda: env.attach("dut3", "mgmt"))
        d1data = dut1["data"]
        d1link = dut1["link"]
        d2data = dut2["data"]
        d2west = dut2["west"]
        d2east = dut2["east"]
        d3data = dut3["data"]
        d3link = dut3["link"]

        hsend = env["host"]["data1"]
        hnojoin = env["host"]["data2"]
        hreceive = env["host"]["data3"]

    with test.step("Configure bridges with IGMP snooping on all DUTs, querier on dut1 with 10.0.0.1/24"):
        parallel(lambda: dut1.put_config_dicts(bridge([d1data, d1link], "10.0.0.1")),
                 lambda: dut2.put_config_dicts(bridge([d2data, d2west, d2east])),
                 lambda: dut3.put_config_dicts(bridge([d3data, d3link])))

    with infamy.IsolatedMacVlan(hsend) as send_ns, \
         infamy.IsolatedMacVlan(hnojoin) as nojoin_ns, \
         infamy.IsolatedMacVlan(hreceive) as receive_ns:

        with test.step("Verify host:data1, host:data2 and host:data3 can reach 10.0.0.1"):
            send_ns.addip("10.0.0.2")
            nojoin_ns.addip("10.0.0.3")
            receive_ns.addip("10.0.0.4")
            parallel(lambda: send_ns.must_reach("10.0.0.1"),
                     lambda: nojoin_ns.must_reach("10.0.0.1"),
                     lambda: receive_ns.must_reach("10.0.0.1"))

        with test.step("Start multicast sender on host:data1, group 224.1.1.1"):
            with mcast.MCastSender(send_ns, "224.1.1.1"):
                with test.step("Verify group 224.1.1.1 is flooded to host:data2 and host:data3"):
                    parallel(lambda: nojoin_ns.must_receive("ip dst 224.1.1.1"),
                             lambda: receive_ns.must_receive("ip dst 224.1.1.1"))

                with test.step("Join multicast group 224.1.1.1 on host:data3"):
                    with mcast.MCastReceiver(receive_ns, "224.1.1.1"):
                        with test.step("Verify dut3 has learned group 224.1.1.1 on dut3:data"):
                            until(lambda: iface.exist_bridge_multicast_filter(dut3, "224.1.1.1", d3data, "br0"), attempts=10)

                        with test.step("Verify dut2 has learned group 224.1.1.1 on dut2:east"):
                            until(lambda: iface.exist_bridge_multicast_filter(dut2, "224.1.1.1", d2east, "br0"), attempts=10)

                        with test.step("Verify dut1 has learned group 224.1.1.1 on dut1:link"):
                            until(lambda: iface.exist_bridge_multicast_filter(dut1, "224.1.1.1", d1link, "br0"), attempts=10)

                        with test.step("Verify group 224.1.1.1 is still received on host:data3"):
                            receive_ns.must_receive("ip dst 224.1.1.1")

                        with test.step("Verify group 224.1.1.1 is no longer received on host:data2"):
                            nojoin_ns.must_not_receive("ip dst 224.1.1.1")

    test.succeed()
