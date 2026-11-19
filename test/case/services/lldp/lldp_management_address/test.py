#!/usr/bin/env python3
"""LLDP management address

Verify that LLDP advertises all addresses of the device as management
addresses, and that management-interface limits them to the addresses
of the listed interfaces.

"""
import re
import time
import infamy
from infamy.util import until

DATA_ADDR = "10.0.0.10"
DUMMY_ADDR = "192.168.50.1"


def advertised(netns):
    """Management addresses in the last LLDP frame seen in netns"""
    snif = infamy.Sniffer(netns, "ether proto 0x88cc")
    with snif:
        time.sleep(2)

    frames = re.split(r"^\S.*LLDP, length", snif.tcpdump("-v"), flags=re.M)
    return set(re.findall(r"AFI IPv4 \(1\): (\S+)", frames[-1]))


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        tdata = target["data"]
        hdata = env["host"]["data"]

    with test.step("Configure 10.0.0.10/24 on target:data and 192.168.50.1/24 on dummy0"):
        target.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": tdata,
                        "enabled": True,
                        "ipv4": {
                            "address": [{
                                "ip": DATA_ADDR,
                                "prefix-length": 24
                            }]
                        }
                    }, {
                        "name": "dummy0",
                        "type": "infix-if-type:dummy",
                        "enabled": True,
                        "ipv4": {
                            "address": [{
                                "ip": DUMMY_ADDR,
                                "prefix-length": 24
                            }]
                        }
                    }]
                }
            }
        })

    with test.step("Enable LLDP"):
        target.put_config_dicts({
            "ieee802-dot1ab-lldp": {
                "lldp": {
                    "enabled": True,
                    "message-tx-interval": 1
                }
            }
        })

    with infamy.IsolatedMacVlan(hdata) as netns:
        def all_advertised():
            return {DATA_ADDR, DUMMY_ADDR} <= advertised(netns)

        with test.step("Verify 10.0.0.10 and 192.168.50.1 are advertised on host:data"):
            until(all_advertised, attempts=10, interval=0)

        with test.step("Set management-interface to target:data"):
            target.put_config_dicts({
                "ieee802-dot1ab-lldp": {
                    "lldp": {
                        "management-interface": [tdata]
                    }
                }
            })

        with test.step("Verify only 10.0.0.10 is advertised on host:data"):
            until(lambda: advertised(netns) == {DATA_ADDR}, attempts=10, interval=0)

        with test.step("Remove management-interface"):
            target.put_config_dicts({}, delete=[
                f"/ieee802-dot1ab-lldp:lldp/infix-lldp:management-interface[.='{tdata}']"
            ])

        with test.step("Verify 10.0.0.10 and 192.168.50.1 are advertised on host:data"):
            until(all_advertised, attempts=10, interval=0)

    test.succeed()
