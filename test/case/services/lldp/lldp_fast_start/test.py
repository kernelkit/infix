#!/usr/bin/env python3
"""LLDP fast start

Verify that a new LLDP neighbor is answered at once with four frames,
sent message-fast-tx seconds apart.

"""
import re
import time
import infamy

from scapy.all import Ether, sendp
from scapy.contrib.lldp import (
    LLDPDU, LLDPDUChassisID, LLDPDUPortID, LLDPDUTimeToLive, LLDPDUEndOfLLDPDU
)

HOST_MAC = "02:01:02:03:04:05"
FROM_TARGET = f"ether proto 0x88cc and not ether src {HOST_MAC}"


def send_lldp_packet():
    frame = Ether(src=HOST_MAC, dst="01:80:C2:00:00:0E", type=0x88cc)
    frame /= LLDPDU()
    frame /= LLDPDUChassisID(subtype=7, id="Chassis ID 007")
    frame /= LLDPDUPortID(subtype=5, id="iface")
    frame /= LLDPDUTimeToLive(ttl=120) / LLDPDUEndOfLLDPDU()
    sendp(frame, iface="iface", verbose=False)


def timestamps(snif):
    """Arrival time, in seconds, of every LLDP frame in the capture"""
    return [float(ts) for ts in
            re.findall(r"^(\d+\.\d+) LLDP", snif.tcpdump("-tt"), flags=re.M)]


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        hdata = env["host"]["data"]

    with infamy.IsolatedMacVlan(hdata) as netns:
        snif = infamy.Sniffer(netns, FROM_TARGET)
        with snif:
            with test.step("Enable LLDP with message-tx-interval 30 and message-fast-tx 2"):
                target.put_config_dicts({
                    "ietf-interfaces": {
                        "interfaces": {
                            "interface": [{
                                "name": target["data"],
                                "enabled": True
                            }]
                        }
                    },
                    "ieee802-dot1ab-lldp": {
                        "lldp": {
                            "enabled": True,
                            "message-tx-interval": 30,
                            "message-fast-tx": 2
                        }
                    }
                })
                time.sleep(5)

        with test.step("Verify LLDP frame on host:data"):
            if not timestamps(snif):
                test.fail()

        snif = infamy.Sniffer(netns, FROM_TARGET)
        with snif:
            with test.step("Send LLDP frame from host:data"):
                netns.call(send_lldp_packet)
                time.sleep(8)

        with test.step("Verify four LLDP frames, 2 seconds apart, on host:data"):
            seen = timestamps(snif)
            gaps = [round(b - a, 1) for a, b in zip(seen, seen[1:])]
            print(f"host: LLDP frames at {seen}, {gaps} seconds apart")
            if len(seen) != 4:
                test.fail()
            if not all(1.5 < gap < 2.5 for gap in gaps):
                test.fail()

    test.succeed()
