#!/usr/bin/env python3
"""IGMP basic

Verify basic IGMP snooping behavior on a bridge.  Without any IGMP
membership, multicast should be flooded to all ports.  Once a host
joins a group, the bridge should learn the membership via IGMP and
only forward matching multicast to the member port, pruning it from
non-member ports.

....
              .1
 .---------------------------.
 |            DUT            |
 '-data1-----data2-----data3-'
     |         |         |
     |         |         |      10.0.0.0/24
     |         |         |
 .-data1-. .-data2-. .-data3-.
 | msend | | mrecv | | !memb |
 '-------' '-------' '-------'
    .2         .3        .4
             HOST
....

A multicast sender on `msend` sends to group 224.1.1.1.  First, with
no IGMP joins, verify the group is flooded to both `mrecv` and `!memb`.
Then `mrecv` joins the group and the test waits for the bridge MDB to
reflect the membership before verifying that `!memb` no longer receives
the group.

"""

import infamy
import infamy.iface as iface
import infamy.multicast as mcast
from infamy.util import until

query_interval = 4

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, msend = env.ltop.xlate("target", "data1")
        _, mreceive = env.ltop.xlate("target", "data2")
        _, nojoin = env.ltop.xlate("target", "data3")

    with test.step("Configure device"):
        target.put_config_dicts({"ietf-interfaces":
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
                        }})

    _, hsend = env.ltop.xlate("host", "data1")
    _, hreceive = env.ltop.xlate("host", "data2")
    _, hnojoin = env.ltop.xlate("host", "data3")
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
                with test.step("Verify that the group 224.1.1.1 is flooded on host:data2 and host:data3"):
                    infamy.parallel(lambda: receive_ns.must_receive("ip dst 224.1.1.1"),
                                    lambda: nojoin_ns.must_receive("ip dst 224.1.1.1"))

                with test.step("Join multicast group 224.1.1.1 on host:data2"):
                    with mcast.MCastReceiver(receive_ns, "224.1.1.1"):
                        with test.step("Verify group 224.1.1.1 is received on host:data2"):
                            receive_ns.must_receive("ip dst 224.1.1.1")

                        with test.step("Verify that the group 224.1.1.1 is no longer received on host:data3"):
                            until(lambda: iface.exist_bridge_multicast_filter(target, "224.1.1.1", mreceive, "br0"), attempts=10)
                            nojoin_ns.must_not_receive("ip dst 224.1.1.1")

    test.succeed()
