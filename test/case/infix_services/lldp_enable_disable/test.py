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

        lldp_link = env.ltop.get_link("host", "target", flt=lambda e: "ieee-mc" in e.get("requires", "").split())
        if not lldp_link:
            print("Skipping test: No link providing ieee-mc found in the topology.")
            test.skip()

        log_hport, _ = lldp_link
        _, phy_hport = env.ltop.xlate("host", log_hport)

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

        with infamy.IsolatedMacVlan(phy_hport) as netns:
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
