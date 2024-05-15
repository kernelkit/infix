#!/usr/bin/env python3
#
# Verify connectivity with a simple web server container that's been
# given a physical interface instead of an end of a VETH pair.
#

import base64
import infamy
from   infamy.util import until

with infamy.Test() as test:
    NAME  = "web-phys"
    IMAGE = "curios-httpd-edge.tar.gz"
    DUTIP = "10.0.0.2"
    OURIP = "10.0.0.1"
    URL   = f"http://{DUTIP}/index.html"

    with test.step("Initialize"):
        env = infamy.Env(infamy.std_topology("1x2"))
        target = env.attach("target", "mgmt")

    with test.step(f"Create {NAME} container from bundled OCI image"):
        _, ifname = env.ltop.xlate("target", "data")

        target.put_config_dict("infix-services", {
            "web": {
                "enabled": False
            }
        })
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
                        "network": {
                            "interface": [
                                { "name": f"{ifname}" }
                            ]
                        }
                    }
                ]
            }
        })

    with test.step(f"Verify {NAME} container has started"):
        c = infamy.Container(target)
        until(lambda: c.running(NAME), attempts=10)

    with test.step(f"Verify {NAME} container responds"):
        _, hport = env.ltop.xlate("host", "data")
        url = infamy.Furl(URL)

        with infamy.IsolatedMacVlan(hport) as ns:
            ns.addip(OURIP)
            ns.must_reach(DUTIP)

            until(lambda: url.nscheck(ns, "It works"), attempts=10)

    test.succeed()
