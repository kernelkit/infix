#!/usr/bin/env python3
"""
VLAN ping connectivity

Very basic test if the VLAN interface configuration works.
"""
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
    with test.step("Configure VLAN 10 interface on target:data with IP 10.0.0.2"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, tport = env.ltop.xlate("target", "data")

        target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": tport,
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

    with test.step("Remove VLAN interface from target:data, and test again (should not be able to ping)"):
        target.delete_xpath(f"/ietf-interfaces:interfaces/interface[name='{tport}.10']")
        _, hport = env.ltop.xlate("host", "data")
        test_ping(hport,False)

    test.succeed()
