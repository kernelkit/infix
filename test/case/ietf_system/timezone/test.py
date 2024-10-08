#!/usr/bin/env python3
"""
Set timezone

Verify that it is possible to set timezone using timezone names.
"""
import random, string
import time
import infamy
import lxml
with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

    with test.step("Set timezone to Australia/Perth"):
          target.put_config_dict("ietf-system", {
            "system": {
                "clock": {
                    "timezone-name": "Australia/Perth" # always +8:00, no DTS
                    }
            }
          })

    with test.step("Verify current time offset is +08:00"):
        current_datetime=target.get_current_time_with_offset()
        offset=current_datetime[-6:]

        assert(offset == "+08:00")

    test.succeed()
