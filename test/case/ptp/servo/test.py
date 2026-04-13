#!/usr/bin/env python3
"""PTP servo step-threshold

Verify that configuring a non-zero `step-threshold` allows the clock servo
to correct a large time offset by stepping rather than slewing.

Two Ordinary Clocks are connected back-to-back using the IEEE 1588 profile.
After initial convergence the receiver is reconfigured with
`step-threshold=1.0 s` and ptp4l restarts.  Because the offset at restart
is near zero, `first_step_threshold` (ptp4l's per-startup step gate) does
not trigger, so the restart itself is convergence-neutral.

Once the receiver has re-locked, the grandmaster clock is stepped forward by
10 seconds using phc_ctl (hardware PHC) or the system clock (software
timestamping).  The 10-second offset exceeds the 1-second step-threshold, so
the servo steps the clock immediately and the receiver converges within a
few seconds.

Note: a negative test (verify that offset=10 s does *not* converge without
step-threshold) is not included here because it is unreliable across
platforms.  On physical hardware the kernel caps clock frequency adjustment
at ~500 ppm, making a 10-second slew take ~5.5 hours; on virtual clocks
(QEMU) no such limit applies and the servo can slew the offset away in
seconds.  Full negative coverage requires exposing first_step_threshold and
max_frequency in the YANG model — see TODO.org.
"""

import infamy
import infamy.ptp as ptp
from infamy import until
from infamy.util import parallel

STEP_SEC = 10


class ArgumentParser(infamy.ArgumentParser):
    def __init__(self):
        super().__init__()
        self.args.add_argument("--threshold-ns", type=int, default=None)


def configure_oc(iface, priority1, client, ip, step_threshold=None):
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
                            "infix-ptp:profile": "ieee1588",
                            "time-receiver-only": client,
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

    iface_cfg = {"name": iface, "enabled": True,
                 "ipv4": {"address": [{"ip": ip, "prefix-length": 30}]}}
    config["ietf-interfaces"] = {"interfaces": {"interface": [iface_cfg]}}

    port_ds = {
        "log-announce-interval": -2,
        "announce-receipt-timeout": 2,
        "log-sync-interval": -2,
        "delay-mechanism": "e2e",
    }
    inst = config["ieee1588-ptp-tt"]["ptp"]["instances"]["instance"][0]
    inst["ports"]["port"][0]["port-ds"] = port_ds

    if step_threshold is not None:
        inst["infix-ptp:servo"] = {"step-threshold": str(step_threshold)}

    return config


def step_clock(ssh, iface, seconds):
    """Step the PTP clock on the node owning iface forward by seconds.

    Uses phc_ctl on the hardware PHC when available (sysfs discovery);
    falls back to date(1) for software-timestamping nodes where ptp4l
    disciplines CLOCK_REALTIME directly.
    """
    rc = ssh.runsh(f"ls /sys/class/net/{iface}/device/ptp/ 2>/dev/null")
    phc = rc.stdout.strip().split()[0] if rc.returncode == 0 and rc.stdout.strip() else None
    if phc:
        ssh.runsh(f"phc_ctl /dev/{phc} adj {float(seconds)}")
    else:
        ssh.runsh(f"date -s @$(( $(date +%s) + {seconds} ))")


with infamy.Test() as test:
    with test.step("Set up topology and attach to DUTs"):
        arg = ArgumentParser()
        env = infamy.Env(args=arg)
        gm       = env.attach("gm",       "mgmt")
        receiver = env.attach("receiver", "mgmt")

        _, gm_iface       = env.ltop.xlate("gm",       "data")
        _, receiver_iface = env.ltop.xlate("receiver", "data")
        threshold_ns = env.args.threshold_ns or ptp.default_threshold(env, "receiver")

    with test.step("Configure grandmaster (OC, IEEE 1588, priority1=1) and time receiver"):
        gm.put_config_dicts(configure_oc(gm_iface, priority1=1,
                                         client=False, ip="192.168.100.1"))
        receiver.put_config_dicts(configure_oc(receiver_iface, priority1=128,
                                               client=True, ip="192.168.100.2"))

    with test.step("Wait for grandmaster and time receiver ports to reach active states"):
        def gm_ready():
            if not ptp.is_time_transmitter(gm):
                # print(f"{ptp.port_state_dbg(gm)}")
                return False
            return True

        def receiver_ready():
            if not ptp.is_time_receiver(receiver):
                # print(f"{ptp.port_state_dbg(receiver)}")
                return False
            return True

        parallel(lambda: until(gm_ready, attempts=60),
                 lambda: until(receiver_ready, attempts=60))

    with test.step("Wait for initial convergence"):
        until(lambda: ptp.has_converged(receiver, threshold_ns), attempts=120)

    with test.step("Reconfigure receiver with step-threshold=1.0 s"):
        # ptp4l restarts while the offset is near zero so first_step_threshold
        # does not trigger; the restart itself is convergence-neutral.
        receiver.put_config_dicts(configure_oc(receiver_iface, priority1=128,
                                               client=True, ip="192.168.100.2",
                                               step_threshold=1.0))
        until(lambda: ptp.has_converged(receiver, threshold_ns), attempts=60)

    with test.step(f"Inject {STEP_SEC}-second offset on grandmaster clock"):
        gmssh = env.attach("gm", "mgmt", "ssh")
        step_clock(gmssh, gm_iface, STEP_SEC)

    with test.step("Verify receiver converges by stepping (step-threshold=1.0 s)"):
        until(lambda: ptp.has_converged(receiver, threshold_ns), attempts=60)

    test.succeed()
