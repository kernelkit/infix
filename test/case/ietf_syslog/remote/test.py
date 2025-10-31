#!/usr/bin/env python3
"""
Remote syslog

Verify logging to remote, acting as a remote, and RFC5424 log format.
"""
import infamy

def syslog_check(clientssh, serverssh):
    clientssh.runsh("logger -t test -m client -p security.notice TestMessage")
    return serverssh.runsh("tail -n10 /log/security |grep 'test - client - TestMessage'").returncode == 0


with infamy.Test() as test:
    with test.step("Set up topology and attach to client and server DUTs"):
        env = infamy.Env()
        client = env.attach("client", "mgmt")
        server = env.attach("server", "mgmt")
        clientssh = env.attach("client", "mgmt", "ssh")
        serverssh = env.attach("server", "mgmt", "ssh")

    with test.step("Configure client DUT as syslog client with server DUT as remote, and configure server DUT as syslog server"):
        _, client_link = env.ltop.xlate("client", "link")
        _, server_link = env.ltop.xlate("server", "link")

        client.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": client_link,
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
                ]}
            },
            "ietf-syslog": {
                "syslog": {
                    "actions": {
                        "file": {
                            "log-file": [{
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
                            }]
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
                                    }]
                                },
                                "infix-syslog:log-format": "rfc5424"
                            }
                            ]
                        }
                    }
                }
            }
        })


    server.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    {
                        "name": server_link,
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
        },
        "ietf-syslog": {
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
        }
    })

    with test.step("Send security:notice log message from client and verify reception of client log message, incl. sorting to /log/security on server"):
       infamy.until(lambda: syslog_check(clientssh, serverssh) == True)


    test.succeed()
