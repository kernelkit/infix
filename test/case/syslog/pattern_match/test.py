#!/usr/bin/env python3
"""
Syslog Pattern Matching

Verify the select-match feature: filtering syslog messages based on
pattern-match (POSIX regex) on message content.  Tests both simple
substring matching and complex regex patterns.
"""

import infamy
import time

TEST_MESSAGES = [
    "ERROR: Connection failed on interface eth0",
    "CRITICAL: System temperature high",
    "Status update from router1: link up",
    "Status update from router42: link down",
    "INFO: Normal operation message",
    "DEBUG: Verbose logging enabled",
]

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        tgtssh = env.attach("target", "mgmt", "ssh")

    with test.step("Clean up old log files from previous test runs"):
        tgtssh.runsh("sudo rm -f /var/log/{errors,routers,all-messages}")

    with test.step("Configure syslog with pattern-match filters"):
        target.put_config_dicts({
            "ietf-syslog": {
                "syslog": {
                    "actions": {
                        "file": {
                            "log-file": [{
                                "name": "file:errors",
                                "pattern-match": "ERROR|CRITICAL",
                            }, {
                                "name": "file:routers",
                                "pattern-match": "router[0-9]+",
                                "facility-filter": {
                                    "facility-list": [{
                                        "facility": "all",
                                        "severity": "info"
                                    }]
                                }
                            }, {
                                "name": "file:all-messages",
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

    with test.step("Send test messages with various patterns"):
        for message in TEST_MESSAGES:
            tgtssh.runsh(f"logger -t test -p daemon.info '{message}'")
        time.sleep(2)

    with test.step("Verify errors log contains ERROR and CRITICAL messages"):
        rc = tgtssh.runsh("cat /var/log/errors 2>/dev/null")
        log_content = rc.stdout if rc.returncode == 0 else ""

        error_messages = [msg for msg in TEST_MESSAGES if "ERROR" in msg or "CRITICAL" in msg]
        missing = [msg for msg in error_messages if msg not in log_content]
        if missing:
            test.fail(f"Missing error messages in /var/log/errors: {missing}")

        # Verify it does NOT contain other messages
        non_error_messages = [msg for msg in TEST_MESSAGES if "ERROR" not in msg and "CRITICAL" not in msg]
        found = [msg for msg in non_error_messages if msg in log_content]
        if found:
            test.fail(f"Found unwanted messages in /var/log/errors: {found}")

    with test.step("Verify routers log contains matching router[0-9]+ pattern"):
        rc = tgtssh.runsh("cat /var/log/routers 2>/dev/null")
        log_content = rc.stdout if rc.returncode == 0 else ""

        router_messages = [msg for msg in TEST_MESSAGES if "router1" in msg or "router42" in msg]
        missing = [msg for msg in router_messages if msg not in log_content]
        if missing:
            test.fail(f"Missing router messages in /var/log/routers: {missing}")

        # Verify it does NOT contain error or normal messages
        non_router_messages = [msg for msg in TEST_MESSAGES if "router" not in msg]
        found = [msg for msg in non_router_messages if msg in log_content]
        if found:
            test.fail(f"Found unwanted messages in /var/log/routers: {found}")

    with test.step("Verify all-messages log contains all test messages"):
        rc = tgtssh.runsh("cat /var/log/all-messages 2>/dev/null")
        log_content = rc.stdout if rc.returncode == 0 else ""

        missing = [msg for msg in TEST_MESSAGES if msg not in log_content]
        if missing:
            test.fail(f"Missing messages in /var/log/all-messages: {missing}")

    test.succeed()
