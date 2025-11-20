#!/usr/bin/env python3
"""Syslog Advanced Compare

Verify the select-adv-compare feature: filtering syslog messages based on
severity with advanced comparison operators (equals vs equals-or-higher) and
actions (log vs block/stop).

"""

import infamy
import time

TEST_MESSAGES = [
    ("daemon.emerg",   "Emergency: system is unusable"),
    ("daemon.alert",   "Alert: immediate action required"),
    ("daemon.crit",    "Critical: critical condition"),
    ("daemon.err",     "Error: error condition"),
    ("daemon.warning", "Warning: warning condition"),
    ("daemon.notice",  "Notice: normal but significant"),
    ("daemon.info",    "Info: informational message"),
    ("daemon.debug",   "Debug: debug-level message"),
]

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        tgtssh = env.attach("target", "mgmt", "ssh")

    with test.step("Clean up old log files from previous test runs"):
        tgtssh.runsh("sudo rm -f /var/log/{exact-errors,no-debug,baseline}")

    with test.step("Configure syslog with advanced-compare"):
        target.put_config_dicts({
            "ietf-syslog": {
                "syslog": {
                    "actions": {
                        "file": {
                            "log-file": [{
                                "name": "file:exact-errors",
                                "facility-filter": {
                                    "facility-list": [{
                                        "facility": "daemon",
                                        "severity": "error",
                                        "advanced-compare": {
                                            "compare": "equals"
                                        }
                                    }]
                                }
                            }, {
                                "name": "file:no-debug",
                                "facility-filter": {
                                    "facility-list": [{
                                        "facility": "daemon",
                                        "severity": "debug",
                                        "advanced-compare": {
                                            "action": "block"
                                        }
                                    }]
                                }
                            }, {
                                "name": "file:baseline",
                                "facility-filter": {
                                    "facility-list": [{
                                        "facility": "daemon",
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

    with test.step("Send test messages at all severity levels"):
        for priority, message in TEST_MESSAGES:
            tgtssh.runsh(f"logger -t advtest -p {priority} '{message}'")
        time.sleep(2)

    with test.step("Verify exact-errors log contains only error messages"):
        rc = tgtssh.runsh("cat /var/log/exact-errors 2>/dev/null")
        log_content = rc.stdout if rc.returncode == 0 else ""

        # Should contain only the error message
        error_messages = [msg for prio, msg in TEST_MESSAGES if "err" in prio]
        missing = [msg for msg in error_messages if msg not in log_content]
        if missing:
            test.fail(f"Missing error messages in /var/log/exact-errors: {missing}")

        # Should NOT contain higher severity (emerg, alert, crit) or lower
        unwanted_messages = [msg for prio, msg in TEST_MESSAGES if "err" not in prio]
        found = [msg for msg in unwanted_messages if msg in log_content]
        if found:
            test.fail(f"Found unwanted messages in /var/log/exact-errors: {found}")

    with test.step("Verify no-debug log blocks all messages"):
        rc = tgtssh.runsh("cat /var/log/no-debug 2>/dev/null")
        log_content = rc.stdout if rc.returncode == 0 else ""

        # Should be empty (all messages blocked)
        all_messages = [msg for _, msg in TEST_MESSAGES]
        found = [msg for msg in all_messages if msg in log_content]
        if found:
            test.fail(f"Expected empty log, found messages in /var/log/no-debug: {found}")

    with test.step("Verify baseline log contains info and higher"):
        rc = tgtssh.runsh("cat /var/log/baseline 2>/dev/null")
        log_content = rc.stdout if rc.returncode == 0 else ""

        # Should contain info and higher (all except debug)
        expected_messages = [msg for prio, msg in TEST_MESSAGES if "debug" not in prio]
        missing = [msg for msg in expected_messages if msg not in log_content]
        if missing:
            test.fail(f"Missing messages in /var/log/baseline: {missing}")

        # Should NOT contain debug
        debug_messages = [msg for prio, msg in TEST_MESSAGES if "debug" in prio]
        found = [msg for msg in debug_messages if msg in log_content]
        if found:
            test.fail(f"Found debug messages in /var/log/baseline: {found}")

    test.succeed()
