#!/usr/bin/env python3
"""Verify that VETH pairs can be deleted

```
     veth0b       veth0a        data1    data2
          `---------'
```

Each test step to create, add address, or delete an interace is distinct
from any other step.  This to trigger a new configuration "generation".

"""

import infamy
import infamy.iface as iface


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

        _, data1 = env.ltop.xlate("target", "data1")
        _, data2 = env.ltop.xlate("target", "data2")

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

    with test.step("Verify interfaces 'veth0a' and 'veth0b' exist"):
        assert iface.exist(target, veth0a), \
            f"Interface <{veth0a}> does not exist."
        assert iface.exist(target, veth0b), \
            f"Interface <{veth0b}> does not exist."

    with test.step("Set IP address on target:data1 (dummy op)"):
        target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [{
                    "name": f"{data1}",
                    "ipv4": {
                        "address": [{
                            "ip": "10.0.0.1",
                            "prefix-length": 24
                        }]
                    }
                }]
            }
        })

    with test.step("Set IP address on target:data2 (dummy op)"):
        target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [{
                    "name": f"{data2}",
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

    with test.step("Verify target:data1 and target:data2 still exist"):
        assert iface.exist(target, data1), \
            f"Interface {data1} missing!"
        assert iface.exist(target, data2), \
            f"Interface {data2} missing!"

    with test.step("Verify VETH pair have been removed"):
        assert not iface.exist(target, veth0a), \
            f"Interface <{veth0a}> still exists!"
        assert not iface.exist(target, veth0b), \
            f"Interface <{veth0b}> still exists!"

    test.succeed()
