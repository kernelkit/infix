#!/usr/bin/env python3
"""Link-down alarm

A veth pair gives a link whose carrier the test controls from the
configuration: disabling one end takes carrier away from the other.
The surviving end must raise a link-down alarm once the hold-off has
passed, and the alarm must clear when the peer comes back.  The
disabled end itself must not alarm, it was taken down on purpose.
"""
import infamy
from infamy import iface
from infamy.util import parallel, until

ALARMS = "/ietf-alarms:alarms"
LINK_DOWN = "infix-alarms:link-down"


def link_alarm(target, ifname):
    resource = f"/ietf-interfaces:interfaces/interface[name='{ifname}']"
    data = target.get_data(f"{ALARMS}/alarm-list")
    for alarm in data["alarms"]["alarm-list"].get("alarm", []):
        if alarm["resource"] == resource and alarm["alarm-type-id"] == LINK_DOWN:
            return alarm
    return None


def is_true(val):
    return str(val).lower() == "true"


def veth(enabled_b):
    return {
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    {
                        "name": "veth0a",
                        "type": "infix-if-type:veth",
                        "enabled": True,
                        "infix-interfaces:veth": {"peer": "veth0b"},
                    },
                    {
                        "name": "veth0b",
                        "type": "infix-if-type:veth",
                        "enabled": enabled_b,
                        "infix-interfaces:veth": {"peer": "veth0a"},
                    },
                ]
            }
        }
    }


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target, tgtssh = parallel(lambda: env.attach("target", "mgmt"),
                                  lambda: env.attach("target", "mgmt", "ssh"))

    with test.step("Configure veth pair veth0a and veth0b, both enabled"):
        target.put_config_dicts(veth(True))

    with test.step("Verify veth0a has link and no alarm"):
        until(lambda: iface.get_param(target, "veth0a", "oper-status") == "up")
        assert link_alarm(target, "veth0a") is None

    with test.step("Disable veth0b"):
        target.put_config_dicts(veth(False))

    with test.step("Verify link-down alarm on veth0a, major and not cleared"):
        def raised():
            alarm = link_alarm(target, "veth0a")
            return alarm and alarm["perceived-severity"] == "major" \
                and not is_true(alarm["is-cleared"])
        until(raised, attempts=20)

    with test.step("Verify no link-down alarm on veth0b, it was disabled on purpose"):
        assert link_alarm(target, "veth0b") is None

    with test.step("Verify 'show alarms' lists veth0a as active"):
        out = tgtssh.runsh('klish -c "show alarms"').stdout
        assert "veth0a" in out and "link-down" in out and "active" in out, out

    with test.step("Enable veth0b"):
        target.put_config_dicts(veth(True))

    with test.step("Verify link-down alarm on veth0a is cleared"):
        def cleared():
            alarm = link_alarm(target, "veth0a")
            return alarm and is_true(alarm["is-cleared"])
        until(cleared)

    with test.step("Remove veth pair"):
        target.put_config_dicts({}, delete=["/ietf-interfaces:interfaces/interface[name='veth0a']",
                                            "/ietf-interfaces:interfaces/interface[name='veth0b']"])

    with test.step("Purge cleared alarms"):
        target.call_action_output(f"{ALARMS}/alarm-list/purge-alarms",
                                  {"alarm-clearance-status": "cleared"})

    test.succeed()
