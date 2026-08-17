#!/usr/bin/env python3
"""
Set timezone using timezone name

Verify that it is possible to set timezone using timezone names.
"""
import random, string
import time
import infamy
import lxml

from infamy.util import until

def get_timezone_name():
    try:
        tz=target.get_data("/ietf-system:system/clock/timezone-name")
        name=tz.get("system", {}).get("clock",{}).get("timezone-name", "")
        return name
    except:
        return None

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

    with test.step("Set timezone to Australia/Perth"):
          target.put_config_dicts({"ietf-system": {
            "system": {
                "clock": {
                    "timezone-name": "Australia/Perth"
                    }
            }
          }})

    with test.step("Verify timezone is Australia/Perth"):
        until(lambda: get_timezone_name() == "Australia/Perth")

    test.succeed()
