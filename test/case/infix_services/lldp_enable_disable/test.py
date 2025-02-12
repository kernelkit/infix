#!/usr/bin/env python3
"""LLDP enable disable

Verify that LLDP can be enabled and disabled. 
Operation and non-operation are confirmed using tcpdump.

"""
import time
import infamy

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, hdata = env.ltop.xlate("host", "data")

    with test.step("Enable target interface and disable LLDP"):
        target.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [
                        {
                            "name": target["data"],
                            "enabled": True
                        }
                    ]
                }
            }
        })

        target.put_config_dicts({
            "ieee802-dot1ab-lldp": {
                "lldp": {
                    "enabled": False
                }
            }
        })

    sniffing_period = 10

    def verify(enabled, sec):
        """Verify lldp traffic, or no traffic if lldp is disabled."""

        with infamy.IsolatedMacVlan(hdata) as netns:
            snif = infamy.Sniffer(netns, "ether proto 0x88cc")
            act = "enabling" if enabled else "disabling"

            target.put_config_dicts({
                "ieee802-dot1ab-lldp": {
                    "lldp": {
                        "enabled": enabled
                    }
                }
            })

            if enabled:
                target.put_config_dicts({
                    "ieee802-dot1ab-lldp": {
                        "lldp": {
                            "message-tx-interval": 1
                        }
                    }
                })

            with snif:
                print("host: collecting network traffic ...")
                print(f"target: {act} LLDP service ...")
                time.sleep(sec)

            return snif.output()

    with test.step("Enable LLDP"):
        rc = verify(True, sniffing_period)
        with test.step("Verify LLDP packets arrival on host:data"):
            if "LLDP" not in rc.stdout:
                test.fail()

    with test.step("Disable LLDP"):
        rc = verify(False, sniffing_period)
        with test.step("Verify no LLDP packets on host:data"):
            if "LLDP" in rc.stdout:
                test.fail()

    test.succeed()
