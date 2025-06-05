#!/usr/bin/env python3
"""
Interface Speed Duplex

Verify that auto-negotiation results in expected speed/duplex mode.
"""

import infamy
import infamy.iface as iface
import subprocess

def get_target_speed_duplex(target, interface):
    path = iface.get_xpath(interface)
    content = target.get_data(path)
    if not content or "interfaces" not in content:
        return None, None

    for intf in content["interfaces"].get("interface", []):
        if intf.get("name") != interface:
            continue

        eth = intf.get("ethernet")
        if not eth:
            return None, None
        
        return eth.get("speed"), eth.get("duplex")

    return None, None

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

def run_speed_duplex_case(target, t_iface, h_iface, speed, duplex):
        set_host_speed_duplex(h_iface, speed, duplex)
        set_target_speed_duplex(target, t_iface, speed, duplex)
        verify_speed_duplex(target, t_iface, speed, duplex)

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, hdata = env.ltop.xlate("host", "data")
        _, tdata = env.ltop.xlate("target", "data")

    with test.step("Enable target interface and disable auto-negotiation"):
        target.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": target["data"], 
                        "enabled": True,
                    }]
                }
            }
        })

    with test.step("Verify 10/Full"):
        run_speed_duplex_case(target, tdata, hdata, 10, "full")

    with test.step("Verify 10/Half"):
        run_speed_duplex_case(target, tdata, hdata, 10, "half")

    with test.step("Verify 100/Full"):
        run_speed_duplex_case(target, tdata, hdata, 100, "full")

    with test.step("Verify 100/Half"):
        run_speed_duplex_case(target, tdata, hdata, 100, "half")

    test.succeed()