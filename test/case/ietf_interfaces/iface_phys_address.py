#!/usr/bin/env python3

import copy
import infamy
import infamy.iface as iface


with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env(infamy.std_topology("1x2"))
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
                    "type": "infix-if-type:ethernet",
                    "phys-address": f"{cmac}"
                }]
            }
        }
        target.put_config_dict("ietf-interfaces", config)
        mac = iface.get_phys_address(target, tport)
        print(f"Target iface {tport} current mac: {mac}")
        assert mac == cmac

    with test.step(f"Remove custom MAC address"):
        running = target.get_config_dict("/ietf-interfaces:interfaces")
        new = copy.deepcopy(running)
        for i in new["interfaces"]["interface"]:
            if i["name"] == tport:
                del i["phys-address"]
                break
        target.put_diff_dicts("ietf-interfaces", running, new)
        mac = iface.get_phys_address(target, tport)
        print(f"Target iface {tport} current mac: {mac}")
        assert mac == pmac

    test.succeed()
