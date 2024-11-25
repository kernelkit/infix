#!/usr/bin/env python3
"""
Upgrade

Verify it is possible to upgrade.
"""
import os
import time
import netifaces
import infamy
import infamy.file_server as srv

SRVPORT = 8008

BUNDLEDIR = os.path.join(
    os.path.dirname(__file__),
    "bundles"
)

PKGPATH = os.path.join(
    BUNDLEDIR,
    "package"
)

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
        os.symlink(os.path.abspath(env.args.package), PKGPATH)

        target = env.attach("target", "mgmt")

        _, hport = env.ltop.xlate("host", "mgmt")
        _, tport = env.ltop.xlate("target", "mgmt")
        hip = netifaces.ifaddresses(hport)[netifaces.AF_INET6][0]["addr"]
        hip = hip.replace(f"%{hport}", f"%{tport}")

    with srv.FileServer(("::", SRVPORT), BUNDLEDIR):

        with test.step("Start installation of selected package"):
            print(f"Installing {os.path.basename(env.args.package)}")
            target.call_dict("infix-system", {
                "install-bundle": {
                    "url": f"http://[{hip}]:{SRVPORT}/package",
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

                    test.succeed()

                time.sleep(1)

            print("Timeout, last state:", oper)
            test.fail()
