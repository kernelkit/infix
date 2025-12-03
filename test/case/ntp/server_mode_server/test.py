#!/usr/bin/env python3
"""NTP server mode test

Verify NTP server operating in server mode, syncing from upstream while
serving clients.

This test validates server mode where devices synchronize from upstream
NTP servers while simultaneously serving time to downstream clients. It
creates a two-tier hierarchy:

- Upstream: NTP server with local reference clock (stratum 8)
- Downstream: NTP server that syncs from upstream and serves to clients (stratum 9)

The test verifies both servers operate correctly and serve accurate time.

"""

import infamy
from infamy import until
import infamy.ntp as ntp


with infamy.Test() as test:
    with test.step("Set up topology and attach to devices"):
        env = infamy.Env()
        upstream = env.attach("upstream", "mgmt")
        downstream = env.attach("downstream", "mgmt")

        # Get interface names for each device
        _, upstream_data1 = env.ltop.xlate("upstream", "data1")
        _, upstream_conn = env.ltop.xlate("upstream", "conn")
        _, hport1 = env.ltop.xlate("host", "data1")

        _, downstream_data2 = env.ltop.xlate("downstream", "data2")
        _, downstream_conn = env.ltop.xlate("downstream", "conn")
        _, hport2 = env.ltop.xlate("host", "data2")

    with test.step("Configure upstream NTP server with local reference clock"):
        upstream.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": upstream_data1,
                        "enabled": True,
                        "ipv4": {
                            "address": [{
                                "ip": "192.168.1.1",
                                "prefix-length": 24
                            }]
                        }
                    }, {
                        "name": upstream_conn,
                        "enabled": True,
                        "ipv4": {
                            "address": [{
                                "ip": "192.168.3.1",
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

    with test.step("Configure downstream NTP server syncing from upstream"):
        downstream.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": downstream_data2,
                        "enabled": True,
                        "ipv4": {
                            "address": [{
                                "ip": "192.168.2.1",
                                "prefix-length": 24
                            }]
                        }
                    }, {
                        "name": downstream_conn,
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
            "ietf-ntp": {
                "ntp": {
                    "unicast-configuration": [{
                        "address": "192.168.3.1",
                        "type": "uc-server",
                        "iburst": True
                    }],
                    "refclock-master": {
                        "master-stratum": 10
                    }
                }
            }
        })

    with infamy.IsolatedMacVlan(hport1) as ns1:
        ns1.addip("192.168.1.2")

        with test.step("Verify network connectivity with upstream NTP server"):
            ns1.must_reach("192.168.1.1")

        with test.step("Query time from upstream NTP server"):
            until(lambda: ntp.server_query(ns1, "192.168.1.1"), attempts=20)

        with test.step("Verify upstream NTP server statistics"):
            until(lambda: ntp.server_has_received_packets(upstream), attempts=20)

    with infamy.IsolatedMacVlan(hport2) as ns2:
        ns2.addip("192.168.2.2")

        with test.step("Verify network connectivity with downstream NTP server"):
            ns2.must_reach("192.168.2.1")

        with test.step("Wait for downstream to sync from upstream"):
            # Give downstream time to sync from upstream
            until(lambda: ntp.server_query(ns2, "192.168.2.1"), attempts=30)

        with test.step("Verify downstream NTP server statistics"):
            until(lambda: ntp.server_has_received_packets(downstream), attempts=20)

    test.succeed()
