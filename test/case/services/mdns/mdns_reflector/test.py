#!/usr/bin/env python3
"""mDNS reflector

Verify the mDNS reflector functionality.  The reflector forwards mDNS
requests and responses between different network interfaces, allowing
service discovery across network segments.

We verify operation with two scenarios:

 1. Reflector enabled: mDNS traffic from host:data1 SHOULD be reflected
    to host:data2 and host:data3
 2. Reflector disabled: mDNS traffic from host:data1 should NOT be
    reflected to host:data2 and host:data3

"""
import infamy
from time import sleep

UNIQUE_MARKER = "infix-reflector-test"


def mdns_reflect_test():
    """Send mDNS from host:data1, check if it's reflected to host:data2/data3"""
    snif2 = infamy.Sniffer(hdata2, "port 5353")
    snif3 = infamy.Sniffer(hdata3, "port 5353")

    with snif2, snif3:
        # Send mDNS query for "infix-reflector-test.local" (raw DNS packet)
        hdata1.runsh(
            "echo -ne '\\x00\\x00\\x00\\x00\\x00\\x01\\x00\\x00\\x00\\x00\\x00\\x00"
            "\\x14infix-reflector-test\\x05local\\x00\\x00\\xff\\x00\\x01' | "
            "socat - UDP4-DATAGRAM:224.0.0.251:5353,bind=10.0.1.2"
        )
        sleep(5)

    out2 = snif2.packets()
    out3 = snif3.packets()

    return (UNIQUE_MARKER in out2, UNIQUE_MARKER in out3)


def disable_reflector():
    """Disable mDNS reflector"""
    dut.put_config_dict("infix-services", {
        "mdns": {
            "reflector": {
                "enabled": False
            }
        }
    })


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        dut = env.attach("dut", "mgmt")
        _, ddata1 = env.ltop.xlate("dut", "data1")
        _, ddata2 = env.ltop.xlate("dut", "data2")
        _, ddata3 = env.ltop.xlate("dut", "data3")
        _, hdata1 = env.ltop.xlate("host", "data1")
        _, hdata2 = env.ltop.xlate("host", "data2")
        _, hdata3 = env.ltop.xlate("host", "data3")

    with test.step("Configure device interfaces and enable mDNS"):
        dut.put_config_dicts(
            {
                "ietf-interfaces": {
                    "interfaces": {
                        "interface": [{
                            "name": ddata1,
                            "enabled": True,
                            "ipv4": {
                                "address": [{
                                    "ip": "10.0.1.1",
                                    "prefix-length": 24
                                }]
                            }
                        }, {
                            "name": ddata2,
                            "enabled": True,
                            "ipv4": {
                                "address": [{
                                    "ip": "10.0.2.1",
                                    "prefix-length": 24
                                }]
                            }
                        }, {
                            "name": ddata3,
                            "enabled": True,
                            "ipv4": {
                                "address": [{
                                    "ip": "10.0.3.1",
                                    "prefix-length": 24
                                }]
                            }
                        }]
                    }
                },
                "infix-services": {
                    "mdns": {
                        "enabled": True,
                        "reflector": {
                            "enabled": True
                        }
                    }
                }
            }
        )

    with infamy.IsolatedMacVlan(hdata1) as hdata1, \
         infamy.IsolatedMacVlan(hdata2) as hdata2, \
         infamy.IsolatedMacVlan(hdata3) as hdata3:
        hdata1.addip("10.0.1.2")
        hdata2.addip("10.0.2.2")
        hdata3.addip("10.0.3.2")

        with test.step("Verify mDNS from host:data1 is reflected to host:data2 and host:data3"):
            hdata2.must_receive("port 5353")
            hdata3.must_receive("port 5353")

            reflected = mdns_reflect_test()
            if reflected != (True, True):
                test.fail(f"Expected reflection (True, True), got {reflected}")

        with test.step("Verify mDNS from host:data1 is not reflected after disabling reflector"):
            disable_reflector()
            hdata2.must_receive("port 5353")
            hdata3.must_receive("port 5353")

            reflected = mdns_reflect_test()
            if reflected != (False, False):
                test.fail(f"Expected no reflection (False, False), got {reflected}")

    test.succeed()
