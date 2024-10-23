#!/usr/bin/env python3
#
# Verify connectivity with a simple web server container from behind a
# regular bridge, a VETH pair connects the container to the bridge.
#
r"""
Container with VETH pair

Verify connectivity with a simple web server container from behind a
regular bridge, a VETH pair connects the container to the bridge.

....
  .-------------.         .---------------.     .--------.
  |      | mgmt |---------| mgmt |        |     |  web-  |
  | host | data |---------| data | target |     | server |
  '-------------'         '---------------'     '--------'
                             |                    /
                            br0                  /
                              `----- veth0 -----'
....

"""
import base64
import infamy
from   infamy.util import until

with infamy.Test() as test:
    NAME  = "web-br0-veth"
    DUTIP = "10.0.0.2"
    OURIP = "10.0.0.1"
    URL   = f"http://{DUTIP}:91/index.html"

    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

        if not target.has_model("infix-containers"):
            test.skip()

    with test.step("Create 'web-br0-veth' container from bundled OCI image"):
        _, ifname = env.ltop.xlate("target", "data")
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
                        "image": f"oci-archive:{infamy.Container.HTTPD_IMAGE}",
                        "command": "/usr/sbin/httpd -f -v -p 91",
                        "network": {
                            "interface": [
                                { "name": f"{NAME}" }
                            ]
                        }
                    }
                ]
            }
        })

    with test.step("Verify container 'web-br0-veth' has started"):
        c = infamy.Container(target)
        until(lambda: c.running(NAME), attempts=10)

    _, hport = env.ltop.xlate("host", "data")
    url = infamy.Furl(URL)

    with infamy.IsolatedMacVlan(hport) as ns:
        ns.addip(OURIP)
        with test.step("Verify basic DUT connectivity, host:data can ping DUT 10.0.0.2"):
            ns.must_reach(DUTIP)
        with test.step("Verify container 'web-br0-veth' is reachable on http://10.0.0.2:91"):
            until(lambda: url.nscheck(ns, "It works"), attempts=10)

    test.succeed()
