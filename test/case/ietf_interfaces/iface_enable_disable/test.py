#!/usr/bin/env python3
"""
Interface status

Verify interface status properly propagate changes when an interface
is disabled and then re-enabled.

Both admin-status and oper-status are verified.
"""

import infamy
import infamy.iface as iface

from infamy import until

def print_error_message(iface, param, exp_val, act_val):
    return f"'{param}' failure for interface '{iface}'. Expected '{exp_val}', Actual: '{act_val}'"

def assert_param(target, interface, parameter, expected_value):
    def check_param():
        actual_value = iface.get_param(target, interface, parameter)
        if actual_value is None:
            raise ValueError(f"Failed to retrieve '{parameter}' for interface '{interface}'")
        return actual_value == expected_value

    until(check_param)

    actual_value = iface.get_param(target, interface, parameter)
    assert (expected_value == actual_value), print_error_message(
        iface=interface,
        param = parameter,
        exp_val = expected_value,
        act_val = actual_value
        )

def configure_interface(target, iface_name, iface_type=None, enabled=True, ip_address=None, bridge=None):

    interface_config = {
        "name": iface_name,
        "enabled": enabled
    }

    if iface_type:
        interface_config["type"] = iface_type

    if ip_address:
        interface_config["ipv4"] = {
            "address": [
                {
                    "ip": ip_address, 
                    "prefix-length": 24
                }]}

    if bridge:
        interface_config["infix-interfaces:bridge-port"] = {
            "bridge": bridge
        }

    target.put_config_dict( "ietf-interfaces", {
        "interfaces": {
            "interface": [
                interface_config
            ]}})

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        env = infamy.Env()
        target1 = env.attach("target1", "mgmt")
        target2 = env.attach("target2", "mgmt")

        _, data1 = env.ltop.xlate("target1", "data")
        _, link1 = env.ltop.xlate("target1", "link")

        _, iface_under_test = env.ltop.xlate("target2", "link")
        _, host_send_iface = env.ltop.xlate("host", "data")
        _bridge = "br_0"

        target_address = "10.10.10.2"
        host_address = "10.10.10.1"        

    with test.step("Configure bridge and associated interfaces in target1"):
        configure_interface(target1, _bridge, enabled=True, iface_type="infix-if-type:bridge")
        configure_interface(target1, data1, enabled=True, bridge=_bridge)
        configure_interface(target1, link1, enabled=True, bridge=_bridge)

    with test.step("Disable interface in target2"):
        configure_interface(target2, iface_under_test, enabled=False)

    with test.step("Verify the interface is disabled"):
        assert_param(target2, iface_under_test, "admin-status", "down")
        assert_param(target2, iface_under_test, "oper-status", "down")

    with test.step("Enable the interface and assign an IP address"):
        configure_interface(target2, iface_under_test, enabled=True, ip_address=target_address)
    
    with test.step("Verify the interface is enabled"):
        assert_param(target2, iface_under_test, "admin-status", "up")
        assert_param(target2, iface_under_test, "oper-status", "up")

    with infamy.IsolatedMacVlan(host_send_iface) as send_ns:
        with test.step("Verify it is possible to ping the interface"):
            send_ns.addip(host_address)
            send_ns.must_reach(target_address)
        
    test.succeed()