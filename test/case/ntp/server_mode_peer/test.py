#!/usr/bin/env python3
"""NTP peer mode test

Verify NTP server operating in peer mode with bidirectional
synchronization.

This test validates peer mode where two NTP servers synchronize with
each other bidirectionally. Each server acts as both client and server
to the other:

- peer1: Stratum 8 local clock, peered with peer2
- peer2: Stratum 8 local clock, peered with peer1

The test verifies mutual synchronization and clock selection between
peers.  When both peers have the same stratum, NTP's clock selection
algorithm uses the Reference ID (derived from the IP address) as its
tie-breaker.  The peer with the numerically lower IP address will be
selected as sync source by the other peer.

"""

import infamy
from infamy import until
import infamy.ntp as ntp


def configure_peer(dut, iface, addr, peer, stratum=8):
    """Configure NTP peer with interface and peer relationship."""
    dut.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": iface,
                    "enabled": True,
                    "ipv4": {
                        "address": [{
                            "ip": addr,
                            "prefix-length": 24
                        }]
                    }
                }]
            }
        },
        "ietf-ntp": {
            "ntp": {
                "unicast-configuration": [{
                    "address": peer,
                    "type": "uc-peer",
                    "minpoll": 2
                }],
                "refclock-master": {
                    "master-stratum": stratum
                }
            }
        }
    })


def has_selected_peer(peers):
    """Check if any peer has selected another as sync source."""
    for target, _, _, _, peer in peers:
        try:
            data = target.get_data("/ietf-ntp:ntp/associations")
            if not data:
                continue

            assoc = data.get("ntp", {}).get("associations", {}).get("association", [])
            if not assoc:
                continue

            for assoc in assoc:
                if assoc.get("prefer", False) and assoc.get("address") == peer:
                    return True
        except Exception:
            continue
    return False


with infamy.Test() as test:
    with test.step("Set up topology and attach to devices"):
        env = infamy.Env()
        peer1 = env.attach("peer1", "mgmt")
        peer2 = env.attach("peer2", "mgmt")

        _, if1 = env.ltop.xlate("peer1", "data")
        _, if2 = env.ltop.xlate("peer2", "data")

        duts = [
            (peer1, if1, "peer1", "192.168.3.1", "192.168.3.2"),
            (peer2, if2, "peer2", "192.168.3.2", "192.168.3.1")
        ]

    with test.step("Configure DUTs with bidirectional peer relationships"):
        for dut, interface, name, local_ip, peer_ip in duts:
            configure_peer(dut, interface, local_ip, peer_ip)
            print(f"Configured {name}: {local_ip} peered with {peer_ip}")

    with test.step("Verify peers see each other in associations"):
        for dut, _, name, _, peer_ip in duts:
            until(lambda t=dut, p=peer_ip: ntp.server_has_peer(t, p), attempts=20)
            print(f"{name} sees {peer_ip} in associations")

    with test.step("Verify peers can reach each other"):
        for dut, _, name, _, peer_ip in duts:
            until(lambda t=dut, p=peer_ip: ntp.server_peer_reachable(t, p), attempts=60)
            print(f"{name} can reach {peer_ip}")

    with test.step("Wait for one peer to select the other as sync source"):
        until(lambda: has_selected_peer(duts), attempts=120)

    test.succeed()
