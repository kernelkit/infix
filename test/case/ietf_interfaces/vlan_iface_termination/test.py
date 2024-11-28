#!/usr/bin/env python3
r"""VLAN Interface Termination

Verify that VLANs stacked on top of an interfaces that are also
attached to a VLAN filtering bridge are always locally terminated.

....
.---------------------------.
|           target          |
|                           |
|  data0.10  br0  data1.10  |
|      \    /   \    /      |
'------data0-----data1------'
         |         |
         |         |
.------data0-----data1------.
|      /      :     \       |
|  data0.10   :   data1.10  |
|             :             |
|           host            |
|             :             |
'---------------------------'
....

In this setup, even though VLAN 10 is allowed to ingress and egress on
both `data0` and `data1`, _bridging_ of packets from one to the other
must _not_ be allowed.

"""
import infamy

with infamy.Test() as test:
    with test.step("Set up topology and attach to target"):
        env = infamy.Env()
        tgt = env.attach("target", "mgmt")

    with test.step("Configure bridge and VLAN interfaces on target"):
        _, hdata0 = env.ltop.xlate(  "host", "data0")
        _, hdata1 = env.ltop.xlate(  "host", "data1")
        _, ddata0 = env.ltop.xlate("target", "data0")
        _, ddata1 = env.ltop.xlate("target", "data1")

        tgt.put_config_dicts({
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
                                            "untagged": [ddata0, ddata1]
                                        },
                                    ]
                                }
                            }
                        },
                        {
                            "name": ddata0,
                            "infix-interfaces:bridge-port": {
                                "pvid": 1,
                                "bridge": "br0"
                            }
                        },
                        {
                            "name": f"{ddata0}.10",
                            "type": "infix-if-type:vlan",
                            "vlan": {
                                "id": 10,
                                "lower-layer-if": ddata0,
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
                            "name": ddata1,
                            "infix-interfaces:bridge-port": {
                                "pvid": 1,
                                "bridge": "br0"
                            }
                        },
                        {
                            "name": f"{ddata1}.10",
                            "type": "infix-if-type:vlan",
                            "vlan": {
                                "id": 10,
                                "lower-layer-if": ddata1,
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

    with infamy.IsolatedMacVlan(hdata0) as ns0, \
         infamy.IsolatedMacVlan(hdata1) as ns1:

        with test.step("Configure IP addresses and VLAN interfaces on host"):
            ns0.addip("10.0.1.1")
            ns0.runsh("""
                  set -ex
                  ip link add dev vlan10 link iface up type vlan id 10
                  ip addr add 10.10.1.1/24 dev vlan10
                  """)

            ns1.addip("10.0.1.2")
            ns1.runsh("""
                  set -ex
                  ip link add dev vlan10 link iface up type vlan id 10
                  ip addr add 10.10.2.1/24 dev vlan10
                  """)

        with test.step("Verify that host:data0 reaches host:data1 with untagged packets"):
            ns0.must_reach("10.0.1.2")

        with test.step("Verify that traffic on VLAN 10 from host:data0 does not reach host:data1"):
            infamy.parallel(lambda: ns0.runsh("timeout -s INT 5 ping -i 0.2 -b 10.10.1.255 || true"),
                            lambda: ns1.must_not_receive("ip src 10.10.1.1"))

        with test.step("Verify that host:data0 can reach target on VLAN 10"):
            ns0.must_reach("10.10.1.2")

        with test.step("Verify that host:data1 can reach target on VLAN 10"):
            ns1.must_reach("10.10.2.2")

    test.succeed()
