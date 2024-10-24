#!/usr/bin/env python3
"""Interface Description (ifAlias)

Verify interface description (ifAlias) can be set on an interface and
then be read back from the operational datastore.
"""
import infamy
import infamy.iface as iface

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        DESC = "Kilroy was here"

    with test.step("Set up interface target:data with description"):
        _, tport = env.ltop.xlate("target", "data")

        target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": tport,
                        "description": DESC,
                        "enabled": True,
                    }
                ]
            }
        })

    with test.step("Verify description can be read back from operational"):
        text = iface.get_param(target, tport, "description")
        if text != DESC:
            test.fail()

    test.succeed()
