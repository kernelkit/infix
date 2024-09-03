#!/usr/bin/env python3
"""
Interface IPv6 autoconf for bridges

Verify IPv6 autoconf on a bridge is properly set up for global prefix.
See issue #473 for details.
"""
import infamy

with infamy.Test() as test:
    with test.step("Initializing ..."):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        tgtssh = env.attach("target", "mgmt", "ssh")

    with test.step("Setting up bridge with IPv6 SLAAC for global prefix ..."):
        _, tport = env.ltop.xlate("target", "data")

        target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": "br0",
                        "type": "infix-if-type:bridge",
                        "enabled": True,
                        "ipv6": {
                            "enabled": True,
                            "autoconf": {
                                    "create-global-addresses": True
                            }
                        }
                    },
                    {
                        "name": tport,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "bridge": "br0"
                        }
                    },
                ]
            }
        })

    with test.step("Verifying sysctl autoconf setting ..."):
        out = tgtssh.runsh("sysctl net.ipv6.conf.br0.autoconf").stdout
        print(out)
        if "autoconf = 1" not in out:
            test.fail()

    test.succeed()
