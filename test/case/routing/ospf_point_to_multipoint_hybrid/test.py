#!/usr/bin/env python3
"""{version} Point-to-Multipoint Hybrid

Verify {version} point-to-multipoint hybrid (broadcast) interface type by
configuring three routers on a shared multi-access network with the
ietf-ospf 'hybrid' interface type.  This maps to the point-to-multipoint
network type using multicast for neighbor discovery.

R2 acts as the hub, bridging two physical links (link1, link2) into a
single broadcast domain (br0).  R1 and R3 each connect to one of R2's
ports.  The test verifies that all routers form OSPF adjacencies, exchange
routes, and that the interface type is correctly reported as hybrid.

....
  +------------------+                                   +------------------+
  |       R1         |                                   |       R3         |
  |  10.0.1.1/32     |                                   |  10.0.3.1/32     |
  |     (lo)         |                                   |     (lo)         |
  +--------+---------+                                   +--------+---------+
           |  .1                                                  |  .3
           |               +------------------+                   |
           +----link1------+       R2         +------link2--------+
                           |  10.0.2.1/32     |
                           |     (lo)         |
                           | br0: 10.0.123.2  |
                           +------------------+
                              10.0.123.0/24
                        (P2MP hybrid / shared segment)
....
The OSPFv3 run uses 2001:db8:123::/64 for the shared segment.
"""

import infamy
import infamy.route as route
from infamy.util import until, parallel


class ArgumentParser(infamy.ArgumentParser):
    def __init__(self):
        super().__init__()
        self.add_argument("--version", type=str.lower, choices=["ospfv2", "ospfv3"])


PARAM = {
    "ospfv2": {
        "af":      "ipv4",
        "len":     24,
        "hostlen": 32,
        "R1": {"link": "10.0.123.1", "data": "10.0.10.1", "lo": "10.0.1.1"},
        "R2": {"link": "10.0.123.2", "lo": "10.0.2.1"},
        "R3": {"link": "10.0.123.3", "data": "10.0.30.1", "lo": "10.0.3.1"},
        "PC1": "10.0.10.2",
        "PC2": "10.0.30.2",
    },
    "ospfv3": {
        "af":      "ipv6",
        "len":     64,
        "hostlen": 128,
        "router-id": {"R1": "1.1.1.1", "R2": "2.2.2.2", "R3": "3.3.3.3"},
        "R1": {"link": "2001:db8:123::1", "data": "2001:db8:10::1", "lo": "2001:db8:1::1"},
        "R2": {"link": "2001:db8:123::2", "lo": "2001:db8:2::1"},
        "R3": {"link": "2001:db8:123::3", "data": "2001:db8:30::1", "lo": "2001:db8:3::1"},
        "PC1": "2001:db8:10::2",
        "PC2": "2001:db8:30::2",
    },
}


def iface(p, name, addr, prefix_length=None, forwarding=True, **kwargs):
    """Interface with a single address of the tested address family"""
    ip = {"address": [{"ip": addr, "prefix-length": prefix_length or p["len"]}]}
    if forwarding:
        ip["forwarding"] = True

    return {"name": name, "enabled": True, p["af"]: ip} | kwargs


def routing(p, name, link):
    """OSPF instance with the hybrid link and the loopback in area 0"""
    ospf = {}
    if "router-id" in p:
        ospf["explicit-router-id"] = p["router-id"][name]

    ospf |= {
        "redistribute": {
            "redistribute": [{
                "protocol": "connected"
            }]
        },
        "areas": {
            "area": [{
                "area-id": "0.0.0.0",
                "interfaces": {
                    "interface": [{
                        "name": link,
                        "enabled": True,
                        "interface-type": "hybrid",
                        "hello-interval": 1,
                        "dead-interval": 3
                    }, {
                        "name": "lo",
                        "enabled": True
                    }]
                }
            }]
        }
    }

    return {
        "routing": {
            "control-plane-protocols": {
                "control-plane-protocol": [{
                    "type": f"infix-routing:{p['version']}",
                    "name": "default",
                    "ospf": ospf
                }]
            }
        }
    }


