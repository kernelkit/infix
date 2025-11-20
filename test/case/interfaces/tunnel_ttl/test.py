#!/usr/bin/env python3
"""Tunnel TTL Verification

Verify that GRE and VXLAN tunnels use a fixed TTL (default 64) for
encapsulated frames instead of inheriting the TTL from inner packets.
Critical for protocols like OSPF that use TTL=1 for their packets.

The test setup creates a tunnel between R1 and R3 so that injecting
a frame with TTL=3 from PC:west, routing it through the tunnel, it
would still reach PC:east.  (Had it been routed via R2 it would be too
many hops and the TTL would reach zero before the last routing step.)

    PC:west -- R1 -- R2 -- R3 -- PC:east
               `== Tunnel =='

"""

import infamy


class ArgumentParser(infamy.ArgumentParser):
    def __init__(self):
        super().__init__()
        self.add_argument("--tunnel")

# IP address plan
# PC:west subnet
PC_WEST_R1   = "192.168.10.1"
PC_WEST_HOST = "192.168.10.2"
PC_WEST_NET  = "192.168.10.0/24"

# R1-R2 underlay subnet
R1_R2_R1  = "192.168.50.1"
R1_R2_R2  = "192.168.50.2"
R1_R2_NET = "192.168.50.0/24"

# R2-R3 underlay subnet
R2_R3_R2  = "192.168.60.1"
R2_R3_R3  = "192.168.60.2"
R2_R3_NET = "192.168.60.0/24"

# PC:east subnet
PC_EAST_R3   = "192.168.70.1"
PC_EAST_HOST = "192.168.70.2"
PC_EAST_NET  = "192.168.70.0/24"

# Tunnel subnet
TUNNEL_R1  = "10.255.0.1"
TUNNEL_R3  = "10.255.0.2"
TUNNEL_NET = "10.255.0.0/30"

# Prefix lengths
PREFIX_24 = 24
PREFIX_30 = 30

# Test TTL value
TEST_TTL = 3

