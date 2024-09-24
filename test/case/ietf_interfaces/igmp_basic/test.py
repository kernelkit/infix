#!/usr/bin/env python3
#
# Add join the group 224.1.1.1 from data1 on the host. Send to that
# group from `msend`, verify that `mrecv` receives it and that `!memb`
# does not.
#
#              .1
# .---------------------------.
# |            DUT            |
# '-data0-----data1-----data2-'
#     |         |         |
#     |         |         |      10.0.0.0/24
#     |         |         |
# .-data0-. .-data1-. .-data2-.
# | msend | | mrecv | | !memb |
# '-------' '-------' '-------'
#    .2         .3        .4

"""
IGMP basic

Test that all multicast gets flooded when IGMP is disabled and
the flooding stops as soon a join arrives
"""

import infamy
import time
import infamy.multicast as mcast

query_interval = 4

with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, msend = env.ltop.xlate("target", "data0")
        _, mreceive = env.ltop.xlate("target", "data1")
        _, nojoin = env.ltop.xlate("target", "data2")

    with test.step("Configure device"):
        target.put_config_dict("ietf-interfaces",
                        {
                            "interfaces": {
                                "interface": [
                                    {
                                        "name": msend,
                                        "enabled": True,
                                        "infix-interfaces:bridge-port": {
                                            "bridge": "br0"
                                        }
                                    },
                                    {
                                        "name": mreceive,
                                        "enabled": True,
                                        "infix-interfaces:bridge-port": {
                                            "bridge": "br0"
                                        }
                                    },
                                    {
                                        "name": nojoin,
                                        "enabled": True,
                                        "infix-interfaces:bridge-port": {
                                            "bridge": "br0"
                                        }
                                    },
                                    {
                                        "name": "br0",
                                        "enabled": True,
                                        "type": "infix-if-type:bridge",
                                        "ipv4": {
                                            "address": [
                                                {
                                                    "ip": "10.0.0.1",
                                                    "prefix-length": 24
                                                }
                                            ]
                                        },

                                        "bridge": {
                                            "multicast": {
                                                "query-interval": query_interval,
                                                "snooping": True
                                            }

                                        }
                                    }
                                ]

                            }
                        })

    _, hsend = env.ltop.xlate("host", "data0")
    _, hreceive = env.ltop.xlate("host", "data1")
    _, hnojoin = env.ltop.xlate("host", "data2")
    with infamy.IsolatedMacVlan(hsend) as send_ns, \
         infamy.IsolatedMacVlan(hreceive) as receive_ns, \
         infamy.IsolatedMacVlan(hnojoin) as nojoin_ns:

        with test.step("Start multicast sender on host:data0, group 224.1.1.1"):
            send_ns.addip("10.0.0.2")
            receive_ns.addip("10.0.0.3")
            nojoin_ns.addip("10.0.0.4")
            send_ns.must_reach("10.0.0.1")
            receive_ns.must_reach("10.0.0.1")
            nojoin_ns.must_reach("10.0.0.1")

            with mcast.MCastSender(send_ns, "224.1.1.1"):
                with test.step("Verify that the group 224.1.1.1 is flooded on host:data1 and host:data2"):
                    infamy.parallel(lambda: receive_ns.must_receive("ip dst 224.1.1.1"),
                                    lambda: nojoin_ns.must_receive("ip dst 224.1.1.1"))

                with test.step("Join multicast group 224.1.1.1 on host:data1"):
                    with mcast.MCastReceiver(receive_ns, "224.1.1.1"):
                        with test.step("Verify group 224.1.1.1 is received on host:data1"):
                            receive_ns.must_receive("ip dst 224.1.1.1")

                        with test.step("Verify that the group 224.1.1.1 is no longer received to host:data2"):
                            attempt = 0

                            # This retry loop exists to handle the case where the first query is lost due
                            # to the network just starting up. In particular there's a case where a newly
                            # created bridge uses the "NOOP" scheduler (noop_enqueue()) for a split second
                            # while it's starting up, which might drop the first query msg.
                            while attempt <= query_interval:
                                try:
                                    nojoin_ns.must_not_receive("ip dst 224.1.1.1")
                                    break
                                except Exception as e:
                                    attempt += 1
                                    if attempt > query_interval:
                                        test.fail()
                                    else:
                                        print(f"Got mcast flood, retrying ({attempt}/{query_interval})")

    test.succeed()
