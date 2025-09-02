#!/usr/bin/env python3
"""mDNS allow/deny interfaces

Verify the mDNS responder interface allow/deny configuration.  Both
settings can be used independently and in concert.  We verify operation
with three scenarios:

 1. Allow p2, no mDNS traffic should be received on p1 and p3
 2. Deny p2, mDNS traffic should only be received on p1 and p3
 3. Allow p1 and p3, deny p2 and p3, traffic only on p1

"""

import time
import infamy
from infamy.util import parallel


def mdns_scan(tgt):
    """Trigger mDNS scan with initial delay for listener setup"""
    time.sleep(1)  # Give listeners time to start
    tgt.runsh("logger -t scan 'calling avahi-browse ...'")
    tgt.runsh("avahi-browse -lat")


def mdns_recv(ns, expr, must):
    """Check for mDNS traffic with proper timing"""
    return ns.must_receive(expr, timeout=6, must=must)


def check(dut, ssh, listeners_config, allow=None, deny=None):
    """Execute complete mDNS test scenario with proper sequencing"""
    try:
        dut.delete_xpath("/infix-services:mdns/interfaces")
    except ValueError:
        # Ignore if xpath doesn't exist (first run)
        pass

    mdns_config = {"mdns": {"interfaces": {}}}
    if allow:
        mdns_config["mdns"]["interfaces"]["allow"] = allow
    if deny:
        mdns_config["mdns"]["interfaces"]["deny"] = deny

    dut.put_config_dict("infix-services", mdns_config)

    # Create parallel tasks for traffic generation and listening
    tasks = [lambda: mdns_scan(ssh)]
    for ns, expr, should_receive in zip(*listeners_config):
        tasks.append(lambda ns=ns, expr=expr, should_receive=should_receive:
                     mdns_recv(ns, expr, should_receive))

    # Execute all tasks in parallel with proper timing
    parallel(*tasks)


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        dut = env.attach("dut", "mgmt")
        ssh = env.attach("dut", "mgmt", "ssh")
        _, p1 = env.ltop.xlate("dut", "p1")
        _, p2 = env.ltop.xlate("dut", "p2")
        _, p3 = env.ltop.xlate("dut", "p3")
        _, eth1 = env.ltop.xlate("host", "eth1")
        _, eth2 = env.ltop.xlate("host", "eth2")
        _, eth3 = env.ltop.xlate("host", "eth3")

    with test.step("Configure device"):
        dut.put_config_dicts(
            {
                "ietf-interfaces": {
                    "interfaces": {
                        "interface": [{
                            "name": p1,
                            "enabled": True,
                            "ipv4": {
                                "address": [{
                                    "ip": "10.0.1.1",
                                    "prefix-length": 24
                                }]
                            }
                        }, {
                            "name": p2,
                            "enabled": True,
                            "ipv4": {
                                "address": [{
                                    "ip": "10.0.2.1",
                                    "prefix-length": 24
                                }]
                            }
                        }, {
                            "name": p3,
                            "enabled": True,
                            "ipv4": {
                                "address": [{
                                    "ip": "10.0.3.1",
                                    "prefix-length": 24
                                }]
                            }
                        }]
                    }
                },
                "ietf-system": {
                    "system": {"hostname": "dut"}
                },
                "infix-services": {
                    "mdns": {"enabled": True}
                }
            }
        )

    with infamy.IsolatedMacVlan(eth1) as ns1, \
         infamy.IsolatedMacVlan(eth2) as ns2, \
         infamy.IsolatedMacVlan(eth3) as ns3:
        expressions = [
            "host 10.0.1.1 and port 5353",
            "host 10.0.2.1 and port 5353",
            "host 10.0.3.1 and port 5353"
        ]
        namespaces = [ns1, ns2, ns3]

        ns1.addip("10.0.1.2")
        ns2.addip("10.0.2.2")
        ns3.addip("10.0.3.2")

        with test.step("Allow mDNS on a single interface: p2"):
            # p1:no, p2:yes, p3:no
            expect = [False, True, False]
            config = (namespaces, expressions, expect)
            check(dut, ssh, config, allow=[p2])

        with test.step("Deny mDNS on a single interface: p2"):
            # p1:yes, p2:no, p3:yes
            expect = [True, False, True]
            config = (namespaces, expressions, expect)
            check(dut, ssh, config, deny=[p2])

        with test.step("Allow mDNS on p1, p3 deny on p2, p3"):
            # p1:yes, p2:no, p3:no (deny overrides allow)
            expect = [True, False, False]
            config = (namespaces, expressions, expect)
            check(dut, ssh, config, allow=[p1, p3], deny=[p2, p3])

    test.succeed()
