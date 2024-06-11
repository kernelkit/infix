#!/usr/bin/env python3
#
# Verify connectivity with a simple web server container from behind a
# docker0 bridge.  As an added twist, this test also verifies content
# mounts, i.e., custom index.html from running-config.
#

import base64
import infamy
from   infamy.util import until

with infamy.Test() as test:
    NAME  = "web-docker0"
    IMAGE = "curios-httpd-edge.tar.gz"
    DUTIP = "10.0.0.2"
    OURIP = "10.0.0.1"
    BODY  = "<html><body><p>Kilroy was here</p></body></html>"
    URL   = f"http://{DUTIP}:8080/index.html"

    with test.step("Initialize"):
        env = infamy.Env(infamy.std_topology("1x2"))
        target = env.attach("target", "mgmt")

    with test.step(f"Create {NAME} container from bundled OCI image"):
        _, ifname = env.ltop.xlate("target", "data")
        enc = base64.b64encode(BODY.encode('utf-8'))
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
                        }
                    },
                    {
                        "name": "docker0",
                        "type": "infix-if-type:bridge",
                        "container-network": {
                            "type": "bridge",
                            "subnet": [
                                { "subnet": "172.17.0.0/16" },
                                { "subnet": "2a02:2789:724:eb8:1::/80" }
                            ]
                        }
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
                        "mount": [
                            {
                                "name": "index.html",
                                "content": f"{enc.decode('utf-8')}",
                                "target": "/var/www/index.html"
                            }
                        ],
                        "network": {
                            "interface": [
                                { "name": "docker0" }
                            ],
                            "publish": [ "8080:80" ]
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

            until(lambda: url.nscheck(ns, "Kilroy was here"), attempts=10)

    test.succeed()
