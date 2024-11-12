#!/usr/bin/env python3
r"""OSPF Container

This use-case test verifies connectivity in an OSPF network to services
running as hosted containers inside each router.

NOTE: The _Controller_, _ABR_, and `data` connections are simulated by
the test PC. The `ringN` ports are connected to other DUTs via the test
PC, which can act as a link breaker.

.Use-case overview.
[#img-overview]
image::overview.svg[]

The DUTs are connected in a routed topology inside their own OSPF area.
A single area border router (ABR) is used to access the controller
network in OSPF area 0.  Each router also has "test point" connections
where the controller can attach other than its connection in area 0.

 - The ringN ports are intended to be connected to neighboring DUTs, but
   may at each end of the bus be used as test points
 - The data ports are intended to be test points for verifying
   connectivity with container B via br1
 - The uplink ports are for connecting to the ABR, at least one of the
   DUTs should not have a connection to the ABR, this to verify routing
   via another DUT
 - Area 1 is 10.1.Rn.0/16, and each router is assigned a /24

Each DUT hosts one application container and one system container, all
have the same setup, with only different subnets assigned.  A third
container is used to manipulate the firewall of each DUT, providing port
forwarding and masquerading.

Devices attached to the first bridge, `br0`, are supposed to be easily
accesible using IPv4, so internally they use IPv4 too, and to avoid any
risk of clashing with external IP subnets, IPv4 link-local addresses are
employed: `br0` request 169.254.1.1, so the second container (B) always
can reach it, the first container (A) reqquest 169.254.1.2 and the
second (B) request 169.254.1.3.  The network for devices attached to the
second bridge, `br1`, only use IPv6 link-local addresses.

.Internal network setup, here router R1 on subnet 10.1.1.1/24.
[#img-setup]
image::internal-network.svg[Internal networks]

 - *Container A* runs a very basic web server, it runs on port 80 inside
   the container, and `br0`, but is accessible outside on port 8080.
   The controller connects to each of these servers from OSPF area 0.
   For the controller to be able to distinguish between the servers,
   they all serve slightly different content
 - *Container B* runs a complete system with an SSH server.  During the
   test, the controller connects to this container using the `data` port
   to ensure the container can access all other parts of the network.
   To distinguish between the different container B's, each container
   will have a unique hostname derived from the chassis MAC address

"""
import infamy
import infamy.util as util
import infamy.route as route
from infamy.furl import Furl

BODY  = "<html><body><p>Router responding</p></body></html>"

def create_vlan_bridge(ns):
    return ns.runsh("""
    ip link add dev br0 type bridge
    ip link set dev br0 up
    ip link set dev iface1 up
    ip link set dev iface2 up
    ip link set dev iface1 master br0
    ip link set dev iface2 master br0
    ip link set dev br0 type bridge vlan_filtering 1
    bridge vlan del dev br0 vid 1 self
    bridge vlan del dev iface1 vid 1
    bridge vlan del dev iface2 vid 1
    bridge vlan add dev br0 vid 8 self
    bridge vlan add dev iface1 vid 8
    bridge vlan add dev iface2 vid 8
    """)





