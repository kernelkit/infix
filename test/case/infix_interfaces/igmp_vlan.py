#!/usr/bin/env python3
#
#
#            VLAN55             VLAN77                          VLAN55              VLAN77
#           10.0.1.1           10.0.2.1                       10.0.1.2            10.0.2.2
#             \                    /                           \                     /
#              \                  /                             \                   /
#               \                /                               \                 /
#                \--------------/        VLAN 1,2 T               \---------------/
#                |    DUT1      +---------------------------------+      DUT2     |
#                |              |                                 |               |
#                +--------------+                                 +-----+---------+
#        VLAN55 U|      VLAN77 U|                              VLAN55 U |        | VLAN77 U
#                |              |                                       |        |
#                |              |                                       |        |
#                |              |                                       |        |
#                |              |                                       |        |
#     +-------+  |             +----------+                 +------------+       +--------+
#     | msend +--+             | mreceive |                 | mreceive   |       | msend |
#     +-------+                +----------+                 +------------+       +--------+
#      10.0.1.11                 10.0.2.11                      10.0.1.22         10.0.2.22
#



import infamy
import time
import infamy.multicast as mcast

with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env(infamy.std_topology("2x4"))
        dut1 = env.attach("dut1", "mgmt")
        _, d1send = env.ltop.xlate("dut1", "data0")
        _, d1receiver = env.ltop.xlate("dut1", "data1")
        _, d1trunk = env.ltop.xlate("dut1", "data2")
        dut2 = env.attach("dut2", "mgmt")
        _, d2receive = env.ltop.xlate("dut2", "data0")
        _, d2sender = env.ltop.xlate("dut2", "data1")
        _, d2trunk = env.ltop.xlate("dut2", "data2")


    with test.step("Configure device"):
        dut1.put_config_dict("ietf-interfaces",
                             {
                                 "interfaces": {
                                     "interface": [
                                         {
                                             "name": d1send,
                                             "enabled": True,
                                             "bridge-port": {
                                                 "bridge": "br0",
                                                 "pvid": 55

                                             }
                                         },
                                         {
                                             "name": d1receiver,
                                             "enabled": True,
                                             "bridge-port": {
                                                 "bridge": "br0",
                                                 "pvid": 77
                                             }
                                         },
                                         {
                                             "name": d1trunk,
                                             "enabled": True,
                                             "bridge-port": {
                                                 "bridge": "br0"
                                             }
                                         },
                                         {
                                             "name": "vlan55",
                                             "enabled": True,
                                             "type": "infix-if-type:vlan",
                                             "ipv4": {
                                                 "address": [
                                                     {
                                                         "ip": "10.0.1.1",
                                                         "prefix-length": 24
                                                     }
                                                 ]
                                             },
                                             "vlan": {
                                                 "id": 55,
                                                 "lower-layer-if": "br0"
                                             }

                                         },
                                         {
                                             "name": "vlan77",
                                             "enabled": True,
                                             "type": "infix-if-type:vlan",
                                             "ipv4": {
                                                 "address": [
                                                     {
                                                         "ip": "10.0.2.1",
                                                         "prefix-length": 24
                                                     }
                                                 ]
                                             },
                                             "vlan": {
                                                 "id": 77,
                                                 "lower-layer-if": "br0"
                                             }

                                         },
                                         {
                                             "name": "br0",
                                             "enabled": True,
                                             "type": "infix-if-type:bridge",

                                             "bridge": {
                                                 "multicast": {
                                                     "snooping": True
                                                 },
                                                 "vlans": {
                                                     "vlan": [
                                                         {
                                                             "vid": 55,
                                                             "untagged": [ d1send ],
                                                             "tagged": [ d1trunk, "br0" ]
                                                         },
                                                         {
                                                             "vid": 77,
                                                             "untagged": [ d1receiver ],
                                                             "tagged": [ d1trunk, "br0" ]
                                                         }
                                                     ]
                                                 }
                                             }
                                         }
                                     ]

                                 }
                             })
        dut1.put_config_dict("ietf-system", {
            "system": {
                "hostname": "dut1"
            }
        })
        dut2.put_config_dict("ietf-interfaces",
                             {
                                 "interfaces": {
                                    "interface": [
                                        {
                                             "name": d2receive,
                                             "enabled": True,
                                             "bridge-port": {
                                                 "bridge": "br0",
                                                 "pvid": 55
                                             }
                                         },
                                         {
                                             "name": d2sender,
                                             "enabled": True,
                                             "bridge-port": {
                                                 "bridge": "br0",
                                                 "pvid": 77
                                             }
                                         },
                                         {
                                             "name": d2trunk,
                                             "enabled": True,
                                             "bridge-port": {
                                                 "bridge": "br0"
                                             }
                                         },
                                         {
                                             "name": "vlan55",
                                             "enabled": True,
                                             "type": "infix-if-type:vlan",
                                             "ipv4": {
                                                 "address": [
                                                     {
                                                         "ip": "10.0.1.2",
                                                         "prefix-length": 24
                                                     }
                                                 ]
                                             },
                                             "vlan": {
                                                 "id": 55,
                                                 "lower-layer-if": "br0"
                                             }

                                         },
                                         {
                                             "name": "vlan77",
                                             "enabled": True,
                                             "type": "infix-if-type:vlan",
                                             "ipv4": {
                                                 "address": [
                                                     {
                                                         "ip": "10.0.2.2",
                                                         "prefix-length": 24
                                                     }
                                                 ]
                                             },
                                             "vlan": {
                                                 "id": 77,
                                                 "lower-layer-if": "br0"
                                             }

                                         },
                                         {
                                             "name": "br0",
                                             "enabled": True,
                                             "type": "infix-if-type:bridge",

                                             "bridge": {
                                                 "multicast": {
                                                     "snooping": True
                                                 },
                                                 "vlans": {
                                                     "vlan": [
                                                         {
                                                             "vid": 55,
                                                             "untagged": [ d2receive ],
                                                             "tagged": [ d2trunk, "br0" ]
                                                         },
                                                         {
                                                             "vid": 77,
                                                             "untagged": [ d2sender ],
                                                             "tagged": [ d2trunk, "br0" ]
                                                         }
                                                     ]
                                                 }
                                             }
                                         }
                                     ]

                                 }
                        })

        dut2.put_config_dict("ietf-system", {
            "system": {
                "hostname": "dut2"
            }
        })

    with test.step("Check multicast receieved on correct port and VLAN"):
        _, d1send = env.ltop.xlate("host", "data10")
        _, d1receive = env.ltop.xlate("host", "data11")
        _, d2receive = env.ltop.xlate("host", "data20")
        _, d2send = env.ltop.xlate("host", "data21")
        with infamy.IsolatedMacVlan(d1send) as d1send_ns, \
             infamy.IsolatedMacVlan(d1receive) as d1receive_ns, \
             infamy.IsolatedMacVlan(d2receive) as d2receive_ns, \
             infamy.IsolatedMacVlan(d2send) as d2send_ns:
            d1send_ns.addip("10.0.1.11")
            d1receive_ns.addip("10.0.2.11")
            d2receive_ns.addip("10.0.1.22")
            d2send_ns.addip("10.0.2.22")

            d1send_ns.must_reach("10.0.1.2")
            d1receive_ns.must_reach("10.0.2.2")

            vlan55_sender = mcast.MCastSender(d1send_ns, "224.1.1.1")
            vlan77_sender= mcast.MCastSender(d2send_ns, "224.2.2.2")
            vlan55_receiver = mcast.MCastReceiver(d2receive_ns, "224.1.1.1")

            snif_vlan55_sender_incorrect = infamy.Sniffer(d1send_ns, "host 224.2.2.2")
            snif_vlan77_receiver_incorrect = infamy.Sniffer(d1receive_ns, "host 224.1.1.1")
            snif_vlan55_receiver_incorrect = infamy.Sniffer(d2receive_ns, "host 224.2.2.2")
            snif_vlan77_sender_incorrect = infamy.Sniffer(d2send_ns, "host 224.1.1.1")

            snif_vlan55_receiver_correct = infamy.Sniffer(d2receive_ns, "host 224.1.1.1")
            snif_vlan77_receiver_correct = infamy.Sniffer(d1receive_ns, "host 224.2.2.2")

            with vlan55_sender, vlan77_sender, vlan55_receiver:
                with snif_vlan77_sender_incorrect, \
                     snif_vlan77_receiver_incorrect, snif_vlan55_receiver_incorrect, \
                     snif_vlan55_sender_incorrect, snif_vlan55_receiver_correct, \
                     snif_vlan77_receiver_correct:
                        time.sleep(5)
                # TODO: Here should we check for 224.1.1.1 in mdb, also
                # verify that 224.2.2.2 does not exist in mdb

                assert(snif_vlan77_sender_incorrect.output().stdout == "")
                assert(snif_vlan77_receiver_incorrect.output().stdout == "")
                assert(snif_vlan55_receiver_incorrect.output().stdout == "")
                assert(snif_vlan55_sender_incorrect.output().stdout == "")
                print("Multicast does not exist on ports/VLANs where they should not be")
                assert(snif_vlan55_receiver_correct.output().stdout != "")
                assert(snif_vlan77_receiver_correct.output().stdout != "")
                print("Multicast received on correct port and VLAN")

        test.succeed()
