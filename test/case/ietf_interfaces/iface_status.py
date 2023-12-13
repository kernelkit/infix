#!/usr/bin/env python3

import infamy
import infamy.iface as iface


def print_err_msg(interface, param, val):
    f"{param} failure for interface <{interface}> [{param}: {val}]"

def assert_if_index(target, interface):
    if_index = iface.get_if_index(target, interface)
    if interface == "lo":
        assert (if_index == 1), print_err_msg(interface, "if-index", if_index)
    else:
        assert (if_index > 1 and if_index < 65535), print_err_msg(interface, "if-index", if_index)

def assert_oper_status(target, interface):
    oper_status = iface.get_oper_status(target, interface)
    if interface == "lo":
        assert (oper_status == "up" or oper_status == "unknown"), print_err_msg(interface, "oper-status", oper_status)
    else:
        assert (oper_status == "up"), print_err_msg(interface, "oper-status", oper_status)

def asser_iface_exists(target, interface):
    assert iface.interface_exist(target, interface), f"Interface <{interface}> does not exist."


with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env(infamy.std_topology("1x1"))
        target = env.attach("target", "mgmt")
      
    iface.print_all(target)

    loopback_iface = "lo"
    _, mgmt_iface = env.ltop.xlate("target", "mgmt") 
    ifaces_under_test = [loopback_iface, mgmt_iface]
    print(f"Interfaces under test: {ifaces_under_test}")
    
    for interface in ifaces_under_test:
        with test.step(f"Verifying <{interface}> interface"):
            asser_iface_exists(target, interface)
    
        with test.step(f"Verifying <if-index> for <{interface}> interface"):
            assert_if_index(target, interface)
        
        with test.step(f"Verifying <oper-status> for <{interface}> interface"):
            assert_oper_status(target, interface)

    test.succeed()
