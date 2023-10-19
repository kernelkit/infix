#!/usr/bin/env python3

import random, string
import time
import infamy
import lxml
with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env(infamy.std_topology("1x1"))
        target = env.attach("target", "mgmt")

    with test.step("Set timezone"):
          target.put_config_dict("ietf-system", {
            "system": {
                "clock": {
                    "timezone-name": "Australia/Perth" # always +8:00, no DTS
                    }
            }
          })

    with test.step("Verify current time."):
        root = target.get_dict("/ietf-system:system-state/clock",as_xml=True)
        current_datetime = root.find('.//{urn:ietf:params:xml:ns:yang:ietf-system}current-datetime').text
        offset=current_datetime[-6:]

        assert(offset == "+08:00")

    test.succeed()
