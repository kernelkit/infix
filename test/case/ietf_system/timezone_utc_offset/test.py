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

    with test.step("Verify current time offset is +12:00"):
        current_datetime=target.get_current_time_with_offset()
        offset=current_datetime[-6:]

        assert(offset == "+12:00")

    test.succeed()
