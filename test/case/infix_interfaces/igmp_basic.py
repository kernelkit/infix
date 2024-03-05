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
#     | msend |       | mreceive |      + no join     |
#     +-------+       +----------+      +-------------+

import infamy
import time
import infamy.multicast as mcast

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

                                        "bridge": {
                                            "multicast": {
                                                "snooping": True
                                            }

                                        }
                                    }
                                ]

                            }
                        })
    with test.step("Check multicast receieved on correct port"):
        _, hport0 = env.ltop.xlate("host", "data0")
        _, hport1 = env.ltop.xlate("host", "data1")
        _, hport2 = env.ltop.xlate("host", "data2")
        with infamy.IsolatedMacVlan(hport0) as ns0, \
             infamy.IsolatedMacVlan(hport1) as ns1, \
             infamy.IsolatedMacVlan(hport2) as ns2:
            ns0.addip("10.0.0.2")
            ns1.addip("10.0.0.3")
            ns2.addip("10.0.0.4")
            ns0.must_reach("10.0.0.1")
            ns1.must_reach("10.0.0.1")
            ns2.must_reach("10.0.0.1")

            sender = mcast.MCastSender(ns0, "224.1.1.1")
            receiver = mcast.MCastReceiver(ns1, "224.1.1.1")
            snif_nojoin = infamy.Sniffer(ns2, "host 224.1.1.1")
            snif_receiver = infamy.Sniffer(ns1, "host 224.1.1.1")
            with sender:
                time.sleep(5)
                with snif_nojoin:
                    time.sleep(5)
                assert(snif_nojoin.output().stdout != "")

                with snif_receiver:
                    time.sleep(5)
                assert(snif_receiver.output().stdout != "")
                print("Multicast is received on ports without join")

                with receiver:
                    with snif_receiver:
                        time.sleep(5)
                        with snif_nojoin:
                            time.sleep(5)
                        assert(snif_nojoin.output().stdout == "")
                        print("Multicast is NOT received on port when join exist")
                    assert(snif_receiver.output().stdout != "")
                    print("Multicast is received after join")

        test.succeed()