def config_generic(target, router, ring1, ring2, link):
    router_ip=f"10.1.{router}.1"
    link_ip=f"10.1.{router}.{101}"
    firewall_config = util.to_binary(f"""#!/usr/sbin/nft -f

flush ruleset

define UPLINK = "{link}"
define WIP = 169.254.1.2
                          """
                          """
table ip nat {
    chain prerouting {
        type nat hook prerouting priority filter; policy accept;
        iif $UPLINK tcp dport 8080 dnat to 169.254.1.2:91
        iif "br0" ip daddr 169.254.1.2 dnat to 169.254.1.1
    }

    chain postrouting {
        type nat hook postrouting priority srcnat; policy accept;
        oif $UPLINK masquerade
        oif "br0" ip daddr $WIP snat to 169.254.1.1
    }
}

""")
    data = util.to_binary(BODY)

    target.put_config_dicts({
        "ietf-interfaces": {
        "interfaces": {
            "interface": [
                {
                    "name": "lo",
                "type": "infix-if-type:loopback",
                "ietf-ip:ipv4": {
                    "address": [
                        {
                            "ip": "127.0.0.1",
                            "prefix-length": 8
                        }
                    ]
                },
                "ietf-ip:ipv6": {
                    "address": [
                        {
                            "ip": "::1",
                            "prefix-length": 128
                        }
                    ]
                }
            },

          {
              "name": ring1,
              "ietf-ip:ipv6": {},
              "infix-interfaces:bridge-port": {
                  "bridge": "br0"
              }
          },
            {
                "name": f"{ring1}.8",
                "type": "infix-if-type:vlan",
                "infix-interfaces:vlan": {
                    "tag-type": "ieee802-dot1q-types:c-vlan",
                    "id": 8,
                    "lower-layer-if": ring1
                },
                "ipv4": {
                    "address": [{
                        "ip": router_ip,
                        "prefix-length": 32
                    }]
                }

          },
            {
                "name": ring2,
                "ietf-ip:ipv6": {},
                "infix-interfaces:bridge-port": {
                    "bridge": "br0"
                }
            },
            {
                "name": f"{ring2}.8",
                "type": "infix-if-type:vlan",
                "infix-interfaces:vlan": {
                    "tag-type": "ieee802-dot1q-types:c-vlan",
                    "id": 8,
                    "lower-layer-if": ring2
                },
                "ipv4": {
                    "address": [{
                        "ip": router_ip,
                        "prefix-length": 32
                    }]
                }
            },
            {
                "name": link,
                "ipv4": {
                    "address": [{
                        "ip": link_ip,
                        "prefix-length": 24
                    }],
                    "forwarding": True
                },
                "ietf-ip:ipv6": {}
            },
            {
                "name": "br0",
                "type": "infix-if-type:bridge",
                "ietf-ip:ipv4": {
                    "enabled": True,
                    "forwarding": True,
                    "infix-ip:autoconf": {
                        "enabled": True,
                        "request-address": "169.254.1.1"
                    }
                },
                "ietf-ip:ipv6": {
                    "enabled": True
                },
                "infix-interfaces:bridge": {
                    "ieee-group-forward": [
                        "lldp"
                    ]
                }
            },
            {
                "name": "br1",
                "type": "infix-if-type:bridge",
                "infix-interfaces:bridge": {
                    "vlans": {
                        "vlan": [
                            {
                                "vid": 6,
                                "untagged": [
                                    "veth1b"
                                ]
                            }
                        ]
                    }
                }
            },
            {
                "name": "veth0a",
                "type": "infix-if-type:veth",
                "infix-interfaces:container-network": {
                    "type": "host",
                },
                "ipv4": {
                    "address": [
                        {
                            "ip": "169.254.1.2", # This only for test, use zeroconf in real-life
                            "prefix-length": 16
                        }
                    ]
                },
                "infix-interfaces:veth": {
                    "peer": "veth0b"
                },
                "infix-interfaces:custom-phys-address": {
                    "chassis": {
                        "offset": "06:00:00:00:00:00"
                    }
                }
            },
            {
                "name": "veth0b",
                "type": "infix-if-type:veth",
                "infix-interfaces:bridge-port": {
                    "bridge": "br0"
                },
                "infix-interfaces:veth": {
                    "peer": "veth0a"
                }
            },
            {
                "name": "veth1a",
                "type": "infix-if-type:veth",
                "infix-interfaces:container-network": {
                    "type": "host"
                },
                "infix-interfaces:veth": {
                    "peer": "veth1b"
                },
                "infix-interfaces:custom-phys-address": {
                    "chassis": {
                        "offset": "06:00:00:00:00:00"
                    }
                }
            },
            {
                "name": "veth1b",
                "type": "infix-if-type:veth",
                "infix-interfaces:bridge-port": {
                    "bridge": "br1",
                    "pvid": 6
                },
                "infix-interfaces:veth": {
                    "peer": "veth1a"
                }
            },
            {
                "name": "veth2a",
                "type": "infix-if-type:veth",
                "infix-interfaces:container-network": {
                    "type": "host"
                },
                "infix-interfaces:veth": {
                    "peer": "veth2b"
                },
                "infix-interfaces:custom-phys-address": {
                    "chassis": {
                        "offset": "06:00:00:00:00:00"
                    }
                }
            },
            {
                "name": "veth2b",
                "type": "infix-if-type:veth",
                "infix-interfaces:bridge-port": {
                    "bridge": "br0"
                },
                "infix-interfaces:veth": {
                    "peer": "veth2a"
                }
            },
            {
                "name": "veth3a",
                "type": "infix-if-type:veth",
                "infix-interfaces:container-network": {
                    "type": "host"
                },
                "infix-interfaces:veth": {
                    "peer": "veth3b"
                },
                "infix-interfaces:custom-phys-address": {
                    "chassis": {
                        "offset": "06:00:00:00:00:00"
                    }
                }
            },
            {
                "name": "veth3b",
                "type": "infix-if-type:veth",
                "infix-interfaces:bridge-port": {
                    "bridge": "br1",
                    "pvid": 6
                },
                "infix-interfaces:veth": {
                    "peer": "veth3a"
                }
            }
        ]}
        },
        "infix-containers": {
            "containers": {
                "container": [
                {
                    "name": "container-A",
                    "image": f"oci-archive:{infamy.Container.HTTPD_IMAGE}",
                    "command": "/usr/sbin/httpd -f -v -p 91",
                    "hostname": "web-container-%m",
                    "restart-policy": "retry",
                    "mount": [{
                            "name": "index.html",
                            "content": f"{data}",
                            "target": "/var/www/index.html"
                    }],
                    "network": {
                        "interface": [{
                            "name": "veth0a",
                            "option": [
                                "interface_name=br0",
                            ]},
                            {
                                "name": "veth1a",
                                "option": [
                                "interface_name=br1"
                            ]}
                        ],
                        "publish": [ "91" ]
                    }
                },
                {
                    "name": "container-B",
                    "image": f"oci-archive:{infamy.Container.HTTPD_IMAGE}",
                    "hostname": "web-container-%m",
                    "privileged": True,
                    "restart-policy": "retry",
                    "network": {
                        "interface": [
                        {
                            "name": "veth2a",
                            "option": ["ip=169.254.1.3"] # This only for test, use zeroconf in real-life
                        },
                        {
                            "name": "veth3a"
                        }
                        ]
                        }
                    },
                    {
                        "name": "firewall",
                        "image": f"oci-archive:{infamy.Container.NFTABLES_IMAGE}",
                        "network": {
                                "host": True
                            },
                            "mount": [{
                                "name": "nftables.conf",
                                "content": firewall_config,
                                "target": "/etc/nftables.conf"
                              }
                            ],
                            "privileged": True

                    }
                ]

            }
        },
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [{
                            "type": "infix-routing:ospfv2",
                            "name": "default",
                            "ietf-ospf:ospf": {
                                "explicit-router-id": router_ip,
                                "areas": {
                                    "area": [{
                                        "area-id": "0.0.0.1",
                                        "area-type": "nssa-area",
                                        "interfaces": {
                                            "interface": [{
                                                "name": f"{ring1}.8",
                                                "interface-type": "point-to-point",
                                                "enabled": True
                                            },
                                            {
                                                "name": f"{ring2}.8",
                                                "interface-type": "point-to-point",
                                                "enabled": True
                                            },
                                            {
                                                "name": link,
                                                "enabled": True
                                            }]
                                        }
                                    }]
                                }
                            }
                    }]
                }
            }
        }
    })
