#!/usr/bin/env python3
#
# Verify that basic services like SSDP, mDNS and LLDP can be enabled and
# disabled.  We verify operation and non-operation by using tcpdump.
#

import time
import infamy
from infamy.ssdp import SsdpClient

def verify(enabled, sec):
    """Verify SSDP traffic, or no traffic in case not enabled"""
    _, hport = env.ltop.xlate("host", "data")

    with infamy.IsolatedMacVlan(hport) as netns:
        snif = infamy.Sniffer(netns, "port 1900")
        ssdp = SsdpClient(netns, retries=sec)

        netns.addip("10.0.0.1")
        netns.addroute("0.0.0.0/0", "10.0.0.1")

        with snif:
            target.put_config_dict("infix-services", {
                "ssdp": {
                    "enabled": enabled
                }
            })
            running = target.get_config_dict("/infix-services:ssdp")
            assert running["ssdp"]["enabled"] == enabled

            ssdp.start()
            time.sleep(sec)
            ssdp.stop()

        return snif.output()

with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env(infamy.std_topology("1x2"))
        target = env.attach("target", "mgmt")

    with test.step("Enable IPv4 address and disable service SSDP"):
        _, tport = env.ltop.xlate("target", "data")

        target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": tport,
                        "type": "infix-if-type:ethernet",
                        "enabled": True,
                        "ipv4": {
                            "address": [
                                {
                                    "ip": "10.0.0.10",
                                    "prefix-length": 24
                                }
                            ]
                        }
                    }
                ]
            }
        })
        target.put_config_dict("infix-services", {
            "ssdp": {
                "enabled": False
            }
        })
        cfg = target.get_config_dict("/infix-services:ssdp")
        assert not cfg["ssdp"]["enabled"]

    with test.step("Start SSDP sniffer and enable SSDP on target ..."):
        rc = verify(True, 3)
        print(rc.stdout)
        # breakpoint()
        if "10.0.0.10.1900 > 10.0.0.1" not in rc.stdout:
            test.fail()

    with test.step("Disable SSDP on target and start SSDP sniffer again ..."):
        rc = verify(False, 3)
        print(rc.stdout)
        if "10.0.0.10.1900 > 10.0.0.1" in rc.stdout:
            test.fail()

    test.succeed()
