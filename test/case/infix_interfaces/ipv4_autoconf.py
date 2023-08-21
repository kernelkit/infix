#!/usr/bin/env python3
#
#   PC ---- e0: 10.0.0.2
#

import infamy

with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env(infamy.std_topology("1x2"))
        target = env.attach("target", "mgmt")

    with test.step("Configure an interface with IPv4 ZeroConf IP"):
        _, tport = env.ltop.xlate("target", "data")

        target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": tport,
                        "type": "infix-if-type:ethernet",
                        "enabled": True,
                        "ipv4": {
                            "autoconf": {
                                "enabled": True
                            }
                        }
                    }
                ]
            }
        })

    with test.step("Wait for mDNS message from a 169.254 address ..."):
        _, hport = env.ltop.xlate("host", "data")

        with infamy.IsolatedMacVlan(hport) as ns:
            vrfy = ns.runsh("""
            set -ex

            ip link set iface up
            ip addr add 10.0.0.1/24 dev iface

            fakeroot tcpdump -q -i iface -c 1 -n host 224.0.0.251 and port 5353 --print 2>/dev/null \
		| awk 'match($0,/169\\.254\\.[0-9]+\\.[0-9]+/) {print substr($0,RSTART,RLENGTH)}'   \
		| grep . || exit 1
            """)

        if vrfy.returncode:
            print(vrfy.stdout)
            test.fail()

    test.succeed()
