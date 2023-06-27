#!/usr/bin/env python3

import concurrent.futures
import functools
import http.server
import os
import socket
import time

import netifaces

import infamy

SRVPORT = 8008

BUNDLEDIR = os.path.join(
    os.path.dirname(__file__),
    "bundles"
)

PKGPATH = os.path.join(
    BUNDLEDIR,
    "package"
)

class FileServer(http.server.HTTPServer):
    class RequestHandler(http.server.SimpleHTTPRequestHandler):
        def log_message(*args, **kwargs):
            pass

    address_family = socket.AF_INET6

    def __init__(self, server_address, directory):
        rh = functools.partial(FileServer.RequestHandler, directory=directory)
        self.__tp = concurrent.futures.ThreadPoolExecutor(max_workers=1)
        super().__init__(server_address, rh)

    def __enter__(self):
        self.__tp.submit(self.serve_forever)

    def __exit__(self, _, __, ___):
        self.shutdown()
        self.__tp.shutdown()

with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env(infamy.std_topology("1x1"))
        if not env.args.package:
            print("No package supplied")
            test.skip()

        try:
            os.unlink(PKGPATH)
        except FileNotFoundError:
            pass
        os.symlink(os.path.abspath(env.args.package), PKGPATH)

        target = env.attach("target", "mgmt")

        _, hport = env.ltop.xlate("host", "tgt")
        _, tport = env.ltop.xlate("target", "mgmt")
        hip = netifaces.ifaddresses(hport)[netifaces.AF_INET6][0]["addr"]
        hip = hip.replace(f"%{hport}", f"%{tport}")

    with FileServer(("::", SRVPORT), BUNDLEDIR):

        with test.step(f"Start installation of {os.path.basename(env.args.package)}"):
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
