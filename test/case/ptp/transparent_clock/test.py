#!/usr/bin/env python3
"""PTP transparent clock

Verify that an E2E or P2P Transparent Clock (TC) passes timing transparently
through a hardware switch without adding a boundary-clock hop, and that the
downstream time receiver converges to the grandmaster's time.

Three nodes are connected in a chain: a grandmaster Ordinary Clock
(`priority1=1`), a Transparent Clock, and a time-receiver Ordinary Clock
(`priority1=128`).

The TC updates the correction field in each Sync and Delay_Req message to
account for its own residence time.  Because a TC is transparent, the time
receiver's `steps-removed` counter must equal 1 — unlike a Boundary Clock,
which would give 2.  A TC passes ANNOUNCE messages unchanged (`stepsRemoved=0`
from the GM), and the time receiver adds 1 when it stores the value in
`currentDS`, giving a total of 1.  A BC increments `stepsRemoved` to 1 before
forwarding, and the receiver adds 1 more, giving 2.  The time receiver's offset must converge within the configured threshold
(default is tighter when the topology provides hardware timestamping links).

The delay mechanism (E2E or P2P) is controlled by the test suite for
IEEE 1588 runs.  When the profile is IEEE 802.1AS the delay mechanism is
always P2P (mandated by the standard) and Layer 2 transport is used.
"""

import infamy
import infamy.ptp as ptp
from infamy import until


class ArgumentParser(infamy.ArgumentParser):
    def __init__(self):
        super().__init__()
        self.args.add_argument("--profile", default="ieee1588",
                               choices=["ieee1588", "ieee802-dot1as"])
        self.args.add_argument("--delay-mechanism", default="e2e",
                               choices=["e2e", "p2p"])
        self.args.add_argument("--threshold-ns", type=int, default=None)


def configure_oc(iface, priority1, profile, client_only=False, ip=None, dm="e2e"):
    iface_cfg = {"name": iface, "enabled": True}
    if profile == "ieee1588":
        iface_cfg["ipv4"] = {"address": [{"ip": ip, "prefix-length": 30}]}

    port_ds = {
        "log-announce-interval": -2,
        "announce-receipt-timeout": 2,
        "log-sync-interval": -2,
    }
    if profile == "ieee1588":
        port_ds["delay-mechanism"] = dm

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
                            "time-receiver-only": client_only,
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


def configure_tc(uplink_iface, dnlink_iface, profile, dm="e2e",
                 uplink_ip=None, dnlink_ip=None):
    if profile == "ieee802-dot1as":
        instance_type = "p2p-tc"
    else:
        instance_type = "p2p-tc" if dm == "p2p" else "e2e-tc"

    ifaces = [{"name": uplink_iface, "enabled": True},
              {"name": dnlink_iface, "enabled": True}]
    if profile == "ieee1588":
        ifaces[0]["ipv4"] = {"address": [{"ip": uplink_ip, "prefix-length": 30}]}
        ifaces[1]["ipv4"] = {"address": [{"ip": dnlink_ip, "prefix-length": 30}]}

    return {
        "ietf-interfaces": {
            "interfaces": {"interface": ifaces}
        },
        "ieee1588-ptp-tt": {
            "ptp": {
                "instances": {
                    "instance": [{
                        "instance-index": 0,
                        "default-ds": {
                            "instance-type": instance_type,
                            "domain-number": 0,
                            "infix-ptp:profile": profile,
                        },
                        "ports": {
                            "port": [
                                {
                                    "port-index": 1,
                                    "underlying-interface": uplink_iface,
                                    "port-ds": {"log-sync-interval": -2},
                                },
                                {
                                    "port-index": 2,
                                    "underlying-interface": dnlink_iface,
                                    "port-ds": {"log-sync-interval": -2},
                                }
                            ]
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
        profile  = env.args.profile
        dm       = "p2p" if profile == "ieee802-dot1as" else env.args.delay_mechanism
        gm       = env.attach("gm",       "mgmt")
        tc       = env.attach("tc",       "mgmt")
        receiver = env.attach("receiver", "mgmt")

        gm_iface       = gm["data"]
        tc_uplink      = tc["uplink"]
        tc_dnlink      = tc["dnlink"]
        receiver_iface = receiver["data"]
        threshold_ns = env.args.threshold_ns or ptp.default_threshold(env, "tc")

    with test.step(f"Configure grandmaster (OC, priority1=1, {dm})"):
        gm.put_config_dicts(configure_oc(gm_iface, priority1=1,
                                         profile=profile, ip="192.168.100.1", dm=dm))

    with test.step(f"Configure transparent clock ({dm}-tc, {profile})"):
        tc.put_config_dicts(configure_tc(tc_uplink, tc_dnlink, profile=profile, dm=dm,
                                         uplink_ip="192.168.100.2",
                                         dnlink_ip="192.168.101.1"))

    with test.step("Configure time receiver (OC, priority1=128, client-only)"):
        receiver.put_config_dicts(configure_oc(receiver_iface, priority1=128,
                                               profile=profile, client_only=True,
                                               ip="192.168.101.2", dm=dm))

    with test.step("Wait for grandmaster port to become time-transmitter"):
        until(lambda: ptp.is_time_transmitter(gm), attempts=60)

    with test.step("Wait for time receiver to reach time-receiver state"):
        until(lambda: ptp.is_time_receiver(receiver), attempts=60)

    with test.step("Verify time receiver steps-removed equals 1"):
        until(lambda: ptp.steps_removed(receiver) == 1, attempts=60)

    with test.step("Wait for time receiver offset to converge"):
        until(lambda: ptp.has_converged(receiver, threshold_ns), attempts=180)

    test.succeed()
