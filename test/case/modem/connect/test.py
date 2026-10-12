#!/usr/bin/env python3
"""Cellular modem data connection

Verify a data connection over a modem, from detection to an IP link.  On
the QEMU test target the modem is virtual: a USB modem looped back in the
DUT's own kernel, answering AT commands like an AT-only cellular modem,
with a PPP peer playing the network.

The modem is anchored to the USB port it was detected on, then its SIM
and a wwan interface with a data bearer are configured.  The modem must
register on the network, and the wwan interface come up with the address
and default route from the network.  Locking the modem must take the
data connection down again.

The nameservers from the network are added next to those from other
interfaces, which must remain.
"""
import infamy
import infamy.route as route
from infamy.util import until

ADDRESS = "10.66.0.2"       # handed out by the virtual modem's network
MODEM_DNS = "10.66.0.1"     # ditto
OTHER_DNS = "192.0.2.53"
OTHER_IF = "test0"


def components(target):
    """Hardware components in operational, by name."""
    data = target.get_data("/ietf-hardware:hardware") or {}
    return {c.get("name"): c for c in
            data.get("hardware", {}).get("component", [])}


def state(comp, key):
    """Augmented state container, with or without module prefix."""
    comp = comp or {}
    return comp.get(key, comp.get("infix-hardware:" + key, {}))


def registered(target):
    comps = components(target)
    return state(comps.get("modem0"), "modem-state").get("state") in \
        ("registered", "connecting", "connected") and \
        state(comps.get("sim0"), "sim-state").get("state") == "unlocked"


def has_address(target):
    ifc = target.get_iface("wwan0")
    if not ifc:
        return False
    ipv4 = ifc.get("ipv4") or ifc.get("ietf-ip:ipv4") or {}
    return any(a.get("ip") == ADDRESS for a in ipv4.get("address", []))


def nameservers(tgtssh):
    out = tgtssh.runsh("sudo resolvconf -l").stdout
    return [line.split()[1] for line in out.splitlines()
            if line.startswith("nameserver")]


def modem(state):
    return {
        "name": "modem0",
        "class": "infix-hardware:modem",
        "uri": [uri],
        "state": {"admin-state": state},
    }


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        tgtssh = env.attach("target", "mgmt", "ssh")

        if not target.has_feature("infix-hardware", "modem"):
            print("DUT does not advertise the 'modem' feature, skipping")
            test.skip()

    with test.step("Find the detected modem and its USB port"):
        detected = until(lambda: components(target).get("modem0"), attempts=30)
        uri = next(iter(detected["uri"]))
        print(f"modem0 detected at {uri}")

    with test.step("Add a nameserver from another interface"):
        tgtssh.runsh(f"echo 'nameserver {OTHER_DNS}' | sudo resolvconf -a {OTHER_IF}")

    with test.step("Configure modem0 anchored to its port, sim0 and wwan0"):
        target.put_config_dicts({
            "ietf-hardware": {"hardware": {"component": [
                modem("unlocked"),
                {
                    "name": "sim0",
                    "class": "infix-hardware:sim",
                    "state": {"admin-state": "unlocked"},
                },
            ]}},
            "ietf-interfaces": {"interfaces": {"interface": [{
                "name": "wwan0",
                "type": "infix-if-type:modem",
                "infix-interfaces:wwan": {
                    "modem": "modem0",
                    "sim": "sim0",
                    "bearer": {"apn": "internet", "ip-type": "ipv4"},
                },
            }]}},
        })

    with test.step("Verify modem0 registers on the network with sim0 unlocked"):
        until(lambda: registered(target), attempts=90)

    with test.step("Verify wwan0 is up with the address from the network"):
        until(lambda: has_address(target), attempts=60)

    with test.step("Verify the default route via wwan0"):
        until(lambda: route.ipv4_route_exist(target, "0.0.0.0/0", pref=200),
              attempts=30)

    with test.step("Verify the nameservers from wwan0 and the other interface"):
        until(lambda: MODEM_DNS in nameservers(tgtssh), attempts=30)
        if OTHER_DNS not in nameservers(tgtssh):
            test.fail(f"nameserver {OTHER_DNS} from {OTHER_IF} is gone")

    with test.step("Lock modem0 and verify the data connection goes down"):
        target.put_config_dicts({"ietf-hardware": {"hardware": {
            "component": [modem("locked")]}}})
        until(lambda: not has_address(target), attempts=30)

    with test.step("Remove the nameserver from the other interface"):
        tgtssh.runsh(f"sudo resolvconf -d {OTHER_IF}")

    test.succeed()
