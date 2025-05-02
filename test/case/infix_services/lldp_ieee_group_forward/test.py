#!/usr/bin/env python3

"""LLDP IEEE Group Forward

Verify that LLDP packets can be flooded to all ports on a bridge.
Operation and non-operation are confirmed using tcpdump.
"""

import time
import threading
import infamy

from scapy.all import Ether, sendp
from scapy.contrib.lldp import (
    LLDPDU, LLDPDUChassisID, LLDPDUPortID, 
    LLDPDUTimeToLive, LLDPDUEndOfLLDPDU
)

SNIFFING_DURATION = 3

LLDP_INTERVAL = 1
LLDP_TTL = 3
LLDP_MULTICAST_MAC = "01:80:C2:00:00:0E"
LLDP_ETHERTYPE = 0x88cc

CHASSIS_ID = "Chassis-Test"
SRC_MAC = "02:01:02:03:04:05"

BRIDGE = "br0"

def send_lldp_packet(iface):
    eth = Ether(src=SRC_MAC, dst=LLDP_MULTICAST_MAC, type=LLDP_ETHERTYPE)
    lldpdu = eth / LLDPDU()
    lldpdu /= LLDPDUChassisID(subtype=7, id=CHASSIS_ID)
    lldpdu /= LLDPDUPortID(subtype=5, id=iface)
    lldpdu /= LLDPDUTimeToLive(ttl=LLDP_TTL)
    lldpdu /= LLDPDUEndOfLLDPDU()
    sendp(lldpdu, iface=iface, verbose=False)

def send_lldp_packets_continuously(iface, stop_event):
    time.sleep(LLDP_INTERVAL/2)
    while not stop_event.is_set():
        send_lldp_packet(iface)
        time.sleep(LLDP_INTERVAL)

def capture_traffic(iface):
    with infamy.IsolatedMacVlan(iface) as netns:
        sniffer = infamy.Sniffer(netns, f"ether proto {LLDP_ETHERTYPE}")
        with sniffer:
            time.sleep(SNIFFING_DURATION)
        return sniffer.output()

def start_lldp_sender_thread(iface):
    stop = threading.Event()
    thread = threading.Thread(
        target=send_lldp_packets_continuously,
        args=(iface, stop)
    )
    thread.start()
    return stop, thread

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, hdata1 = env.ltop.xlate("host", "data1")
        _, hdata2 = env.ltop.xlate("host", "data2")

    with test.step("Configure interfaces and disable LLDP daemon"):
        target.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [
                        {
                            "name": BRIDGE,
                            "type": "infix-if-type:bridge",
                            "enabled": True,
                        },
                        {
                            "name": target["data1"],
                            "enabled": True,
                            "infix-interfaces:bridge-port": {
                                "bridge": "br0"
                            }
                        },
                        {
                            "name": target["data2"],
                            "enabled": True,
                            "infix-interfaces:bridge-port": {
                                "bridge": "br0"
                            }
                        }
                    ]
                }
            },
            "ieee802-dot1ab-lldp": {
                "lldp": {
                    "enabled": False
                }
            }
        })

    stop_event, sender_thread = start_lldp_sender_thread(hdata1)

    try:
        with test.step("Verify LLDP absence on host:data2"):
            captured_traffic = capture_traffic(hdata2)
            if "LLDP" in captured_traffic.stdout:
                test.fail()

    finally:
        stop_event.set()
        sender_thread.join()

    with test.step("Enable LLDP flooding by setting group forward"):
        target.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [
                        {
                            "name": BRIDGE,
                            "infix-interfaces:bridge": {
                                "ieee-group-forward": [
                                    "lldp"
                                ]
                            }
                        }
                    ]
                }
            }
        })

    stop_event, sender_thread = start_lldp_sender_thread(hdata1)

    try:
        with test.step("Verify LLDP arrival on host:data2"):
            captured_traffic = capture_traffic(hdata2)
            if "LLDP" not in captured_traffic.stdout:
                test.fail()

    finally:
        stop_event.set()
        sender_thread.join()

    test.succeed()