def config_abr(target, data, link1, link2, link3):
    target.put_config_dicts({
        "ietf-interfaces": {
        "interfaces": {
                "interface": [
                {
                    "name": "lo",
                    "type": "infix-if-type:loopback",
                    "ietf-ip:ipv4": {
                        "address": [
                            {
                                "ip": "127.0.0.1",
                                "prefix-length": 8
                            }
                        ]
                    },
                    "ietf-ip:ipv6": {
                        "address": [
                            {
                                "ip": "::1",
                                "prefix-length": 128
                            }
                        ]
                    }
                },
                {
                    "name": data,
                    "ipv4": {
                        "forwarding": True,
                        "address": [{
                            "ip": "192.168.100.1",
                            "prefix-length": 24
                        }]
                    }
                },
                {
                    "name": link1,
                    "ipv4": {
                        "forwarding": True,
                        "address": [{
                            "ip": "10.1.1.100",
                            "prefix-length": 24
                        }]
                    }
                },
                {
                    "name": link2,
                    "ipv4": {
                        "forwarding": True,
                        "address": [{
                            "ip": "10.1.2.100",
                            "prefix-length": 24
                        }]
                    }
                },
                {
                    "name": link3,
                    "ipv4": {
                        "forwarding": True,
                        "address": [{
                            "ip": "10.1.3.100",
                            "prefix-length": 24
                        }]
                    }
                }]
            }
        },
        "ietf-routing": {
        "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [
                        {
                            "type": "infix-routing:ospfv2",
                            "name": "default",
                            "ietf-ospf:ospf": {
                                "areas": {
                                    "area": [
                                        {
                                            "area-id": "0.0.0.0",
                                            "interfaces": {
                                                "interface": [
                                                    {
                                                        "name": data,
                                                        "enabled": True
                                                    }
                                                ]
                                            }
                                        },
                                        {
                                            "area-id": "0.0.0.1",
                                            "area-type": "nssa-area",
                                            "interfaces": {
                                                "interface":
                                                [
                                                    {
                                                        "name": link1,
                                                        "enabled": True
                                                    },
                                                    {
                                                        "name": link2,
                                                        "enabled": True
                                                    },
                                                    {
                                                        "name": link3,
                                                        "enabled": True
                                                    },
                                                ]
                                            }
                                        }
                                    ]
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
        R3 = env.attach("R3", "mgmt")
        ABR = env.attach("ABR", "mgmt")
        if not R1.has_model("infix-containers"):
            test.skip()
        if not R2.has_model("infix-containers"):
            test.skip()
        if not R3.has_model("infix-containers"):
            test.skip()
    with test.step("Configure DUTs"):
        _, R1ring1 = env.ltop.xlate("R1", "ring1")
        _, R1ring2 = env.ltop.xlate("R1", "ring2")
        _, R2ring1 = env.ltop.xlate("R2", "ring1")
        _, R2ring2 = env.ltop.xlate("R2", "ring2")
        _, R3ring1 = env.ltop.xlate("R3", "ring1")
        _, R3ring2 = env.ltop.xlate("R3", "ring2")

        _, hostR1ring1 = env.ltop.xlate("host", "R1ring1")
        _, hostR1ring2 = env.ltop.xlate("host", "R1ring2")

        _, hostR2ring1 = env.ltop.xlate("host", "R2ring1")
        _, hostR2ring2 = env.ltop.xlate("host", "R2ring2")
        _, hostR3ring1 = env.ltop.xlate("host", "R3ring1")
        _, hostR3ring2 = env.ltop.xlate("host", "R3ring2")

        _, R1link = env.ltop.xlate("R1", "link")
        _, R2link = env.ltop.xlate("R2", "link")
        _, R3link = env.ltop.xlate("R3", "link")

        _, R1data = env.ltop.xlate("R1", "data")
        _, R3data = env.ltop.xlate("R3", "data")
        _, ABRdata =  env.ltop.xlate("ABR", "data")
        _, ABRlink1 = env.ltop.xlate("ABR", "link1")
        _, ABRlink2 = env.ltop.xlate("ABR", "link2")
        _, ABRlink3 = env.ltop.xlate("ABR", "link3")

        util.parallel(lambda: config_generic(R1, 1, R1ring1, R1ring2, R1link),
                      lambda: config_generic(R2, 2, R2ring1, R2ring2, R2link),
                      lambda: config_generic(R3, 3, R3ring1, R3ring2, R3link),
                      lambda: config_abr(ABR, ABRdata, ABRlink1, ABRlink2, ABRlink3))

    with infamy.IsolatedMacVlans({hostR1ring1: "iface1", hostR2ring2: "iface2"}) as sw1,\
         infamy.IsolatedMacVlans({hostR2ring1: "iface1", hostR3ring2: "iface2"}) as sw2, \
         infamy.IsolatedMacVlans({hostR3ring1: "iface1", hostR1ring2: "iface2"}) as sw3:
        create_vlan_bridge(sw1)
        create_vlan_bridge(sw2)
        create_vlan_bridge(sw3)
        #breakpoint()
        _, hport0 = env.ltop.xlate("host", "data4")

        with test.step("Wait for all routers to peer"):
            util.until(lambda: route.ospf_get_neighbor(R1, "0.0.0.1", f"{R1ring1}.8", "10.1.2.1"), attempts=200)
            util.until(lambda: route.ospf_get_neighbor(R1, "0.0.0.1", f"{R1ring2}.8", "10.1.3.1"), attempts=200)
            util.until(lambda: route.ospf_get_neighbor(R2, "0.0.0.1", f"{R2ring1}.8", "10.1.3.1"), attempts=200)
            util.until(lambda: route.ospf_get_neighbor(R2, "0.0.0.1", f"{R2ring2}.8", "10.1.1.1"), attempts=200)
            util.until(lambda: route.ospf_get_neighbor(R3, "0.0.0.1", f"{R3ring1}.8", "10.1.1.1"), attempts=200)
            util.until(lambda: route.ospf_get_neighbor(R3, "0.0.0.1", f"{R3ring2}.8", "10.1.2.1"), attempts=200)

            util.until(lambda: route.ospf_get_neighbor(ABR, "0.0.0.1", ABRlink1, "10.1.1.1"), attempts=200)
            util.until(lambda: route.ospf_get_neighbor(ABR, "0.0.0.1", ABRlink2, "10.1.2.1"), attempts=200)
            util.until(lambda: route.ospf_get_neighbor(ABR, "0.0.0.1", ABRlink3, "10.1.3.1"), attempts=200)

        with infamy.IsolatedMacVlan(hport0) as ns:
            ns.addip("192.168.100.2")
            ns.addroute("0.0.0.0/0", "192.168.100.1")
            #breakpoint()
            with test.step("Verify ABR:data can access container A on R1 (10.1.1.101)"):
                furl=Furl("http://10.1.1.101:8080")
                util.until(lambda: furl.nscheck(ns, BODY))
            with test.step("Verify ABR:data can access container A on R2 (10.1.2.101)"):
                furl=Furl("http://10.1.2.101:8080")
                util.until(lambda: furl.nscheck(ns, BODY))
            with test.step("Verify ABR:data can access container A on R3 (10.1.3.101)"):
                furl=Furl("http://10.1.3.101:8080")
                util.until(lambda: furl.nscheck(ns, BODY))
    test.succeed()
