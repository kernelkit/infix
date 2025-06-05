#!/usr/bin/env python3
"""
Interface Speed Duplex

Verify that auto-negotiation results in expected speed/duplex mode.
"""

import infamy
import infamy.iface as iface
import subprocess

def set_speed_duplex(iface, exp_speed, exp_duplex):
    duplex_flag = "full" if exp_duplex == "full" else "half"
    try:
        subprocess.run([
            "ethtool", "-s", iface,
            "speed", str(exp_speed),
            "duplex", duplex_flag,
            "autoneg", "off"
        ], check=True)
    except subprocess.CalledProcessError as e:
        raise RuntimeError(f"Failed to set speed/duplex via ethtool: {e}")

def verify_speed_duplex(target, interface, exp_speed, exp_duplex):
    act_speed = iface.get_param(target, interface, "speed")
    act_duplex = iface.get_param(target, interface, "duplex")

    if act_speed != exp_speed:
        print(f"act_speed: {act_speed}, exp_speed: {exp_speed}")
        test.fail()

    if act_duplex.lower() != exp_duplex.lower():
        print(f"act_duplex: {act_duplex}, exp_duplex: {exp_duplex}")
        test.fail()

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, hdata = env.ltop.xlate("host", "data")

    with test.step("Enable target interface"):
        target.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": target["data"], 
                        "enabled": True
                    }]
                }
            }
        })
    
    with test.step("Verify 100/Full"):
        set_speed_duplex(hdata, 100, "full")
        verify_speed_duplex(target, hdata, 100, "full")
    with test.step("Verify 10/Full"):
        set_speed_duplex(hdata, 100, "full")
        verify_speed_duplex(target, hdata, 10, "full")
        
        
    test.succeed()