#!/usr/bin/env python3
"""PTP port fault recovery

Verify that the PTP port state machine correctly detects a link fault
and recovers to time-receiver state once the link is restored.

Two Ordinary Clocks are connected back-to-back.  Once the time receiver
has converged, the grandmaster's data interface is disabled.  The time
receiver must leave time-receiver state within a short timeout.  When
the interface is re-enabled, the time receiver must return to
time-receiver state and its offset must converge again to within the
configured threshold.
"""

import infamy
import infamy.ptp as ptp
from infamy import until


def configure_oc(iface, ip, priority1, client_only, dm="e2e"):
    return {
        "ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": iface,
                    "enabled": True,
                    "ipv4": {
                        "address": [{"ip": ip, "prefix-length": 30}]
                    }
                }]
            }
        },
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
                            "infix-ptp:profile": "ieee1588",
                            "time-receiver-only": client_only,
                        },
                        "ports": {
                            "port": [{
                                "port-index": 1,
                                "underlying-interface": iface,
                                "port-ds": {
                                    "delay-mechanism": dm,
                                    "log-announce-interval": -2,
                                    "announce-receipt-timeout": 2,
                                    "log-sync-interval": -2,
                                }
                            }]
                        }
                    }]
                }
            }
        }
    }


def set_iface_enabled(target, iface, enabled):
    target.put_config_dict("ietf-interfaces", {
        "interfaces": {
            "interface": [{
                "name": iface,
                "enabled": enabled,
            }]
        }
    })


class ArgumentParser(infamy.ArgumentParser):
    def __init__(self):
        super().__init__()
        self.args.add_argument("--threshold-ns", type=int, default=None)


with infamy.Test() as test:
    with test.step("Set up topology and attach to DUTs"):
        arg = ArgumentParser()
        env = infamy.Env(args=arg)
        gm       = env.attach("gm",       "mgmt")
        receiver = env.attach("receiver", "mgmt")

        _, gm_iface       = env.ltop.xlate("gm",       "data")
        _, receiver_iface = env.ltop.xlate("receiver", "data")
        threshold_ns = env.args.threshold_ns or ptp.default_threshold(env, "receiver")

    with test.step("Configure grandmaster (priority1=1) and time receiver (client-only)"):
        gm.put_config_dicts(configure_oc(gm_iface, "192.168.100.1",
                                         priority1=1, client_only=False))
        receiver.put_config_dicts(configure_oc(receiver_iface, "192.168.100.2",
                                               priority1=128, client_only=True))

    with test.step("Wait for initial convergence"):
        until(lambda: ptp.is_time_receiver(receiver) and ptp.has_converged(receiver, threshold_ns),
              attempts=120)

    with test.step("Disable grandmaster data interface to trigger fault"):
        set_iface_enabled(gm, gm_iface, False)

    with test.step("Verify time receiver leaves time-receiver state"):
        until(lambda: not ptp.is_time_receiver(receiver), attempts=30)

    with test.step("Re-enable grandmaster data interface"):
        set_iface_enabled(gm, gm_iface, True)

    with test.step("Wait for time receiver to return to time-receiver state after recovery"):
        until(lambda: ptp.is_time_receiver(receiver), attempts=120)

    with test.step("Wait for offset to re-converge"):
        until(lambda: ptp.has_converged(receiver, threshold_ns), attempts=120)

    test.succeed()
