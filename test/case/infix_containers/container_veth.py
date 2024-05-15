#!/usr/bin/env python3
#
# Verify connectivity with a simple web server container from behind a
# regular bridge, a VETH pair connects the container to the bridge.
#

import base64
import infamy
from   infamy.util import until

with infamy.Test() as test:
    NAME  = "web-br0-veth"
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
                        "infix-interfaces:bridge-port": {
                            "bridge": "br0"
                        }
                    },
                    {
                        "name": "br0",
                        "type": "infix-if-type:bridge"
                    },
                    {
                        "name": f"{NAME}",
                        "type": "infix-if-type:veth",
                        "infix-interfaces:veth": {
                            "peer": "veth0b"
                        },
                        "ipv4": {
                            "address": [{
                                "ip": f"{DUTIP}",
                                "prefix-length": 24
                            }]
                        },
                        "container-network": {}
                    },
                    {
                        "name": "veth0b",
                        "type": "infix-if-type:veth",
                        "infix-interfaces:veth": {
                            "peer": f"{NAME}"
                        },
                        "infix-interfaces:bridge-port": {
                            "bridge": "br0"
                        }
                    },
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
                                { "name": f"{NAME}" }
                            ]
                        }
                    }
                ]
            }
        })

    with test.step(f"Verify {NAME} continer has started"):
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
