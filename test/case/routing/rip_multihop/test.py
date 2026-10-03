#!/usr/bin/env python3
"""{version} Multi-hop

Verifies {version} functionality across multiple hops with three routers in a
line topology (R1 -- R2 -- R3). This test ensures:
- RIP routes propagate through multiple hops
- R2 (middle router) has two RIP neighbors
- End-to-end connectivity works across the RIP network

Topology:
  PC:data1 -- R1 -- R2 -- R3 -- PC:data2
"""

import infamy
import infamy.route as route
from infamy.util import until, parallel


class ArgumentParser(infamy.ArgumentParser):
    def __init__(self):
        super().__init__()
        self.add_argument("--version", type=str.lower, choices=["ripv2", "ripng"])


PARAM = {
    "ripv2": {
        "af":      "ipv4",
        "len":     24,
        "hostlen": 32,
        "R1data":  "192.168.10.1",
        "R1link":  "192.168.50.1",
        "R1lo":    "192.168.11.1",
        "R2west":  "192.168.50.2",
        "R2east":  "192.168.60.1",
        "R2lo":    "192.168.22.1",
        "R3link":  "192.168.60.2",
        "R3data":  "192.168.70.1",
        "R3lo":    "192.168.33.1",
        "R1net":   "192.168.10.0/24",
        "R3net":   "192.168.70.0/24",
        "PC1":     "192.168.10.2",
        "PC2":     "192.168.70.2",
        # RIPv2 peers over the interface addresses, RIPng over link-local
        "neighbors": ["192.168.50.1", "192.168.60.2"],
    },
    "ripng": {
        "af":      "ipv6",
        "len":     64,
        "hostlen": 128,
        "R1data":  "2001:db8:10::1",
        "R1link":  "2001:db8:50::1",
        "R1lo":    "2001:db8:11::1",
        "R2west":  "2001:db8:50::2",
        "R2east":  "2001:db8:60::1",
        "R2lo":    "2001:db8:22::1",
        "R3link":  "2001:db8:60::2",
        "R3data":  "2001:db8:70::1",
        "R3lo":    "2001:db8:33::1",
        "R1net":   "2001:db8:10::/64",
        "R3net":   "2001:db8:70::/64",
        "PC1":     "2001:db8:10::2",
        "PC2":     "2001:db8:70::2",
    },
}


def iface(p, name, addr, prefix_length=None, forwarding=True):
    """Interface with a single address of the tested address family"""
    ip = {"address": [{"ip": addr, "prefix-length": prefix_length or p["len"]}]}
    if forwarding:
        ip["forwarding"] = True

    return {"name": name, "enabled": True, p["af"]: ip}


def rip(p, links):
    return {
        "type": f"infix-routing:{p['version']}",
        "name": "default",
        "rip": {
            "timers": {
                "update-interval": 5,
                "invalid-interval": 15,
                "flush-interval": 20
            },
            "redistribute": {
                "redistribute": [{
                    "protocol": "connected"
                }]
            },
            "interfaces": {
                "interface": [{"interface": link} for link in links]
            }
        }
    }


def config_router(target, name, interfaces, links, p):
    """One router, 'interfaces' are (ifname, address) and 'links' run RIP"""
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [iface(p, ifname, addr) for ifname, addr in interfaces] +
                             [iface(p, "lo", p[f"{name}lo"], p["hostlen"], forwarding=False)]
            }
        },
        "ietf-system": {
            "system": {
                "hostname": name
            }
        },
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [rip(p, links)]
                }
            }
        }
    })


