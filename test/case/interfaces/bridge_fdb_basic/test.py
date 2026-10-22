#!/usr/bin/env python3
r"""
Bridge FDB basic

Verify the unicast forwarding database (FDB) of a VLAN filtering bridge:
the configured aging time, a static entry, and an address learned from
traffic through the bridge, all in the operational datastore.

....

,------------------------------------------,
|                                          |
|                    br0 (VLAN 10)         |
|                   /     \                |
| target:mgmt   target:data1  target:data2 |
'------------------------------------------'
        |                |     |
        |                |     |
,------------------------------------------,
|   host:mgmt   host:data1     host:data2  |
|               [10.0.0.1]     [10.0.0.2]  |
|                  (ns1)         (ns2)     |
|                                          |
|                 [ HOST ]                 |
'------------------------------------------'

....

"""
import infamy
import infamy.iface as iface
from infamy.util import until

VID = 10
STATIC = "02:00:00:00:10:01"
AGING = 600


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        tport1 = target["data1"]
        tport2 = target["data2"]

    with test.step("Configure VLAN 10 bridge with aging time and a static FDB entry"):
        target.put_config_dicts({"ietf-interfaces": {
            "interfaces": {
                "interface": [
                    {
                        "name": "br0",
                        "type": "infix-if-type:bridge",
                        "enabled": True,
                        "infix-interfaces:bridge": {
                            "aging-time": AGING,
                            "vlans": {
                                "vlan": [{
                                    "vid": VID,
                                    "untagged": [tport1, tport2],
                                    "forwarding-database": {
                                        "entry": [{
                                            "address": STATIC,
                                            "port": tport2,
                                        }]
                                    }
                                }]
                            }
                        }
                    }, {
                        "name": tport1,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "bridge": "br0",
                            "pvid": VID,
                        }
                    }, {
                        "name": tport2,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "bridge": "br0",
                            "pvid": VID,
                        }
                    }
                ]
            }
        }})

    with test.step("Verify the bridge aging time"):
        until(lambda: iface.get_bridge(target, "br0").get("aging-time") == AGING)

    with test.step("Verify the static FDB entry on target:data2 in VLAN 10"):
        entry = until(lambda: iface.bridge_fdb_entry(target, "br0", VID, STATIC))
        if entry["port"] != tport2 or entry["state"] != "static":
            test.fail(f"Unexpected static entry: {entry}")

    with test.step("Verify host:data1 is learned on target:data1 after a ping"):
        with infamy.IsolatedMacVlan(env["host"]["data1"]) as ns1, \
             infamy.IsolatedMacVlan(env["host"]["data2"]) as ns2:
            ns1.addip("10.0.0.1")
            ns2.addip("10.0.0.2")
            ns1.must_reach("10.0.0.2")

            mac = ns1.mac
            entry = until(lambda: iface.bridge_fdb_entry(target, "br0", VID, mac))
            if entry["port"] != tport1 or entry["state"] != "dynamic" \
               or "age" not in entry:
                test.fail(f"Unexpected learned entry: {entry}")

    with test.step("Remove the static FDB entry and verify it is gone"):
        target.delete_xpath(iface.get_xpath("br0", "infix-interfaces:bridge/vlans"
                                            f"/vlan[vid='{VID}']/forwarding-database"
                                            f"/entry[address='{STATIC}']"))
        until(lambda: iface.bridge_fdb_entry(target, "br0", VID, STATIC) is None)

    test.succeed()
