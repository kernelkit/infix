#!/usr/bin/env python3
#
# Verify connectivity with a simple web server container that's been
# given a physical interface instead of an end of a VETH pair.
#
"""
Container with physical interface

Verify connectivity with a simple web server container that's been
given a physical interface instead of an end of a VETH pair.
"""
import base64
import infamy
from   infamy.util import until

with infamy.Test() as test:
    NAME  = "web-phys"
    IMAGE = "curios-httpd-edge.tar.gz"
    DUTIP = "10.0.0.2"
    OURIP = "10.0.0.1"
    URL   = f"http://{DUTIP}:91/index.html"

    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

        if not target.has_model("infix-containers"):
            test.skip()

    with test.step("Create container 'web-phys' from bundled OCI image"):
        _, ifname = env.ltop.xlate("target", "data")

        target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": f"{ifname}",
                        "ipv4": {
                            "address": [{
                                "ip": f"{DUTIP}",
                                "prefix-length": 24
                            }]
                        },
                        "container-network": {}
                    }
                ]
            }
        })
        target.put_config_dict("infix-containers", {
            "containers": {
                "container": [
                    {
                        "name": f"{NAME}",
                        "image": f"oci-archive:{infamy.Container.IMAGE}",
                        "command": "/usr/sbin/httpd -f -v -p 91",
                        "network": {
                            "interface": [
                                { "name": f"{ifname}" }
                            ]
                        }
                    }
                ]
            }
        })

    with test.step("Verify container 'web-phys' has started"):
        c = infamy.Container(target)
        until(lambda: c.running(NAME), attempts=10)

    _, hport = env.ltop.xlate("host", "data")
    url = infamy.Furl(URL)

    with infamy.IsolatedMacVlan(hport) as ns:
        ns.addip(OURIP)
        with test.step("Verify host:data can ping 10.0.0.2"):
            ns.must_reach(DUTIP)

        with test.step("Verify container 'web-phys' is reachable on http://10.0.0.2:91"):
            until(lambda: url.nscheck(ns, "It works"), attempts=10)

    test.succeed()
