#!/usr/bin/env python3

import copy
import infamy
import infamy.iface as iface

from infamy.util import until

with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, tport = env.ltop.xlate("target", "data")
        pmac = iface.get_phys_address(target, tport)
        cmac = "02:01:00:c0:ff:ee"
        print(f"Target iface {tport} original mac {pmac}")

    with test.step("Set custom MAC address"):
        config = {
            "interfaces": {
                "interface": [{
                    "name": f"{tport}",
                    "phys-address": f"{cmac}"
                }]
            }
        }
        target.put_config_dict("ietf-interfaces", config)
        mac = iface.get_phys_address(target, tport)
        print(f"Target iface {tport} current mac: {mac}")
        assert mac == cmac

    with test.step(f"Remove custom MAC address"):
        xpath=iface.get_iface_xpath(tport, "phys-address")
        target.delete_xpath(xpath)

        until(lambda: iface.get_phys_address(target, tport) == pmac)

    test.succeed()
