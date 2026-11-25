#!/usr/bin/env python3
"""
Container with bridge network

Verify connectivity with a simple web server container from behind a
bridge, the classic docker0 setup built from regular interfaces: the
container gets one end of a VETH pair, with a static address and a
default route, and the other end is a port in the bridge.

The firewall forwards port 8080 on the outside to port 80 in the
container, which has no other way in from the outside.
"""
import infamy
from   infamy.util import until, curl

with infamy.Test() as test:
    NAME  = "web-br0"
    DUTIP = "10.0.0.2"
    OURIP = "10.0.0.1"
    BRIP  = "172.17.0.1"
    WEBIP = "172.17.0.2"
    MESG  = "It works"
    URL   = f"http://{DUTIP}:8080/index.html"

    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        mgmt = target["mgmt"]
        data = target["data"]

        if not target.has_model("infix-containers"):
            test.skip()

    with test.step("Create bridge br0, 172.17.0.1/16, with VETH pair veth0a to veth0b"):
        target.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [
                        {
                            "name": data,
                            "ipv4": {
                                "forwarding": True,
                                "address": [{
                                    "ip": DUTIP,
                                    "prefix-length": 24
                                }]
                            }
                        }, {
                            "name": "br0",
                            "type": "infix-if-type:bridge",
                            "ipv4": {
                                "forwarding": True,
                                "address": [{
                                    "ip": BRIP,
                                    "prefix-length": 16
                                }]
                            }
                        }, {
                            "name": "veth0a",
                            "type": "infix-if-type:veth",
                            "infix-interfaces:veth": {
                                "peer": "veth0b"
                            },
                            "infix-interfaces:bridge-port": {
                                "bridge": "br0"
                            }
                        }, {
                            "name": "veth0b",
                            "type": "infix-if-type:veth",
                            "infix-interfaces:veth": {
                                "peer": "veth0a"
                            },
                            "ipv4": {
                                "address": [{
                                    "ip": WEBIP,
                                    "prefix-length": 16
                                }]
                            },
                            "container-network": {
                                "route": [{
                                    "subnet": "0.0.0.0/0",
                                    "gateway": BRIP
                                }]
                            }
                        }
                    ]
                }
            }})

    with test.step("Forward port 8080 on the data interface to 172.17.0.2 port 80"):
        target.put_config_dicts({
            "infix-firewall": {
                "firewall": {
                    "default": "wan",
                    "zone": [{
                        "name": "wan",
                        "action": "drop",
                        "interface": [data],
                        "port-forward": [{
                            "lower": 8080,
                            "proto": "tcp",
                            "to": {
                                "addr": WEBIP,
                                "port": 80
                            }
                        }]
                    }, {
                        "name": "lan",
                        "action": "accept",
                        "interface": ["br0", mgmt]
                    }]
                }
            }})

    with test.step("Create httpd container from bundled OCI image, on veth0b"):
        target.put_config_dicts({
            "infix-containers": {
                "containers": {
                    "container": [{
                        "name": NAME,
                        "image": infamy.Container.HTTPD_IMAGE,
                        "command": "/usr/sbin/httpd -f -v",
                        "network": {
                            "interface": [{
                                "name": "veth0b",
                                "option": [
                                    "interface_name=wan"
                                ],
                            }]
                        }
                    }]
                }
            }})

    with test.step("Verify container has started"):
        c = infamy.Container(target)
        until(lambda: c.running(NAME))

    hport = env["host"]["data"]

    with infamy.IsolatedMacVlan(hport) as ns:
        ns.addip(OURIP)

        with test.step("Verify container is reachable on http://10.0.0.2:8080"):
            until(lambda: MESG in ns.call(lambda: curl(URL)))

    test.succeed()
