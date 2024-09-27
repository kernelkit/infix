#!/usr/bin/env python3
"""Verify that VETH pairs can be deleted

```
     veth0b       veth0a        e1    e2
          `---------'
```

Each test step to create, add address, or delete an interace is distinct
from any other step.  This to trigger a new configuration "generation".

"""

import infamy
import infamy.iface as iface


with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

        _, eth0 = env.ltop.xlate("target", "eth0")
        _, eth1 = env.ltop.xlate("target", "eth1")

        veth0a = "veth0a"
        veth0b = "veth0b"

    with test.step("Create VETH pair"):
        target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": veth0a,
                        "type": "infix-if-type:veth",
                        "enabled": True,
                        "infix-interfaces:veth": {
                            "peer": veth0b
                        }
                    },
                    {
                        "name": veth0b,
                        "type": "infix-if-type:veth",
                        "enabled": True,
                        "infix-interfaces:veth": {
                            "peer": veth0a
                        }
                    }
                ]
            }
        })

    with test.step("Verify VETH pair exists"):
        assert iface.interface_exist(target, veth0a), \
            f"Interface <{veth0a}> does not exist."
        assert iface.interface_exist(target, veth0b), \
            f"Interface <{veth0b}> does not exist."

    with test.step("Set IP address on target:eth0 (dummy op)"):
        target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [{
                    "name": f"{eth0}",
                    "ipv4": {
                        "address": [{
                            "ip": "10.0.0.1",
                            "prefix-length": 24
                        }]
                    }
                }]
            }
        })

    with test.step("Set IP address on target:eth1 (dummy op)"):
        target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [{
                    "name": f"{eth1}",
                    "ipv4": {
                        "address": [{
                            "ip": "20.0.0.1",
                            "prefix-length": 24
                        }]
                    }
                }]
            }
        })

    # TODO: need target.del_config_dict() or similar for VETH _pairs_,
    #       because both interfaces must be removed at the same time.
    # with test.step("Delete VETH pair"):
    #     xpath = f"/ietf-interfaces:interfaces/interface[name='{veth0a}']"
    #     target.delete_xpath(xpath)
    # XXX: temporary workaround
    with test.step("Reset configuration"):
        # Calls target.test_reset() to apply safe-config
        target = env.attach("target", "mgmt")

    with test.step("Verify target:eth0 and target:eth1 still exist"):
        assert iface.interface_exist(target, eth0), \
            f"Interface {eth0} missing!"
        assert iface.interface_exist(target, eth1), \
            f"Interface {eth1} missing!"

    with test.step("Verify VETH pair have been removed"):
        assert not iface.interface_exist(target, veth0a), \
            f"Interface <{veth0a}> still exists!"
        assert not iface.interface_exist(target, veth0b), \
            f"Interface <{veth0b}> still exists!"

    test.succeed()
