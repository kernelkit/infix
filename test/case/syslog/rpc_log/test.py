#!/usr/bin/env python3
"""Syslog Log RPC

Verify the infix-syslog:log RPC, used by the test system to inject
markers in a DUT's system log.  A message logged with the full RFC 5424
header must show up in an RFC 5424 formatted log file with the given
app-name, msgid, and structured data.  A message logged with only the
mandatory text must fall back to the documented defaults.

"""

import re

import infamy
from infamy.util import parallel, until

LOG_FILE = "/var/log/rpc-log"


def logfile():
    """Return contents of the RFC 5424 test log file, or empty string"""
    rc = tgtssh.runsh(f"cat {LOG_FILE} 2>/dev/null")
    return rc.stdout if rc.returncode == 0 else ""


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target, tgtssh = parallel(lambda: env.attach("target", "mgmt"),
                                  lambda: env.attach("target", "mgmt", "ssh"))

    with test.step("Clean up old log file from previous test runs"):
        tgtssh.runsh(f"sudo rm -f {LOG_FILE}")

    with test.step("Configure an RFC 5424 formatted log file for all facilities"):
        target.put_config_dicts({
            "ietf-syslog": {
                "syslog": {
                    "actions": {
                        "file": {
                            "log-file": [{
                                "name": f"file:{LOG_FILE}",
                                "infix-syslog:log-format": "rfc5424",
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
        until(lambda: tgtssh.runsh(f"test -f {LOG_FILE}").returncode == 0, attempts=10)

    with test.step("Log message with severity, facility, app-name, msgid, and structured data"):
        target.log("Kilroy was here", severity="warning", facility="daemon",
                   app_name="infamy", msgid="test-start",
                   sd={"test@32473": {"name": "rpc_log", "step": "3"}})
        until(lambda: "Kilroy was here" in logfile(), attempts=10)

    with test.step("Verify RFC 5424 header fields of the logged message"):
        line = [ln for ln in logfile().splitlines() if "Kilroy was here" in ln][0]
        if not re.search(r"\binfamy - test-start \[test@32473 [^]]*\] Kilroy was here$", line):
            test.fail(f"Unexpected app-name, msgid, or structured data: {line}")
        for param in ('name="rpc_log"', 'step="3"'):
            if param not in line:
                test.fail(f"Missing structured data param {param}: {line}")

    with test.step("Log message with only the mandatory text"):
        target.log("Plain message, no frills")
        until(lambda: "Plain message" in logfile(), attempts=10)

    with test.step("Verify default app-name is set and msgid and structured data are empty"):
        line = [ln for ln in logfile().splitlines() if "Plain message" in ln][0]
        if not re.search(r" \S+ - - - Plain message, no frills$", line):
            test.fail(f"Unexpected header for default message: {line}")
        if re.search(r" - - - - Plain message", line):
            test.fail(f"Default app-name should not be empty: {line}")

    test.succeed()
