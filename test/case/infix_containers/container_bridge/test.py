#!/usr/bin/env python3
"""
Container with bridge network

Verify connectivity with a simple web server container from behind a
docker0 bridge.  As an added twist, this test also verifies content
mounts, i.e., custom index.html from running-config.

This also verifies port forwarding from container internal port to a
port accessed from the host.
"""
import base64
import infamy
from   infamy.util import until, to_binary

with infamy.Test() as test:
    NAME  = "web-docker0"
    DUTIP = "10.0.0.2"
    OURIP = "10.0.0.1"
    MESG  = "It works"
    URL   = f"http://{DUTIP}:8080/index.html"

    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

        if not target.has_model("infix-containers"):
            test.skip()

    with test.step("Create httpd container from bundled OCI image"):
        _, ifname = env.ltop.xlate("target", "data")
        target.put_config_dicts({
            "ietf-interfaces": {
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
                        }, {
                            "name": "docker0",
                            "type": "infix-if-type:bridge",
                            "container-network": {
                                "type": "bridge",
                                "subnet": [
                                    {"subnet": "172.17.0.0/16"},
                                    {"subnet": "2a02:2789:724:eb8:1::/80"}
                                ]
                            }
                        }
                    ]
                }
            },
            "infix-containers": {
                "containers": {
                    "container": [{
                        "name": f"{NAME}",
                        "image": f"oci-archive:{infamy.Container.HTTPD_IMAGE}",
                        "command": "/usr/sbin/httpd -f -v",
                        "network": {
                            "interface": [{
                                "name": "docker0",
                                "option": [
                                    "interface_name=wan",
                                    "ip=172.17.0.2"
                                ],
                            }],
                            "publish": ["8080:80"]
                        }
                    }]
                }
            }})

    with test.step("Verify container has started"):
        c = infamy.Container(target)
        until(lambda: c.running(NAME), attempts=10)

    _, hport = env.ltop.xlate("host", "data")
    url = infamy.Furl(URL)

    with infamy.IsolatedMacVlan(hport) as ns:
        ns.addip(OURIP)

        with test.step("Verify DUT connectivity, host can reach 10.0.0.2"):
            ns.must_reach(DUTIP)

        with test.step("Verify container is reachable on http://10.0.0.2:8080"):
            until(lambda: url.nscheck(ns, MESG), attempts=10)

    test.succeed()
