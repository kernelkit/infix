#!/usr/bin/env python3
"""PTP BTCA grandmaster election

Verify that the Best TimeTransmitter Clock Algorithm (BTCA) selects the clock
with the lowest `priority1` as grandmaster, and that a change of `priority1`
at runtime triggers a new election with the correct result.

Two Ordinary Clocks are connected back-to-back.  Both announce themselves as
potential grandmasters.  In round one, *alpha* holds `priority1=1` and wins
the election; *beta* (`priority1=128`) becomes the time receiver.  In round
two, *alpha* is reconfigured to priority1=200 without restarting; the BTCA
re-runs and beta wins, becoming the new grandmaster.  The test verifies that
alpha's `parent-ds` `grandmaster-identity` changes to beta's `clock-identity`,
confirming that the re-election is reflected in the operational datastore.

Announce intervals are reduced to 250 ms (`log-announce-interval -2`) and the
announce receipt timeout to 2 intervals (500 ms) to make re-election complete
in roughly one second rather than the default three.

The test is run for both IEEE 1588-2019 (UDP/IPv4, E2E) and IEEE 802.1AS
(Layer 2, P2P) profiles.
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


def configure_oc(iface, priority1, profile, ip=None):
    iface_cfg = {"name": iface, "enabled": True}
    if profile == "ieee1588":
        iface_cfg["ipv4"] = {"address": [{"ip": ip, "prefix-length": 30}]}

    port_ds = {
        "log-announce-interval": -2,
        "announce-receipt-timeout": 2,
        "log-sync-interval": -2,
    }
    if profile == "ieee1588":
        port_ds["delay-mechanism"] = "e2e"

    return {
        "ietf-interfaces": {
            "interfaces": {"interface": [iface_cfg]}
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
                            "infix-ptp:profile": profile,
                            "time-receiver-only": False,
                        },
                        "ports": {
                            "port": [{
                                "port-index": 1,
                                "underlying-interface": iface,
                                "port-ds": port_ds,
                            }]
                        }
                    }]
                }
            }
        }
    }


with infamy.Test() as test:
    with test.step("Set up topology and attach to DUTs"):
        arg = ArgumentParser()
        env = infamy.Env(args=arg)
        profile = env.args.profile
        alpha = env.attach("alpha", "mgmt")
        beta  = env.attach("beta",  "mgmt")

        _, if_alpha = env.ltop.xlate("alpha", "data")
        _, if_beta  = env.ltop.xlate("beta",  "data")

    with test.step(f"Configure both DUTs ({profile}); alpha has lower priority1"):
        alpha.put_config_dicts(configure_oc(if_alpha, priority1=1,
                                            profile=profile, ip="192.168.100.1"))
        beta.put_config_dicts(configure_oc(if_beta, priority1=128,
                                           profile=profile, ip="192.168.100.2"))

    with test.step("Verify initial election: alpha is grandmaster, beta is time receiver"):
        parallel(lambda: until(lambda: ptp.is_own_gm(alpha), attempts=60),
                 lambda: until(lambda: ptp.is_time_receiver(beta), attempts=60))

    with test.step("Reconfigure alpha with worse priority1=200"):
        alpha.put_config_dicts(configure_oc(if_alpha, priority1=200,
                                            profile=profile, ip="192.168.100.1"))

    with test.step("Verify beta wins re-election (is own grandmaster)"):
        until(lambda: ptp.is_own_gm(beta), attempts=30)

    with test.step("Verify alpha tracks beta as grandmaster"):
        until(lambda: ptp.grandmaster_identity(alpha) == ptp.clock_identity(beta),
              attempts=30)

    test.succeed()
