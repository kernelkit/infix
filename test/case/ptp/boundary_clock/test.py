#!/usr/bin/env python3
"""PTP boundary clock

Verify that a Boundary Clock (BC) correctly receives time on one port and
distributes it on another, and that the downstream time receiver sees exactly
one additional hop (`steps-removed=2`).

Three nodes are connected in a chain: a grandmaster Ordinary Clock (OC,
`priority1=1`), a Boundary Clock (BC, `priority1=64`) with two ports, and a
time-receiver Ordinary Clock (OC, `priority1=128`).

The BC's upstream port (toward the GM) must reach time-receiver state; the
downstream port (toward the time receiver) must reach time-transmitter state.
The time receiver's `steps-removed` counter must equal 2: the BC increments
`steps-removed` to 1 in the ANNOUNCE messages it forwards, and the time
receiver adds 1 more when it stores the value in its `currentDS`.  An OC
directly connected to the GM shows 1, so the BC adds exactly one extra hop.

The test is run for both IEEE 1588-2019 (UDP/IPv4, E2E) and IEEE 802.1AS
(Layer 2, P2P) profiles.
"""

import infamy
import infamy.ptp as ptp
from infamy import until


class ArgumentParser(infamy.ArgumentParser):
    def __init__(self):
        super().__init__()
        self.args.add_argument("--profile", default="ieee1588",
                          choices=["ieee1588", "ieee802-dot1as"])
        self.args.add_argument("--threshold-ns", type=int, default=None)


def configure_oc(iface, priority1, profile, client_only=False, ip=None):
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


def configure_bc(uplink_iface, dnlink_iface, profile,
                 uplink_ip=None, dnlink_ip=None, priority1=64):
    ifaces = [{"name": uplink_iface, "enabled": True},
              {"name": dnlink_iface, "enabled": True}]
    if profile == "ieee1588":
        ifaces[0]["ipv4"] = {"address": [{"ip": uplink_ip, "prefix-length": 30}]}
        ifaces[1]["ipv4"] = {"address": [{"ip": dnlink_ip, "prefix-length": 30}]}

    port_ds = {
        "log-announce-interval": -2,
        "announce-receipt-timeout": 2,
        "log-sync-interval": -2,
    }
    if profile == "ieee1588":
        port_ds["delay-mechanism"] = "e2e"

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
                            "instance-type": "bc",
                            "domain-number": 0,
                            "priority1": priority1,
                            "priority2": 128,
                            "infix-ptp:profile": profile,
                        },
                        "ports": {
                            "port": [
                                {
                                    "port-index": 1,
                                    "underlying-interface": uplink_iface,
                                    "port-ds": port_ds,
                                },
                                {
                                    "port-index": 2,
                                    "underlying-interface": dnlink_iface,
                                    "port-ds": port_ds,
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
        gm       = env.attach("gm",       "mgmt")
        bc       = env.attach("bc",       "mgmt")
        receiver = env.attach("receiver", "mgmt")

        _, gm_iface    = env.ltop.xlate("gm",       "data")
        _, bc_uplink   = env.ltop.xlate("bc",       "uplink")
        _, bc_dnlink   = env.ltop.xlate("bc",       "dnlink")
        _, recv_iface  = env.ltop.xlate("receiver", "data")
        threshold_ns = env.args.threshold_ns or ptp.default_threshold(env, "receiver", hops=2)

    with test.step(f"Configure grandmaster (OC, {profile}, priority1=1) and boundary clock (BC, {profile}, priority1=64, two ports)"):
        gm.put_config_dicts(configure_oc(gm_iface, priority1=1,
                                         profile=profile, ip="192.168.100.1"))
        bc.put_config_dicts(configure_bc(bc_uplink, bc_dnlink, profile=profile,
                                         uplink_ip="192.168.100.2",
                                         dnlink_ip="192.168.101.1"))

    with test.step("Wait for BC uplink port to become time-receiver"):
        until(lambda: ptp.is_time_receiver(bc, port_idx=1), attempts=60)

    with test.step("Wait for BC dnlink port to become time-transmitter"):
        until(lambda: ptp.is_time_transmitter(bc, port_idx=2), attempts=60)

    with test.step("Wait for boundary clock offset to converge"):
        bc_threshold_ns = env.args.threshold_ns or ptp.default_threshold(env, "bc", hops=1)
        until(lambda: ptp.has_converged(bc, bc_threshold_ns), attempts=120)

    with test.step(f"Configure time receiver (OC, {profile}, priority1=128, client-only)"):
        receiver.put_config_dicts(configure_oc(recv_iface, priority1=128,
                                               profile=profile, client_only=True,
                                               ip="192.168.101.2"))

    with test.step("Wait for time receiver to reach time-receiver state"):
        until(lambda: ptp.is_time_receiver(receiver), attempts=60)

    with test.step("Verify time receiver steps-removed equals 2 (one BC hop)"):
        until(lambda: ptp.steps_removed(receiver) == 2, attempts=30)

    with test.step("Wait for time receiver offset to converge"):
        until(lambda: ptp.has_converged(receiver, threshold_ns), attempts=120)

    test.succeed()
