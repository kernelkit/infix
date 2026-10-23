#!/usr/bin/env python3
"""MLD basic

Verify basic MLD snooping behavior on a bridge.  Without any MLD
membership, IPv6 multicast should be flooded to all ports.  Once a host
joins a group, the bridge should learn the membership via MLD and only
forward matching multicast to the member port, pruning it from
non-member ports.

....
            2001:db8::1
 .---------------------------.
 |            DUT            |
 '-data1-----data2-----data3-'
     |         |         |
     |         |         |      2001:db8::/64
     |         |         |
 .-data1-. .-data2-. .-data3-.
 | msend | | mrecv | | !memb |
 '-------' '-------' '-------'
    ::2       ::3       ::4
             HOST
....

The bridge has IPv6 enabled, so it is the MLD querier on the LAN, with
its link-local address as source.  A multicast sender on `msend` sends
to group ff2e::1.  First, with no MLD joins, verify the group is flooded
to both `mrecv` and `!memb`.  Then `mrecv` joins the group and the test
waits for the bridge MDB to reflect the membership before verifying
that `!memb` no longer receives the group.

"""

import ipaddress
import infamy
import infamy.iface as iface
import infamy.multicast as mcast
from infamy.util import until, parallel

GROUP = "ff2e::1"
query_interval = 4


def mld_querier(target):
    """The bridge is the local MLD querier, using a link-local address"""
    querier = iface.get_bridge_queriers(target, "br0").get("ipv6")
    if not querier or not querier["local"]:
        return False

    return ipaddress.IPv6Address(querier["address"]).is_link_local


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        msend = target["data1"]
        mreceive = target["data2"]
        nojoin = target["data3"]

    with test.step("Configure bridge with MLD snooping and IPv6 address 2001:db8::1/64"):
        target.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": port,
                        "enabled": True,
                        "bridge-port": {"bridge": "br0"}
                    } for port in (msend, mreceive, nojoin)] + [{
                        "name": "br0",
                        "enabled": True,
                        "type": "infix-if-type:bridge",
                        "ipv6": {
                            "enabled": True,
                            "address": [{
                                "ip": "2001:db8::1",
                                "prefix-length": 64
                            }]
                        },
                        "bridge": {
                            "multicast": {
                                "snooping": True,
                                "query-interval": query_interval
                            }
                        }
                    }]
                }
            }
        })

    with test.step("Verify the bridge is the MLD querier, using its link-local address"):
        until(lambda: mld_querier(target), attempts=30)

    hsend = env["host"]["data1"]
    hreceive = env["host"]["data2"]
    hnojoin = env["host"]["data3"]
    with infamy.IsolatedMacVlan(hsend) as send_ns, \
         infamy.IsolatedMacVlan(hreceive) as receive_ns, \
         infamy.IsolatedMacVlan(hnojoin) as nojoin_ns:

        with test.step("Verify host:data1, host:data2 and host:data3 can reach 2001:db8::1"):
            send_ns.addip("2001:db8::2", prefix_length=64, proto="ipv6")
            receive_ns.addip("2001:db8::3", prefix_length=64, proto="ipv6")
            nojoin_ns.addip("2001:db8::4", prefix_length=64, proto="ipv6")
            parallel(lambda: send_ns.must_reach("2001:db8::1"),
                     lambda: receive_ns.must_reach("2001:db8::1"),
                     lambda: nojoin_ns.must_reach("2001:db8::1"))

        with test.step("Start multicast sender on host:data1, group ff2e::1"):
            with mcast.MCastSender(send_ns, GROUP):
                with test.step("Verify group ff2e::1 is flooded to host:data2 and host:data3"):
                    parallel(lambda: receive_ns.must_receive(f"ip6 dst {GROUP}"),
                             lambda: nojoin_ns.must_receive(f"ip6 dst {GROUP}"))

                with test.step("Join multicast group ff2e::1 on host:data2"):
                    with mcast.MCastReceiver(receive_ns, GROUP):
                        with test.step("Verify the bridge has learned group ff2e::1 on target:data2"):
                            until(lambda: iface.exist_bridge_multicast_filter(target, GROUP, mreceive, "br0"), attempts=10)

                        with test.step("Verify group ff2e::1 is still received on host:data2"):
                            receive_ns.must_receive(f"ip6 dst {GROUP}")

                        with test.step("Verify group ff2e::1 is no longer received on host:data3"):
                            nojoin_ns.must_not_receive(f"ip6 dst {GROUP}")

    test.succeed()
