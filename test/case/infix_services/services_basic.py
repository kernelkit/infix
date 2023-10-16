#!/usr/bin/env python3
#
# Verify that basic services like mDNS and LLDP can be enabled and
# disabled.  We verify operation and non-operation by using tcpdump.
#
# XXX: with socat in the Docker container we could speed up the LLDP
#      detection considerably by sending a probe:
#
# echo -ne "\x01\x80\xc2\x00\x00\x0e\x01\x80\xc2\x00\x00\x0e\x88\xcc\x02\x07\x04\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00" | socat - UDP4-DATAGRAM:255.255.255.255:7010,broadcast
#

import time
import infamy

def verify(enabled, sec):
    """Verify service traffic, or no traffic in case service not enabled"""
    _, hport = env.ltop.xlate("host", "data")

    with infamy.IsolatedMacVlan(hport) as netns:
        snif = infamy.Sniffer(netns, "port 5353 or ether proto 0x88cc")

        netns.addip("10.0.0.1")
        netns.addroute("0.0.0.0/0", "10.0.0.1")

        # Put service enable/disable before starting tcpdump, because
        # LLDP lingers and will send a final shutdown message that
        # otherwise would get in the capture for disable.
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

        with snif:
            time.sleep(sec)

        return snif.output()

with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env(infamy.std_topology("1x2"))
        target = env.attach("target", "mgmt")

    with test.step("Set static IPv4 address and disable services"):
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
            "mdns": {
                "enabled": False
            }
        })
        target.put_config_dict("ieee802-dot1ab-lldp", {
            "lldp": {
                "enabled": False
            }
        })

    with test.step("Start sniffer and enable services on target ..."):
        rc = verify(True, 25)
        print(rc.stdout)
        # breakpoint()
        if "10.0.0.10.5353" not in rc.stdout:
            test.fail()
        if "LLDP" not in rc.stdout:
            test.fail()

    with test.step("Disable services on target, verify they're not running anymore ..."):
        rc = verify(False, 20)
        print(rc.stdout)
        if "10.0.0.10.5353" in rc.stdout:
            test.fail()
        if "LLDP" in rc.stdout:
            test.fail()

    test.succeed()
