#!/usr/bin/env python3

import infamy
import infamy.iface as iface
import copy

from infamy import until

def test_ping(hport, should_pass):
      with infamy.IsolatedMacVlan(hport) as ns:
            pingtest = ns.runsh("""
            set -ex

            ip link set iface up
            ip link add dev vlan10 link iface up type vlan id 10
            ip addr add 10.0.0.1/24 dev vlan10
            """)
            if(should_pass):
                ns.must_reach("10.0.0.2")
            else:
                ns.must_not_reach("10.0.0.2")

with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env(infamy.std_topology("1x2"))
        target = env.attach("target", "mgmt")

    with test.step("Configure VLAN 10 on target:data with IP 10.0.0.2"):
        _, tport = env.ltop.xlate("target", "data")

        target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": tport,
                        "type": "infix-if-type:ethernet",
                        "enabled": True,
                    },
                    {
                        "name": f"{tport}.10",
                        "type": "infix-if-type:vlan",
                        "vlan": {
                            "id": 10,
                            "lower-layer-if": tport,
                        },

                        "ipv4": {
                            "address": [
                                {
                                    "ip": "10.0.0.2",
                                    "prefix-length": 24,
                                }
                            ]
                        }
                    },
                ]
            }
        })

    with test.step("Waiting for links to come up"):
        until(lambda: iface.get_oper_up(target, tport))

    with test.step("Ping 10.0.0.2 from VLAN 10 on host:data with IP 10.0.0.1"):
        _, hport = env.ltop.xlate("host", "data")
        test_ping(hport,True)

    with test.step("Remove VLAN interface, and test again (should not be able to ping)"):
        running = target.get_config_dict("/ietf-interfaces:interfaces")
        new = copy.deepcopy(running)
        new["interfaces"]["interface"] = [d for d in new["interfaces"]["interface"] if not (d["name"] == f"{tport}.10")]
        target.put_diff_dicts("ietf-interfaces",running,new)
        _, hport = env.ltop.xlate("host", "data")
        test_ping(hport,False)

    test.succeed()
