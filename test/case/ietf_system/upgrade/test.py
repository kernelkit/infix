#!/usr/bin/env python3
"""
Upgrade

Verify it is possible to upgrade.
"""
# NOTE: THIS TEST IS HARDCODED TO NETCONF
# There is a bug somewhere in the restconf-code (infamy or rousette)
import os
import time
import netifaces
import infamy
import infamy.file_server as srv
from infamy.util import wait_boot, until

SRVPORT = 8008

BUNDLEDIR = os.path.join(
    os.path.dirname(__file__),
    "bundles"
)

bootloader=None
PKGPATH = os.path.join(
    BUNDLEDIR,
    "package"
)
def get_boot_order(target):
    oper = target.get_dict("/system-state/software")
    return " ".join(oper["system-state"]["software"]["boot-order"])

def set_boot_order(target, order):
    target.call_dict("infix-system", {
        "set-boot-order": {
            "boot-order": order.split(" ")
        }
    })


def cleanup(env, old_bootorder):
    print(f"Restore boot order to {old_bootorder}")
    target = env.attach("target", "mgmt", "netconf")
    set_boot_order(target, old_bootorder)
    target.reboot()
    if not wait_boot(target, env):
        test.fail()
    target = env.attach("target", "mgmt", "netconf")

    print("Verify the boot order is the orignal configured")
    order = get_boot_order(target)
    assert order == old_bootorder, f"Unexpected bootorder: {repr(order)}"

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        if not env.args.package:
            print("No package supplied")
            test.skip()

        try:
            os.unlink(PKGPATH)
        except FileNotFoundError:
            pass
        #os.unlink(PKGPATH)
        os.symlink(os.path.abspath(env.args.package), PKGPATH)

        target = env.attach("target", "mgmt", "netconf")

        old_bootorder=get_boot_order(target)
        print(f"Initial bootorder: {repr(old_bootorder)}")

        _, hport = env.ltop.xlate("host", "data")
        _, tport = env.ltop.xlate("target", "data")
        test.push_test_cleanup(lambda: cleanup(env, old_bootorder))

    netns = infamy.IsolatedMacVlan(hport).start()
    netns.addip("192.168.0.1")

    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    {
                        "name": tport,
                        "ipv4": {
                            "address": [
                                {
                                    "ip": "192.168.0.2",
                                    "prefix-length": 24
                                }
                            ]
                        }
                    }
                ]
            }
        }
    })
    netns.must_reach("192.168.0.2")
    with srv.FileServer(netns, "192.168.0.1", SRVPORT, BUNDLEDIR):
        with test.step("Start installation of selected package"):
            print(f"Installing {os.path.basename(env.args.package)}")

            target.call_dict("infix-system", {
                "install-bundle": {
                    "url": f"http://192.168.0.1:{SRVPORT}/package",
                }
            })

        with test.step("Wait for upgrade to finish"):
            for _ in range(600):
                oper = target.get_dict("/system-state/software")
                installer = oper["system-state"]["software"]["installer"]
                if installer["operation"] == "idle":
                    print(installer)
                    if "last-error" in installer:
                        print("Install failed:", installer["last-error"])
                        test.fail()

                    break
                time.sleep(1)
            else:
                print("Timeout, last state:", oper)
                test.fail()

        with test.step("Verify boot order has changed and reboot"):
            print(get_boot_order(target))
            print(old_bootorder)
            assert(old_bootorder != get_boot_order(target))
            target.reboot()

            if not wait_boot(target, env):
                test.fail()
            target = env.attach("target", "mgmt", "netconf")


        with test.step("Verify that the partition is the booted"):
            should_boot=get_boot_order(target).split()[0]
            oper = target.get_dict("/system-state/software")
            booted = oper["system-state"]["software"]["booted"]
            print(f"Should boot: {should_boot}, booted: {booted}")
            assert(booted == should_boot)

    test.succeed()
