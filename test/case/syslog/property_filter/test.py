#!/usr/bin/env python3
"""Syslog Property Filtering

Verify property-filter feature: filtering syslog messages based on various
message properties with different operators, case-insensitivity, and negation.

"""

import uuid

import infamy
from infamy.util import parallel, until

TEST_MESSAGES = [
    ("myapp", "Application startup"),
    ("myapp", "Processing request"),
    ("otherapp", "Different program"),
    ("test", "ERROR: Connection failed"),
    ("test", "INFO: Normal message"),
    ("test", "WARNING: Check config"),
    ("test", "Warning: lowercase"),
]

# Old messages survive in /var/log on hardware, count only this run's
TOKEN = uuid.uuid4().hex[:8]


def count_lines(ssh, logfile, text):
    """Count lines in /var/log/logfile from this run containing text"""
    rc = ssh.runsh(f"cat /var/log/{logfile} 2>/dev/null")
    return sum(1 for line in rc.stdout.splitlines()
               if TOKEN in line and text in line)


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target, tgtssh = parallel(lambda: env.attach("target", "mgmt"),
                                  lambda: env.attach("target", "mgmt", "ssh"))

    with test.step("Clean up old log files"):
        rc = tgtssh.run_retry("sudo rm -f /var/log/myapp /var/log/not-error /var/log/case-test /var/log/baseline")
        if rc.returncode:
            test.fail("Failed removing old log files")

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

        until(lambda: tgtssh.runsh("test -f /var/log/baseline").returncode == 0, attempts=10)

    with test.step("Send test messages"):
        for tag, msg in TEST_MESSAGES:
            target.log(f"{msg} {TOKEN}", severity="info", app_name=tag)
        until(lambda: count_lines(tgtssh, "baseline", TEST_MESSAGES[-1][1]), attempts=10)

    with test.step("Verify myapp log contains only myapp messages"):
        count = count_lines(tgtssh, "myapp", "myapp")
        if count != 2:
            test.fail(f"Expected 2 myapp messages in /var/log/myapp, got {count}")

        count = count_lines(tgtssh, "myapp", "otherapp")
        if count != 0:
            test.fail(f"Expected 0 otherapp messages in /var/log/myapp, got {count}")

    with test.step("Verify not-error log excludes ERROR messages"):
        count = count_lines(tgtssh, "not-error", "test")
        if count != 3:
            test.fail(f"Expected 3 non-ERROR messages in /var/log/not-error, got {count}")

        count = count_lines(tgtssh, "not-error", "ERROR")
        if count != 0:
            test.fail(f"Expected 0 ERROR messages in /var/log/not-error, got {count}")

    with test.step("Verify case-test log matches case-insensitive 'warning'"):
        count = count_lines(tgtssh, "case-test", "WARNING") + \
            count_lines(tgtssh, "case-test", "Warning")
        if count != 2:
            test.fail(f"Expected 2 warning messages in /var/log/case-test, got {count}")

    with test.step("Verify baseline log contains all messages"):
        for tag, msg in TEST_MESSAGES:
            if not count_lines(tgtssh, "baseline", msg):
                test.fail(f"Expected message '{msg}' not found in /var/log/baseline")

    test.succeed()
