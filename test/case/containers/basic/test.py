#!/usr/bin/env python3
"""Container Basic

Verify that a simple web server container can be configured to run
with host networking, on port 80.  Operation is verified using a
simple GET request for index.html and checking for a key phrase.

The RPC actions: stop + start, and restart are also verified.
"""
import infamy
from infamy.util import until, curl


def _verify(server, silent=False):
    # TODO: Should really use mDNS here....
    url = f"http://[{server}]:91/index.html"
    response = curl(url, silent=silent)
    return response is not None and "It works" in response


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
                        "image": f"oci-archive:{infamy.Container.HTTPD_IMAGE}",
                        "command": "/usr/sbin/httpd -f -v -p 91",
                        "network": {
                            "host": True
                        },
                        "resource-limit": {
                            "memory": 512,  # 512 KiB
                            "cpu": 500      # 50% of one CPU (0.5 cores)
                        }
                    }
                ]
            }
        })

    with test.step("Verify container 'web' has started"):
        c = infamy.Container(target)
        until(lambda: c.running(NAME), attempts=60)

    with test.step("Verify container 'web' is reachable on http://container-host.local:91"):
        until(lambda: _verify(addr, silent=True), attempts=10)

    with test.step("Verify resource constraints and usage are available"):
        data = target.get_data("/infix-containers:containers")
        containers = data.get("containers", {}).get("container", [])
        web = next((c for c in containers if c["name"] == NAME), None)

        limits = web.get("resource-limit", {})
        assert limits.get("memory") == 512, "Memory limit not set correctly"
        assert limits.get("cpu") == 500, "CPU limit not set correctly"

        rusage = web.get("resource-usage", {})
        assert rusage is not None, "Resource usage data not available"

        mem_used = rusage.get("memory")
        assert mem_used is not None, "Memory usage not reported"
        print(f"Container using {mem_used} KiB memory")

    with test.step("Stop container 'web'"):
        c = infamy.Container(target)
        c.action(NAME, "stop")

    with test.step("Verify container 'web' is stopped"):
        until(lambda: not c.running(NAME), attempts=30)

    with test.step("Restart container 'web'"):
        c.action(NAME, "restart")

    with test.step("Verify container 'web' is reachable on http://container-host.local:91"):
        # Wait for it to restart and respond, or fail
        until(lambda: _verify(addr, silent=True), attempts=60)
    test.succeed()
