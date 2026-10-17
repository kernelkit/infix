#!/usr/bin/env python3
"""Cellular modem basic configuration

Verify that a cellular modem can be configured without any modem
hardware present: a modem and a SIM hardware component, and a wwan
interface with a data bearer referencing them.

With no modem attached, the wwan interface is created as a placeholder
once the modem's probe-timeout has expired, so that IP configuration
on top of it can proceed.  The placeholder is removed again when the
interface is removed from the configuration.

The test also verifies that a wwan interface cannot reference a hardware
component of the wrong class, e.g., a SIM card in place of a modem, by
sending the same interface twice: first with the SIM as its modem, which
must be rejected, then with the modem, which must be accepted.

This test does not exercise a modem, only the configuration model and
how it is applied.
"""
import infamy
import infamy.iface as iface
from infamy.util import until


def wwan_iface(name, modem, sim):
    return {"ietf-interfaces": {"interfaces": {"interface": [{
        "name": name,
        "type": "infix-if-type:modem",
        "infix-interfaces:wwan": {
            "modem": modem,
            "sim": sim,
            "bearer": {"apn": "internet"},
        },
    }]}}}


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

        if not target.has_feature("infix-hardware", "modem"):
            print("DUT does not advertise the 'modem' feature, skipping")
            test.skip()

    with test.step("Configure modem and SIM hardware components"):
        target.put_config_dicts({"ietf-hardware": {"hardware": {"component": [
            {
                "name": "modem0",
                "class": "infix-hardware:modem",
                "state": {"admin-state": "unlocked"},
                "infix-hardware:modem": {"probe-timeout": 1},
            },
            {
                "name": "sim0",
                "class": "infix-hardware:sim",
                "state": {"admin-state": "unlocked"},
            },
        ]}}})

    with test.step("Configure wwan0 with a data bearer on modem0 and sim0"):
        target.put_config_dicts(wwan_iface("wwan0", "modem0", "sim0"))

    with test.step("Verify wwan0 exists as a modem interface"):
        ifc = until(lambda: target.get_iface("wwan0"), attempts=30)
        assert ifc["type"] == "infix-if-type:modem", \
            f"unexpected type {ifc['type']}"

    with test.step("Verify a wwan interface cannot use a SIM as modem"):
        try:
            target.put_config_dicts(wwan_iface("wwan1", "sim0", "sim0"),
                                    retries=1)
        except Exception as err:
            print(f"Rejected: {err}")
        else:
            test.fail("wwan1 with a SIM as modem was accepted")

    with test.step("Verify the same wwan interface is accepted on modem0"):
        target.put_config_dicts(wwan_iface("wwan1", "modem0", "sim0"))

    with test.step("Remove wwan0 and wwan1, verify the interfaces are gone"):
        target.delete_xpaths([iface.get_xpath("wwan0"),
                              iface.get_xpath("wwan1")])
        until(lambda: target.get_iface("wwan0") is None
              and target.get_iface("wwan1") is None, attempts=30)

    with test.step("Remove modem and SIM hardware components"):
        target.delete_xpaths([
            "/ietf-hardware:hardware/component[name='modem0']",
            "/ietf-hardware:hardware/component[name='sim0']",
        ])

    test.succeed()
