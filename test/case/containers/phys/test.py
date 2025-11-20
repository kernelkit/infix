#!/usr/bin/env python3
"""
Container with physical interface

Verify connectivity with a simple web server container that's been
given a physical interface instead of an end of a VETH pair.
"""
import infamy
from   infamy.util import until, to_binary, curl

with infamy.Test() as test:
    NAME  = "web-phys"
    DUTIP = "10.0.0.2"
    OURIP = "10.0.0.1"
    MESG1 = "It works"
    MESG2  = "Kilroy was here"
    BODY  = f"<html><body><p>{MESG2}</p></body></html>"
    URL   = f"http://{DUTIP}:91/index.html"

    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

        if not target.has_model("infix-containers"):
            test.skip()

    with test.step("Create httpd container from bundled OCI image"):
        _, ifname = env.ltop.xlate("target", "data")

        target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [{
                    "name": f"{ifname}",
                    "ipv4": {
                        "address": [{
                            "ip": f"{DUTIP}",
                            "prefix-length": 24
                        }]
                    },
                    "container-network": {}
                }]
            }
        })
        target.put_config_dict("infix-containers", {
            "containers": {
                "container": [{
                    "name": f"{NAME}",
                    "image": f"oci-archive:{infamy.Container.HTTPD_IMAGE}",
                    "command": "/usr/sbin/httpd -f -v -p 91",
                    "network": {
                        "interface": [
                            {"name": f"{ifname}"}
                        ]
                    }
                }]
            }
        })

    with test.step("Verify container has started"):
        c = infamy.Container(target)
        until(lambda: c.running(NAME), attempts=60)

    _, hport = env.ltop.xlate("host", "data")

    with infamy.IsolatedMacVlan(hport) as ns:
        ns.addip(OURIP)
        with test.step("Verify host:data can ping 10.0.0.2"):
            ns.must_reach(DUTIP)

        with test.step("Verify container is reachable on http://10.0.0.2:91"):
            until(lambda: MESG1 in ns.call(lambda: curl(URL)), attempts=10)

        with test.step("Add a content mount, overriding index.html"):
            # Verify modifying a running container takes, issue #930
            data = to_binary(BODY)

            target.put_config_dict("infix-containers", {
                "containers": {
                    "container": [{
                        "name": f"{NAME}",
                        "mount": [{
                            "name": "index.html",
                            "content": f"{data}",
                            "target": "/var/www/index.html"
                        }]
                    }]
                }
            })

        with test.step("Verify server is restarted and returns new content"):
            until(lambda: MESG2 in ns.call(lambda: curl(URL)), attempts=60)

    test.succeed()
