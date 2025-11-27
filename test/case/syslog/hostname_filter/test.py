#!/usr/bin/env python3
"""Syslog Hostname Filtering

Verify hostname-filter feature: filtering incoming syslog messages based
on the hostname.  Tests log sink scenario with multiple remote clients.

"""

import infamy
import infamy.iface as iface
from infamy import until

TEST_MESSAGES = [
    ("router1", "Message from router1"),
    ("router1", "Another from router1"),
    ("router2", "Message from router2"),
    ("other",   "Message from a different host"),
]

def verify_log_content(ssh, logfile, expected_hosts, test):
    """Verify log file contains only messages from expected hosts."""
    def check_log():
        rc = ssh.runsh(f"cat /var/log/{logfile} 2>/dev/null")
        log_content = rc.stdout if rc.returncode == 0 else ""

        expected_messages = [msg for host, msg in TEST_MESSAGES if host in expected_hosts]
        missing = [msg for msg in expected_messages if msg not in log_content]
        if missing:
            return False

        if expected_hosts:  # Only check for unwanted if filtering by hostname
            unwanted_messages = [msg for host, msg in TEST_MESSAGES if host not in expected_hosts]
            found = [msg for msg in unwanted_messages if msg in log_content]
            if found:
                test.fail(f"Found unwanted messages in /var/log/{logfile}: {found}")

        return True

    until(check_log, attempts=20)

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

    with test.step("Wait for server interface to be operational"):
        until(lambda: iface.get_param(server, server_link, "oper-status") == "up", attempts=20)

    with test.step("Verify server IP address is configured"):
        until(lambda: iface.address_exist(server, server_link, "10.0.0.1", proto="static"), attempts=20)

    with test.step("Verify syslog server is listening on UDP port 514"):
        until(lambda: "10.0.0.1:514" in serverssh.runsh("ss -ulnp 2>/dev/null | grep :514 || true").stdout, attempts=20)

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

    with test.step("Wait for client interface to be operational"):
        until(lambda: iface.get_param(client, client_link, "oper-status") == "up", attempts=20)

    with test.step("Verify client IP address is configured"):
        until(lambda: iface.address_exist(client, client_link, "10.0.0.2", proto="static"), attempts=20)

    with test.step("Verify network connectivity between client and server"):
        until(lambda: clientssh.runsh("ping -c 1 -W 1 10.0.0.1 >/dev/null 2>&1").returncode == 0, attempts=10)

    with test.step("Send log messages with different hostnames"):
        for hostname, message in TEST_MESSAGES:
            clientssh.runsh(f"logger -t test -p daemon.info -H {hostname} -h 10.0.0.1 '{message}'")

    with test.step("Verify router1 log contains only router1 messages"):
        verify_log_content(serverssh, "router1", ["router1"], test)

    with test.step("Verify router2 log contains only router2 messages"):
        verify_log_content(serverssh, "router2", ["router2"], test)

    with test.step("Verify all-hosts log contains all messages"):
        all_hosts = [host for host, _ in TEST_MESSAGES]
        verify_log_content(serverssh, "all-hosts", all_hosts, test)

    test.succeed()
