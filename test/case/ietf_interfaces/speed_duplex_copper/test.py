#!/usr/bin/env python3
"""
Interface Speed Duplex (Copper)

Verify that the interface operates at the expected speed/duplex in two scenarios:

1. Fixed configuration – host and target are both manually set to a specific speed/duplex
2. Auto-negotiation – host advertises selectable modes and the target negotiates
 to the highest common speed/duplex.
"""

import infamy
import subprocess
from infamy.util import until

ADVERTISE_MODES = {
    # Values from ethtool's ETHTOOL_LINK_MODE bit positions
    # See: https://elixir.bootlin.com/linux/latest/source/include/uapi/linux/ethtool.h
    "10half": 0x0001,
    "10full": 0x0002,
    "100half": 0x0004,
    "100full": 0x0008,
    "1000full": 0x0020
}

def advertise_host_modes(interface, modes):
    mask = 0
    for mode in modes:
        mask |= ADVERTISE_MODES[mode]
    try:
        subprocess.run([
            "ethtool", "-s", interface,
            "advertise", hex(mask)
        ], check=True)
    except subprocess.CalledProcessError as e:
        raise RuntimeError(f"Failed to advertise modes via ethtool: {e}")

def get_target_speed_duplex(target, interface):
    data = target.get_data(f"/ietf-interfaces:interfaces/interface[name='{interface}']") \
                  ["interfaces"]["interface"][interface]
    eth = data.get("ethernet", {})
    
    return eth.get("speed"), eth.get("duplex")

def set_target_speed_duplex(target, interface, speed, duplex):
    target.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": interface,
                        "ethernet": {
                            "auto-negotiation": {
                                "enable": False
                            },
                            "speed": speed / 1000,
                            "duplex": duplex
                        }
                    }]
                }
            }
        })

def set_host_speed_duplex(interface, speed, duplex):
    try:
        subprocess.run([
            "ethtool", "-s", interface,
            "speed", str(speed),
            "duplex", duplex,
            "autoneg", "off"
        ], check=True)
    except subprocess.CalledProcessError as e:
        raise RuntimeError(f"Failed to set speed/duplex via ethtool: {e}")

def verify_speed_duplex(target, ns, interface, exp_speed, exp_duplex):
    until(lambda: speed_duplex_present(target, interface))
    act_speed, act_duplex = get_target_speed_duplex(target, interface)
    if act_speed is None or act_duplex is None:
        print(f"Could not fetch speed or duplex from target for interface {interface}")
        test.fail()

    exp_speed_gbps = exp_speed / 1000
    if float(act_speed) != exp_speed_gbps:
        print(f"act_speed: {act_speed}, exp_speed: {exp_speed_gbps}")
        test.fail()

    if act_duplex.lower() != exp_duplex.lower():
        print(f"act_duplex: {act_duplex}, exp_duplex: {exp_duplex}")
        test.fail()

    ns.must_reach("10.0.0.2")

    print(f"Verified: {interface} is operating at {act_speed} Gbps, {act_duplex} duplex")

def speed_duplex_present(target, interface):
    speed, duplex = get_target_speed_duplex(target, interface)
    return speed is not None and duplex is not None

def enable_target_interface(target, interface):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": interface,
                    "enabled": True,
                    "ipv4": {
                        "address": [
                            {
                                "ip": "10.0.0.2",
                                "prefix-length": 24
                            }
                        ]
                    }
                }]
            }
        }
    })

def enable_target_autoneg(target, interface):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": interface,
                    "ethernet": {
                        "auto-negotiation": {
                            "enable": True
                        }
                    }
                }]
            }
        }
    })

def enable_host_autoneg(interface):
    subprocess.run(["ethtool", "-s", interface, "autoneg", "on"], check=True)

def cleanup(target, hdata, tdata):
    """
    Restore both host and target interfaces to autonegotiation mode
    to ensure clean state for future tests.
    """

    print("Restoring interfaces to default (autoneg on)")
    try:
        enable_host_autoneg(hdata)
    except Exception as e:
        print(f"Host autoneg restore failed: {e}")
    try:
        enable_target_interface(target, tdata)
        enable_target_autoneg(target, tdata)
    except Exception as e:
        print(f"Target autoneg restore failed: {e}")

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, hdata = env.ltop.xlate("host", "data")
        _, tdata = env.ltop.xlate("target", "data")

        # Append a test cleanup function
        test.push_test_cleanup(lambda: cleanup(target, hdata, tdata))
    
    with test.step("Enable target interface"):
        enable_target_interface(target, tdata)

    with infamy.IsolatedMacVlan(hdata) as ns:
        ns.addip("10.0.0.1")

        # Fixed mode tests
        with test.step("Verify fixed 10/full"):
            set_host_speed_duplex(hdata, 10, "full")
            set_target_speed_duplex(target, tdata, 10, "full")
            verify_speed_duplex(target, ns, tdata, 10, "full")

        with test.step("Verify fixed 10/half"):
            set_host_speed_duplex(hdata, 10, "half")
            set_target_speed_duplex(target, tdata, 10, "half")
            verify_speed_duplex(target, ns, tdata, 10, "half")

        with test.step("Verify fixed 100/full"):
            set_host_speed_duplex(hdata, 100, "full")
            set_target_speed_duplex(target, tdata, 100, "full")
            verify_speed_duplex(target, ns, tdata, 100, "full")

        with test.step("Verify fixed 100/half"):
            set_host_speed_duplex(hdata, 100, "half")
            set_target_speed_duplex(target, tdata, 100, "half")
            verify_speed_duplex(target, ns, tdata, 100, "half")

        # Auto-negotiation tests: host advertises, Infix negotiates
        with test.step("Switch to auto-negotiation mode for target and host"):
            enable_host_autoneg(hdata)
            enable_target_autoneg(target, tdata)

        with test.step("Verify auto-negotiation to 10/Full only"):
            advertise_host_modes(hdata, ["10full"])
            verify_speed_duplex(target, ns, tdata, 10, "full")

        with test.step("Verify auto-negotiation to 10/Half only"):
            advertise_host_modes(hdata, ["10half"])
            verify_speed_duplex(target, ns, tdata, 10, "half")

        with test.step("Verify auto-negotiation to 100/Full only"):
            advertise_host_modes(hdata, ["100full"])
            verify_speed_duplex(target, ns, tdata, 100, "full")

        with test.step("Verify auto-negotiation to 100/Half only"):
            advertise_host_modes(hdata, ["100half"])
            verify_speed_duplex(target, ns, tdata, 100, "half")

        with test.step("Verify auto-negotiation to 10/half + 10/full + 100/half"):
            advertise_host_modes(hdata, ["10half", "10full", "100half"])
            verify_speed_duplex(target, ns, tdata, 100, "half")

        with test.step("Verify auto-negotiation to 10/half + 10/full + 100/half + 100/full + 1000/full"):
            advertise_host_modes(hdata, ["10half", "10full", "100half", "100full", "1000full"])
            verify_speed_duplex(target, ns, tdata, 1000, "full")

    test.succeed()