def config_spoke(target, name, link, data, p):
    """R1 and R3, one leg on the shared segment and one data network"""
    addr = p[name]
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    iface(p, link, addr["link"]),
                    iface(p, data, addr["data"]),
                    iface(p, "lo", addr["lo"], p["hostlen"], forwarding=False)
                ]
            }
        },
        "ietf-system": {"system": {"hostname": name}},
        "ietf-routing": routing(p, name, link)
    })


def config_hub(target, link1, link2, p):
    """R2, bridging both links into one broadcast domain"""
    addr = p["R2"]
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    iface(p, "br0", addr["link"], type="infix-if-type:bridge"),
                    {
                        "name": link1,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "bridge": "br0"
                        }
                    },
                    {
                        "name": link2,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "bridge": "br0"
                        }
                    },
                    iface(p, "lo", addr["lo"], p["hostlen"], forwarding=False)
                ]
            }
        },
        "ietf-system": {"system": {"hostname": "R2"}},
        "ietf-routing": routing(p, "R2", "br0")
    })


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        env = infamy.Env(args=ArgumentParser())
        version = env.args.version
        param = PARAM[version] | {"version": version}
        af, proto = param["af"], f"ietf-ospf:{version}"
        instance = f"infix-routing:{version}"

        def lo(name):
            return f"{param[name]['lo']}/{param['hostlen']}"

        R1, R2, R3 = parallel(lambda: env.attach("R1", "mgmt"),
                              lambda: env.attach("R2", "mgmt"),
                              lambda: env.attach("R3", "mgmt"))

    with test.step("Configure targets"):
        _, R1link = env.ltop.xlate("R1", "link")
        _, R1data = env.ltop.xlate("R1", "data")
        _, R2link1 = env.ltop.xlate("R2", "link1")
        _, R2link2 = env.ltop.xlate("R2", "link2")
        _, R3link = env.ltop.xlate("R3", "link")
        _, R3data = env.ltop.xlate("R3", "data")

        parallel(lambda: config_spoke(R1, "R1", R1link, R1data, param),
                 lambda: config_hub(R2, R2link1, R2link2, param),
                 lambda: config_spoke(R3, "R3", R3link, R3data, param))

    with test.step(f"Wait for {version} routes"):
        print("Waiting for OSPF routes to converge")
        until(lambda: route.route_exist(R1, lo("R2"), af=af, proto=proto), attempts=200)
        until(lambda: route.route_exist(R1, lo("R3"), af=af, proto=proto), attempts=200)
        until(lambda: route.route_exist(R2, lo("R1"), af=af, proto=proto), attempts=200)
        until(lambda: route.route_exist(R2, lo("R3"), af=af, proto=proto), attempts=200)
        until(lambda: route.route_exist(R3, lo("R1"), af=af, proto=proto), attempts=200)
        until(lambda: route.route_exist(R3, lo("R2"), af=af, proto=proto), attempts=200)

    with test.step("Verify interface type is hybrid"):
        print("Checking OSPF interface type on all routers")
        assert route.ospf_get_interface_type(R1, "0.0.0.0", R1link, proto=instance) == "hybrid"
        assert route.ospf_get_interface_type(R2, "0.0.0.0", "br0", proto=instance) == "hybrid"
        assert route.ospf_get_interface_type(R3, "0.0.0.0", R3link, proto=instance) == "hybrid"

    with test.step("Verify connectivity between all DUTs"):
        _, hport1 = env.ltop.xlate("PC", "data1")
        _, hport2 = env.ltop.xlate("PC", "data2")
        with infamy.IsolatedMacVlan(hport1) as ns1, \
             infamy.IsolatedMacVlan(hport2) as ns2:
            ns1.addip(param["PC1"], prefix_length=param["len"], proto=af)
            ns2.addip(param["PC2"], prefix_length=param["len"], proto=af)
            ns1.addroute(lo("R3"), param["R1"]["data"], proto=af)
            ns2.addroute(lo("R1"), param["R3"]["data"], proto=af)
            parallel(
                lambda: ns1.must_reach(param["R3"]["lo"]),
                lambda: ns2.must_reach(param["R1"]["lo"]),
            )
    test.succeed()
