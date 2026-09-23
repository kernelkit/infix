#!/usr/bin/env python3
"""SNMP MIB coverage

Walk every MIB the agent is documented to serve and verify each one
answers with at least one object.  The set of MIBs the agent serves is
decided by a hand written module list in the defconfigs, so trimming it,
or a net-snmp release renaming a module, silently shrinks what is served
while everything else still passes.

Values are deliberately not compared.  Counters, uptime and load move on
their own, and which rows exist depends on the interfaces, mounts and
processes of the device under test, so a recorded walk could only ever
be a source of false failures.  A subtree that answers at all is the
property worth guarding.

On top of that, check the relations that must hold whatever the device
looks like: every interface in the operational datastore is reachable
over SNMP under its own if-index, every ifTable row has the matching
ifXTable row, and ifNumber agrees with both.

LLDP-MIB is checked separately because it arrives by a different route,
lldpd registering it with the master agent over AgentX, and is the only
thing here that exercises that path.

"""

import time

import infamy

SYSTEM = ".1.3.6.1.2.1.1"

# The MIBs doc/snmp.md promises, as the narrowest subtree that proves
# the module behind each one is built in and answering.  HOST-RESOURCES
# is split rather than walked whole: hrSWRunTable rereads all of /proc
# per request, which is slow on a device with real work to do.
SUBTREES = [
    ("SNMPv2-MIB      system",      SYSTEM),
    ("IF-MIB          ifTable",     ".1.3.6.1.2.1.2.2"),
    ("IF-MIB          ifXTable",    ".1.3.6.1.2.1.31.1.1"),
    ("HOST-RESOURCES  hrStorage",   ".1.3.6.1.2.1.25.2.3"),
    ("HOST-RESOURCES  hrProcessor", ".1.3.6.1.2.1.25.3.3"),
    ("UCD-SNMP-MIB    laTable",     ".1.3.6.1.4.1.2021.10"),
    ("UCD-SNMP-MIB    memory",      ".1.3.6.1.4.1.2021.4"),
    ("UCD-SNMP-MIB    dskTable",    ".1.3.6.1.4.1.2021.9"),
]

IFINDEX = ".1.3.6.1.2.1.2.2.1.1"
IFNUMBER = ".1.3.6.1.2.1.2.1.0"
IFNAME = ".1.3.6.1.2.1.31.1.1.1.1"

# lldpd serves this one as an AgentX subagent, see doc/snmp.md.
LLDP_LOCAL = ".1.0.8802.1.1.2.1.3"

COMMUNITY = "infix-test"
TARGET_IP = "10.0.0.10"


def retry(fn, attempts=10, interval=3):
    """Poll fn until it returns something truthy, None on timeout.

    infamy.until() raises instead, which would report a quiet subtree as
    a traceback rather than a step saying what was missing.
    """
    for _ in range(attempts):
        val = fn()
        if val:
            return val
        time.sleep(interval)

    return None


def snmpwalk(netns, oid):
    """Varbinds under oid, empty when the agent has nothing there."""
    rc = netns.runsh(f"snmpwalk -v2c -c {COMMUNITY} -r 1 -t 3 -On "
                     f"{TARGET_IP} {oid}")
    if rc.returncode != 0:
        return []

    out = []
    for line in rc.stdout.splitlines():
        line = line.strip()
        # An unimplemented subtree is reported, not an error, so the
        # exit code above does not catch it.
        if not line or "No Such" in line or "No more variables" in line:
            continue
        out.append(line)

    return out


def varbind(line):
    """Split '.1.3.6.1... = STRING: eth0' into its OID and value."""
    oid, _, val = line.partition(" = ")
    return oid.strip(), val.split(":", 1)[-1].strip()


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

    with infamy.IsolatedMacVlan(hport) as netns:
        netns.addip("10.0.0.1")

        with test.step("Verify the host has the net-snmp client tools"):
            if netns.runsh("command -v snmpwalk").returncode != 0:
                print("host: snmpwalk not found, the test container image "
                      "needs the net-snmp-tools package")
                test.fail()

        with test.step("Wait for the agent to answer"):
            # Everything below reads an empty walk as a missing MIB, so
            # settle this first rather than blame the module list.
            if not retry(lambda: snmpwalk(netns, SYSTEM)):
                print("host: agent never answered")
                test.fail()

        with test.step("Verify every documented MIB answers"):
            missing = []
            for label, oid in SUBTREES:
                num = len(snmpwalk(netns, oid))
                print(f"host: {label}  {oid}  {num} objects")
                if not num:
                    missing.append(label)
            if missing:
                print(f"host: no objects served under: {', '.join(missing)}")
                test.fail()

        with test.step("Verify LLDP-MIB is served over AgentX"):
            # Enabling SNMP restarts lldpd with -x, and net-snmp's
            # subagent retries the master every agentxPingInterval,
            # 15s by default, so allow a couple of rounds.
            lldp = retry(lambda: len(snmpwalk(netns, LLDP_LOCAL)))
            print(f"host: LLDP-MIB lldpLocalSystemData {lldp} objects")
            if not lldp:
                print("host: lldpd did not register LLDP-MIB with the agent")
                test.fail()

        with test.step("Verify every interface in operational is in ifXTable"):
            names = {}
            for line in snmpwalk(netns, IFNAME):
                oid, name = varbind(line)
                names[oid[len(IFNAME) + 1:]] = name.strip('"')

            oper = target.get_data("/ietf-interfaces:interfaces")
            for entry in oper["interfaces"]["interface"]:
                idx = str(entry.get("if-index", ""))
                print(f"target: {entry['name']} if-index {idx}"
                      f" -> ifName {names.get(idx)}")
                if names.get(idx) != entry["name"]:
                    test.fail()

        with test.step("Verify every ifTable row has an ifXTable row"):
            indices = [varbind(ln)[1] for ln in snmpwalk(netns, IFINDEX)]
            orphans = [i for i in indices if i not in names]
            print(f"host: ifTable has {len(indices)} rows,"
                  f" ifXTable has {len(names)}")
            if orphans:
                print(f"host: no ifXTable row for ifIndex: {orphans}")
                test.fail()

        with test.step("Verify ifNumber agrees with the ifTable row count"):
            num = snmpwalk(netns, IFNUMBER)
            print(f"host: ifNumber = {varbind(num[0])[1] if num else None}")
            if not num or varbind(num[0])[1] != str(len(indices)):
                test.fail()

    test.succeed()
