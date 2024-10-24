#!/usr/bin/env python3

r"""OSPF container

Good test
"""
import infamy
import infamy.util as util

def config_generic(target, router, ring1, ring2, link):
    target.put_config_dict("ietf-interfaces", {
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
                        "ip": f"10.0.{router}.1",
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
                        "ip": f"10.0.{router}.1",
                        "prefix-length": 32
                    }]
                }
            },
            {
                "name": link,
                "ietf-ip:ipv4": {
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
                    "type": "host"
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
        ]
    }})
    target.put_config_dict("infix-containers", {
        "containers": {
            "container": [
                {
                "name": "container-A",
                "image": f"oci-archive:{infamy.Container.HTTPD_IMAGE}",
                "hostname": "web-container-%m",
                "privileged": True,
                "restart-policy": "retry",
                "network": {
                    "interface": [
                    {
                        "name": "veth0a",
                        "option": [
                            "interface_name=br0"
                        ]
                    },
                    {
                        "name": "veth1a",
                        "option": [
                            "interface_name=br1"
                        ]
                    }
                    ]
                },
                "mount": [
                    {
                        "name": "infix",
                        "source": "/proc/1",
                        "target": "/1"
                    }
                ],
                "volume": [
                    {
                        "name": "persistent",
                        "target": "/var/persistent"
                    }
                ]
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
                        "name": "veth2a"
                    },
                    {
                        "name": "veth3a"
                    }
                    ]
                }
            }
        ]
        }})
    target.put_config_dict("ietf-routing", {
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
                                    "area-id": "0.0.80.79",
                                    "area-type": "nssa-area",
                                    "interfaces": {
                                        "interface": [
                                            {
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
                                                "name": "lo",
                                                "enabled": True
                                            }
                                        ]
                                    }
                                }
                            ]
                        }
                    }
                }
            ]
        }
    }})
def config_abr(target, data, link1, link2, link3):
    target.put_config_dict("ietf-interfaces", {
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
                    "address": [{
                        "ip": "192.168.100.1",
                        "prefix-length": 24
                    }]
                }
            },
            {
                "name": "br0",
                "type": "infix-if-type:bridge",
                "ipv4": {
                    "address": [{
                        "ip": "10.0.0.100",
                        "prefix-length": 24
                    }]
                }
            },
            {
                "name": link1,
                "bridge-port": {
                    "bridge": "br0"
                }
            },
            {
                "name": link2,
                "bridge-port": {
                    "bridge": "br0"
                }
            },
            {
                "name": link3,
                "bridge-port": {
                    "bridge": "br0"
                }
            }
            ]
        }
    })
    target.put_config_dict("ietf-routing", {
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
                                    "area-id": "0.0.80.79",
                                    "area-type": "nssa-area",
                                    "interfaces": {
                                        "interface":
                                        [
                                            {
                                                "name": "br0",
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
    test.succeed()
