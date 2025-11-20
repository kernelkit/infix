#!/usr/bin/env python3
"""Syslog Property Filtering

Verify property-filter feature: filtering syslog messages based on various
message properties with different operators, case-insensitivity, and negation.

"""

import infamy
import time

TEST_MESSAGES = [
    ("myapp", "Application startup"),
    ("myapp", "Processing request"),
    ("otherapp", "Different program"),
    ("test", "ERROR: Connection failed"),
    ("test", "INFO: Normal message"),
    ("test", "WARNING: Check config"),
    ("test", "Warning: lowercase"),
]

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        tgtssh = env.attach("target", "mgmt", "ssh")

    with test.step("Clean up old log files"):
        tgtssh.runsh("sudo rm -f /var/log/{myapp,not-error,case-test,baseline}")

    with test.step("Configure syslog with property filters"):
        target.put_config_dicts({
            "ietf-syslog": {
                "syslog": {
                    "actions": {
                        "file": {
                            "log-file": [{
                                "name": "file:myapp",
                                "infix-syslog:property-filter": {
                                    "property": "programname",
                                    "operator": "isequal",
                                    "value": "myapp"
                                },
                                "facility-filter": {
                                    "facility-list": [{
                                        "facility": "all",
                                        "severity": "info"
                                    }]
                                }
                            }, {
                                "name": "file:not-error",
                                "infix-syslog:property-filter": {
                                    "property": "msg",
                                    "operator": "contains",
                                    "value": "ERROR",
                                    "negate": True
                                },
                                "facility-filter": {
                                    "facility-list": [{
                                        "facility": "all",
                                        "severity": "info"
                                    }]
                                }
                            }, {
                                "name": "file:case-test",
                                "infix-syslog:property-filter": {
                                    "property": "msg",
                                    "operator": "contains",
                                    "value": "warning",
                                    "case-insensitive": True
                                },
                                "facility-filter": {
                                    "facility-list": [{
                                        "facility": "all",
                                        "severity": "info"
                                    }]
                                }
                            }, {
                                "name": "file:baseline",
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

    with test.step("Send test messages"):
        for tag, msg in TEST_MESSAGES:
            tgtssh.runsh(f"logger -t {tag} -p daemon.info '{msg}'")
        time.sleep(1)

    with test.step("Verify myapp log contains only myapp messages"):
        rc = tgtssh.runsh("grep -c 'myapp' /var/log/myapp 2>/dev/null")
        count = int(rc.stdout.strip()) if rc.returncode == 0 else 0
        if count != 2:
            test.fail(f"Expected 2 myapp messages in /var/log/myapp, got {count}")

        rc = tgtssh.runsh("grep -c 'otherapp' /var/log/myapp 2>/dev/null")
        count = int(rc.stdout.strip()) if rc.returncode == 0 else 0
        if count != 0:
            test.fail(f"Expected 0 otherapp messages in /var/log/myapp, got {count}")

    with test.step("Verify not-error log excludes ERROR messages"):
        rc = tgtssh.runsh("grep -c 'test' /var/log/not-error 2>/dev/null")
        count = int(rc.stdout.strip()) if rc.returncode == 0 else 0
        if count != 3:
            test.fail(f"Expected 3 non-ERROR messages in /var/log/not-error, got {count}")

        rc = tgtssh.runsh("grep -c 'ERROR' /var/log/not-error 2>/dev/null")
        count = int(rc.stdout.strip()) if rc.returncode == 0 else 0
        if count != 0:
            test.fail(f"Expected 0 ERROR messages in /var/log/not-error, got {count}")

    with test.step("Verify case-test log matches case-insensitive 'warning'"):
        rc = tgtssh.runsh("grep -c 'WARNING\\|Warning' /var/log/case-test 2>/dev/null")
        count = int(rc.stdout.strip()) if rc.returncode == 0 else 0
        if count != 2:
            test.fail(f"Expected 2 warning messages in /var/log/case-test, got {count}")

    with test.step("Verify baseline log contains all messages"):
        for tag, msg in TEST_MESSAGES:
            rc = tgtssh.runsh(f"grep -q '{msg}' /var/log/baseline 2>/dev/null")
            if rc.returncode != 0:
                test.fail(f"Expected message '{msg}' not found in /var/log/baseline")

    test.succeed()
