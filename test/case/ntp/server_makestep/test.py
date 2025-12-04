#!/usr/bin/env python3
"""NTP server makestep

Verify makestep directive works properly for initial sync.

The makestep directive is critical for embedded systems that boot with
epoch time (no RTC), allowing immediate clock correction instead of the
default slow slewing over several hours.

"""

import infamy


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

    with test.step("Configure NTP server with makestep defaults"):
        target.put_config_dicts({
            "ietf-ntp": {
                "ntp": {
                    "refclock-master": {
                        "master-stratum": 10
                    },
                    "infix-ntp:makestep": {
                        "threshold": 1.0,
                        "limit": 3
                    }
                }
            }
        })

    with test.step("Verify makestep operational state"):
        data = target.get_data("/ietf-ntp:ntp")
        makestep = data.get("ntp", {}).get("makestep", {})

        assert makestep, f"makestep should be present in operational: {data}"
        assert makestep.get("threshold") == 1.0, \
            f"Expected threshold 1.0, got {makestep.get('threshold')}"
        assert makestep.get("limit") == 3, \
            f"Expected limit 3, got {makestep.get('limit')}"

    with test.step("Configure custom makestep values"):
        target.put_config_dicts({
            "ietf-ntp": {
                "ntp": {
                    "refclock-master": {
                        "master-stratum": 10
                    },
                    "infix-ntp:makestep": {
                        "threshold": 2.5,
                        "limit": 1
                    }
                }
            }
        })

    with test.step("Verify custom makestep operational state"):
        data = target.get_data("/ietf-ntp:ntp")
        makestep = data.get("ntp", {}).get("makestep", {})

        assert makestep.get("threshold") == 2.5, \
            f"Expected threshold 2.5, got {makestep.get('threshold')}"
        assert makestep.get("limit") == 1, \
            f"Expected limit 1, got {makestep.get('limit')}"

    with test.step("Delete makestep configuration"):
        target.delete_xpath("/ietf-ntp:ntp/infix-ntp:makestep")

    with test.step("Verify makestep removed from operational state"):
        data = target.get_data("/ietf-ntp:ntp")
        makestep = data.get("ntp", {}).get("makestep", {})

        assert not makestep, \
            f"makestep should not be present after deletion, got: {makestep}"

    test.succeed()
