#!/usr/bin/env python3
"""mDNS enable disable

Verify that mDNS can be enabled and disabled. 
Operation and non-operation are confirmed using tcpdump.

"""

import time
import infamy

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, hport = env.ltop.xlate("host", "data")

    with test.step("Set IPv4 address 10.0.0.10/24 on target:data and disable mDNS"):
        target.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [
                        {
                            "name": target["data"],
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
            }
        })

        target.put_config_dicts({
            "infix-services": {
                "mdns": {
                    "enabled": False
                }
            }
        })

    sniffing_period = 10
    mdns_ip = "10.0.0.10.5353"

    def verify(enabled, sec):
        """Verify mDNS traffic, or no traffic if mDNS is disabled."""

        with infamy.IsolatedMacVlan(hport) as netns:
            snif = infamy.Sniffer(netns, "port 5353")
            act = "enabling" if enabled else "disabling"

            netns.addip("10.0.0.1")
            netns.addroute("0.0.0.0/0", "10.0.0.1")

            target.put_config_dicts({
                "infix-services": {
                    "mdns": {
                        "enabled": enabled
                    }
                }
            })

            with snif:
                print("host: collecting network traffic ...")
                print(f"target: {act} mDNS service ...")
                time.sleep(sec)

            return snif.output()

    with test.step("Enable mDNS"):
        rc = verify(True, sniffing_period)
        with test.step("Verify on host:data there are packets from 10.0.0.10:5354 (mDNS)"):
            if mdns_ip not in rc.stdout:
                test.fail()

    with test.step("Disable mDNS"):
        rc = verify(False, sniffing_period)
        with test.step("Verify on host:data there are no packets from 10.0.0.10:5354 (mDNS)"):
            if mdns_ip in rc.stdout:
                test.fail()

    test.succeed()
