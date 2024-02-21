#!/usr/bin/env python3

import infamy
import copy
import infamy.usb as usb
import time
import infamy.netconf as netconf

from infamy.util import until
def remove_config(target):
    running = target.get_config_dict("/ietf-hardware:hardware")
    new = copy.deepcopy(running)
    new["hardware"].clear()
    target.put_diff_dicts("ietf-hardware",running,new)

with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env(infamy.std_topology("1x1"))
        target = env.attach("target", "mgmt")
        available=usb.get_usb_ports(target)

        if len(available) < 1:
            test.skip()

    with test.step("Lock USB ports"):
        components=[]
        for port in available:
            component = {
                "name": port,
                "state": {
                    "admin-state": "locked"
                }
            }
            components.append(component)

        target.put_config_dict("ietf-hardware", {
            "hardware": {
                "component": components
            }
        })

    with test.step("Verify USB ports locked"):
        for port in available:
            until(lambda: usb.get_usb_state(target, port) == "locked")


    with test.step("Unlock USB ports"):
        components=[]
        for port in available:
            component = {
                "name": port,
                "state": {
                    "admin-state": "unlocked"
                }
            }
            components.append(component)

        target.put_config_dict("ietf-hardware", {
            "hardware": {
                "component": components
            }
        })


    with test.step("Verify USB ports unlocked"):
        for port in available:
            until(lambda: usb.get_usb_state(target, port) == "unlocked")

    if len(available) > 1:
        with test.step("Lock one port"):
            components=[]
            component = {
                "name": available[1],
                "class": "infix-hardware:usb",
                "state": {
                    "admin-state": "locked"
                }
            }
            components.append(component)

        target.put_config_dict("ietf-hardware", {
            "hardware": {
                "component": components
            }
        })
        with test.step("Verify one port is locked and one unlocked"):
            until(lambda: usb.get_usb_state(target, available[1]) == "locked")
            until(lambda: usb.get_usb_state(target, available[0]) == "unlocked")

    with test.step("Remove all hardware configuration"):
        remove_config(target)

    with test.step("Verify USB ports locked"):
        for port in available:
            until(lambda: usb.get_usb_state(target, port) == "locked")


    with test.step("Unlock USB ports"):
        components=[]
        for port in available:
            component = {
                "name": port,
                "class": "infix-hardware:usb",
                "state": {
                    "admin-state": "unlocked"
                }
            }
            components.append(component)

        target.put_config_dict("ietf-hardware", {
            "hardware": {
                "component": components
            }
        })

    with test.step("Verify USB ports unlocked"):
        for port in available:
            until(lambda: usb.get_usb_state(target, port) == "unlocked")

    with test.step("Remove USB configuration, and reboot"):
        remove_config(target)
        target.copy("running", "startup")
        target.reboot()
        until(lambda: target.reachable() == False, attempts = 100)
        print("Device reboots..")
        until(lambda: target.reachable() == True, attempts = 300)
        print("Device has come online")
        iface=target.get_mgmt_iface()
        neigh=infamy.neigh.ll6ping(iface)
        assert(neigh)
        until(lambda: netconf.netconf_syn(neigh) == True, attempts = 300)
        print("NETCONF reachable")
        target = env.attach("target", "mgmt", factory_default = False)

    with test.step("Verify that all ports are locked"):
        for port in available:
            until(lambda: usb.get_usb_state(target, port) == "locked")
    test.succeed()
