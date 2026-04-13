#!/usr/bin/env python3
"""PTP basic

Verify basic PTP operation end-to-end: clock configuration, port state
transitions, and clock servo convergence.

Two Ordinary Clocks are connected back-to-back.  The grandmaster is
configured with `priority1=1` so it always wins the BTCA election; the
time receiver is configured with `time-receiver-only` so it never
attempts to become grandmaster.  The test is run once per supported
profile, covering both IEEE 1588-2019 (UDP/IPv4, E2E) and IEEE 802.1AS
(Layer 2, P2P).
"""

import infamy
import infamy.ptp as ptp
from infamy import until
from infamy.util import parallel


class ArgumentParser(infamy.ArgumentParser):
    def __init__(self):
        super().__init__()
        self.args.add_argument("--profile", default="ieee1588",
                               choices=["ieee1588", "ieee802-dot1as"])
        self.args.add_argument("--threshold-ns", type=int, default=None)


def configure_oc(iface, priority1, client_only, profile, ip=None):
    config = {
        "ieee1588-ptp-tt": {
            "ptp": {
                "instances": {
                    "instance": [{
                        "instance-index": 0,
                        "default-ds": {
                            "instance-type": "oc",
                            "domain-number": 0,
                            "priority1": priority1,
                            "priority2": 128,
                            "infix-ptp:profile": profile,
                            "time-receiver-only": client_only,
                        },
                        "ports": {
                            "port": [{
                                "port-index": 1,
                                "underlying-interface": iface,
                            }]
                        }
                    }]
                }
            }
        }
    }

    # Always enable the underlying interface — ptp4l needs it up regardless
    # of profile. IEEE 1588 also needs an IPv4 address for UDP transport.
    iface_cfg = {"name": iface, "enabled": True}
    if profile == "ieee1588":
        iface_cfg["ipv4"] = {"address": [{"ip": ip, "prefix-length": 30}]}
    config["ietf-interfaces"] = {
        "interfaces": {"interface": [iface_cfg]}
    }

    # Fast timers: 250 ms announce/sync intervals speed up port state transitions
    # and convergence compared to the 1 s defaults.
    port_ds = {
        "log-announce-interval": -2,
        "announce-receipt-timeout": 2,
        "log-sync-interval": -2,
    }
    if profile == "ieee1588":
        port_ds["delay-mechanism"] = "e2e"
    config["ieee1588-ptp-tt"]["ptp"]["instances"]["instance"][0] \
        ["ports"]["port"][0]["port-ds"] = port_ds

    return config


with infamy.Test() as test:
    with test.step("Set up topology and attach to DUTs"):
        arg = ArgumentParser()
        env = infamy.Env(args=arg)
        profile  = env.args.profile
        gm       = env.attach("gm",       "mgmt")
        receiver = env.attach("receiver", "mgmt")

        _, gm_iface       = env.ltop.xlate("gm",       "data")
        _, receiver_iface = env.ltop.xlate("receiver", "data")
        threshold_ns = env.args.threshold_ns or ptp.default_threshold(env, "receiver")

    with test.step(f"Configure grandmaster (OC, {profile}, priority1=1) and time receiver ({profile}, priority1=128, client-only)"):
        gm.put_config_dicts(configure_oc(gm_iface, priority1=1,
                                         client_only=False, profile=profile,
                                         ip="192.168.100.1"))
        receiver.put_config_dicts(configure_oc(receiver_iface, priority1=128,
                                               client_only=True, profile=profile,
                                               ip="192.168.100.2"))

    with test.step("Wait for grandmaster and time receiver ports to reach active states"):
        parallel(lambda: until(lambda: ptp.is_time_transmitter(gm), attempts=60),
                 lambda: until(lambda: ptp.is_time_receiver(receiver), attempts=60))

    with test.step("Wait for time receiver offset to converge"):
        until(lambda: ptp.has_converged(receiver, threshold_ns), attempts=120)

    test.succeed()
