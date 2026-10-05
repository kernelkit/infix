#!/usr/bin/env python3
"""
Basic NTP client test

Verify NTP client with multiple servers, ensure one get selected.
"""

import infamy
import infamy.ntp_server as ntp_server
import infamy.ntp as ntp
import infamy.util as util


def config_target(dut, data1, data2, data3):
    dut.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": data1,
                    "enabled": True,
                    "ipv4": {
                        "address": [{
                            "ip": "192.168.1.2",
                            "prefix-length": 24
                            }]
                    }
                }, {
                    "name": data2,
                    "enabled": True,
                    "ipv4": {
                        "address": [{
                            "ip": "192.168.2.2",
                            "prefix-length": 24
                        }]
                    }
                }, {
                    "name": data3,
                    "enabled": True,
                    "ipv4": {
                        "address": [{

                            "ip": "192.168.3.2",
                            "prefix-length": 24
                        }]
                    }
                }]
            }
        },
        "ietf-system": {
            "system": {
                "ntp": {
                    "enabled": True,
                    "server": [{
                        "name": "Server1",
                        "udp": {
                            "address": "192.168.1.1"
                        },
                        "iburst": True
                    }, {
                        "name": "Server2",
                        "udp": {
                            "address": "192.168.2.1"
                        },
                        "iburst": True
                    }, {
                        "name": "Server3",
                        "udp": {
                            "address": "192.168.3.1"
                        },
                        "iburst": True
                    }]
                }
            }
        }
    })


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

    with test.step("Configure NTP client on 'target'"):
        data1 = target["data1"]
        data2 = target["data2"]
        data3 = target["data3"]

        config_target(target, data1, data2, data3)

    hport1 = env["host"]["data1"]
    hport2 = env["host"]["data2"]
    hport3 = env["host"]["data3"]

    with infamy.IsolatedMacVlan(hport1) as ns1, \
         infamy.IsolatedMacVlan(hport2) as ns2, \
         infamy.IsolatedMacVlan(hport3) as ns3:
        ns1.addip("192.168.1.1")
        ns2.addip("192.168.2.1")
        ns3.addip("192.168.3.1")

        with ntp_server.Server(ns1) as ntp1, \
             ntp_server.Server(ns2) as ntp2, \
             ntp_server.Server(ns3) as ntp3:
            with test.step("Verify three sources exist in NTP client on 'target'"):
                util.until(lambda: ntp.number_of_sources(target) == 3, attempts=200)
            with test.step("Verify one source is in 'selected' state on 'target'"):
                util.until(lambda: ntp.any_source_selected(target), attempts=200)

    test.succeed()
