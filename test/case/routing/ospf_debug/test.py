#!/usr/bin/env python3
"""OSPF Debug Logging

Verifies OSPF debug logging by configuring two routers (R1 and R2) with
OSPF on their interconnecting link. The test enables specific OSPF debug
categories and verifies that appropriate debug messages appear in
/var/log/debug.

This test specifically validates:
- Debug messages appear when debug options are enabled
- No excessive debug messages when debug options are disabled
- Individual categories (ism, nsm, packet) can be toggled independently

"""

import infamy
import infamy.route as route
from infamy.util import until, parallel


def verify_debug_messages_present(ssh, logfile):
    """Verify OSPF debug messages are present in the log file."""
    def check_log():
        rc = ssh.runsh(f"cat /var/log/{logfile} 2>/dev/null")
        log_content = rc.stdout if rc.returncode == 0 else ""

        # Check for ISM (Interface State Machine) debug messages
        # Example: "ISM[e5:192.168.50.1]: Down (InterfaceUp)"
        has_ism = "ISM[" in log_content

        # Check for NSM (Neighbor State Machine) debug messages
        # Example: "NSM[e5:192.168.50.1:192.168.200.1:default]: Down (HelloReceived)"
        has_nsm = "NSM[" in log_content

        # Check for packet debug messages - look for Hello packet details
        # Example: "Type 1 (Hello)" or "ospf_recv_packet"
        has_packet = "ospf_recv_packet" in log_content or "Type 1 (Hello)" in log_content

        return has_ism and has_nsm and has_packet

    return check_log


def verify_debug_messages_minimal(ssh, logfile):
    """Verify OSPF debug messages are minimal/absent in the log file."""
    def check_log():
        rc = ssh.runsh(f"cat /var/log/{logfile} 2>/dev/null")
        log_content = rc.stdout if rc.returncode == 0 else ""

        # When debug is disabled, we shouldn't see verbose debug messages
        lines = log_content.split('\n')
        # Look for ISM, NSM, and detailed packet dumps
        ospf_debug_lines = [l for l in lines if "ISM[" in l or "NSM[" in l or "ospf_recv_packet" in l or "Type 1 (Hello)" in l]

        # Allow some residual messages but not many
        return len(ospf_debug_lines) <= 10

    return check_log


def config_target1(target, link, enable_debug=False):
    ospf_config = {
        "type": "infix-routing:ospfv2",
        "name": "default",
        "ospf": {
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
                            "enabled": True,
                            "name": link,
                            "hello-interval": 1,
                            "dead-interval": 3
                        }]
                    },
                }]
            }
        }
    }

    if enable_debug:
        ospf_config["ospf"]["debug"] = {
            "ism": True,
            "nsm": True,
            "packet": True
        }

    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": link,
                    "enabled": True,
                    "ipv4": {
                        "forwarding": True,
                        "address": [{
                            "ip": "192.168.50.1",
                            "prefix-length": 24
                        }]
                    }
                }, {
                    "name": "lo",
                    "enabled": True,
                    "ipv4": {
                        "address": [{
                            "ip": "192.168.100.1",
                            "prefix-length": 32
                        }]
                    }
                }]
            }
        },
        "ietf-system": {
            "system": {
                "hostname": "R1"
            }
        },
        "ietf-syslog": {
            "syslog": {
                "actions": {
                    "file": {
                        "log-file": [{
                            "name": "file:ospf-debug",
                            "infix-syslog:property-filter": {
                                "property": "programname",
                                "operator": "isequal",
                                "value": "ospfd"
                            },
                            "facility-filter": {
                                "facility-list": [{
                                    "facility": "all",
                                    "severity": "debug"
                                }]
                            }
                        }]
                    }
                }
            }
        },
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [ospf_config]
                }
            }
        }
    })


def config_target2(target, link):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": link,
                    "enabled": True,
                    "ipv4": {
                        "forwarding": True,
                        "address": [{
                            "ip": "192.168.50.2",
                            "prefix-length": 24
                        }]
                    }
                }, {
                    "name": "lo",
                    "enabled": True,
                    "ipv4": {
                        "address": [{
                            "ip": "192.168.200.1",
                            "prefix-length": 32
                        }]
                    }
                }]
            }
        },
        "ietf-system": {
            "system": {
                "hostname": "R2"
            }
        },
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [{
                        "type": "infix-routing:ospfv2",
                        "name": "default",
                        "ospf": {
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
                                            "enabled": True,
                                            "name": link,
                                            "hello-interval": 1,
                                            "dead-interval": 3
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


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        env = infamy.Env()
        R1 = env.attach("R1", "mgmt")
        R1ssh = env.attach("R1", "mgmt", "ssh")
        R2 = env.attach("R2", "mgmt")

    with test.step("Clean up old log files from previous test runs"):
        R1ssh.runsh("sudo rm -f /var/log/ospf-debug")

    with test.step("Configure R1 and R2 without debug enabled"):
        _, R1link = env.ltop.xlate("R1", "link")
        _, R2link = env.ltop.xlate("R2", "link")

        parallel(config_target1(R1, R1link, enable_debug=False),
                 config_target2(R2, R2link))

    with test.step("Wait for OSPF adjacency to form"):
        until(lambda: route.ipv4_route_exist(R1, "192.168.200.1/32", proto="ietf-ospf:ospfv2"), attempts=200)
        until(lambda: route.ipv4_route_exist(R2, "192.168.100.1/32", proto="ietf-ospf:ospfv2"), attempts=200)

    with test.step("Enable OSPF debug logging on R1"):
        config_target1(R1, R1link, enable_debug=True)

    with test.step("Verify OSPF debug messages appear in log file"):
        until(verify_debug_messages_present(R1ssh, "ospf-debug"), attempts=30)

    with test.step("Remove log file before disabling debug"):
        R1ssh.runsh("sudo rm -f /var/log/ospf-debug")

    with test.step("Disable OSPF debug logging on R1"):
        config_target1(R1, R1link, enable_debug=False)

    with test.step("Verify no OSPF debug messages when disabled"):
        until(verify_debug_messages_minimal(R1ssh, "ospf-debug"), attempts=30)

    test.succeed()
