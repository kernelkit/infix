#!/usr/bin/env python3
"""
Bridge STP on a VLAN filtering bridge

Spanning tree is enabled on a VLAN filtering bridge whose port already
is a member of a VLAN.  The port has to keep its VLAN, and later
changes to the bridge, here a second VLAN, have to be applied.
"""
import infamy
import infamy.iface as iface
from infamy.util import until


def stp_state(target, port):
    data = target.get_data(f"/ietf-interfaces:interfaces/interface[name='{port}']")
    brport = data["interfaces"]["interface"][port].get("bridge-port", {})
    return brport.get("stp", {}).get("cist", {}).get("state")


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        tport = target["data"]
        hport = env["host"]["data"]

    with test.step("Configure br0 with VLAN 10: target:data untagged with PVID 10, address 10.0.0.2/24 on vlan10"):
        target.put_config_dicts({"ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": "br0",
                    "type": "infix-if-type:bridge",
                    "bridge": {
                        "vlans": {
                            "vlan": [{
                                "vid": 10,
                                "untagged": [tport],
                                "tagged": ["br0"],
                            }]
                        }
                    }
                }, {
                    "name": "vlan10",
                    "type": "infix-if-type:vlan",
                    "vlan": {
                        "lower-layer-if": "br0",
                        "id": 10,
                    },
                    "ipv4": {
                        "address": [{
                            "ip": "10.0.0.2",
                            "prefix-length": 24,
                        }]
                    }
                }, {
                    "name": tport,
                    "bridge-port": {
                        "bridge": "br0",
                        "pvid": 10,
                    }
                }]
            }
        }})

    with infamy.IsolatedMacVlan(hport) as ns:
        ns.addip("10.0.0.1")

        with test.step("Verify host:data reaches 10.0.0.2"):
            ns.must_reach("10.0.0.2")

        with test.step("Enable spanning tree on br0"):
            target.put_config_dicts({"ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": "br0",
                        "bridge": {
                            "stp": {},
                        }
                    }]
                }
            }})

        with test.step("Verify target:data is forwarding in the spanning tree"):
            until(lambda: stp_state(target, tport) == "forwarding")

        with test.step("Verify host:data reaches 10.0.0.2"):
            ns.must_reach("10.0.0.2")

        with test.step("Add VLAN 20 to br0 with target:data tagged"):
            target.put_config_dicts({"ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": "br0",
                        "bridge": {
                            "vlans": {
                                "vlan": [{
                                    "vid": 20,
                                    "tagged": [tport],
                                }]
                            }
                        }
                    }]
                }
            }})

        with test.step("Verify target:data is a tagged member of VLAN 20"):
            until(lambda: tport in iface.bridge_vlan_members(target, "br0", 20, "tagged"))

        with test.step("Verify host:data reaches 10.0.0.2"):
            ns.must_reach("10.0.0.2")

    test.succeed()
