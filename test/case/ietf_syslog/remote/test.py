#!/usr/bin/env python3
"""
Remote syslog

Verify logging to remote, acting as a remote, and RFC5424 log format.
"""
import infamy

with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env()
        client = env.attach("target1", "mgmt")
        server = env.attach("target2", "mgmt")
        clientssh = env.attach("target1", "mgmt", "ssh")
        serverssh = env.attach("target2", "mgmt", "ssh")

    with test.step("Topology setup"):
        _, client_e0 = env.ltop.xlate("target1", "data")
        _, client_e1 = env.ltop.xlate("target1", "target2")
        _, server_e0 = env.ltop.xlate("target2", "target1")

        client.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": client_e1,
                        "enabled": True,
                        "ipv4": {
                            "address": [
                                {
                                    "ip": "10.0.0.2",
                                    "prefix-length": 24,
                                }
                            ]
                        }
                    }
                ]
            }
        })

        server.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": server_e0,
                        "type": "infix-if-type:bridge",
                        "enabled": True,
                        "ipv4": {
                            "address": [
                                {
                                    "ip": "10.0.0.1",
                                    "prefix-length": 24,
                                }
                            ]
                        }
                    }
                ]
            }
        })

    with test.step("Syslog setup"):
        client.put_config_dict("ietf-syslog", {
            "syslog": {
                "actions": {
                    "file": {
                        "log-file": [
                            {
                                "name": "file:security",
                                "facility-filter": {
                                    "facility-list": [
                                        {
                                            "facility": "auth",
                                            "severity": "all"
                                        },
                                        {
                                            "facility": "audit",
                                            "severity": "all"
                                        }
                                    ]
                                },
                                "infix-syslog:log-format": "rfc5424"
                            }
                        ]
                    },
                    "remote": {
                        "destination": [
                            {
                                "name": "server",
                                "udp": {
                                    "address": "10.0.0.1"
                                },
                                "facility-filter": {
                                    "facility-list": [
                                        {
                                            "facility": "audit",
                                            "severity": "all"
                                        },
                                        {
                                            "facility": "auth",
                                            "severity": "all"
                                        }
                                    ]
                                },
                                "infix-syslog:log-format": "rfc5424"
                            }
                        ]
                    }
                }
            }
        })

        server.put_config_dict("ietf-syslog", {
            "syslog": {
                "actions": {
                    "file": {
                        "log-file": [
                            {
                                "name": "file:security",
                                "facility-filter": {
                                    "facility-list": [
                                        {
                                            "facility": "auth",
                                            "severity": "all"
                                        },
                                        {
                                            "facility": "audit",
                                            "severity": "all"
                                        }
                                    ]
                                },
                                "infix-syslog:log-format": "rfc5424"
                            }
                        ]
                    }
                },
                "infix-syslog:server": {
                    "enabled": True,
                    "listen": {
                        "udp": [
                            {
                                "port": 514,
                                "address": "10.0.0.1"
                            }
                        ]
                    }
                }
            }
        })

    with test.step("Verify logging from client to server"):
        clientssh.runsh("logger -t test -m client -p security.notice Hej")
        infamy.until(lambda: serverssh.runsh(
            "grep 'test - client - Hej' /log/security").returncode == 0)

    test.succeed()
