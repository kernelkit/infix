#!/usr/bin/env python3
"""NTP client stratum selection test

Verify NTP client properly selects between multiple servers based on
stratum level.

This test validates NTP clock selection algorithm by configuring a client
to sync from two servers with different stratum levels:

- srv1: Test PC running chronyd, serving its local clock at stratum 5
  with an honest root distance so clients tolerate startup transients
- srv2: NTP server DUT syncing from srv1 (stratum 6)
- client: NTP client DUT syncing from both servers

Both servers sync to the same time source (srv2 syncs from srv1),
ensuring time agreement and avoiding the "falseticker" problem. The client
should then select srv1 (lower stratum) as its sync source.

NOTE: srv1 serves the test PC's system clock, so the test depends on that
clock being stable for the duration of the run.  A host time daemon that
applies discrete corrections, e.g. systemd-timesyncd, makes the client
flag srv1 as unstable and refuse to select it.  Test PCs should keep the
clock free-running during the test, or discipline it with a slewing
daemon such as chronyd.

"""

import infamy
from infamy.util import parallel, until
import infamy.ntp as ntp
import infamy.ntp_server as ntp_server

# Network configuration
ips = {
    "srv1":   "192.168.1.1",   # chronyd on test PC
    "srv2":   "192.168.1.2",   # Infix NTP server
    "client": "192.168.1.3"    # Infix NTP client
}

# 16 s polls so selection re-evaluates quickly after iburst
POLL = {"infix-system:minpoll": 4, "infix-system:maxpoll": 6}

with infamy.Test() as test:
    with test.step("Set up topology and attach to devices"):
        env = infamy.Env()
        srv2, client = parallel(lambda: env.attach("srv2", "mgmt"),
                                lambda: env.attach("client", "mgmt"))

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
                                "iburst": True,
                                # Poll every 16 s so sub-threshold
                                # offsets drain quickly (corrections
                                # are spread over ~3 poll intervals)
                                "minpoll": 4,
                                "maxpoll": 6
                            }]
                        }
                    }
                })

            with test.step("Wait for srv2 to sync from srv1"):
                # Converged, not just associated, see the docstring of
                # server_source_synced.  iburst + makestep take 10-20 s,
                # the rest of the budget is only used when broken
                try:
                    until(lambda: ntp.server_source_synced(srv2, ips["srv1"]),
                          attempts=60)
                except Exception:
                    print("DEBUG: srv2 did not converge on srv1. Associations:")
                    for assoc in ntp.server_get_associations(srv2):
                        print(f"  {assoc}")
                    raise

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
                                "infix-system:stratum-weight": 1.0,
                                "server": [{
                                    "name": "srv1",
                                    "udp": {
                                        "address": ips["srv1"]
                                    },
                                    "iburst": True,
                                    **POLL
                                }, {
                                    "name": "srv2",
                                    "udp": {
                                        "address": ips["srv2"]
                                    },
                                    "iburst": True,
                                    **POLL
                                }]
                            }
                        }
                    }
                })

            with test.step("Wait for client to see both servers"):
                until(lambda: ntp.number_of_sources(client) == 2, attempts=30)

            with test.step("Wait for srv2 stratum to stabilize"):
                # Ensure srv2 has synced with srv1 and is advertising
                # stratum 2.  This prevents race where both advertise
                # stratum 1, causing wrong selection
                def check_stratums():
                    stratum = {src.get("address"): src.get("stratum")
                               for src in ntp.get_sources(client)}
                    srv1_stratum = stratum.get(ips["srv1"])
                    srv2_stratum = stratum.get(ips["srv2"])

                    # Both must have valid stratums and srv1 < srv2
                    return bool(srv1_stratum and srv2_stratum
                                and srv1_stratum < srv2_stratum)

                # srv2 synced before the client was configured, so the
                # client's iburst samples already carry stratum 6
                until(check_stratums, attempts=30)
                print(f"srv1 and srv2 stratums verified as different")

            with test.step("Verify client selects srv1 (lower stratum)"):
                def srv1_selected():
                    source = ntp.any_source_selected(client)
                    if source and source.get("address") == ips["srv1"]:
                        return source
                    return None

                try:
                    # Selection normally happens at the end of iburst;
                    # with minpoll 4 this covers two extra 16 s poll
                    # cycles plus slack
                    selected = until(srv1_selected, attempts=45)
                except Exception:
                    # Timeout - print diagnostic info
                    print("DEBUG: Failed to select srv1. Sources:")
                    for src in ntp.get_sources(client):
                        print(f"  {src}")
                    raise

                assert selected is not None, "srv1 was not selected"
                print(f"Client correctly selected srv1 ({ips['srv1']}) "
                      f"with stratum {selected.get('stratum')}")

    test.succeed()
