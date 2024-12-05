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

class Uboot:
    def __init__(self, ssh):
        self.ssh=ssh

    def get_boot_order(self):
        order=self.ssh.runsh("sudo fw_printenv BOOT_ORDER").stdout.split("=")
        return order[1].strip()

    def set_boot_order(self, order):
        return self.ssh.run(f"sudo fw_setenv BOOT_ORDER '{order}'".split()).returncode

class Grub:
    def __init__(self, ssh):
        self.ssh = ssh

    def get_boot_order(self):
        lines=self.ssh.runsh("grub-editenv /mnt/aux/grub/grubenv list").stdout.split("\n")
        for line in lines:
            if "ORDER" in line:
                return line.split("=")[1].strip()

    def set_boot_order(self, order):
        return self.ssh.run(f"sudo grub-editenv /mnt/aux/grub/grubenv set ORDER='{order}'".split()).returncode

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
        target_ssh = env.attach("target", "mgmt", "ssh")
        if target_ssh.run("test -e /sys/firmware/devicetree/base/chosen/u-boot,version".split()).returncode == 0:
            bootloader=Uboot(target_ssh)
        elif target_ssh.run("test -e /mnt/aux/grub/grubenv".split()).returncode == 0:
            bootloader=Grub(target_ssh)
        else:
            print("No supported bootloader found")
            test.skip()

        old_bootorder=bootloader.get_boot_order()
        print(f"Initial bootorder: {repr(old_bootorder)}")

        _, hport = env.ltop.xlate("host", "data")
        _, tport = env.ltop.xlate("target", "data")

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
                    if "last-error" in installer:
                        print("Install failed:", installer["last-error"])
                        test.fail()

                    break
                time.sleep(1)
            else:
                print("Timeout, last state:", oper)
                test.fail()

        with test.step("Verify boot order has changed and reboot"):
            assert(old_bootorder != bootloader.get_boot_order())
            target.reboot()

            if not wait_boot(target, env):
                test.fail()
            target = env.attach("target", "mgmt", "netconf")


        with test.step("Verify that the partition is the booted"):
            should_boot=bootloader.get_boot_order().split()[0]
            oper = target.get_dict("/system-state/software")
            booted = oper["system-state"]["software"]["booted"]
            print(f"Should boot: {should_boot}, booted: {booted}")
            assert(booted == should_boot)

        with test.step("Restore boot order to original configured"):
            print(f"Restore boot order to {old_bootorder}")
            if bootloader.set_boot_order(old_bootorder) != 0:
                test.fail()
            target = env.attach("target", "mgmt", "netconf")
            target.reboot()
            if not wait_boot(target, env):
                test.fail()
            target = env.attach("target", "mgmt", "netconf")

        with test.step("Verify the boot order is the orignal configured"):
            order = bootloader.get_boot_order()
            assert order == old_bootorder, f"Unexpected bootorder: {repr(order)}"

    test.succeed()
