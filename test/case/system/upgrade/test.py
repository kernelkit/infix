#!/usr/bin/env python3
"""
System upgrade

Verify system upgrade functionality.
"""
# NOTE: THIS TEST IS HARDCODED TO NETCONF
# There is a bug somewhere in the restconf-code (infamy or rousette)
import os
import time
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
    # yangerd populates boot-order from the bootloader env via the
    # fswatcher; right after a reboot/restart it may not be there yet, so
    # return None rather than KeyError and let callers poll.
    oper = target.get_dict("/ietf-system:system-state/software")
    bo = (oper or {}).get("system-state", {}).get("software", {}).get("boot-order")
    return " ".join(bo) if bo else None

def get_booted(target):
    oper = target.get_dict("/ietf-system:system-state/software")
    return (oper or {}).get("system-state", {}).get("software", {}).get("booted")

def get_installer(target):
    # Empty while yangerd restarts, e.g. after the OOM killer picks it
    # during the install; callers poll.
    try:
        oper = target.get_dict("/ietf-system:system-state/software")
    except Exception:
        return None
    return (oper or {}).get("system-state", {}).get("software", {}).get("installer")

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
    until(lambda: get_boot_order(target) == old_bootorder, attempts=30)

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

        hport = env["host"]["data"]
        tport = target["data"]
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

        with test.step("Wait for upgrade to start"):
            for _ in range(600):
                installer = get_installer(target)
                if installer and installer["operation"] != "idle":
                    print(installer)
                    error = installer.get("last-error")
                    message = installer.get("progress", {}).get("message", "")
                    if error or "failed" in message.lower():
                        test.fail(f"Install failed: {error or message}")

                    break
                time.sleep(1)
            else:
                print("Timeout, last state:", installer)
                test.fail()

        with test.step("Wait for upgrade to finish"):
            for _ in range(600):
                installer = get_installer(target)
                if installer and installer["operation"] == "idle":
                    print(installer)
                    if "last-error" in installer:
                        print("Install failed:", installer["last-error"])
                        test.fail()

                    break
                time.sleep(1)
            else:
                print("Timeout, last state:", installer)
                test.fail()

        with test.step("Verify boot order has changed and reboot"):
            print(get_boot_order(target))
            print(old_bootorder)
            # Wait for a populated boot-order that differs from the
            # original; None means operational has not caught up yet.
            until(lambda: get_boot_order(target) not in (None, old_bootorder),
                  attempts=30)
            target.reboot()

            if not wait_boot(target, env):
                test.fail()
            target = env.attach("target", "mgmt", "netconf")


        with test.step("Verify that the partition is the booted"):
            # After the reboot yangerd repopulates boot-order from the
            # bootloader env on startup; poll until it is available.
            until(lambda: get_boot_order(target) is not None, attempts=30)
            should_boot = get_boot_order(target).split()[0]
            booted = until(lambda: get_booted(target), attempts=30)
            print(f"Should boot: {should_boot}, booted: {booted}")
            assert(booted == should_boot)

    test.succeed()
