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

        eth = intf.get("ethernet") or intf.get("ieee802-ethernet-interface:ethernet")
        if not eth:
            return None, None

        speed = eth.get("speed")
        duplex = eth.get("duplex")

        return speed, duplex

    return None, None

def set_host_speed_duplex(iface, exp_speed, exp_duplex):
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
    
def set_target_speed_duplex(interface, speed, duplex):
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

    # RESTCONF returns speed as float in Gbps, e.g. 0.1 = 100Mb/s
    expected_speed_in_gbps = exp_speed / 1000
    if float(act_speed) != expected_speed_in_gbps:
        print(f"act_speed: {act_speed}, exp_speed: {expected_speed_in_gbps}")
        test.fail()

    if act_duplex.lower() != exp_duplex.lower():
        print(f"act_duplex: {act_duplex}, exp_duplex: {exp_duplex}")
        test.fail()

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
        set_host_speed_duplex(hdata, 10, "full")
        set_target_speed_duplex(tdata, 10, "full" )
        verify_speed_duplex(target, tdata, 10, "full")

    with test.step("Verify 10/Half"):
        set_host_speed_duplex(hdata, 10, "half")
        set_target_speed_duplex(tdata, 10, "half" )
        verify_speed_duplex(target, tdata, 10, "half")

    with test.step("Verify 100/Full"):
        set_host_speed_duplex(hdata, 100, "full")
        set_target_speed_duplex(tdata, 100, "full" )
        verify_speed_duplex(target, tdata, 100, "full")

    with test.step("Verify 100/Half"):
        set_host_speed_duplex(hdata, 100, "half")
        set_target_speed_duplex(tdata, 100, "half" )
        verify_speed_duplex(target, tdata, 100, "half")

    test.succeed()