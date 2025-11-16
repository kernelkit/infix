#!/usr/bin/env python3
"""
Syslog Pattern Matching

Verify the select-match feature: filtering syslog messages based on
pattern-match (POSIX regex) on message content.  Tests both simple
substring matching and complex regex patterns.
"""

import infamy
import time

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

    with test.step("Send test messages with various patterns"):
        tgtssh.runsh("logger -t test -p daemon.info 'ERROR: Connection failed on interface eth0'")
        tgtssh.runsh("logger -t test -p daemon.info 'CRITICAL: System temperature high'")
        tgtssh.runsh("logger -t test -p daemon.info 'Status update from router1: link up'")
        tgtssh.runsh("logger -t test -p daemon.info 'Status update from router42: link down'")
        tgtssh.runsh("logger -t test -p daemon.info 'INFO: Normal operation message'")
        tgtssh.runsh("logger -t test -p daemon.info 'DEBUG: Verbose logging enabled'")
        time.sleep(1)

    with test.step("Verify errors log contains ERROR and CRITICAL messages"):
        rc = tgtssh.runsh("grep -c 'ERROR\\|CRITICAL' /var/log/errors 2>/dev/null")
        count = int(rc.stdout.strip()) if rc.returncode == 0 else 0
        if count != 2:
            test.fail(f"Expected 2 ERROR/CRITICAL messages in /var/log/errors, got {count}")

        # Verify it does NOT contain other messages
        rc = tgtssh.runsh("grep -c 'router1\\|Normal operation\\|Verbose' /var/log/errors 2>/dev/null")
        count = int(rc.stdout.strip()) if rc.returncode == 0 else 0
        if count != 0:
            test.fail(f"Expected 0 non-error messages in /var/log/errors, got {count}")

    with test.step("Verify routers log contains matching router[0-9]+ pattern"):
        rc = tgtssh.runsh("grep -c 'router[0-9]\\+' /var/log/routers 2>/dev/null")
        count = int(rc.stdout.strip()) if rc.returncode == 0 else 0
        if count != 2:
            test.fail(f"Expected 2 router messages in /var/log/routers, got {count}")

        # Verify both router1 and router42 are present
        rc = tgtssh.runsh("grep -q 'router1' /var/log/routers 2>/dev/null")
        if rc.returncode != 0:
            test.fail("Expected router1 message in /var/log/routers")

        rc = tgtssh.runsh("grep -q 'router42' /var/log/routers 2>/dev/null")
        if rc.returncode != 0:
            test.fail("Expected router42 message in /var/log/routers")

        # Verify it does NOT contain error or normal messages
        rc = tgtssh.runsh("grep -c 'ERROR\\|CRITICAL\\|Normal operation\\|Verbose' /var/log/routers 2>/dev/null")
        count = int(rc.stdout.strip()) if rc.returncode == 0 else 0
        if count != 0:
            test.fail(f"Expected 0 non-router messages in /var/log/routers, got {count}")

    with test.step("Verify all-messages log contains all test messages"):
        rc = tgtssh.runsh("grep -c 'test' /var/log/all-messages 2>/dev/null")
        count = int(rc.stdout.strip()) if rc.returncode == 0 else 0
        if count != 6:
            test.fail(f"Expected 6 total messages in /var/log/all-messages, got {count}")

    test.succeed()
