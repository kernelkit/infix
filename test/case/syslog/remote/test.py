#!/usr/bin/env python3
"""Remote syslog

Verify logging to remote, acting as a remote, RFC5424 log format, TCP
transport (RFC 6587), and TCP message queueing: messages sent while the
link is down are delivered after link recovery within two retry timeouts.
"""
import uuid
import infamy


def unique_msg():
    return "TestMessage-" + uuid.uuid4().hex[:8]


def syslog_check(clientssh, serverssh, msgid="client"):
    msg = unique_msg()
    clientssh.runsh(f"logger -t test -m {msgid} -p security.notice {msg}")
    return serverssh.runsh(f"grep '{msg}' /log/security 2>/dev/null").returncode == 0


def queue_drain_check(clientssh, serverssh, *msgs):
    """Send a trigger to wake the TCP retry handler; verify queued messages arrived."""
    clientssh.runsh("logger -t test -m trigger -p security.notice Trigger")
    return all(
        serverssh.runsh(f"grep '{msg}' /log/security 2>/dev/null").returncode == 0
        for msg in msgs
    )


with infamy.Test() as test:
    with test.step("Set up topology and attach to client and server DUTs"):
        env = infamy.Env()
        client = env.attach("client", "mgmt")
        server = env.attach("server", "mgmt")
        clientssh = env.attach("client", "mgmt", "ssh")
        serverssh = env.attach("server", "mgmt", "ssh")

    with test.step("Configure client as syslog forwarder and server as syslog sink"):
        _, client_link = env.ltop.xlate("client", "link")
        _, server_link = env.ltop.xlate("server", "link")

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
                        "file": {
                            "log-file": [{
                                "name": "file:security",
                                "facility-filter": {
                                    "facility-list": [{
                                        "facility": "auth",
                                        "severity": "all"
                                    }, {
                                        "facility": "audit",
                                        "severity": "all"
                                    }]
                                },
                                "infix-syslog:log-format": "rfc5424"
                            }]
                        },
                        "remote": {
                            "destination": [{
                                "name": "server",
                                "udp": {
                                    "address": "10.0.0.1"
                                },
                                "facility-filter": {
                                    "facility-list": [{
                                        "facility": "audit",
                                        "severity": "all"
                                    }, {
                                        "facility": "auth",
                                        "severity": "all"
                                    }]
                                },
                                "infix-syslog:log-format": "rfc5424"
                            }]
                        }
                    }
                }
            }
        })

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
                                "name": "file:security",
                                "facility-filter": {
                                    "facility-list": [{
                                        "facility": "auth",
                                        "severity": "all"
                                    }, {
                                        "facility": "audit",
                                        "severity": "all"
                                    }]
                                },
                                "infix-syslog:log-format": "rfc5424"
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

    with test.step("Send security:notice log message from client and verify reception on server via UDP"):
        infamy.until(lambda: syslog_check(clientssh, serverssh))

    with test.step("Reconfigure client to forward via TCP and server to accept TCP connections"):
        # Configure server first: sysklogd suspends TCP forwarding for
        # tcp-retry-timeout seconds on ECONNREFUSED, so the server must be
        # listening before the client syslogd reloads and attempts its first
        # TCP connection.
        server.delete_xpath("/ietf-syslog:syslog/infix-syslog:server/listen/udp[port='514']")
        server.patch_config("ietf-syslog", {
            "syslog": {
                "infix-syslog:server": {
                    "enabled": True,
                    "listen": {
                        "tcp": [{
                            "port": 514,
                            "address": "10.0.0.1"
                        }]
                    }
                }
            }
        })
        infamy.until(lambda: "10.0.0.1:514" in serverssh.runsh("ss -tlnp 2>/dev/null | grep :514 || true").stdout, attempts=20)

        # Use patch_config (direct PATCH to running) to bypass the candidate
        # round-trip that can silently drop augment-module data.
        client.delete_xpath("/ietf-syslog:syslog/actions/remote/destination[name='server']")
        client.patch_config("ietf-syslog", {
            "syslog": {
                "actions": {
                    "remote": {
                        "destination": [{
                            "name": "server",
                            "infix-syslog:tcp": {
                                "address": "10.0.0.1"
                            },
                            "facility-filter": {
                                "facility-list": [{
                                    "facility": "audit",
                                    "severity": "all"
                                }, {
                                    "facility": "auth",
                                    "severity": "all"
                                }]
                            },
                            "infix-syslog:log-format": "rfc5424"
                        }]
                    }
                }
            }
        })

    with test.step("Verify server is listening on TCP port 514"):
        infamy.until(lambda: "10.0.0.1:514" in serverssh.runsh("ss -tlnp 2>/dev/null | grep :514 || true").stdout, attempts=20)

    with test.step("Send security:notice log message from client and verify reception on server via TCP"):
        infamy.until(lambda: syslog_check(clientssh, serverssh, "tcp"), attempts=20)

    with test.step("Verify TCP message queueing: messages sent while link is down are delivered after recovery"):
        # Shorten the retry timeout so the test completes in a reasonable time.
        client.patch_config("ietf-syslog", {
            "syslog": {
                "infix-syslog:tcp-retry-timeout": 10
            }
        })

        # Bring the data link down to force TCP connection failure.
        client.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{"name": client_link, "enabled": False}]
                }
            }
        })

        # These two messages cannot be delivered; sysklogd queues them.
        # Unique message bodies ensure grep cannot match a stale entry from
        # a previous stage or test run.
        q1 = unique_msg()
        q2 = unique_msg()
        clientssh.runsh(f"logger -t test -m q1 -p security.notice {q1}")
        clientssh.runsh(f"logger -t test -m q2 -p security.notice {q2}")

        # Restore the link.  sysklogd retries after tcp-retry-timeout (10 s)
        # and flushes the queue.  queue_drain_check sends a trigger message on
        # each attempt: once the 10 s window has elapsed the trigger fires the
        # F_FORW_TCP_SUSP retry path, reconnects, and drains q1 + q2.
        client.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{"name": client_link, "enabled": True}]
                }
            }
        })
        infamy.until(lambda: queue_drain_check(clientssh, serverssh, q1, q2), attempts=30)

    test.succeed()
