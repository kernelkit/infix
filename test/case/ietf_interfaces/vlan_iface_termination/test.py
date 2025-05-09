#!/usr/bin/env python3
r"""VLAN Interface Termination

Verify that VLANs stacked on top of an interfaces that are also
attached to a VLAN filtering bridge are always locally terminated.

....
.---------------------------.
|           target          |
|                           |
|  data1.10  br0  data2.10  |
|      \    /   \    /      |
'------data1-----data2------'
         |         |
         |         |
.------data1-----data2------.
|      /      :     \       |
|  data1.10   :   data2.10  |
|             :             |
|           host            |
|             :             |
'---------------------------'
....

In this setup, even though VLAN 10 is allowed to ingress and egress on
both `data1` and `data2`, _bridging_ of packets from one to the other
must _not_ be allowed.

"""
import infamy

with infamy.Test() as test:
    with test.step("Set up topology and attach to target"):
        env = infamy.Env()
        tgt = env.attach("target", "mgmt")

    with test.step("Configure bridge and VLAN interfaces on target"):
        _, hdata1 = env.ltop.xlate(  "host", "data1")
        _, hdata2 = env.ltop.xlate(  "host", "data2")
        _, ddata1 = env.ltop.xlate("target", "data1")
        _, ddata2 = env.ltop.xlate("target", "data2")

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
                                            "untagged": [ddata1, ddata2]
                                        },
                                    ]
                                }
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
                                        "ip": "10.10.1.2",
                                        "prefix-length": 24,
                                    }
                                ]
                            }
                        },
                        {
                            "name": ddata2,
                            "infix-interfaces:bridge-port": {
                                "pvid": 1,
                                "bridge": "br0"
                            }
                        },
                        {
                            "name": f"{ddata2}.10",
                            "type": "infix-if-type:vlan",
                            "vlan": {
                                "id": 10,
                                "lower-layer-if": ddata2,
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

    with infamy.IsolatedMacVlan(hdata1) as ns0, \
         infamy.IsolatedMacVlan(hdata2) as ns1:

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

        with test.step("Verify that host:data1 reaches host:data2 with untagged packets"):
            ns0.must_reach("10.0.1.2")

        with test.step("Verify that traffic on VLAN 10 from host:data1 does not reach host:data2"):
            infamy.parallel(lambda: ns0.runsh("timeout -s INT 5 ping -i 0.2 -b 10.10.1.255 || true"),
                            lambda: ns1.must_not_receive("ip src 10.10.1.1"))

        with test.step("Verify that host:data1 can reach target on VLAN 10"):
            ns0.must_reach("10.10.1.2")

        with test.step("Verify that host:data2 can reach target on VLAN 10"):
            ns1.must_reach("10.10.2.2")

    test.succeed()
