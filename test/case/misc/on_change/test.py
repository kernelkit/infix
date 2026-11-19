#!/usr/bin/env python3
"""
On-change subscription

Verify that a client subscribed to changes in the operational datastore
is told within a second when an interface loses its link and when the
link returns, without polling.  A veth pair gives a link the test
controls: disabling one end takes the link away from the other.  One
subscription follows the operational state of the interfaces, a second
one the alarm list, where the lost link raises a link-down alarm that
is cleared when the link returns.
"""
import json

import infamy
from infamy import iface
from infamy.util import until

ALARMS = "/ietf-alarms:alarms"


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


def edits(event):
    update = event.get("ietf-yang-push:push-change-update", {})
    return update.get("datastore-changes", {}).get("yang-patch", {}).get("edit", [])


def oper_status(port, up):
    path = f"/ietf-interfaces:interfaces/interface[name='{port}']/oper-status"

    def match(event):
        return any(edit["target"] == path and
                   (edit.get("value", {}).get("ietf-interfaces:oper-status") == "up") == up
                   for edit in edits(event))

    return match


def alarm_cleared(name, cleared):
    def match(event):
        return any(f"interface[name='{name}']" in edit["target"] and
                   f"is-cleared: {cleared}" in json.dumps(edit.get("value")).replace('"', "")
                   for edit in edits(event))

    return match


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

    with test.step("Configure veth pair veth0a and veth0b, both enabled"):
        target.put_config_dicts(veth(True))

    with test.step("Verify veth0a is up"):
        until(lambda: iface.get_param(target, "veth0a", "oper-status") == "up")

    with test.step("Subscribe to changes of oper-status in the operational datastore"):
        status = target.subscribe({
            "ietf-yang-push:datastore": "ietf-datastores:operational",
            "ietf-yang-push:datastore-xpath-filter":
                "/ietf-interfaces:interfaces/interface/oper-status",
            "ietf-yang-push:on-change": {}
        })

    with test.step("Verify the subscription starts with the oper-status of all interfaces"):
        assert status.wait_for(lambda event: "ietf-yang-push:push-update" in event, 10)

    with test.step("Subscribe to changes of the alarm list in the operational datastore"):
        alarms = target.subscribe({
            "ietf-yang-push:datastore": "ietf-datastores:operational",
            "ietf-yang-push:datastore-xpath-filter": "/ietf-alarms:alarms/alarm-list",
            "ietf-yang-push:on-change": {}
        })

    with test.step("Verify the alarm subscription starts with the alarm list"):
        assert alarms.wait_for(lambda event: "ietf-yang-push:push-update" in event, 10)

    with test.step("Disable veth0b"):
        target.put_config_dicts(veth(False))

    with test.step("Verify the subscription reports veth0a no longer up within 1 second"):
        assert status.wait_for(oper_status("veth0a", False), 1)

    with test.step("Verify the alarm subscription reports a link-down alarm on veth0a within 15 seconds"):
        assert alarms.wait_for(alarm_cleared("veth0a", "false"), 15)

    with test.step("Enable veth0b"):
        target.put_config_dicts(veth(True))

    with test.step("Verify the subscription reports veth0a up within 1 second"):
        assert status.wait_for(oper_status("veth0a", True), 1)

    with test.step("Verify the alarm subscription reports the alarm on veth0a cleared within 2 seconds"):
        assert alarms.wait_for(alarm_cleared("veth0a", "true"), 2)

    with test.step("Remove veth pair"):
        target.put_config_dicts({}, delete=["/ietf-interfaces:interfaces/interface[name='veth0a']",
                                            "/ietf-interfaces:interfaces/interface[name='veth0b']"])

    with test.step("Purge cleared alarms"):
        target.call_action_output(f"{ALARMS}/alarm-list/purge-alarms",
                                  {"alarm-clearance-status": "cleared"})

    test.succeed()
