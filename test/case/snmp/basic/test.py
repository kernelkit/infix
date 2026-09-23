#!/usr/bin/env python3
"""SNMP basic

Enable the SNMP agent with a read-only community and query it over
SNMPv2c from the host.  Verify sysObjectID, that IF-MIB ifName at a given index
matches the interface of that if-index in the operational datastore,
that ifHCInOctets is consistent with the interface statistics, that an
unknown community is refused, and that disabling the agent stops it
answering.

Queries use numeric OIDs, so the test does not depend on MIB text files
being installed in the container.

"""

import time

import infamy
import infamy.iface as iface

# SNMPv2-MIB::sysObjectID.0, and the arc confd hands out for Infix
SYSOBJECTID = ".1.3.6.1.2.1.1.2.0"
INFIX_OID = ".1.3.6.1.4.1.61046.1.1.1"

# IF-MIB::ifName (ifXTable) and IF-MIB::ifHCInOctets
IFNAME = ".1.3.6.1.2.1.31.1.1.1.1"
IFHCINOCTETS = ".1.3.6.1.2.1.31.1.1.1.6"

COMMUNITY = "infix-test"
TARGET_IP = "10.0.0.10"


def retry(fn, attempts=10, interval=1):
    """Poll fn until it returns something truthy, None on timeout.

    infamy.until() raises instead, which turns a quiet agent into a
    traceback rather than a failed step saying what was expected.
    """
    for _ in range(attempts):
        val = fn()
        if val:
            return val
        time.sleep(interval)

    return None


def snmpget(netns, oid, community=COMMUNITY):
    """Return the value half of a single-varbind response, or None."""
    rc = netns.runsh(f"snmpget -v2c -c {community} -r 1 -t 2 -On -Oqv "
                     f"{TARGET_IP} {oid}")
    if rc.returncode != 0:
        return None

    out = rc.stdout.strip()
    return out if out and "No Such" not in out and "Timeout" not in out else None


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, hport = env.ltop.xlate("host", "data")

        if not target.has_model("ietf-snmp"):
            test.skip()

    with test.step("Set IPv4 address 10.0.0.10/24 on target:data"):
        target.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": target["data"],
                        "enabled": True,
                        "ipv4": {
                            "address": [{
                                "ip": TARGET_IP,
                                "prefix-length": 24
                            }]
                        }
                    }]
                }
            }
        })

    with test.step("Enable SNMP agent with a read-only community"):
        # No version and no listen: the agent should then serve every
        # version it supports, on every address, port 161.
        target.put_config_dicts({
            "ietf-snmp": {
                "snmp": {
                    "engine": {"enabled": True},
                    "community": [{
                        "index": "monitor",
                        "text-name": COMMUNITY,
                        "security-name": "monitor"
                    }]
                }
            }
        })

    with test.step("Read if-index and in-octets for target:data from operational"):
        ifindex = iface.get_param(target, target["data"], "if-index")
        stats = iface.get_param(target, target["data"], "statistics") or {}
        octets = int(stats.get("in-octets", 0))
        print(f"target: {target['data']} is if-index {ifindex}, in-octets {octets}")
        if ifindex is None:
            print(f"target: no if-index for {target['data']}")
            test.fail()

    with infamy.IsolatedMacVlan(hport) as netns:
        netns.addip("10.0.0.1")

        with test.step("Verify the host has the net-snmp client tools"):
            # Without this the queries below all come back empty and
            # every failure looks like the agent not answering.
            if netns.runsh("command -v snmpget").returncode != 0:
                print("host: snmpget not found, the test container image "
                      "needs the net-snmp-tools package")
                test.fail()

        with test.step("Verify agent reports the Infix sysObjectID"):
            # The agent has just been started, give it a moment to bind.
            val = retry(lambda: snmpget(netns, SYSOBJECTID))
            print(f"host: sysObjectID.0 = {val}")
            if val != INFIX_OID:
                test.fail()

        with test.step("Verify IF-MIB ifName at if-index matches the interface name"):
            val = snmpget(netns, f"{IFNAME}.{ifindex}")
            print(f"host: ifName.{ifindex} = {val}")
            if val != target["data"]:
                test.fail()

        with test.step("Verify IF-MIB ifHCInOctets is consistent with operational data"):
            val = snmpget(netns, f"{IFHCINOCTETS}.{ifindex}")
            print(f"host: ifHCInOctets.{ifindex} = {val}")
            if val is None or not val.isdigit() or int(val) < octets:
                test.fail()

        with test.step("Verify an unknown community is refused"):
            if snmpget(netns, SYSOBJECTID, "wrong-community") is not None:
                test.fail()

        with test.step("Disable SNMP agent"):
            target.put_config_dicts({
                "ietf-snmp": {"snmp": {"engine": {"enabled": False}}}
            })

        with test.step("Verify agent no longer responds"):
            if not retry(lambda: snmpget(netns, SYSOBJECTID) is None):
                print("host: agent still answering after being disabled")
                test.fail()

    test.succeed()
