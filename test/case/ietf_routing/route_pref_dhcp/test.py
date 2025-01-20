#!/usr/bin/env python3
"""
Route preference: DHCP vs Static

This test configures a device with both a DHCP-acquired route on a
dedicated interface and a static route to the same destination on
another interface.

Initially, DHCP is preferred over Static. Afterwards, the static
route takes precedence by adjusting the routing preference value
to the one lower than DHCP.
"""

import infamy
import infamy.route as route
import infamy.dhcp
from infamy.util import until, parallel
from infamy.netns import IsolatedMacVlans

def configure_interface(name, ip=None, prefix_length=None, forwarding=True):
    interface_config = {
        "name": name,
        "enabled": True,
        "ipv4": {
            "forwarding": forwarding,
            "address": []
        }
    }
    if ip and prefix_length:
        interface_config["ipv4"]["address"].append({"ip": ip, "prefix-length": prefix_length})
    return interface_config

def config_target1(target, data1, data2, link):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    configure_interface(data1),
                    configure_interface(data2, "192.168.30.1", 24),
                    configure_interface(link, "192.168.50.1", 24)
                ]
            }
        },
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [
                        {
                            "type": "infix-routing:static",
                            "name": "default",
                            "static-routes": {
                                "ipv4": {
                                    "route": [{
                                        "destination-prefix": "0.0.0.0/0",
                                        "next-hop": {"next-hop-address": "192.168.50.2"},
                                        "route-preference": 120
                                    }]
                                }
                            }
                        }
                    ]
                }
            }
        },
        "infix-dhcp-client": {
            "dhcp-client": {
                "enabled": True,
                "client-if": [
                    {
                        "if-name": data1,
                        "enabled": True,
                        "option": [
                            {"id": "broadcast"},
                            {"id": "dns"},
                            {"id": "domain"},
                            {"id": "hostname"},
                            {"id": "ntpsrv"},
                            {"id": "router"},
                            {"id": "subnet"}
                        ],
                        "route-preference": 5
                    }
                ]
            }
        }
    })

def config_target2(target, data, link):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    configure_interface(data, "192.168.20.2", 24),
                    configure_interface(link, "192.168.50.2", 24),
                    configure_interface("lo", "192.168.200.1", 32)
                ]
            }
        },
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [
                        {
                            "type": "infix-routing:static",
                            "name": "default",
                            "static-routes": {
                                "ipv4": {
                                    "route": [{
                                        "destination-prefix": "0.0.0.0/0",
                                        "next-hop": {"next-hop-address": "192.168.50.1"}
                                    }]
                                }
                            }
                        }
                    ]
                }
            }
        }
    })

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        env = infamy.Env()
        R1 = env.attach("R1", "mgmt")
        R2 = env.attach("R2", "mgmt")

    with test.step("Configure targets. Assign higher priority to the dhcp route"):
        _, R1data1 = env.ltop.xlate("R1", "data1")
        _, R1data2 = env.ltop.xlate("R1", "data2")
        _, R1link = env.ltop.xlate("R1", "link")
        _, R2data = env.ltop.xlate("R2", "data")
        _, R2link = env.ltop.xlate("R2", "link")

        parallel(config_target1(R1, R1data1, R1data2, R1link), 
                 config_target2(R2, R2data, R2link))

        _, hport_data12 = env.ltop.xlate("PC", "data12")
        ns0 = infamy.IsolatedMacVlan(hport_data12).start()
        ns0.addip("192.168.30.3", prefix_length=24)
        ns0.addroute("default", "192.168.30.1")

        _, hport_data11 = env.ltop.xlate("PC", "data11")
        _, hport_data2 = env.ltop.xlate("PC", "data2")
        ifmap={ hport_data11: "a", hport_data2: "b" }
        ns1 = IsolatedMacVlans(ifmap, False).start()

        ns1.addip(ifname="b", addr="192.168.20.3", prefix_length=24)
        ns1.addroute("192.168.200.1/32", "192.168.20.2")

        ns1.addip(ifname="a", addr="192.168.10.3", prefix_length=24)
        ns1.runsh("ip route add default dev a")

        with infamy.dhcp.Server(netns=ns1, ip="192.168.10.1", netmask="255.255.255.0", router="192.168.10.3", iface="a"):
            with test.step("Wait for DHCP and static routes"):
                print("Waiting for DHCP and static routes...")
                until(lambda: route.ipv4_route_exist(R1, "0.0.0.0/0", proto="ietf-routing:static", pref=5), attempts=200)
                until(lambda: route.ipv4_route_exist(R1, "0.0.0.0/0", proto="ietf-routing:static", pref=120), attempts=200)

            with test.step("Verify connectivity from PC:data12 to R2:lo via DHCP"):
                dhcp_route_active = route.ipv4_route_exist(R1, "0.0.0.0/0", pref=5, active_check=True)
                assert dhcp_route_active, "Failed to activate DHCP route: Expected DHCP-acquired route not found."

                ns0.must_reach("192.168.200.1")

            with test.step("Assign higher priority to the static route"):
                R1.put_config_dicts({
                    "ietf-routing": {
                        "routing": {
                            "control-plane-protocols": {
                                "control-plane-protocol": [
                                    {
                                        "type": "infix-routing:static",
                                        "name": "default",
                                        "static-routes": {
                                            "ipv4": {
                                                "route": [{
                                                    "destination-prefix": "0.0.0.0/0",
                                                    "next-hop": {"next-hop-address": "192.168.50.2"},
                                                    "route-preference": 1
                                                }]
                                            }
                                        }
                                    }
                                ]
                            }
                        }
                    }
                })

            with test.step("Verify connectivity from PC:data12 to R2:lo via static route"):
                ns0.must_reach("192.168.200.1")

                static_route_active = route.ipv4_route_exist(R1, "0.0.0.0/0", pref=1, active_check=True)
                assert static_route_active, "Static route activation failed: Verify route-preference adjustment on R1."

            test.succeed()
