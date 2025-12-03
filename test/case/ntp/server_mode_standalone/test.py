#!/usr/bin/env python3
"""NTP server standalone mode test

Verify NTP server operating in standalone mode with only a local reference clock.

This test validates the basic standalone mode where the NTP server uses only
its local reference clock (stratum 8) to serve time to clients, without
syncing from any upstream sources.
"""

import infamy
from infamy import until
import infamy.ntp as ntp


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, data1 = env.ltop.xlate("target", "data1")
        _, hport1 = env.ltop.xlate("host", "data1")

    with test.step("Configure interface and NTP server"):

        target.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": data1,
                        "enabled": True,
                        "ipv4": {
                            "address": [{
                                "ip": "192.168.1.1",
                                "prefix-length": 24
                            }]
                        }
                    }]
                }
            },
            "ietf-ntp": {
                "ntp": {
                    "refclock-master": {
                        "master-stratum": 8
                    }
                }
            }
        })

    with infamy.IsolatedMacVlan(hport1) as ns1:
        ns1.addip("192.168.1.2")

        with test.step("Verify network connectivity with NTP server"):
            ns1.must_reach("192.168.1.1")

        with test.step("Query time from NTP server"):
            until(lambda: ntp.server_query(ns1, "192.168.1.1"), attempts=20)

        with test.step("Verify NTP server statistics"):
            until(lambda: ntp.server_has_received_packets(target), attempts=20)

    test.succeed()
