#!/usr/bin/env python3
"""
Interface Speed Duplex (Copper)

Verify that auto-negotiation results in expected speed/duplex mode.
"""

import infamy
import infamy.iface as iface
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
            "autoneg", "on",
            "advertise", hex(mask)
        ], check=True)
    except subprocess.CalledProcessError as e:
        raise RuntimeError(f"Failed to advertise modes via ethtool: {e}")

def get_target_speed_duplex(target, interface):
    path = iface.get_xpath(interface)
    content = target.get_data(path)
    if not content or "interfaces" not in content:
        return None, None

    for intf in content["interfaces"].get("interface", []):
        if intf.get("name") != interface:
            continue

        eth = intf.get("ethernet") or intf.get("ieee802-ethernet-interface:ethernet")
        if not eth:
            return None, None

        return eth.get("speed"), eth.get("duplex")

    return None, None

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

def verify_speed_duplex(target, interface, exp_speed, exp_duplex):
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

def speed_duplex_present(target, interface):
    speed, duplex = get_target_speed_duplex(target, interface)
    return speed is not None and duplex is not None

def enable_target_autoneg(target, interface):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": interface,
                    "enabled": True,
                    "ethernet": {
                        "auto-negotiation": {
                            "enable": True
                        }
                    }
                }]
            }
        }
    })

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, hdata = env.ltop.xlate("host", "data")
        _, tdata = env.ltop.xlate("target", "data")

    with test.step("Enable target interface and autonegotiation"):
        enable_target_autoneg(target, tdata)

    with test.step("Advertise 10/Full only"):
        advertise_host_modes(hdata, ["10full"])
        verify_speed_duplex(target, tdata, 10, "full")

    with test.step("Advertise 10/Half only"):
        advertise_host_modes(hdata, ["10half"])
        verify_speed_duplex(target, tdata, 10, "half")

    with test.step("Advertise 100/Full only"):
        advertise_host_modes(hdata, ["100full"])
        verify_speed_duplex(target, tdata, 100, "full")

    with test.step("Advertise 100/Half only"):
        advertise_host_modes(hdata, ["100half"])
        verify_speed_duplex(target, tdata, 100, "half")

    with test.step("Advertise 10/half + 10/full + 100/half"):
        advertise_host_modes(hdata, ["10half", "10full", "100half"])
        verify_speed_duplex(target, tdata, 100, "half")

    with test.step("Advertise 10/half + 10/full + 100/half + 100/full + 1000/full"):
        advertise_host_modes(hdata, ["10half", "10full", "100half", "100full", "1000full"])
        verify_speed_duplex(target, tdata, 1000, "full")

    test.succeed()