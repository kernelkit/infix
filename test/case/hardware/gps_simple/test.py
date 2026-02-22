#!/usr/bin/env python3
"""GPS receiver basic test

Verify that a simulated GPS receiver is detected and reports a valid
fix via the ietf-hardware operational datastore.

The test injects NMEA sentences through a QEMU pipe chardev FIFO,
which appears as a virtio serial port inside the guest.
"""
import infamy
import infamy.gps as gps
from infamy.util import until, wait_boot

lat=27.9881
lon=86.9250
alt=8848.86


def _near(a, b, tol):
    return abs(a - b) <= tol

def verify_position(target):
    state = gps.get_gps_state(target)

    try:
        lat = float(state["latitude"])
        lon = float(state["longitude"])
        alt = float(state["altitude"])
    except (KeyError, TypeError, ValueError):
        test.fail()

        if not _near(lat, lat, 1e-4):
            test.fail()
        if not _near(lon, lon, 1e-4):
            test.fail()
        if not _near(alt, alt, 0.2):
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

        # This is a hack, will only work on virtual devices
        pipe_path = f"/tmp/{phys_name}-gps"

    with gps.NMEAGenerator(
            pipe_path,
            lat=lat,
            lon=lon,
            alt=alt):


        with test.step("Configure GPS hardware component"):
            target.put_config_dict("ietf-hardware", {
                "hardware": {
                    "component": [{
                        "name": "gps0",
                        "class": "infix-hardware:gps",
                        "infix-hardware:gps-receiver": {}
                    }]
                }
            })

        with test.step("Verify GPS is activated"):
            until(lambda: gps.is_activated(target), attempts=500)

        with test.step("Verify GPS has a fix"):
            until(lambda: gps.has_fix(target), attempts=60)

        with test.step("Verify the position is near the coordinates you test with"):
            verify_position(target)

        with test.step("Save the configuration to startup configuration and reboot"):
            target.startup_override()
            target.copy("running", "startup")
            target.reboot()
            if not wait_boot(target, env):
                test.fail()
            target = env.attach("target", "mgmt", test_reset=False)

        with test.step("Verify GPS is activated"):
            until(lambda: gps.is_activated(target), attempts=500)

        with test.step("Verify GPS has a fix"):
            until(lambda: gps.has_fix(target), attempts=60)
        with test.step("Verify the position is near the coordinates you test with"):
            verify_position(target)
    test.succeed()
