#!/usr/bin/env python3
#                    10.0.0.1
#                +--------------+
#                |    DUT       |
#                |              |
#                +----------+---\
#                |          |    \
#                |          |     \
#                |          |      \
#                |          |       \
#    10.0.0.2    |  10.0.0.3|        \ 10.0.0.4
#     +----------+    +---+------+    \ +-------------+
#     | msend    |    | Static   |      + no join     |
#     +----------+    | MC-filter|      +-------------+
#                     +----------+

import infamy
import time
import infamy.multicast as mcast
import infamy.iface as iface
from infamy.util import until

def set_static_multicast_filter(target, address):
    target.put_config_dict("ietf-interfaces",
                           {
                               "interfaces": {
                                   "interface": [
                                       {
                                           "name": "br0",
                                           "infix-interfaces:bridge": {
                                               "multicast-filters": {
                                                   "multicast-filter": [
                                                       {
                                                           "group": "224.1.1.1",
                                                           "ports": [
                                                               {
                                                                   "port": mreceive
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
        env = infamy.Env(infamy.std_topology("1x4"))
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
                                        "infix-interfaces:bridge": {
                                            "multicast": {
                                                "snooping": True
                                            }
                                        }
                                    }
                                ]

                            }
                    })

    with test.step("Check multicast receieved on correct port"):
        _, hsend = env.ltop.xlate("host", "data0")
        _, hreceive = env.ltop.xlate("host", "data1")
        _, hnojoin = env.ltop.xlate("host", "data2")
        with infamy.IsolatedMacVlan(hsend) as send_ns, \
             infamy.IsolatedMacVlan(hreceive) as receive_ns, \
             infamy.IsolatedMacVlan(hnojoin) as nojoin_ns:
            send_ns.addip("10.0.0.2")
            receive_ns.addip("10.0.0.3")
            nojoin_ns.addip("10.0.0.4")
            send_ns.must_reach("10.0.0.1")
            receive_ns.must_reach("10.0.0.1")
            nojoin_ns.must_reach("10.0.0.1")

            sender = mcast.MCastSender(send_ns, "224.1.1.1")
            snif_nojoin = infamy.Sniffer(nojoin_ns, "host 224.1.1.1")
            snif_receiver = infamy.Sniffer(receive_ns, "host 224.1.1.1")
            with sender:
                with snif_receiver:
                    time.sleep(5)
                    with snif_nojoin:
                        time.sleep(5)
                assert(snif_receiver.packets() != "")
                assert(snif_nojoin.packets() != "")
                print("As expected, unregistered multicast is received on both ports")

                set_static_multicast_filter(target, "224.1.1.1")
                until(lambda: iface.exist_bridge_multicast_filter(target, "224.1.1.1", mreceive, "br0"))
                with snif_receiver,snif_nojoin:
                    time.sleep(5)

                assert(snif_nojoin.packets() == "")
                print("As expected, registered multicast is NOT forwarded to non-member port")
                assert(snif_receiver.packets() != "")
                print("As expected, registered multicast is forwarded to the member port")

        test.succeed()
