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
    """Trigger Avahi to send traffic on allowed interfaces"""
    time.sleep(2)
    tgt.runsh("logger -t scan 'calling avahi-browse ...'")
    tgt.runsh("avahi-browse -lat")


def check(ns, expr, must):
    """Wrap netns.must_receive() with common defaults"""
    return ns.must_receive(expr, timeout=3, must=must)


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
                         "interface": [
                             {
                                 "name": p1,
                                 "enabled": True,
                                 "ipv4": {
                                     "address": [
                                         {
                                             "ip": "10.0.1.1",
                                             "prefix-length": 24
                                         }
                                     ]
                                 }

                             },
                             {
                                 "name": p2,
                                 "enabled": True,
                                 "ipv4": {
                                     "address": [
                                         {
                                             "ip": "10.0.2.1",
                                             "prefix-length": 24
                                         }
                                     ]
                                 }

                             },
                             {
                                 "name": p3,
                                 "enabled": True,
                                 "ipv4": {
                                     "address": [
                                         {
                                             "ip": "10.0.3.1",
                                             "prefix-length": 24
                                         }
                                     ]
                                 }

                             },
                         ]
                     }
                },
                "ietf-system": {
                    "system": {
                        "hostname": "dut"
                    }
                },
                "infix-services": {
                    "mdns": {
                        "enabled": True
                    }
                }
            }
        )

    with infamy.IsolatedMacVlan(eth1) as ns1, \
         infamy.IsolatedMacVlan(eth2) as ns2, \
         infamy.IsolatedMacVlan(eth3) as ns3:
        ns1.addip("10.0.1.2")
        ns2.addip("10.0.2.2")
        ns3.addip("10.0.3.2")

        EXPR1 = "host 10.0.1.1 and port 5353"
        EXPR2 = "host 10.0.2.1 and port 5353"
        EXPR3 = "host 10.0.3.1 and port 5353"

        with test.step("Allow mDNS on a single interface: p2"):
            dut.put_config_dict("infix-services", {
                "mdns": {
                    "interfaces": {
                        "allow": [p2],
                    }
                }
            })

            parallel(lambda: mdns_scan(ssh),
                     lambda: check(ns1, EXPR1, False),
                     lambda: check(ns2, EXPR2, True),
                     lambda: check(ns3, EXPR3, False))

        with test.step("Deny mDNS on a single interface: p2"):
            dut.delete_xpath("/infix-services:mdns/interfaces")
            dut.put_config_dict("infix-services", {
                "mdns": {
                    "interfaces": {
                        "deny": [p2],
                    }
                }
            })

            parallel(lambda: mdns_scan(ssh),
                     lambda: check(ns1, EXPR1, True),
                     lambda: check(ns2, EXPR2, False),
                     lambda: check(ns3, EXPR3, True))

        with test.step("Allow mDNS on p1, p3 deny on p2, p3"):
            dut.delete_xpath("/infix-services:mdns/interfaces")
            dut.put_config_dict("infix-services", {
                "mdns": {
                    "interfaces": {
                        "allow": [p1, p3],
                        "deny":  [p2, p3],
                    }
                }
            })

            parallel(lambda: mdns_scan(ssh),
                     lambda: check(ns1, EXPR1, True),
                     lambda: check(ns2, EXPR2, False),
                     lambda: check(ns3, EXPR3, False))

    test.succeed()
