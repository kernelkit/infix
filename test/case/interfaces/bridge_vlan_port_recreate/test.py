#!/usr/bin/env python3
"""
Bridge port keeps its VLANs when recreated

A VLAN filtering bridge with two ports in VLAN 10: the physical port
untagged with PVID 10, and a GRETAP tunnel tagged only.  A change of
the tunnel's TTL recreates its interface.  The port has to come back
as a tagged member of VLAN 10, the membership a port holds without a
PVID is the one that is easy to lose.
"""
import infamy
import infamy.iface as iface
from infamy.util import until


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        tport = target["data"]

    with test.step("Configure br0 with VLAN 10: target:data untagged with PVID 10, gretap0 tagged"):
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
                                "tagged": ["gretap0"],
                            }]
                        }
                    }
                }, {
                    "name": tport,
                    "bridge-port": {
                        "bridge": "br0",
                        "pvid": 10,
                    }
                }, {
                    "name": "gretap0",
                    "type": "infix-if-type:gretap",
                    "gre": {
                        "local": "192.168.50.1",
                        "remote": "192.168.50.2",
                    },
                    "bridge-port": {
                        "bridge": "br0",
                    }
                }]
            }
        }})

    with test.step("Verify gretap0 is a tagged and target:data an untagged member of VLAN 10"):
        until(lambda: "gretap0" in iface.bridge_vlan_members(target, "br0", 10, "tagged"))
        until(lambda: tport in iface.bridge_vlan_members(target, "br0", 10, "untagged"))
        ifindex = iface.get_param(target, "gretap0", "if-index")

    with test.step("Set the TTL of gretap0 to 32"):
        target.put_config_dicts({"ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": "gretap0",
                    "gre": {"ttl": 32},
                }]
            }
        }})

    with test.step("Verify gretap0 was recreated with a new if-index"):
        until(lambda: iface.get_param(target, "gretap0", "if-index") not in (None, ifindex))

    with test.step("Verify gretap0 is still a tagged member of VLAN 10"):
        until(lambda: "gretap0" in iface.bridge_vlan_members(target, "br0", 10, "tagged"))

    test.succeed()
