#!/usr/bin/env python3
"""NTP peer mode test

Verify NTP server operating in peer mode with bidirectional synchronization.

This test validates peer mode where two NTP servers synchronize with each other
bidirectionally. Each server acts as both client and server to the other:
- peer1: Stratum 8 local clock, peered with peer2
- peer2: Stratum 8 local clock, peered with peer1

The test verifies mutual synchronization between peers.
"""

import infamy
from infamy import until
import infamy.ntp as ntp


with infamy.Test() as test:
    with test.step("Set up topology and attach to devices"):
        env = infamy.Env()
        peer1 = env.attach("peer1", "mgmt")
        peer2 = env.attach("peer2", "mgmt")

        _, peer1_data = env.ltop.xlate("peer1", "data")
        _, peer2_data = env.ltop.xlate("peer2", "data")

    with test.step("Configure peer1 with peer relationship to peer2"):
        peer1.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": peer1_data,
                        "enabled": True,
                        "ipv4": {
                            "address": [{
                                "ip": "192.168.3.1",
                                "prefix-length": 24
                            }]
                        }
                    }]
                }
            },
            "ietf-ntp": {
                "ntp": {
                    "unicast-configuration": [{
                        "address": "192.168.3.2",
                        "type": "uc-peer",
                        "minpoll": 2
                    }],
                    "refclock-master": {
                        "master-stratum": 8
                    }
                }
            }
        })

    with test.step("Configure peer2 with peer relationship to peer1"):
        peer2.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": peer2_data,
                        "enabled": True,
                        "ipv4": {
                            "address": [{
                                "ip": "192.168.3.2",
                                "prefix-length": 24
                            }]
                        }
                    }]
                }
            },
            "ietf-ntp": {
                "ntp": {
                    "unicast-configuration": [{
                        "address": "192.168.3.1",
                        "type": "uc-peer",
                        "minpoll": 2
                    }],
                    "refclock-master": {
                        "master-stratum": 8
                    }
                }
            }
        })

    with test.step("Verify peer1 sees peer2 in sources"):
        until(lambda: ntp.server_has_peer(peer1, "192.168.3.2"), attempts=20)

    with test.step("Verify peer2 sees peer1 in sources"):
        until(lambda: ntp.server_has_peer(peer2, "192.168.3.1"), attempts=20)

    with test.step("Verify peer1 can reach peer2"):
        until(lambda: ntp.server_peer_reachable(peer1, "192.168.3.2"), attempts=60)

    with test.step("Verify peer2 can reach peer1"):
        until(lambda: ntp.server_peer_reachable(peer2, "192.168.3.1"), attempts=60)

    test.succeed()
