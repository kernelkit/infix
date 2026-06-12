#!/usr/bin/env python3
"""LLDP admin status

Verify that LLDP admin status is set properly by lldpd

"""
import time
import infamy
import infamy.lldp as lldp
from infamy.util import until

from scapy.all import Ether, sendp
from scapy.contrib.lldp import (
    LLDPDU, LLDPDUChassisID, LLDPDUPortID, LLDPDUTimeToLive, LLDPDUEndOfLLDPDU
)

def capture_traffic(iface, sec):
    with infamy.IsolatedMacVlan(iface) as netns:
        sniffer = infamy.Sniffer(netns, "ether proto 0x88cc")               
        with sniffer:
            print("Capturing network traffic ...")
            time.sleep(sec)
        return sniffer.output()
        
def send_lldp_packet(iface, chassis_id, chassis_id_subtype, ttl=3):
    eth = Ether(src="02:01:02:03:04:05", dst="01:80:C2:00:00:0E", type=0x88cc)
    lldpdu = eth / LLDPDU() 
    lldpdu /= LLDPDUChassisID(subtype=chassis_id_subtype, id=chassis_id)
    lldpdu /= LLDPDUPortID(subtype=5, id=iface) 
    lldpdu /= LLDPDUTimeToLive(ttl=ttl) / LLDPDUEndOfLLDPDU()
    sendp(lldpdu, iface=iface, verbose=False)

def verify_admin_status(test, target, port, admin_status, local_capture, remote_detect):
    target.put_config_dicts({
        "ieee802-dot1ab-lldp": {
            "lldp": {
                "port": [{
                    "name": target["data"],
                    "dest-mac-address": "01:80:C2:00:00:0E",
                    "admin-status": admin_status
                }]
            }
        }
    })

    rc = capture_traffic(port, 5)

    if local_capture and "LLDP" not in rc.stdout:
        test.fail()
    if not local_capture and "LLDP" in rc.stdout:
        test.fail()
    
    if remote_detect:
        # A single ttl=3 LLDP frame expires before the neighbor reaches
        # operational (lldpd -> yangerd -> statd), so resend on every poll
        # until the target registers it.
        def detected():
            send_lldp_packet(port, "Chassis ID 007", 7)
            return bool(lldp.get_remote_systems_data(target, target["data"]))
        until(detected, attempts=30)
    else:
        # rx disabled: a received frame must not register, and any neighbor
        # from a previous step must age out (ttl).  Send one frame, then
        # wait until no neighbor is present.
        send_lldp_packet(port, "Chassis ID 007", 7)
        until(lambda: not lldp.get_remote_systems_data(target, target["data"]),
              attempts=30)

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, hdata = env.ltop.xlate("host", "data")
    
    with test.step("Enable target interface and enable LLDP"):
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
                    "message-tx-interval": 1
                }
            }
        })
    
    with test.step("Verify admin-status: 'rx-only'"):
        verify_admin_status(test, target, hdata, "rx-only", False, True)
    with test.step("Verify admin-status: 'tx-only'"):
        verify_admin_status(test, target, hdata, "tx-only", True, False)
    with test.step("Verify admin-status: 'disabled'"):
        verify_admin_status(test, target, hdata, "disabled", False, False)
    with test.step("Verify admin-status: 'tx-and-rx'"): 
        verify_admin_status(test, target, hdata, "tx-and-rx", True, True)
    
    test.succeed()
