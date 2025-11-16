#!/usr/bin/env python3
"""Syslog Advanced Compare

Verify the select-adv-compare feature: filtering syslog messages based on
severity with advanced comparison operators (equals vs equals-or-higher) and
actions (log vs block/stop).

"""

import infamy
import time

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

    with test.step("Send test messages at all severity levels"):
        tgtssh.runsh("logger -t advtest -p daemon.emerg 'Emergency: system is unusable'")
        tgtssh.runsh("logger -t advtest -p daemon.alert 'Alert: immediate action required'")
        tgtssh.runsh("logger -t advtest -p daemon.crit 'Critical: critical condition'")
        tgtssh.runsh("logger -t advtest -p daemon.err 'Error: error condition'")
        tgtssh.runsh("logger -t advtest -p daemon.warning 'Warning: warning condition'")
        tgtssh.runsh("logger -t advtest -p daemon.notice 'Notice: normal but significant'")
        tgtssh.runsh("logger -t advtest -p daemon.info 'Info: informational message'")
        tgtssh.runsh("logger -t advtest -p daemon.debug 'Debug: debug-level message'")
        time.sleep(1)

    with test.step("Verify exact-errors log contains only error messages"):
        rc = tgtssh.runsh("grep -c 'advtest' /var/log/exact-errors 2>/dev/null")
        count = int(rc.stdout.strip()) if rc.returncode == 0 else 0
        if count != 1:
            test.fail(f"Expected 1 message in /var/log/exact-errors (error only), got {count}")

        rc = tgtssh.runsh("grep -q 'Error: error condition' /var/log/exact-errors 2>/dev/null")
        if rc.returncode != 0:
            test.fail("Expected error message in /var/log/exact-errors")

        rc = tgtssh.runsh("grep -c 'Emergency\\|Alert\\|Critical' /var/log/exact-errors 2>/dev/null")
        count = int(rc.stdout.strip()) if rc.returncode == 0 else 0
        if count != 0:
            test.fail(f"Expected 0 higher severity messages in /var/log/exact-errors, got {count}")

    with test.step("Verify no-debug log blocks all messages"):
        rc = tgtssh.runsh("grep -c 'advtest' /var/log/no-debug 2>/dev/null")
        count = int(rc.stdout.strip()) if rc.returncode == 0 else 0
        if count != 0:
            test.fail(f"Expected 0 messages in /var/log/no-debug (all blocked), got {count}")

    with test.step("Verify baseline log contains info and higher"):
        rc = tgtssh.runsh("grep -c 'advtest' /var/log/baseline 2>/dev/null")
        count = int(rc.stdout.strip()) if rc.returncode == 0 else 0
        if count != 7:
            test.fail(f"Expected 7 messages in /var/log/baseline (info and higher), got {count}")

        rc = tgtssh.runsh("grep -c 'Debug: debug-level' /var/log/baseline 2>/dev/null")
        count = int(rc.stdout.strip()) if rc.returncode == 0 else 0
        if count != 0:
            test.fail(f"Expected 0 debug messages in /var/log/baseline, got {count}")

        rc = tgtssh.runsh("grep -q 'Info: informational' /var/log/baseline 2>/dev/null")
        if rc.returncode != 0:
            test.fail("Expected info message in /var/log/baseline")

    test.succeed()