def rip_neighbors(target, p):
    """RIP neighbor addresses of an instance, as reported in operational data"""
    routing_data = target.get_data("/ietf-routing:routing/control-plane-protocols")
    protocols = routing_data.get("routing", {}).get("control-plane-protocols", {}).get("control-plane-protocol", [])
    if not protocols:
        raise Exception("No protocols found")

    rip = None
    for protocol in protocols:
        if protocol.get("type") == f"infix-routing:{p['version']}" and protocol.get("name") == "default":
            rip = protocol.get("rip", {})
            break

    if not rip:
        raise Exception("RIP protocol not found in control-plane-protocols")

    neighbors = rip.get(p["af"], {}).get("neighbors", {}).get("neighbor", [])

    return [neighbor.get(f"{p['af']}-address") for neighbor in neighbors]


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        env = infamy.Env(args=ArgumentParser())
        version = env.args.version
        param = PARAM[version] | {"version": version}
        af, plen = param["af"], param["hostlen"]

        def lo(name):
            return f"{param[f'{name}lo']}/{plen}"

        R1, R2, R3 = parallel(lambda: env.attach("R1", "mgmt"),
                              lambda: env.attach("R2", "mgmt"),
                              lambda: env.attach("R3", "mgmt"))

    with test.step("Configure routers"):
        _, R1data = env.ltop.xlate("R1", "data")
        _, R1link = env.ltop.xlate("R1", "link")
        _, R2west = env.ltop.xlate("R2", "west")
        _, R2east = env.ltop.xlate("R2", "east")
        _, R3link = env.ltop.xlate("R3", "link")
        _, R3data = env.ltop.xlate("R3", "data")

        parallel(lambda: config_router(R1, "R1", [(R1data, param["R1data"]),
                                                  (R1link, param["R1link"])], [R1link], param),
                 lambda: config_router(R2, "R2", [(R2west, param["R2west"]),
                                                  (R2east, param["R2east"])], [R2west, R2east], param),
                 lambda: config_router(R3, "R3", [(R3link, param["R3link"]),
                                                  (R3data, param["R3data"])], [R3link], param))

    with test.step(f"Wait for {version} routes to be exchanged"):
        print("Waiting for RIP routes to propagate...")
        # R1 should learn R2's loopback
        until(lambda: route.route_exist(R1, lo("R2"), af=af, proto="ietf-rip:rip", active_check=True), attempts=40)
        # R1 should learn R3's loopback (via R2)
        until(lambda: route.route_exist(R1, lo("R3"), af=af, proto="ietf-rip:rip", active_check=True), attempts=40)
        # R2 should learn R1's loopback
        until(lambda: route.route_exist(R2, lo("R1"), af=af, proto="ietf-rip:rip", active_check=True), attempts=40)
        # R2 should learn R3's loopback
        until(lambda: route.route_exist(R2, lo("R3"), af=af, proto="ietf-rip:rip", active_check=True), attempts=40)
        # R3 should learn R2's loopback
        until(lambda: route.route_exist(R3, lo("R2"), af=af, proto="ietf-rip:rip", active_check=True), attempts=40)
        # R3 should learn R1's loopback (via R2)
        until(lambda: route.route_exist(R3, lo("R1"), af=af, proto="ietf-rip:rip", active_check=True), attempts=40)
        until(lambda: route.route_exist(R2, param["R1net"], af=af, proto="ietf-rip:rip", active_check=True), attempts=40)
        until(lambda: route.route_exist(R3, param["R1net"], af=af, proto="ietf-rip:rip", active_check=True), attempts=40)
        until(lambda: route.route_exist(R2, param["R3net"], af=af, proto="ietf-rip:rip", active_check=True), attempts=40)
        until(lambda: route.route_exist(R1, param["R3net"], af=af, proto="ietf-rip:rip", active_check=True), attempts=40)

    with test.step(f"Verify R2 has two {version} neighbors"):
        print("Checking R2 has two RIP neighbors...")
        neighbors = rip_neighbors(R2, param)
        assert len(neighbors) == 2, f"Expected 2 neighbors, found {len(neighbors)}"

        # RIPng peers over link-local addresses, which are not known in advance
        for peer in param.get("neighbors", []):
            assert peer in neighbors, f"{peer} not in neighbor list"
        print(f"R2 has correct neighbors: {neighbors}")

    with test.step("Test end-to-end connectivity PC:data1 to R3 loopback"):
        _, hport1 = env.ltop.xlate("PC", "data1")
        with infamy.IsolatedMacVlan(hport1) as ns1:
            ns1.addip(param["PC1"], prefix_length=param["len"], proto=af)
            ns1.addroute(lo("R3"), param["R1data"], proto=af)
            ns1.must_reach(param["R3lo"])

    with test.step("Test end-to-end connectivity PC:data2 to R1 loopback"):
        _, hport2 = env.ltop.xlate("PC", "data2")
        with infamy.IsolatedMacVlan(hport2) as ns2:
            ns2.addip(param["PC2"], prefix_length=param["len"], proto=af)
            ns2.addroute(lo("R1"), param["R3data"], proto=af)
            ns2.must_reach(param["R1lo"])

    test.succeed()
