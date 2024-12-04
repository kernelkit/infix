#!/usr/bin/env python3
"""Services basic

Verify that basic services, such as mDNS and LLDP, can be enabled and
disabled. Operation and non-operation are confirmed using tcpdump.

"""
import time
import infamy


def toggle(updown):
    """Toggle port down/up to kick services"""
    _, port = env.ltop.xlate("target", "data")
    act = "UP" if updown else "DOWN"

    print(f"target: taking interface {port} {act} ...")
    target.put_config_dict("ietf-interfaces", {
        "interfaces": {
            "interface": [
                {
                    "name": port,
                    "enabled": updown
                }
            ]
        }
    })

def verify(enabled, sec):
    """Verify service traffic, or no traffic in case service not enabled"""
    _, hport = env.ltop.xlate("host", "data")

    with infamy.IsolatedMacVlan(hport) as netns:
        snif = infamy.Sniffer(netns, "port 5353 or ether proto 0x88cc")
        act = "enabling" if enabled else "disabling"

        netns.addip("10.0.0.1")
        netns.addroute("0.0.0.0/0", "10.0.0.1")

        # Prevent old-state traffic from being captured before reconf
        toggle(False)

        with snif:
            print("host: collecting network traffic ...")

            # Put service enable/disable before starting tcpdump,
            # because LLDP lingers and will send a final shutdown
            # message that otherwise would get in the capture for
            # disable.
            print(f"target: {act} LLDP and mDNS services ...")
            target.put_config_dict("infix-services", {
                "mdns": {
                    "enabled": enabled
                }
            })

            target.put_config_dict("ieee802-dot1ab-lldp", {
                    "lldp": {
                        "enabled": enabled
                    }
                })

            toggle(True)
            time.sleep(sec)

        return snif.output()


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

    with test.step("Set IPv4 address 10.0.0.10/24 on target:data and disable mDNS and LLDP"):
        _, tport = env.ltop.xlate("target", "data")

        target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": tport,
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
            "mdns": {
                "enabled": False
            }
        })
        target.put_config_dict("ieee802-dot1ab-lldp", {
            "lldp": {
                "enabled": False
            }
        })
    with test.step("Enable mDNS and LLDP and toggle target:data DOWN and UP to trigger service"):
        rc = verify(True, 10)
        print(rc.stdout)
        with test.step("Verify on host:data there are packets from 10.0.0.10:5354 (mDNS)"):
            if "10.0.0.10.5353" not in rc.stdout:
                test.fail()
        with test.step("Verify on host:data there are LLDP packets sent from 10.0.0.10"):
            if "LLDP" not in rc.stdout:
                test.fail()

    with test.step("Disable mDNS and LLDP"):
        rc = verify(False, 10)
        print(rc.stdout)
        with test.step("Verify on host:data there are no packets from 10.0.0.10:5354 (mDNS)"):
            if "10.0.0.10.5353" in rc.stdout:
                test.fail()
        with test.step("Verify on host:data there are no LLDP packets sent from 10.0.0.10"):
            if "LLDP" in rc.stdout:
                test.fail()

    test.succeed()
