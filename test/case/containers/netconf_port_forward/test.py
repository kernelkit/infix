#!/usr/bin/env python3
r"""Container NETCONF Port Forwarding

Verify that a NETCONF agent in a container can be reached on port 830
from the outside, and that the agent can reach the NETCONF server on
the target over the internal network.

....
                                                         <-- container -->
.-------------.            .----------------------.      .---------------.
|      | mgmt |------------| mgmt |        |      |      |      |  nc    |
| host | data |------------| ext0 | target | int0 |------| eth0 | agent  |
'-------------'.42       .1'----------------------'.1  .2'---------------'
              192.168.0.0/24                       10.0.0.0/24
                                                    VETH pair
....

The target's firewall puts `ext0` in a `wan` zone, which drops all
traffic to the target except TCP port 830.  That port is forwarded to
the container at 10.0.0.2:830, so a connection to port 830 on `ext0`
ends up in the container, not at the target's NETCONF server.  The
target end of the VETH pair, `int0`, is in an `int` zone that allows
only the `netconf` service.

The agent is a netcat listener on port 830.  For each connection it
prints a known greeting, connects to the target at 10.0.0.1:830, and
relays the first line it receives, which is the SSH banner of the
target's NETCONF server.

The test host connects to 192.168.0.1:830.  If the greeting arrives,
the port forward works.  If the SSH banner follows it, the container
reached NETCONF on the target.
"""
import infamy
from infamy import netutil
from infamy.util import until, to_binary


with infamy.Test() as test:
    NFTABLES = f"oci-archive:{infamy.Container.NFTABLES_IMAGE}"
    NETCONF_CONTAINER_IP = "10.0.0.2"
    INTIP = "10.0.0.1"
    EXTIP = "192.168.0.1"
    OURIP = "192.168.0.42"
    NETCONF_CONTAINER = "netconf_container"
    NETCONF_IF = "netconf0"
    GREETING = "Hello from the NETCONF agent container"

    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, mgmt = env.ltop.xlate("target", "mgmt")
        _, ext0 = env.ltop.xlate("target", "ext0")
        _, hport = env.ltop.xlate("host", "data")

        if not target.has_model("infix-containers"):
            test.skip()
        if not target.has_model("infix-firewall"):
            test.skip()

    with test.step("Configure ext0 and VETH pair for agent container"):
        target.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [
                        {
                            "name": ext0,
                            "ipv4": {
                                "forwarding": True,
                                "address": [{
                                    "ip": EXTIP,
                                    "prefix-length": 24
                                }]
                            }
                        },
                        {
                            "name": "int0",
                            "type": "infix-if-type:veth",
                            "enabled": True,
                            "infix-interfaces:veth": {
                                "peer": NETCONF_IF
                            },
                            "ipv4": {
                                "forwarding": True,
                                "address": [{
                                    "ip": INTIP,
                                    "prefix-length": 24
                                }]
                            }
                        },
                        {
                            "name": NETCONF_IF,
                            "type": "infix-if-type:veth",
                            "enabled": True,
                            "infix-interfaces:veth": {
                                "peer": "int0"
                            },
                            "ipv4": {
                                "address": [{
                                    "ip": NETCONF_CONTAINER_IP,
                                    "prefix-length": 24
                                }]
                            },
                            "container-network": {
                                "route": [{
                                    "subnet": "0.0.0.0/0",
                                    "gateway": INTIP
                                }]
                            }
                        }
                    ]
                }
            }
        })

    with test.step("Forward port 830 on ext0 to agent container"):
        target.put_config_dicts({
            "infix-firewall": {
                "firewall": {
                    "default": "wan",
                    "zone": [
                        {
                            "name": "wan",
                            "action": "drop",
                            "interface": [ext0],
                            "port-forward": [{
                                "lower": 830,
                                "proto": "tcp",
                                "to": {
                                    "addr": NETCONF_CONTAINER_IP
                                }
                            }]
                        },
                        {
                            "name": "int",
                            "action": "reject",
                            "interface": ["int0"],
                            "service": ["netconf"]
                        },
                        {
                            "name": "mgmt",
                            "action": "accept",
                            "interface": [mgmt]
                        }
                    ]
                }
            }
        })

        infamy.Firewall.wait_for_operational(target, {
            "wan": {"action": "drop"},
            "int": {"action": "reject"},
            "mgmt": {"action": "accept"}
        })

    with test.step("Create agent container from bundled OCI image"):
        agent = to_binary(f"""#!/bin/sh
echo "{GREETING}"
# nc exits when stdin closes, before the server has sent its banner
sleep 3 | timeout 5 nc {INTIP} 830 | head -n 1
""")
        rclocal = to_binary("""#!/bin/sh
nc -lk -p 830 -e /usr/bin/agent &
""")

        target.put_config_dicts({
            "infix-containers": {
                "containers": {
                    "container": [
                        {
                            "name": NETCONF_CONTAINER,
                            "image": NFTABLES,
                            "network": {
                                "interface": [
                                    {"name": NETCONF_IF}
                                ]
                            },
                            "mount": [
                                {
                                    "name": "agent",
                                    "content": agent,
                                    "target": "/usr/bin/agent",
                                    "mode": "0755"
                                },
                                {
                                    "name": "rc.local",
                                    "content": rclocal,
                                    "target": "/etc/rc.local",
                                    "mode": "0755"
                                }
                            ]
                        }
                    ]
                }
            }
        })

    with test.step("Verify agent container has started"):
        c = infamy.Container(target)
        until(lambda: c.running(NETCONF_CONTAINER), attempts=60)

    with infamy.IsolatedMacVlan(hport) as ns:
        ns.addip(OURIP)

        with test.step("Verify port 830 on ext0 reaches the agent container"):
            until(lambda: GREETING in ns.call(
                lambda: netutil.tcp_read(EXTIP, 830)), attempts=30)

        with test.step("Verify agent container reaches NETCONF on the target"):
            until(lambda: "SSH-2.0-" in ns.call(
                lambda: netutil.tcp_read(EXTIP, 830)), attempts=10)

    test.succeed()
