#!/usr/bin/env python3
r"""
IGMP VLAN

Test tagged IGMP control traffic and that VLAN separation is respected for multicast

....
       VLAN55             VLAN77                          VLAN55              VLAN77
      10.0.1.1           10.0.2.1                       10.0.1.2            10.0.2.2
          \                /                               \                 /
           \--------------/        VLAN 1,2 T               \---------------/
           |    DUT1      +---------------------------------+      DUT2     |
           |              |dut1:link               dut2:link|               |
           +--------------+                                 +-----+---------+
 dut1:data1|              |dut1:data2                   dut2:data1|        |dut2:data2
  VLAN55 U |              | VLAN77 U                     VLAN55 U |        | VLAN77 U
           |              |                                       |        |
+-------+  |             +----------+                 +------------+       +--------+
| msend +--+             | mreceive |                 | mreceive   |       | msend  |
+-------+                +----------+                 +------------+       +--------+
 10.0.1.11                 10.0.2.11                      10.0.1.22         10.0.2.22
....

"""

import infamy
import infamy.multicast as mcast
from infamy.util import parallel

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        dut1 = env.attach("dut1", "mgmt")
        _, d1send = env.ltop.xlate("dut1", "data1")
        _, d1receiver = env.ltop.xlate("dut1", "data2")
        _, d1trunk = env.ltop.xlate("dut1", "link")
        dut2 = env.attach("dut2", "mgmt")
        _, d2receive = env.ltop.xlate("dut2", "data1")
        _, d2sender = env.ltop.xlate("dut2", "data2")
        _, d2trunk = env.ltop.xlate("dut2", "link")

        _, hsendd1 = env.ltop.xlate("host", "data11")
        _, hreceived1 = env.ltop.xlate("host", "data12")
        _, hreceived2 = env.ltop.xlate("host", "data21")
        _, hsendd2 = env.ltop.xlate("host", "data22")


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
                                                 "vlans": {
                                                     "vlan": [
                                                         {
                                                             "vid": 55,
                                                             "untagged": [ d1send ],
                                                             "tagged": [ d1trunk, "br0" ],
                                                             "multicast": {
                                                                 "snooping": True
                                                             }
                                                         },
                                                         {
                                                             "vid": 77,
                                                             "untagged": [ d1receiver ],
                                                             "tagged": [ d1trunk, "br0" ],
                                                             "multicast": {
                                                                 "snooping": True
                                                             }
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
                                                 "vlans": {
                                                     "vlan": [
                                                         {
                                                             "vid": 55,
                                                             "untagged": [ d2receive ],
                                                             "tagged": [ d2trunk, "br0" ],
                                                             "multicast": {
                                                                 "snooping": True
                                                             }
                                                         },
                                                         {
                                                             "vid": 77,
                                                             "untagged": [ d2sender ],
                                                             "tagged": [ d2trunk, "br0" ],
                                                             "multicast": {
                                                                 "snooping": True
                                                             }
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

    with infamy.IsolatedMacVlan(hsendd1) as d1send_ns, \
         infamy.IsolatedMacVlan(hreceived1) as d1receive_ns, \
         infamy.IsolatedMacVlan(hreceived2) as d2receive_ns, \
         infamy.IsolatedMacVlan(hsendd2) as d2send_ns:
        d1send_ns.addip("10.0.1.11")
        d1receive_ns.addip("10.0.2.11")
        d2receive_ns.addip("10.0.1.22")
        d2send_ns.addip("10.0.2.22")
        d1send_ns.must_reach("10.0.1.2")
        d1receive_ns.must_reach("10.0.2.2")
        with test.step("Start multicast sender on host:data11, group 224.2.2.2"):
            vlan55_sender = mcast.MCastSender(d2send_ns, "224.2.2.2")
        with test.step("Start multicast sender on host:data22, group 224.1.1.1"):
            vlan77_sender= mcast.MCastSender(d1send_ns, "224.1.1.1")


        with vlan55_sender, vlan77_sender:
            with test.step("Verify group 224.2.2.2 is flooded to host:data21"):
                d1receive_ns.must_receive("ip dst 224.2.2.2")
            with test.step("Verify group 224.1.1.1 is flooded to host:data12"):
                d2receive_ns.must_receive("ip dst 224.1.1.1")
            with test.step("Verify group 224.2.2.2 on host:data11, 224.1.1.1 on host:data21, 224.2.2.2 on host:data12 and 224.1.1.1 on host:data22 is not received"):
                parallel(d1send_ns.must_not_receive("host 224.2.2.2"),
                         d1receive_ns.must_not_receive("host 224.1.1.1"),
                         d2receive_ns.must_not_receive("host 224.2.2.2"),
                         d2send_ns.must_not_receive("host 224.1.1.1"))
            with test.step("Join multicast group 224.2.2.2 on host:data21"):
                vlan55_receiver = mcast.MCastReceiver(d1receive_ns, "224.2.2.2")
            with vlan55_receiver:
                with test.step("Verify group 224.2.2.2 on host:data11, 224.1.1.1 on host:data21, 224.2.2.2 on host:data12 and 224.1.1.1 on host:data22 is not received"):
                    parallel(d1send_ns.must_not_receive("host 224.2.2.2"),
                             d1receive_ns.must_not_receive("host 224.1.1.1"),
                             d1send_ns.must_not_receive("host 224.2.2.2"),
                             d2send_ns.must_not_receive("host 224.1.1.1"))
                with test.step("Verify group 224.2.2.2 is forwarded to host:data21"):
                    d1receive_ns.must_receive("host 224.2.2.2")
                with test.step("Verify group 224.1.1.1 is forwarded to host:data12"):
                    d2receive_ns.must_receive("ip dst 224.1.1.1")

        test.succeed()
