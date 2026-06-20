#!/usr/bin/env python3
r"""VETH Pair Between Two Containers

Verify that a VETH pair can connect two containers directly, with *both*
ends handed to containers and neither remaining in the host namespace.

....
  .------------.                          .------------.
  |    left    |                          |    right   |
  |  veth0a ===|========= veth ===========|=== veth0b  |
  '------------' 10.0.0.1        10.0.0.2 '------------'
....

The pair is created in the host namespace then each end is moved into
its container when starting up.  Connectivity is verified by pinging
across the pair, from inside one container's network namespace to the
other end's address.

"""

import infamy
from infamy.util import until

# Regression test for #941: previously, when both ends of a pair were
# assigned to a container, neither side created the pair.
with infamy.Test() as test:
    LEFT,  IFACE_LEFT,  IP_LEFT  = "left",  "veth0a", "10.0.0.1"
    RIGHT, IFACE_RIGHT, IP_RIGHT = "right", "veth0b", "10.0.0.2"
    IMAGE = f"oci-archive:{infamy.Container.HTTPD_IMAGE}"

    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        tgtssh = env.attach("target", "mgmt", "ssh")

        if not target.has_model("infix-containers"):
            test.skip()

    with test.step("Create VETH pair with both ends assigned to containers"):
        target.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [
                        {
                            "name": IFACE_LEFT,
                            "type": "infix-if-type:veth",
                            "enabled": True,
                            "infix-interfaces:veth": {"peer": IFACE_RIGHT},
                            "ipv4": {
                                "address": [{"ip": IP_LEFT, "prefix-length": 24}]
                            },
                            "container-network": {}
                        },
                        {
                            "name": IFACE_RIGHT,
                            "type": "infix-if-type:veth",
                            "enabled": True,
                            "infix-interfaces:veth": {"peer": IFACE_LEFT},
                            "ipv4": {
                                "address": [{"ip": IP_RIGHT, "prefix-length": 24}]
                            },
                            "container-network": {}
                        },
                    ]
                }
            },
            "infix-containers": {
                "containers": {
                    "container": [
                        {
                            "name": LEFT,
                            "image": IMAGE,
                            "command": "/usr/sbin/httpd -f -v -p 91",
                            "network": {"interface": [{"name": IFACE_LEFT}]}
                        },
                        {
                            "name": RIGHT,
                            "image": IMAGE,
                            "command": "/usr/sbin/httpd -f -v -p 91",
                            "network": {"interface": [{"name": IFACE_RIGHT}]}
                        },
                    ]
                }
            }
        })

    c = infamy.Container(target)
    with test.step("Verify both containers have started"):
        until(lambda: c.running(LEFT), attempts=60)
        until(lambda: c.running(RIGHT), attempts=60)

    with test.step(f"Verify {LEFT} reaches {RIGHT} over the internal VETH pair"):
        pid = tgtssh.runsh(f"sudo podman inspect --format '{{{{.State.Pid}}}}' {LEFT}").stdout.strip()
        assert pid.isdigit(), f"failed to get pid for container {LEFT}: {pid!r}"

        def reachable():
            return tgtssh.runsh(f"sudo nsenter -t {pid} -n ping -c 2 -w 5 {IP_RIGHT}").returncode == 0

        until(reachable, attempts=30)

    test.succeed()
