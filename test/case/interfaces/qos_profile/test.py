#!/usr/bin/env python3
"""
QoS Profiles

Define one QoS profile and put two ports on it: both must run the
profile's traffic class table and report the profile as the one in
use.  Change the profile's table and both ports must follow without
being touched themselves.  Deleting a profile that ports still use must
be rejected.  A port taken off the profile runs its own settings, and
once no port uses the profile it can be deleted.
"""
import infamy
import infamy.qos as qos
from infamy.util import until

PROFILE = "/ietf-system:system/infix-qos:qos/profile"


def runs(ssh, port, num_tc, table):
    return qos.scheduler_matches(qos.scheduler(ssh, port), num_tc, table[num_tc], strict=num_tc)


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        tgtssh = env.attach("target", "mgmt", "ssh")
        _, p0 = env.ltop.xlate("target", "data1")
        _, p1 = env.ltop.xlate("target", "data2")
        port0 = infamy.capability.Port(target, p0)
        port1 = infamy.capability.Port(target, p1)
        num_tc = min(port0.traffic_classes, port1.traffic_classes)

    with test.step("Define profile 'sr' with the ieee-sr traffic class table and put both ports on it"):
        target.put_config_dicts({
            "ietf-system": {"system": {"infix-qos:qos": {"profile": [{
                "name": "sr",
                "description": "reserved streams on top",
                "egress": {"traffic-class-table": {"preset": "ieee-sr"}},
            }]}}},
            "ietf-interfaces": {"interfaces": {"interface": [
                {"name": p0, "enabled": True, "infix-interfaces:qos": {"profile": "sr"}},
                {"name": p1, "enabled": True, "infix-interfaces:qos": {"profile": "sr"}},
            ]}},
        })
        until(lambda: runs(tgtssh, p0, num_tc, qos.TABLE_34_1))
        until(lambda: runs(tgtssh, p1, num_tc, qos.TABLE_34_1))

    with test.step("Verify both ports report profile 'sr' in use"):
        until(lambda: port0.profile == "sr" and port1.profile == "sr")

    with test.step("Change the profile to the ieee table, both ports must follow"):
        target.put_config_dicts({"ietf-system": {"system": {"infix-qos:qos": {"profile": [{
            "name": "sr", "egress": {"traffic-class-table": {"preset": "ieee"}},
        }]}}}})
        until(lambda: runs(tgtssh, p0, num_tc, qos.TABLE_8_5))
        until(lambda: runs(tgtssh, p1, num_tc, qos.TABLE_8_5))

    with test.step("Reject deleting the profile while ports use it"):
        qos.must_reject("delete of a referenced profile",
                        lambda: target.delete_xpath(f"{PROFILE}[name='sr']"))
        assert port0.profile == "sr" and port1.profile == "sr"

    with test.step("Take the first port off the profile and give it inline settings"):
        target.delete_xpath(f"/ietf-interfaces:interfaces/interface[name='{p0}']"
                            "/infix-interfaces:qos/profile")
        target.put_config_dicts({"ietf-interfaces": {"interfaces": {"interface": [
            {"name": p0, "infix-interfaces:qos": {"ingress": {"trust": "dscp"}}},
        ]}}})
        until(lambda: port0.profile is None and port1.profile == "sr")

    with test.step("Take the second port off the profile and delete it"):
        target.delete_xpath(f"/ietf-interfaces:interfaces/interface[name='{p1}']"
                            "/infix-interfaces:qos/profile")
        until(lambda: port1.profile is None)
        target.delete_xpath(f"{PROFILE}[name='sr']")

    test.succeed()
