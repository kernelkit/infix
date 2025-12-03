#!/usr/bin/env python3
"""NTP client stratum selection test

Verify NTP client properly selects between multiple servers based on
stratum level.

This test validates NTP clock selection algorithm by configuring a client
to sync from two servers with different stratum levels:

- srv1: Test PC running BusyBox ntpd (stratum ~1 via -l flag)
- srv2: NTP server DUT syncing from srv1 (stratum ~2)
- client: NTP client DUT syncing from both servers

Both servers sync to the same time source (srv2 syncs from srv1),
ensuring time agreement and avoiding the "falseticker" problem. The client
should then select srv1 (lower stratum) as its sync source.

"""

import infamy
from infamy import until
import infamy.ntp as ntp
import infamy.ntp_server as ntp_server

# Network configuration
ips = {
    "srv1":   "192.168.1.1",   # BusyBox ntpd on test PC
    "srv2":   "192.168.1.2",   # Infix NTP server
    "client": "192.168.1.3"    # Infix NTP client
}

with infamy.Test() as test:
    with test.step("Set up topology and attach to devices"):
        env = infamy.Env()
        srv2 = env.attach("srv2", "mgmt")
        client = env.attach("client", "mgmt")

        _, swp1 = env.ltop.xlate("srv2", "swp1")
        _, swp2 = env.ltop.xlate("srv2", "swp2")
        _, eth0 = env.ltop.xlate("client", "eth0")
        _, srv1 = env.ltop.xlate("host", "srv1")

    with infamy.IsolatedMacVlan(srv1) as ns_srv1:
        ns_srv1.addip(ips["srv1"])

        with ntp_server.Server(ns_srv1):
            with test.step("Configure srv2 to sync from srv1 and serve with higher stratum"):
                srv2.put_config_dicts({
                    "ietf-interfaces": {
                        "interfaces": {
                            "interface": [{
                                "name": "br0",
                                "type": "infix-if-type:bridge",
                                "enabled": True,
                                "ipv4": {
                                    "address": [{
                                        "ip": ips["srv2"],
                                        "prefix-length": 24,
                                    }]
                                }
                            }, {
                                "name": swp1,
                                "enabled": True,
                                "infix-interfaces:bridge-port": {
                                    "bridge": "br0"
                                }
                            }, {
                                "name": swp2,
                                "enabled": True,
                                "infix-interfaces:bridge-port": {
                                    "bridge": "br0"
                                }
                            }]
                        }
                    },
                    "ietf-ntp": {
                        "ntp": {
                            "unicast-configuration": [{
                                "address": ips["srv1"],  # Sync from srv1
                                "type": "uc-server",
                                "iburst": True
                            }]
                        }
                    }
                })

            with test.step("Wait for srv2 to sync from srv1"):
                until(lambda: ntp.server_has_associations(srv2), attempts=60)

            with test.step("Configure client to sync from both servers"):
                client.put_config_dicts({
                    "ietf-interfaces": {
                        "interfaces": {
                            "interface": [{
                                "name": eth0,
                                "enabled": True,
                                "ipv4": {
                                    "address": [{
                                        "ip": ips["client"],
                                        "prefix-length": 24
                                    }]
                                }
                            }]
                        }
                    },
                    "ietf-system": {
                        "system": {
                            "ntp": {
                                "enabled": True,
                                "server": [{
                                    "name": "srv1",
                                    "udp": {
                                        "address": ips["srv1"]
                                    },
                                    "iburst": True
                                }, {
                                    "name": "srv2",
                                    "udp": {
                                        "address": ips["srv2"]
                                    },
                                    "iburst": True
                                }]
                            }
                        }
                    }
                })

            with test.step("Wait for client to see both servers"):
                until(lambda: ntp.number_of_sources(client) == 2, attempts=60)

            with test.step("Wait for srv2 stratum to stabilize"):
                # Ensure srv2 has synced with srv1 and is advertising
                # stratum 2.  This prevents race where both advertise
                # stratum 1, causing wrong selection
                def check_stratums():
                    srv1 = ntp.get_source_by_address(client, ips["srv1"])
                    srv2 = ntp.get_source_by_address(client, ips["srv2"])

                    if not srv1 or not srv2:
                        return False

                    srv1_stratum = srv1.get("stratum")
                    srv2_stratum = srv2.get("stratum")

                    # Both must have valid stratums and srv1 < srv2
                    if srv1_stratum and srv2_stratum and srv1_stratum < srv2_stratum:
                        return True
                    return False

                until(check_stratums, attempts=60)
                print(f"srv1 and srv2 stratums verified as different")

            with test.step("Verify client selects srv1 (lower stratum)"):
                def srv1_selected():
                    source = ntp.any_source_selected(client)
                    if source and source.get("address") == ips["srv1"]:
                        return source
                    return None

                try:
                    selected = until(srv1_selected, attempts=120)
                except Exception:
                    # Timeout - print diagnostic info
                    sources = ntp.get_sources(client)
                    print("DEBUG: Failed to select srv1. Source details:")
                    for src in sources:
                        print(f"  {src.get('address')}: stratum={src.get('stratum')}, "
                              f"state={src.get('state')}, poll={src.get('poll')}, "
                              f"offset={src.get('offset')}")
                    raise

                assert selected is not None, "srv1 was not selected"
                print(f"Client correctly selected srv1 ({ips['srv1']}) "
                      f"with stratum {selected.get('stratum')}")

    test.succeed()
