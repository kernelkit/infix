#!/usr/bin/env python3
"""
Set timezone with UTC offset

Verify that it is possible to set timezone using UTC offset
"""
import infamy
import lxml

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

    with test.step("Set timezone UTC offset to +12"):
          target.put_config_dicts({"ietf-system": {
            "system": {
                "clock": {
                    "timezone-utc-offset": "12"
                    }
            }
          }})

    with test.step("Verify current timezone is UTC+12:00"):
        tz=target.get_data("/ietf-system:system/clock/timezone-utc-offset")
        offset=tz.get("system", {}).get("clock",{}).get("timezone-utc-offset", 0)
        assert(offset == 12)

    test.succeed()
