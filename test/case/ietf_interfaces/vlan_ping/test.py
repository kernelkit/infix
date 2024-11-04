#!/usr/bin/env python3
"""
VLAN ping connectivity

Very basic test if the VLAN interface configuration works.
"""

import infamy
import infamy.iface as iface

from infamy import until

with infamy.Test() as test:
      with test.step("Set up topology and attach to target DUT"):
            env = infamy.Env()
            target = env.attach("target", "mgmt")
            _, tport = env.ltop.xlate("target", "data")

      with test.step("Set up VLAN interface on host:data with IP 10.0.0.1"):
            _, hport = env.ltop.xlate("host", "data")
            datanet = infamy.IsolatedMacVlan(hport).start()
            datanet.runsh("""
                  set -ex
                  ip link set iface up
                  ip link add dev vlan10 link iface up type vlan id 10
                  ip addr add 10.0.0.1/24 dev vlan10
                  """)

      with test.step("Configure VLAN 10 interface on target:data with IP 10.0.0.2"):
            target.put_config_dict("ietf-interfaces", {
                  "interfaces": {
                        "interface": [
                              {
                                    "name": tport,
                                    "enabled": True,
                              },
                              {
                                    "name": f"{tport}.10",
                                    "type": "infix-if-type:vlan",
                                    "vlan": {
                                          "id": 10,
                                          "lower-layer-if": tport,
                                    },
                                    "ipv4": {
                                          "address": [
                                                {
                                                      "ip": "10.0.0.2",
                                                      "prefix-length": 24,
                                                }
                                          ]
                                    }
                              },
                        ]
                  }
            })

      with test.step("Wait for links to come up"):
            until(lambda: iface.get_param(target, tport, "oper-status") == "up")

      with test.step("Verify that host:data can reach 10.0.0.2"):
            _, hport = env.ltop.xlate("host", "data")
            datanet.must_reach("10.0.0.2")

      with test.step("Remove VLAN interface from target:data"):
            target.delete_xpath(f"/ietf-interfaces:interfaces/interface[name='{tport}.10']")

      with test.step("Verify that host:data can no longer reach 10.0.0.2"):
            _, hport = env.ltop.xlate("host", "data")
            datanet.must_not_reach("10.0.0.2")

      test.succeed()
