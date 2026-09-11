#!/usr/bin/env python3
#
#
#             10.0.0.1/32 (lo)
#                |
#         +------+---------+ .1   10.0.12.0/30         .2+--------------------+
#         |      R1        +-----------------------------+      R2            |
#         |                |       AREA0                 |                    +-10.0.0.2/32 (lo)
#         +-------+--------+--.1                     .1 -+--------+-----------+
#              .2 |           \---                  ---/          |.1
#                 |               \---         ----/              |
#   10.0.41.0 /30 | AREA2             \--- ---/                   | 10.0.23.0/30
#                 |       10.0.24.0/30 ---/\---  10.0.13.0/30     |
#                 |               ----/        \---       AREA1   |
#              .1 |        .2 ---/                  \--- .2       |.2
#         +-------+--------+-/                        \-+--------+----------+
#         |   R4        .2 |                             |   R3              |
#         |                +---------+                 .1|                   +-10.0.0.3/32 (lo)
#         +------+---------+.1       | 192.168.4.0/24    +-------------------+
#                |                   |.2
#          10.0.0.4/32 (lo)  +-------+
#                            |       |
#                            |  PC   |
#                            |       |
#                            +-------+
#
#
"""{version} with multiple areas

This test evaluates various {version} features across three areas (one NSSA
area) to ensure that route distribution is deterministic (based on cost).  It
also tests link failures using BFD, though BFD is not yet implemented in test
framework (Infamy).

This test also verifies broadcast and point-to-point interface types on the
transit links, and explicit router-id.

OSPFv2 runs the NSSA area as totally-NSSA (summary false), so R3 must see a
default route and none of the area 0 networks.  OSPFv3 has no totally-NSSA, so
there R3 is expected to learn the same networks as inter-area summaries.

....
  +-------------+  +---------------+  +-------------+  +---------------+
  |     R1      |  |      R2       |  |     R3      |  |      R4       |
  | 10.0.0.1/32 |  |  10.0.0.2/32  |  | 10.0.0.3/32 |  |  10.0.0.4/32  |
  |   (lo)      |  |  11.0.9.1/24  |  |   (lo)      |  |      (lo)     |
  +-------------+  |  11.0.10.1/24 |  +-------------+   +---------------+
                   |  11.0.11.1/24 |
                   |  11.0.12.1/24 |
                   |  11.0.13.1/24 |
                   |  11.0.14.1/24 |
                   |  11.0.15.1/24 |
                   |      (lo)     |
                   +---------------+
....
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
        "len":     30,          # transit links
        "netlen":  24,          # data and stub networks
        "hostlen": 32,          # loopbacks
        "default": "0.0.0.0/0",
        # Totally-NSSA, R3 gets a default route instead of the area 0 networks
        "summary": False,
        "R1": {"ring1": "10.0.12.1", "ring2": "10.0.41.2", "cross": "10.0.13.1",
               "lo": "10.0.0.1", "stub": "11.0.8.1", "stubnet": "11.0.8.0/24"},
        "R2": {"ring1": "10.0.23.1", "ring2": "10.0.12.2", "cross": "10.0.24.1",
               "lo": "10.0.0.2",
               "stubs": ["11.0.9.1", "11.0.10.1", "11.0.11.1", "11.0.12.1",
                         "11.0.13.1", "11.0.14.1", "11.0.15.1"],
               "stubnets": ["11.0.9.0/24", "11.0.10.0/24", "11.0.11.0/24", "11.0.12.0/24",
                            "11.0.13.0/24", "11.0.14.0/24", "11.0.15.0/24"]},
        "R3": {"ring2": "10.0.23.2", "cross": "10.0.13.2", "data": "192.168.3.1",
               "lo": "10.0.0.3"},
        "R4": {"ring1": "10.0.41.1", "cross": "10.0.24.2", "data": "192.168.4.1",
               "lo": "10.0.0.4"},
        "PC3": "192.168.3.2",
        "PC4": "192.168.4.2",
        "R4net": "192.168.4.0/24",
        "area0net": "10.0.12.0/30",
    },
    "ospfv3": {
        "af":      "ipv6",
        "len":     64,
        "netlen":  64,
        "hostlen": 128,
        "default": "::/0",
        "R1": {"ring1": "2001:db8:12::1", "ring2": "2001:db8:41::2", "cross": "2001:db8:13::1",
               "lo": "2001:db8::1", "stub": "2001:db8:8::1", "stubnet": "2001:db8:8::/64"},
        "R2": {"ring1": "2001:db8:23::1", "ring2": "2001:db8:12::2", "cross": "2001:db8:24::1",
               "lo": "2001:db8::2",
               "stubs": ["2001:db8:200::1", "2001:db8:201::1", "2001:db8:202::1",
                         "2001:db8:203::1", "2001:db8:204::1", "2001:db8:205::1",
                         "2001:db8:206::1"],
               "stubnets": ["2001:db8:200::/64", "2001:db8:201::/64", "2001:db8:202::/64",
                            "2001:db8:203::/64", "2001:db8:204::/64", "2001:db8:205::/64",
                            "2001:db8:206::/64"]},
        "R3": {"ring2": "2001:db8:23::2", "cross": "2001:db8:13::2", "data": "2001:db8:3::1",
               "lo": "2001:db8::3"},
        "R4": {"ring1": "2001:db8:41::1", "cross": "2001:db8:24::2", "data": "2001:db8:4::1",
               "lo": "2001:db8::4"},
        "PC3": "2001:db8:3::2",
        "PC4": "2001:db8:4::2",
        "R4net": "2001:db8:4::/64",
        "area0net": "2001:db8:12::/64",
    },
}


def iface(p, name, addrs, forwarding=True):
    """Interface with one or more (address, prefix-length) of the tested family"""
    ip = {"address": [{"ip": addr, "prefix-length": plen} for addr, plen in addrs]}
    if forwarding:
        ip["forwarding"] = True

    return {"name": name, "enabled": True, p["af"]: ip}


def area(p, area_id, interfaces, nssa=False):
    conf = {"area-id": area_id, "interfaces": {"interface": interfaces}}
    if nssa:
        conf["area-type"] = "nssa-area"
        # OSPFv3 has no totally-NSSA, the summary flag is only set for OSPFv2
        if "summary" in p:
            conf["summary"] = p["summary"]

    return conf


def ospf_if(name, cost=None, itype=None, bfd=True, passive=False):
    conf = {"name": name, "enabled": True}
    if bfd:
        conf["bfd"] = {"enabled": True}
        conf["hello-interval"] = 1
    if cost:
        conf["cost"] = cost
    if itype:
        conf["interface-type"] = itype
    if passive:
        conf["passive"] = True

    return conf


def routing(p, router_id, areas, redistribute=None):
    ospf = {"explicit-router-id": router_id, "areas": {"area": areas}}
    if redistribute:
        ospf["redistribute"] = {"redistribute": [{"protocol": proto} for proto in redistribute]}

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


def config_target1(target, ring1, ring2, cross, p):
    addr = p["R1"]
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    iface(p, ring1, [(addr["ring1"], p["len"])]),
                    iface(p, ring2, [(addr["ring2"], p["len"])]),
                    iface(p, cross, [(addr["cross"], p["len"])]),
                    iface(p, "lo", [(addr["stub"], p["netlen"]),
                                    (addr["lo"], p["hostlen"])], forwarding=False)
                ]
            }
        },
        "ietf-system": {"system": {"hostname": "R1"}},
        "ietf-routing": routing(p, "10.0.0.1", [
            area(p, "0.0.0.0", [ospf_if(ring1)]),
            area(p, "0.0.0.1", [ospf_if(cross, cost=2000),
                                ospf_if("lo", bfd=False)], nssa=True),
            area(p, "0.0.0.2", [ospf_if(ring2, itype="point-to-point")]),
        ])
    })


def config_target2(target, ring1, ring2, cross, p):
    addr = p["R2"]
    stubs = [(stub, p["netlen"]) for stub in addr["stubs"]]
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    iface(p, ring1, [(addr["ring1"], p["len"])]),
                    iface(p, ring2, [(addr["ring2"], p["len"])]),
                    iface(p, cross, [(addr["cross"], p["len"])]),
                    iface(p, "lo", [(addr["lo"], p["hostlen"])] + stubs, forwarding=False)
                ]
            }
        },
        "ietf-system": {"system": {"hostname": "R2"}},
        "ietf-routing": routing(p, "1.1.1.1", [
            area(p, "0.0.0.0", [ospf_if(ring2), ospf_if("lo", bfd=False)]),
            area(p, "0.0.0.1", [ospf_if(ring1)], nssa=True),
            area(p, "0.0.0.2", [ospf_if(cross, cost=2000)]),
        ])
    })


def config_target3(target, ring2, cross, link, p):
    addr = p["R3"]
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    iface(p, ring2, [(addr["ring2"], p["len"])]),
                    iface(p, link, [(addr["data"], p["netlen"])]),
                    iface(p, cross, [(addr["cross"], p["len"])]),
                    iface(p, "lo", [(addr["lo"], p["hostlen"])], forwarding=False)
                ]
            }
        },
        "ietf-system": {"system": {"hostname": "R3"}},
        "ietf-routing": routing(p, "10.0.0.3", [
            area(p, "0.0.0.1", [ospf_if(cross, cost=2000),
                                ospf_if(ring2),
                                ospf_if(link, bfd=False, passive=True),
                                ospf_if("lo", bfd=False)], nssa=True),
        ])
    })


def config_target4(target, ring1, cross, link, p):
    addr = p["R4"]
    # Advertise the PC:data4 network as an intra-area prefix (passive), except
    # for OSPFv2 where the totally-NSSA gives R3 a default route, and where
    # redistribute-connected is exercised instead.  A redistributed external
    # would never reach R3 through a regular NSSA, which blocks AS-external
    # (Type-5) LSAs, breaking its return path.
    if "summary" in p:
        interfaces = [ospf_if(ring1, itype="point-to-point"),
                      ospf_if(cross, cost=5000),
                      ospf_if("lo", bfd=False)]
        redistribute = ["connected"]
    else:
        interfaces = [ospf_if(ring1, itype="point-to-point"),
                      ospf_if(cross, cost=5000),
                      ospf_if(link, bfd=False, passive=True),
                      ospf_if("lo", bfd=False)]
        redistribute = None

    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    iface(p, ring1, [(addr["ring1"], p["len"])]),
                    iface(p, cross, [(addr["cross"], p["len"])]),
                    iface(p, link, [(addr["data"], p["netlen"])]),
                    iface(p, "lo", [(addr["lo"], p["hostlen"])], forwarding=False)
                ]
            }
        },
        "ietf-system": {"system": {"hostname": "R4"}},
        "ietf-routing": routing(p, "10.0.0.4", [
            area(p, "0.0.0.2", interfaces),
        ], redistribute)
    })


def disable_link(target, link):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [{"name": link, "enabled": False}]
            }
        }
    })


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        env = infamy.Env(args=ArgumentParser())
        version = env.args.version
        param = PARAM[version] | {"version": version}
        af, proto = param["af"], f"ietf-ospf:{version}"
        instance = f"infix-routing:{version}"
        totally_nssa = "summary" in param

        # OSPFv3 installs IPv6 link-local next-hops, which cannot be matched
        # against the configured addresses.
        nexthop = (lambda addr: addr) if af == "ipv4" else (lambda addr: None)

        def lo(name):
            return f"{param[name]['lo']}/{param['hostlen']}"

        R1, R2, R3, R4 = parallel(lambda: env.attach("R1", "mgmt"),
                                  lambda: env.attach("R2", "mgmt"),
                                  lambda: env.attach("R3", "mgmt"),
                                  lambda: env.attach("R4", "mgmt"))

        _, R1ring1 = env.ltop.xlate("R1", "ring1")
        _, R1ring2 = env.ltop.xlate("R1", "ring2")
        _, R2ring1 = env.ltop.xlate("R2", "ring1")
        _, R2ring2 = env.ltop.xlate("R2", "ring2")
        _, R3ring2 = env.ltop.xlate("R3", "ring2")
        _, R4ring1 = env.ltop.xlate("R4", "ring1")

        _, R3data = env.ltop.xlate("R3", "data")
        _, R4data = env.ltop.xlate("R4", "data")

        _, R1cross = env.ltop.xlate("R1", "cross")
        _, R2cross = env.ltop.xlate("R2", "cross")
        _, R3cross = env.ltop.xlate("R3", "cross")
        _, R4cross = env.ltop.xlate("R4", "cross")

    with test.step("Configure targets"):
        parallel(lambda: config_target1(R1, R1ring1, R1ring2, R1cross, param),
                 lambda: config_target2(R2, R2ring1, R2ring2, R2cross, param),
                 lambda: config_target3(R3, R3ring2, R3cross, R3data, param),
                 lambda: config_target4(R4, R4ring1, R4cross, R4data, param))

    with test.step("Wait for all neighbors to peer"):
        print("Waiting for neighbors to peer")
        until(lambda: route.ospf_get_neighbor(R1, "0.0.0.0", R1ring1, "1.1.1.1", proto=instance), attempts=200)
        until(lambda: route.ospf_get_neighbor(R1, "0.0.0.1", R1cross, "10.0.0.3", proto=instance), attempts=200)
        until(lambda: route.ospf_get_neighbor(R2, "0.0.0.1", R2ring1, "10.0.0.3", proto=instance), attempts=200)
        until(lambda: route.ospf_get_neighbor(R2, "0.0.0.0", R2ring2, "10.0.0.1", proto=instance), attempts=200)
        until(lambda: route.ospf_get_neighbor(R2, "0.0.0.2", R2cross, "10.0.0.4", proto=instance), attempts=200)

    with test.step(f"Wait for routes from {version} on all routers"):
        print("Waiting for routes from OSPF")
        until(lambda: route.route_exist(R1, lo("R2"), af=af, proto=proto,
                                        nexthop=nexthop(param["R2"]["ring2"])), attempts=200)
        until(lambda: route.route_exist(R1, lo("R3"), af=af, proto=proto,
                                        nexthop=nexthop(param["R3"]["cross"])), attempts=200)
        until(lambda: route.route_exist(R1, lo("R4"), af=af, proto=proto,
                                        nexthop=nexthop(param["R4"]["ring1"])), attempts=200)
        until(lambda: route.route_exist(R2, lo("R1"), af=af, proto=proto,
                                        nexthop=nexthop(param["R3"]["ring2"])), attempts=200)
        until(lambda: route.route_exist(R2, lo("R3"), af=af, proto=proto,
                                        nexthop=nexthop(param["R3"]["ring2"])), attempts=200)
        until(lambda: route.route_exist(R2, lo("R4"), af=af, proto=proto,
                                        nexthop=nexthop(param["R4"]["cross"])), attempts=200)
        until(lambda: route.route_exist(R4, lo("R3"), af=af, proto=proto,
                                        nexthop=nexthop(param["R1"]["ring2"])), attempts=200)
        until(lambda: route.route_exist(R1, param["R4net"], af=af, proto=proto,
                                        nexthop=nexthop(param["R4"]["ring1"])), attempts=200)

    with test.step("Verify Area 0.0.0.1 on R3 is NSSA area"):
        assert route.ospf_is_area_nssa(R3, "0.0.0.1", proto=instance)

    with test.step("Verify R1:ring2 is of type point-to-point"):
        assert route.ospf_get_interface_type(R1, "0.0.0.2", R1ring2, proto=instance) == "point-to-point"

    with test.step("Verify R4:ring1 is of type point-to-point"):
        assert route.ospf_get_interface_type(R4, "0.0.0.2", R4ring1, proto=instance) == "point-to-point"

    with test.step("Verify what the NSSA area lets through to R3"):
        if totally_nssa:
            # Totally-NSSA, only a default route out of the area.
            parallel(lambda: until(lambda: route.route_exist(R3, param["default"], af=af), attempts=200),
                     lambda: until(lambda: route.route_exist(R3, param["area0net"], af=af) is False, attempts=5),
                     lambda: until(lambda: route.route_exist(R3, param["R1"]["stubnet"], af=af) is False, attempts=5),
                     *[(lambda net=net: until(lambda: route.route_exist(R3, net, af=af) is False, attempts=5))
                       for net in param["R2"]["stubnets"]])
        else:
            # Regular NSSA, inter-area (Type-3) summaries are let through.
            parallel(lambda: until(lambda: route.route_exist(R3, param["R1"]["stubnet"], af=af, proto=proto), attempts=200),
                     lambda: until(lambda: route.route_exist(R3, param["R4net"], af=af, proto=proto), attempts=200),
                     *[(lambda net=net: until(lambda: route.route_exist(R3, net, af=af, proto=proto), attempts=200))
                       for net in param["R2"]["stubnets"]])

    _, hport0 = env.ltop.xlate("PC", "data3")
    with infamy.IsolatedMacVlan(hport0) as ns0:
        with test.step("Testing connectivity through NSSA area, from PC:data3 to R1's stub network"):
            ns0.addip(param["PC3"], prefix_length=param["netlen"], proto=af)
            ns0.addroute(param["default"], param["R3"]["data"], proto=af)
            ns0.must_reach(param["R1"]["stub"])

    _, hport0 = env.ltop.xlate("PC", "data4")
    with infamy.IsolatedMacVlan(hport0) as ns0:
        ns0.addip(param["PC4"], prefix_length=param["netlen"], proto=af)
        ns0.addroute(param["default"], param["R4"]["data"], proto=af)

        with test.step("Verify that the route to R3 from PC:data4 goes through R1"):
            ns0.must_reach(param["R3"]["lo"])
            hops = [row[1] for row in ns0.traceroute(param["R3"]["lo"])]
            assert param["R1"]["ring2"] in hops, f"Path to R3 ({repr(hops)}) does not go through R1"

        with test.step("Break link R1:ring2 --- R4:ring1"):
            # Here we should test with link breakers, to test BFD
            # recouppling, for now disable the link
            disable_link(R1, R1ring2)

        with test.step("Verify that the route to R3 from PC:data4 fails over through R2"):
            # A plain "route exists" check passes immediately on the stale route
            # still pointing at the now-dead R1 link, so wait until R4's
            # adjacency to R1 is gone, which guarantees SPF has recomputed the
            # path towards R2.
            until(lambda: not route.ospf_get_neighbor(R4, "0.0.0.2", R4ring1, "10.0.0.1", proto=instance), attempts=200)
            until(lambda: route.route_exist(R4, lo("R3"), af=af, proto=proto,
                                            nexthop=nexthop(param["R2"]["cross"])), attempts=100)
            ns0.must_reach(param["R3"]["lo"])
            hops = [row[1] for row in ns0.traceroute(param["R3"]["lo"])]
            assert param["R2"]["cross"] in hops, f"Path to R3 ({repr(hops)}) did not fail over via R2"

    test.succeed()
