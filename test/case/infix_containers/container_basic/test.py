#!/usr/bin/env python3
#
# Verify that a simple web server container can be configured to run
# with host networking, on port 80.  Operation is verified using a
# simple GET request for index.html and checking for a key phrase.
#
# The RPC actions: stop + start, and restart are also verified.
#
"""
Container basic

Verify that a simple web server container can be configured to run
with host networking, on port 80.  Operation is verified using a
simple GET request for index.html and checking for a key phrase.

The RPC actions: stop + start, and restart are also verified.
"""
import infamy
from infamy.util import until


def _verify(server):
    # Should really use mDNS here....
    url = infamy.Furl(f"http://[{server}]:91/index.html")
    return url.check("It works")


with infamy.Test() as test:
    NAME = "web"

    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        addr = target.get_mgmt_ip()

        if not target.has_model("infix-containers"):
            test.skip()

    with test.step("Set hostname to 'container-host'"):
        target.put_config_dict("ietf-system", {
            "system": {
                "hostname": "container-host"
                }
            })

    with test.step("Create container 'web' from bundled OCI image"):
        target.put_config_dict("infix-containers", {
            "containers": {
                "container": [
                    {
                        "name": f"{NAME}",
                        "image": f"oci-archive:{infamy.Container.IMAGE}",
                        "command": "/usr/sbin/httpd -f -v -p 91",
                        "network": {
                            "host": True
                        }
                    }
                ]
            }
        })

    with test.step("Verify container 'web' has started"):
        c = infamy.Container(target)
        until(lambda: c.running(NAME), attempts=10)

    with test.step("Verify container 'web' is reachable on http://container-host.local:91"):
        until(lambda: _verify(addr), attempts=10)

    with test.step("Stop container 'web'"):
        c = infamy.Container(target)
        c.action(NAME, "stop")

    with test.step("Verify container 'web' is stopped"):
        until(lambda: not c.running(NAME), attempts=10)

    with test.step("Restart container 'web'"):
        c.action(NAME, "restart")

    with test.step("Verify container 'web' is reachable on http://container-host.local:91"):
        # Wait for it to restart and respond, or fail
        until(lambda: _verify(addr), attempts=10)
    test.succeed()
