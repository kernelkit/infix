#!/usr/bin/env python3
r"""VLAN Interface Termination

Verify that VLANs stacked on top of an interfaces that are also
attached to a VLAN filtering bridge are always locally terminated.

....
.-------------------.
|        dut        |
|                   |
|  a.10  br0  b.10  |
|     \ /   \ /     |
'------a-----b------'
       |     |
       |     |
.------a-----b------.
|                   |
|       host        |
|                   |
'-------------------'
....

In this setup, even though VLAN 10 is allowed to ingress and egress on
both `a` and `b`, _bridging_ of packets from one to the other must
_not_ be allowed.

"""
import infamy

with infamy.Test() as test:
    with test.step("Set up topology and attach to dut"):
        env = infamy.Env()
        dut = env.attach("dut", "mgmt")

    with test.step("Configure bridge and VLAN interfaces on dut"):
        _, hporta = env.ltop.xlate("host", "a")
        _, hportb = env.ltop.xlate("host", "b")
        _, dporta = env.ltop.xlate( "dut", "a")
        _, dportb = env.ltop.xlate( "dut", "b")

        dut.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [
                        {
                            "name": "br0",
                            "type": "infix-if-type:bridge",
                            "enabled": True,
                            "bridge": {
                                "vlans": {
                                    "vlan": [
                                        {
                                            "vid": 1,
                                            "untagged": [dporta, dportb]
                                        },
                                    ]
                                }
                            }
                        },
                        {
                            "name": dporta,
                            "infix-interfaces:bridge-port": {
                                "pvid": 1,
                                "bridge": "br0"
                            }
                        },
                        {
                            "name": f"{dporta}.10",
                            "type": "infix-if-type:vlan",
                            "vlan": {
                                "id": 10,
                                "lower-layer-if": dporta,
                            },
                            "ipv4": {
                                "address": [
                                    {
                                        "ip": "10.10.1.2",
                                        "prefix-length": 24,
                                    }
                                ]
                            }
                        },
                        {
                            "name": dportb,
                            "infix-interfaces:bridge-port": {
                                "pvid": 1,
                                "bridge": "br0"
                            }
                        },
                        {
                            "name": f"{dportb}.10",
                            "type": "infix-if-type:vlan",
                            "vlan": {
                                "id": 10,
                                "lower-layer-if": dportb,
                            },
                            "ipv4": {
                                "address": [
                                    {
                                        "ip": "10.10.2.2",
                                        "prefix-length": 24,
                                    }
                                ]
                            }
                        },
                    ]
                }
            }
        })

    with infamy.IsolatedMacVlan(hporta) as nsa, \
         infamy.IsolatedMacVlan(hportb) as nsb:

        with test.step("Configure IP addresses and VLAN interfaces on host"):
            nsa.addip("10.0.1.1")
            nsa.runsh("""
                  set -ex
                  ip link add dev vlan10 link iface up type vlan id 10
                  ip addr add 10.10.1.1/24 dev vlan10
                  """)

            nsb.addip("10.0.1.2")
            nsb.runsh("""
                  set -ex
                  ip link add dev vlan10 link iface up type vlan id 10
                  ip addr add 10.10.2.1/24 dev vlan10
                  """)

        with test.step("Verify that host:a reaches host:b with untagged packets"):
            nsa.must_reach("10.0.1.2")

        with test.step("Verify that traffic on VLAN 10 from host:a does not reach host:b"):
            infamy.parallel(lambda: nsa.runsh("timeout -s INT 5 ping -i 0.2 -b 10.10.1.255 || true"),
                            lambda: nsb.must_not_receive("ip src 10.10.1.1"))

        with test.step("Verify that host:a can reach dut on VLAN 10"):
            nsa.must_reach("10.10.1.2")

        with test.step("Verify that host:b can reach dut on VLAN 10"):
            nsb.must_reach("10.10.2.2")

    test.succeed()