# VXLAN VNI
VNI = 10


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        arg = ArgumentParser()
        env = infamy.Env(args=arg)
        tunnel = env.args.tunnel
        r1 = env.attach("R1", "mgmt")
        r2 = env.attach("R2", "mgmt")
        r3 = env.attach("R3", "mgmt")

    with test.step(f"Configure R1 with {tunnel} tunnel to R3"):
        # R1: Entry point, west facing PC, east facing R2, tunnel to R3

        # Build tunnel container configs
        container_r1 = {
            "local": R1_R2_R1,
            "remote": R2_R3_R3
        }
        container_r3 = {
            "local": R2_R3_R3,
            "remote": R1_R2_R1
        }

        CONTAINER_TYPE = tunnel
        if tunnel == "vxlan":
            container_r1["vni"] = VNI
            container_r3["vni"] = VNI

        r1.put_config_dicts({"ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": r1["west"],
                    "ipv4": {
                        "address": [{
                            "ip": PC_WEST_R1,
                            "prefix-length": PREFIX_24
                        }],
                        "forwarding": True
                    }
                }, {
                    "name": r1["east"],
                    "ipv4": {
                        "address": [{
                            "ip": R1_R2_R1,
                            "prefix-length": PREFIX_24
                        }],
                        "forwarding": True
                    }
                }, {
                    "name": f"{tunnel}0",
                    "type": f"infix-if-type:{tunnel}",
                    "ipv4": {
                        "address": [{
                            "ip": TUNNEL_R1,
                            "prefix-length": PREFIX_30
                        }],
                        "forwarding": True
                    },
                    CONTAINER_TYPE: container_r1
                }]
            }
        }, "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [{
                        "type": "infix-routing:static",
                        "name": "default",
                        "static-routes": {
                            "ipv4": {
                                "route": [{
                                    "destination-prefix": R2_R3_NET,
                                    "next-hop": {
                                        "next-hop-address": R1_R2_R2
                                    }
                                }, {
                                    "destination-prefix": PC_EAST_NET,
                                    "next-hop": {
                                        "next-hop-address": TUNNEL_R3
                                    }
                                }]
                            }
                        }
                    }]
                }
            }
        }})

    with test.step("Configure R2 as intermediate router (underlay forwarding)"):
        # R2: Intermediate router, just forwards packets between west and east
        r2.put_config_dicts({"ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": r2["west"],
                    "ipv4": {
                        "address": [{
                            "ip": R1_R2_R2,
                            "prefix-length": PREFIX_24
                        }],
                        "forwarding": True
                    }
                }, {
                    "name": r2["east"],
                    "ipv4": {
                        "address": [{
                            "ip": R2_R3_R2,
                            "prefix-length": PREFIX_24
                        }],
                        "forwarding": True
                    }
                }]
            }
        }})

    with test.step(f"Configure R3 with {tunnel} tunnel to R1"):
        # R3: Exit point, west facing R2, east facing PC, tunnel to R1
        r3.put_config_dicts({"ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": r3["west"],
                    "ipv4": {
                        "address": [{
                            "ip": R2_R3_R3,
                            "prefix-length": PREFIX_24
                        }],
                        "forwarding": True
                    }
                }, {
                    "name": r3["east"],
                    "ipv4": {
                        "address": [{
                            "ip": PC_EAST_R3,
                            "prefix-length": PREFIX_24
                        }],
                        "forwarding": True
                    }
                }, {
                    "name": f"{tunnel}0",
                    "type": f"infix-if-type:{tunnel}",
                    "ipv4": {
                        "address": [{
                            "ip": TUNNEL_R3,
                            "prefix-length": PREFIX_30
                        }],
                        "forwarding": True
                    },
                    CONTAINER_TYPE: container_r3
                }]
            }
        }, "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [{
                        "type": "infix-routing:static",
                        "name": "default",
                        "static-routes": {
                            "ipv4": {
                                "route": [{
                                    "destination-prefix": R1_R2_NET,
                                    "next-hop": {
                                        "next-hop-address": R2_R3_R2
                                    }
                                }, {
                                    "destination-prefix": PC_WEST_NET,
                                    "next-hop": {
                                        "next-hop-address": TUNNEL_R1
                                    }
                                }]
                            }
                        }
                    }]
                }
            }
        }})

    with test.step("Send ping from PC:west to PC:east with low TTL"):
        _, pc_east = env.ltop.xlate("PC", "east")

        with infamy.IsolatedMacVlan(pc_east) as east_ns:
            east_ns.addip(PC_EAST_HOST)
            east_ns.addroute("default", PC_EAST_R3)

            pcap = east_ns.pcap("icmp")
            with pcap:
                _, pc_west = env.ltop.xlate("PC", "west")
                with infamy.IsolatedMacVlan(pc_west) as west_ns:
                    west_ns.addip(PC_WEST_HOST)
                    west_ns.addroute("default", PC_WEST_R1)

                    # Send 10 pings with TTL=3, TTL is decremented before each
                    # router hop.  So at PC:west (TTL=3) -> R1 routed to GRE
                    # tunnel (TTL=2) -> frame egresses tunnel -> R3 where it
                    # is routed to PC:east (TTL=1).
                    #
                    # If outer TTL was inherited (TTL=2), packet would be
                    # dropped at R3.
                    west_ns.runsh(f"ping -c 10 -t {TEST_TTL} {PC_EAST_HOST}")

    with test.step("Verify packets arrived at PC:east"):
        packets = pcap.tcpdump()
        print("Captured packets on PC:east:")
        print(packets)

        pings = [line for line in packets.splitlines()
                 if f"{PC_WEST_HOST} > {PC_EAST_HOST}: ICMP echo request" in line]

        assert len(pings) >= 1, f"Expected at least 1 ping, got {len(pings)}."

    test.succeed()
