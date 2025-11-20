#!/usr/bin/env python3
"""Syslog Hostname Filtering

Verify hostname-filter feature: filtering incoming syslog messages based
on the hostname.  Tests log sink scenario with multiple remote clients.

"""

import infamy
import time

TEST_MESSAGES = [
    ("router1", "Message from router1"),
    ("router1", "Another from router1"),
    ("router2", "Message from router2"),
    ("other",   "Message from a different host"),
]

with infamy.Test() as test:
    with test.step("Set up topology and attach to DUTs"):
        env = infamy.Env()
        client = env.attach("client", "mgmt")
        server = env.attach("server", "mgmt")
        clientssh = env.attach("client", "mgmt", "ssh")
        serverssh = env.attach("server", "mgmt", "ssh")

    with test.step("Clean up old log files on server"):
        serverssh.runsh("sudo rm -f /var/log/{router1,router2,all-hosts}")

    with test.step("Configure server as syslog sink with hostname filtering"):
        _, server_link = env.ltop.xlate("server", "link")

        server.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": server_link,
                        "enabled": True,
                        "ipv4": {
                            "address": [{
                                "ip": "10.0.0.1",
                                "prefix-length": 24,
                            }]
                        }
                    }]
                }
            },
            "ietf-syslog": {
                "syslog": {
                    "actions": {
                        "file": {
                            "log-file": [{
                                "name": "file:router1",
                                "infix-syslog:hostname-filter": ["router1"],
                                "facility-filter": {
                                    "facility-list": [{
                                        "facility": "all",
                                        "severity": "info"
                                    }]
                                }
                            }, {
                                "name": "file:router2",
                                "infix-syslog:hostname-filter": ["router2"],
                                "facility-filter": {
                                    "facility-list": [{
                                        "facility": "all",
                                        "severity": "info"
                                    }]
                                }
                            }, {
                                "name": "file:all-hosts",
                                "facility-filter": {
                                    "facility-list": [{
                                        "facility": "all",
                                        "severity": "info"
                                    }]
                                }
                            }]
                        }
                    },
                    "infix-syslog:server": {
                        "enabled": True,
                        "listen": {
                            "udp": [{
                                "port": 514,
                                "address": "10.0.0.1"
                            }]
                        }
                    }
                }
            }
        })

    with test.step("Configure client to forward logs to server"):
        _, client_link = env.ltop.xlate("client", "link")

        client.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": client_link,
                        "enabled": True,
                        "ipv4": {
                            "address": [{
                                "ip": "10.0.0.2",
                                "prefix-length": 24,
                            }]
                        }
                    }]
                }
            },
            "ietf-syslog": {
                "syslog": {
                    "actions": {
                        "remote": {
                            "destination": [{
                                "name": "server",
                                "udp": {
                                    "address": "10.0.0.1"
                                },
                                "facility-filter": {
                                    "facility-list": [{
                                        "facility": "all",
                                        "severity": "info"
                                    }]
                                }
                            }]
                        }
                    }
                }
            }
        })

        time.sleep(2)

    with test.step("Send log messages with different hostnames"):
        for hostname, message in TEST_MESSAGES:
            clientssh.runsh(f"logger -t test -p daemon.info -H {hostname} -h 10.0.0.1 '{message}'")
        time.sleep(2)

    with test.step("Verify router1 log contains only router1 messages"):
        rc = serverssh.runsh("cat /var/log/router1 2>/dev/null")
        log_content = rc.stdout if rc.returncode == 0 else ""

        router1_messages = [msg for host, msg in TEST_MESSAGES if host == "router1"]
        missing = [msg for msg in router1_messages if msg not in log_content]
        if missing:
            test.fail(f"Missing router1 messages in /var/log/router1: {missing}")

        unwanted_messages = [msg for host, msg in TEST_MESSAGES if host != "router1"]
        found = [msg for msg in unwanted_messages if msg in log_content]
        if found:
            test.fail(f"Found unwanted messages in /var/log/router1: {found}")

    with test.step("Verify router2 log contains only router2 messages"):
        rc = serverssh.runsh("cat /var/log/router2 2>/dev/null")
        log_content = rc.stdout if rc.returncode == 0 else ""

        router2_messages = [msg for host, msg in TEST_MESSAGES if host == "router2"]
        missing = [msg for msg in router2_messages if msg not in log_content]
        if missing:
            test.fail(f"Missing router2 messages in /var/log/router2: {missing}")

        unwanted_messages = [msg for host, msg in TEST_MESSAGES if host != "router2"]
        found = [msg for msg in unwanted_messages if msg in log_content]
        if found:
            test.fail(f"Found unwanted messages in /var/log/router2: {found}")

    with test.step("Verify all-hosts log contains all messages"):
        rc = serverssh.runsh("cat /var/log/all-hosts 2>/dev/null")
        log_content = rc.stdout if rc.returncode == 0 else ""

        expected_messages = [msg for _, msg in TEST_MESSAGES]
        missing = [msg for msg in expected_messages if msg not in log_content]

        if missing:
            test.fail(f"Missing messages in /var/log/all-hosts: {missing}")

    test.succeed()
