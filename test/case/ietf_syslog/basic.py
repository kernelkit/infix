#!/usr/bin/env python3
"""
  - Add syslog actions to log to local files
  - Verify new log files have been created
"""

import infamy
import infamy.ssh as ssh

with infamy.Test() as test:
    with test.step("Initializing ..."):
        env = infamy.Env(infamy.std_topology("1x1"))
        target = env.attach("target", "mgmt")
        tgtssh = env.attach("target", "mgmt", "ssh")
        factory = env.get_password("target")
        address = target.get_mgmt_ip()

    with test.step("Add new syslog file action"):
        target.put_config_dict("ietf-syslog", {
            "syslog": {
                "actions": {
                    "file": {
                        "log-file": [
                            {
                                "name": "file:foo",
                                "facility-filter": {
                                    "facility-list": [
                                        {
                                            "facility": "auth",
                                            "severity": "info"
                                        },
                                        {
                                            "facility": "authpriv",
                                            "severity": "debug"
                                        }
                                    ]
                                }
                            },
                            {
                                "name": "file:/log/bar.log",
                                "facility-filter": {
                                    "facility-list": [
                                        {
                                            "facility": "all",
                                            "severity": "critical"
                                        },
                                        {
                                            "facility": "mail",
                                            "severity": "warning"
                                        }
                                    ]
                                }
                            }
                        ]
                    }
                }
            }
        })

    with test.step("Verify log files have been created ..."):
        user = tgtssh.runsh("ls /var/log/{foo,bar.log}").stdout
        if "/var/log/foo" not in user:
            test.fail()
        if "/var/log/bar.log" not in user:
            test.fail()

    test.succeed()
