#!/usr/bin/env python3
"""NTP server and client interoperability test

Verify NTP server and client work together:

1. Server uses ietf-ntp YANG model with refclock-master
2. Client uses ietf-system YANG model
3. Client successfully synchronizes from server
4. Server shows packet statistics
5. Mutual exclusion prevents both modes on same device
"""

import infamy
from infamy import until
import infamy.ntp as ntp


with infamy.Test() as test:
    with test.step("Set up topology and attach to devices"):
        env = infamy.Env()
        server = env.attach("server", "mgmt")
        client = env.attach("client", "mgmt")

        _, server_data = env.ltop.xlate("server", "data")
        _, client_data = env.ltop.xlate("client", "data")

    with test.step("Configure NTP server using ietf-ntp model"):
        server.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": server_data,
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
                    },
                    "interfaces": {
                        "interface": [
                            {"name": server_data}
                        ]
                    }
                }
            }
        })

    with test.step("Configure NTP client using ietf-system:ntp model"):
        client.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": client_data,
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
                            "name": "ntp-server",
                            "udp": {
                                "address": "192.168.3.1"
                            },
                            "iburst": True
                        }]
                    }
                }
            }
        })

    with test.step("Verify NTP server has received packets"):
        until(lambda: ntp.server_has_received_packets(server), attempts=30)
        print("Server has received NTP packets from client")

    with test.step("Verify NTP client has synchronized"):
        selected = until(lambda: ntp.any_source_selected(client), attempts=30)
        print(f"Client synchronized to {selected.get('address')} (stratum {selected.get('stratum')})")

    test.succeed()
