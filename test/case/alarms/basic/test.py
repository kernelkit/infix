#!/usr/bin/env python3
"""Alarm list, operator state, shelving and purge

Raise an external alarm with the raise-alarm operation and follow it
through the ietf-alarms alarm list: acknowledge it, clear it, shelve it
by configuration, and purge it.  A marker touched on the device shows
that a script raises an alarm the same way, and the CLI commands show,
acknowledge and clear it.  A device with unpatched ports has a link-down
alarm on each, so those are shelved first to keep the counts exact.
"""
import infamy
from infamy.util import parallel, until

ALARMS = "/ietf-alarms:alarms"
EXTERNAL = "infix-alarms:external"
ETH9 = "/ietf-interfaces:interfaces/interface[name='eth9']"


def entry(target, resource, container="alarm-list", key="alarm", kind=EXTERNAL):
    data = target.get_data(f"{ALARMS}/{container}")
    for alarm in data["alarms"][container].get(key, []):
        if alarm["resource"] == resource and alarm["alarm-type-id"] == kind:
            return alarm
    return None


def is_true(val):
    return str(val).lower() == "true"


def action(resource):
    return (f"{ALARMS}/alarm-list/alarm[resource='{resource}']"
            f"[alarm-type-id='{EXTERNAL}'][alarm-type-qualifier='']")


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target, tgtssh = parallel(lambda: env.attach("target", "mgmt"),
                                  lambda: env.attach("target", "mgmt", "ssh"))

    with test.step("Shelve link-down alarms on every interface of the target in shelf 'ports'"):
        data = target.get_data("/ietf-interfaces:interfaces")
        names = [iface["name"] for iface in data["interfaces"]["interface"]]
        target.put_config_dicts({
            "ietf-alarms": {
                "alarms": {
                    "control": {
                        "alarm-shelving": {
                            "shelf": [{
                                "name": "ports",
                                "resource": names,
                                "alarm-type": [{
                                    "alarm-type-id": "infix-alarms:link-down",
                                    "alarm-type-qualifier-match": ".*",
                                }],
                            }]
                        }
                    }
                }
            }
        })

    with test.step("Verify the login banner is silent"):
        out = tgtssh.runsh("sh -l -c true").stdout
        assert "active alarm" not in out, out

    with test.step("Raise external alarm 'backup' with severity critical"):
        target.call_dict("infix-alarms", {
            "raise-alarm": {
                "resource": "backup",
                "severity": "critical",
                "text": "Nightly backup failed",
            }
        })

    with test.step("Verify alarm 'backup' is listed, critical and not cleared"):
        def raised():
            alarm = entry(target, "backup")
            return alarm and alarm["perceived-severity"] == "critical" \
                and not is_true(alarm["is-cleared"]) \
                and alarm["alarm-text"] == "Nightly backup failed"
        until(raised, attempts=10)

    with test.step("Verify summary counts one critical alarm, not cleared"):
        data = target.get_data(f"{ALARMS}/summary")
        for row in data["alarms"]["summary"]["alarm-summary"]:
            if row["severity"] == "critical":
                assert str(row["not-cleared"]) == "1", row
                break
        else:
            test.fail()

    with test.step("Acknowledge alarm 'backup' with the text 'On it'"):
        target.call_action(f"{action('backup')}/set-operator-state",
                           {"state": "ack", "text": "On it"})

    with test.step("Verify alarm 'backup' has an operator state change 'ack' by admin"):
        def acked():
            alarm = entry(target, "backup")
            changes = alarm.get("operator-state-change", []) if alarm else []
            return any(c["state"] == "ack" and c.get("text") == "On it"
                       and c.get("operator") == "admin" for c in changes)
        until(acked, attempts=10)

    with test.step("Clear external alarm 'backup'"):
        target.call_dict("infix-alarms", {"clear-alarm": {"resource": "backup"}})

    with test.step("Verify alarm 'backup' is cleared but still listed"):
        def cleared():
            alarm = entry(target, "backup")
            return alarm and is_true(alarm["is-cleared"])
        until(cleared, attempts=10)

    with test.step("Touch marker /run/alarm/active/external/disk-full on target"):
        tgtssh.runsh("sudo touch /run/alarm/active/external/disk-full")

    with test.step("Verify alarm 'disk-full' is listed with severity indeterminate"):
        def touched():
            alarm = entry(target, "disk-full")
            return alarm and alarm["perceived-severity"] == "indeterminate" \
                and not is_true(alarm["is-cleared"])
        until(touched, attempts=10)

    with test.step("Add an alarm profile raising external alarms on 'disk-full' to warning"):
        target.put_config_dicts({
            "ietf-alarms": {
                "alarms": {
                    "alarm-profile": [{
                        "alarm-type-id": EXTERNAL,
                        "alarm-type-qualifier-match": "",
                        "resource": "disk-full",
                        "description": "Known",
                        "alarm-severity-assignment-profile": {
                            "severity-level": ["warning"],
                        },
                    }]
                }
            }
        })

    with test.step("Verify alarm 'disk-full' now has perceived severity warning"):
        def assigned():
            alarm = entry(target, "disk-full")
            return alarm and alarm["perceived-severity"] == "warning"
        until(assigned, attempts=10)

    with test.step("Remove the alarm profile"):
        target.put_config_dicts({}, delete=[f"{ALARMS}/alarm-profile[alarm-type-id='{EXTERNAL}']"
                                            "[alarm-type-qualifier-match=''][resource='disk-full']"])

    with test.step("Verify alarm 'disk-full' is back at severity indeterminate"):
        until(lambda: (entry(target, "disk-full") or {}).get("perceived-severity") == "indeterminate",
              attempts=10)

    with test.step("Verify 'show alarms' lists 'disk-full' as active"):
        def shown():
            out = tgtssh.runsh('klish -c "show alarms"').stdout
            return "disk-full" in out and "active" in out
        until(shown, attempts=10)

    with test.step("Verify the login banner counts one active alarm"):
        out = tgtssh.runsh("sh -l -c true").stdout
        assert "1 active alarm(s)" in out, out

    with test.step("Verify 'show alarms disk-full' shows the alarm in full"):
        out = tgtssh.runsh('klish -c "show alarms disk-full"').stdout
        assert "disk-full  external" in out and "Severity" in out, out

    with test.step("Acknowledge alarm 'disk-full' from the CLI with the text 'Known'"):
        tgtssh.runsh('klish -c "alarm ack disk-full text Known"')

    with test.step("Verify alarm 'disk-full' has an operator state change 'ack' by admin"):
        def disk_acked():
            alarm = entry(target, "disk-full")
            changes = alarm.get("operator-state-change", []) if alarm else []
            return any(c["state"] == "ack" and c.get("text") == "Known"
                       and c.get("operator") == "admin" for c in changes)
        until(disk_acked, attempts=10)

    with test.step("Touch marker /run/alarm/active/link-down/eth9 on target"):
        tgtssh.runsh("sudo mkdir -p /run/alarm/active/link-down && "
                     "sudo touch /run/alarm/active/link-down/eth9")

    with test.step("Verify the link-down alarm names eth9 by its instance-identifier"):
        until(lambda: entry(target, ETH9, kind="infix-alarms:link-down") is not None,
              attempts=10)

    with test.step("Close alarm 'eth9' from the CLI without giving its type"):
        tgtssh.runsh('klish -c "alarm close eth9 text Spare"')

    with test.step("Verify alarm 'eth9' has an operator state change 'closed'"):
        def eth9_closed():
            alarm = entry(target, ETH9, kind="infix-alarms:link-down")
            changes = alarm.get("operator-state-change", []) if alarm else []
            return any(c["state"] == "closed" and c.get("text") == "Spare"
                       for c in changes)
        until(eth9_closed, attempts=10)

    with test.step("Verify the login banner leaves the closed 'eth9' out"):
        out = tgtssh.runsh("sh -l -c true").stdout
        assert "1 active alarm(s)" in out, out

    with test.step("Remove marker /run/alarm/active/link-down/eth9 on target"):
        tgtssh.runsh("sudo rm /run/alarm/active/link-down/eth9")

    with test.step("Set update source to the unreachable http://192.0.2.1/releases.atom"):
        target.put_config_dicts({
            "ietf-system": {
                "system": {
                    "infix-system:software": {
                        "update-url": "http://192.0.2.1/releases.atom",
                    }
                }
            }
        })

    with test.step("Run the update check on target"):
        tgtssh.runsh("sudo check-update")

    with test.step("Verify alarm 'software' of type update-check-failed is listed, minor"):
        def check_failed():
            alarm = entry(target, "software", kind="infix-alarms:update-check-failed")
            return alarm and alarm["perceived-severity"] == "minor" \
                and not is_true(alarm["is-cleared"])
        until(check_failed, attempts=10)

    with test.step("Remove marker /run/alarm/active/update-check-failed/software on target"):
        tgtssh.runsh("sudo rm /run/alarm/active/update-check-failed/software")

    with test.step("Shelve alarms for resource 'disk-full'"):
        target.put_config_dicts({
            "ietf-alarms": {
                "alarms": {
                    "control": {
                        "alarm-shelving": {
                            "shelf": [{
                                "name": "known-full-disk",
                                "resource": ["disk-full"],
                            }]
                        }
                    }
                }
            }
        })

    with test.step("Verify alarm 'disk-full' moved to the shelved alarms"):
        def shelved():
            alarm = entry(target, "disk-full", "shelved-alarms", "shelved-alarm")
            return alarm and alarm["shelf-name"] == "known-full-disk" \
                and entry(target, "disk-full") is None
        until(shelved, attempts=10)

    with test.step("Verify 'show alarms' hides 'disk-full' and 'show alarms shelved' lists it"):
        out = tgtssh.runsh('klish -c "show alarms"').stdout
        assert "disk-full" not in out and "shelved, see 'show alarms shelved'" in out, out
        out = tgtssh.runsh('klish -c "show alarms shelved"').stdout
        assert "disk-full" in out and "known-full-disk" in out, out

    with test.step("Verify the login banner is silent while 'disk-full' is shelved"):
        out = tgtssh.runsh("sh -l -c true").stdout
        assert "active alarm" not in out, out

    with test.step("Verify the CLI refuses to acknowledge the shelved 'disk-full'"):
        out = tgtssh.runsh('klish -c "alarm ack disk-full"').stdout
        assert "shelved by 'known-full-disk'" in out, out

    with test.step("Remove shelf 'known-full-disk'"):
        target.put_config_dicts({}, delete=[f"{ALARMS}/control/alarm-shelving/shelf[name='known-full-disk']"])

    with test.step("Verify alarm 'disk-full' is back in the alarm list"):
        until(lambda: entry(target, "disk-full") is not None, attempts=10)

    with test.step("Close every active alarm with one set-operator-state on the alarm list"):
        output = target.call_action_output(f"{ALARMS}/alarm-list/infix-alarms:set-operator-state",
                                           {"alarm-clearance-status": "not-cleared",
                                            "state": "closed", "text": "Sweep"})
        assert str(output["changed-alarms"]) == "1", output

    with test.step("Verify alarm 'disk-full' has an operator state change 'closed' with the text 'Sweep'"):
        def disk_closed():
            alarm = entry(target, "disk-full")
            changes = alarm.get("operator-state-change", []) if alarm else []
            return any(c["state"] == "closed" and c.get("text") == "Sweep" for c in changes)
        until(disk_closed, attempts=10)

    with test.step("Clear alarm 'disk-full' from the CLI"):
        tgtssh.runsh('klish -c "alarm clear disk-full"')

    with test.step("Verify alarm 'disk-full' is cleared"):
        def disk_cleared():
            alarm = entry(target, "disk-full")
            return alarm and is_true(alarm["is-cleared"])
        until(disk_cleared, attempts=10)

    with test.step("Purge cleared alarms"):
        output = target.call_action_output(f"{ALARMS}/alarm-list/purge-alarms",
                                           {"alarm-clearance-status": "cleared"})
        assert str(output["purged-alarms"]) == "4", output

    with test.step("Verify alarms 'backup', 'disk-full', 'eth9' and 'software' are gone"):
        until(lambda: entry(target, "backup") is None
              and entry(target, "disk-full") is None
              and entry(target, ETH9, kind="infix-alarms:link-down") is None
              and entry(target, "software", kind="infix-alarms:update-check-failed") is None,
              attempts=10)

    test.succeed()
