#!/usr/bin/env python3
#
# Add static filter on DUT for 224.1.1.1 containing only data1. Send
# to that group from `msend`, verify that `mrecv` receives it and that
# `!memb` does not.
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
#    .2        .3        .4

"""
Static multicast filters

Verify that static multicast filters work (remember that snooping needs to
enabled when using static multicast filters)
"""
import infamy
import time
import infamy.multicast as mcast
import infamy.iface as iface
from infamy.util import until

def set_static_multicast_filter(target, address, port):
    target.put_config_dict("ietf-interfaces", {
        "interfaces": {
            "interface": [
                {
                    "name": "br0",
                    "infix-interfaces:bridge": {
                        "multicast-filters": {
                            "multicast-filter": [
                                {
                                    "group": address,
                                    "ports": [
                                        {
                                            "port": port,
                                        }
                                    ]
                                }
                            ],
                        },
                    }
                }
            ]
        }
    })

with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, msend = env.ltop.xlate("target", "data0")
        _, mreceive = env.ltop.xlate("target", "data1")
        _, nojoin = env.ltop.xlate("target", "data2")

    with test.step("Configure device without static filter"):
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
                                        "infix-interfaces:bridge": {
                                            "multicast": {
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

            print("Starting sender")
            with mcast.MCastSender(send_ns, "224.1.1.1"):
                with test.step("Verify that 224.1.1.1 is flooded to host:data1 and host:data2"):
                    infamy.parallel(lambda: receive_ns.must_receive("ip dst 224.1.1.1"),
                                    lambda: nojoin_ns.must_receive("ip dst 224.1.1.1"))

                with test.step("Enable multicast filter on host:data1, group 224.1.1.1"):
                    set_static_multicast_filter(target, "224.1.1.1", mreceive)
                    until(lambda: iface.exist_bridge_multicast_filter(target, "224.1.1.1", mreceive, "br0"))

                    with test.step("Verify that the group is still forwarded host:data1"):
                        receive_ns.must_receive("ip dst 224.1.1.1")

                    with test.step("Verify that the group is no longer forwarded host:data2"):
                        nojoin_ns.must_not_receive("ip dst 224.1.1.1")

        test.succeed()
