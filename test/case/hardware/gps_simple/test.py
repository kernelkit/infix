#!/usr/bin/env python3
"""GPS receiver basic test

Verify that two simulated GPS receivers are detected and report valid
fixes via the ietf-hardware operational datastore.

The test injects NMEA sentences through QEMU pipe chardev FIFOs,
which appear as virtio serial ports inside the guest.
"""
import infamy
import infamy.gps as gps
from infamy.util import until, wait_boot

# Fun facts: The top of mount everest
test_lat = 27.9881
test_lon = 86.9250
test_alt = 8848.86


def _near(a, b, tol):
    return abs(a - b) <= tol

def verify_position(target, name="gps0"):
    state = gps.get_gps_state(target, name)

    try:
        lat = float(state["latitude"])
        lon = float(state["longitude"])
        alt = float(state["altitude"])
    except (KeyError, TypeError, ValueError):
        test.fail()

    if not _near(lat, test_lat, 0.01):
        test.fail()
    if not _near(lon, test_lon, 0.01):
        test.fail()
    if not _near(alt, test_alt, 100):
        test.fail()

    try:
        sat_used = int(state["satellites-used"])
    except (KeyError, TypeError, ValueError):
        test.fail()

    if sat_used != 8:
        test.fail()

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

        phys_name = env.ltop.mapping["target"][None]

        if not target.has_feature("infix-hardware", "gps"):
            test.skip()

        # These are hacks, will only work on virtual devices
        pipe0 = f"/tmp/{phys_name}-gps"
        pipe1 = f"/tmp/{phys_name}-gps1"

    with gps.NMEAGenerator(pipe0, lat=test_lat, lon=test_lon, alt=test_alt), \
         gps.NMEAGenerator(pipe1, lat=test_lat, lon=test_lon, alt=test_alt):

        with test.step("Configure GPS hardware components"):
            target.put_config_dicts({"ietf-hardware": {
                "hardware": {
                    "component": [
                        {
                            "name": "gps0",
                            "class": "infix-hardware:gps",
                            "infix-hardware:gps-receiver": {}
                        },
                        {
                            "name": "gps1",
                            "class": "infix-hardware:gps",
                            "infix-hardware:gps-receiver": {}
                        }
                    ]
                }
            }})

        with test.step("Verify both GPS receivers are activated"):
            until(lambda: gps.is_activated(target, "gps0"), attempts=500)
            until(lambda: gps.is_activated(target, "gps1"), attempts=500)

        with test.step("Verify both GPS receivers report full position data"):
            until(lambda: gps.has_position(target, "gps0"), attempts=60)
            until(lambda: gps.has_position(target, "gps1"), attempts=60)

        with test.step("Verify both GPS receivers have a satellite fix"):
            until(lambda: gps.has_fix(target, "gps0"), attempts=60)
            until(lambda: gps.has_fix(target, "gps1"), attempts=60)

        with test.step("Verify gps0 position is near the coordinates"):
            verify_position(target, "gps0")

        with test.step("Verify gps1 position is near the coordinates"):
            verify_position(target, "gps1")

        with test.step("Save the configuration to startup configuration and reboot"):
            target.startup_override()
            target.copy("running", "startup")
            target.reboot()
            if not wait_boot(target, env):
                test.fail()
            target = env.attach("target", "mgmt", test_reset=False)

        with test.step("Verify both GPS receivers are activated"):
            until(lambda: gps.is_activated(target, "gps0"), attempts=500)
            until(lambda: gps.is_activated(target, "gps1"), attempts=500)

        with test.step("Verify both GPS receivers report full position data"):
            until(lambda: gps.has_position(target, "gps0"), attempts=60)
            until(lambda: gps.has_position(target, "gps1"), attempts=60)

        with test.step("Verify both GPS receivers have a satellite fix"):
            until(lambda: gps.has_fix(target, "gps0"), attempts=60)
            until(lambda: gps.has_fix(target, "gps1"), attempts=60)

        with test.step("Verify gps0 position is near the coordinates"):
            verify_position(target, "gps0")

        with test.step("Verify gps1 position is near the coordinates"):
            verify_position(target, "gps1")

    test.succeed()
