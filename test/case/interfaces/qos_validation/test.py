#!/usr/bin/env python3
"""
QoS Configuration Validation

Verify that traffic class layouts the device cannot render are rejected
at commit time, with the running configuration left untouched:

 - a strict-priority class below a weighted class
 - weighted shares that do not sum to 100 percent
 - a transmission selection algorithm outside strict-priority and
   enhanced-transmission-selection
 - a traffic class beyond the port's class count
 - a priority mapped to a class beyond the port's class count

The last two apply on ports with fewer than eight classes.  A valid
layout must still be accepted afterwards.
"""
import infamy

STRICT = "ieee802-dot1q-types:strict-priority"
ETS = "ieee802-dot1q-types:enhanced-transmission-selection"
CBS = "ieee802-dot1q-types:credit-based-shaper"


def egress_config(port, egress):
    return {"ietf-interfaces": {
        "interfaces": {
            "interface": [{
                "name": port,
                "enabled": True,
                "infix-interfaces:qos": {"egress": egress}
            }]
        }
    }}


def must_reject(target, port, egress, what, *reasons):
    """The commit must fail, and the error must name one of the reasons"""
    try:
        target.put_config_dicts(egress_config(port, egress))
    except Exception as err:
        text = getattr(getattr(err, "response", None), "text", None) or str(err)
        matched = [reason for reason in reasons if reason in text]
        assert matched, f"{what} rejected for another reason:\n{text}"
        print(f"Rejected as expected: {matched[0]}")
        return
    raise AssertionError(f"{what} was accepted")


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, port = env.ltop.xlate("target", "data")
        num_tc = infamy.capability.Port(target, port).traffic_classes
        print(f"{port}: {num_tc} traffic classes")
        assert num_tc and num_tc >= 2, f"max-traffic-classes {num_tc}"
        top = num_tc - 1

    with test.step("Reject strict-priority class below a weighted class"):
        must_reject(target, port, {
            "traffic-class": [
                {"id": top, "algorithm": ETS, "bandwidth": 100},
                {"id": top - 1, "algorithm": STRICT},
            ]
        }, "strict class below weighted class",
           "strict-priority classes must be the highest-numbered")

    with test.step("Reject weighted shares that do not sum to 100"):
        # A must in the model, not the daemon: NETCONF relays the model's
        # message, the RESTCONF error body only says validation failed
        must_reject(target, port, {
            "traffic-class": [
                {"id": 1, "algorithm": ETS, "bandwidth": 60},
                {"id": 0, "algorithm": ETS, "bandwidth": 30},
            ]
        }, "shares summing to 90", "must sum to 100", "Validation failed")

    with test.step("Reject credit-based-shaper algorithm"):
        must_reject(target, port, {
            "traffic-class": [{"id": top, "algorithm": CBS}]
        }, "credit-based-shaper", "not supported, use strict-priority")

    if num_tc < 8:
        with test.step("Reject traffic class beyond the port's class count"):
            must_reject(target, port, {
                "traffic-class": [{"id": num_tc}]
            }, f"traffic class {num_tc} on a {num_tc} class port",
               f"traffic class {num_tc}, port has {num_tc} classes")

        with test.step("Reject priority mapped beyond the port's class count"):
            must_reject(target, port, {
                "traffic-class-table": {"priority7": num_tc},
            }, f"priority7 mapped to class {num_tc} on a {num_tc} class port",
               f"priority7 maps to traffic class {num_tc}, port has {num_tc} classes")

    with test.step("Accept a valid layout with strict classes above weighted"):
        target.put_config_dicts(egress_config(port, {
            "traffic-class": [
                {"id": top, "algorithm": STRICT},
                {"id": 1, "algorithm": ETS, "bandwidth": 67},
                {"id": 0, "algorithm": ETS, "bandwidth": 33},
            ]
        }))

    test.succeed()
