#!/usr/bin/env python3
r"""
Bridge forwarding single DUTs

Tests forwarding through a DUT with two bridged interfaces on one DUT,
and that the frame counters of the ports can be read afterwards.

....

,------------------------------------------,
|                                          |
|                          br0             |
|                         /   \            |
| target:mgmt   target:data1  target:data2 |
'------------------------------------------'
        |                |     |
        |                |     |
,------------------------------------------,
|   host:mgmt   host:data1     host:data2  |
|               [10.0.0.1]     [10.0.0.2]  |
|                  (ns0)         (ns1)     |
|                                          |
|                 [ HOST ]                 |
'------------------------------------------'

....

"""
import infamy
import infamy.iface as iface
from infamy.util import parallel

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target, tssh = parallel(lambda: env.attach("target", "mgmt"),
                                lambda: env.attach("target", "mgmt", "ssh"))

    with test.step("Configure a bridge with dual physical port"):
        tport1 = target["data1"]
        tport2 = target["data2"]

        target.put_config_dicts({"ietf-interfaces": {
            "interfaces": {
                "interface": [
                    {
                        "name": "br0",
                        "type": "infix-if-type:bridge",
                        "enabled": True,
                    },
                    {
                        "name": tport1,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "bridge": "br0"
                        }
                    },
                    {
                        "name": tport2,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "bridge": "br0"
                        }
                    }
                ]
            }
        }})

    with test.step("Verify ping from host:data1 to 10.0.0.2"):
        hport1 = env["host"]["data1"]
        hport2 = env["host"]["data2"]

        with infamy.IsolatedMacVlan(hport1) as ns1, \
             infamy.IsolatedMacVlan(hport2) as ns2 :

            ns2.addip("10.0.0.2")
            ns1.addip("10.0.0.1")

            ns1.must_reach("10.0.0.2")

    with test.step("Read the frame counters of the bridge ports"):
        for port in (tport1, tport2):
            frame = iface.eth_statistics(target, port)
            if frame:
                assert frame["in-frames"] > 0, f"{port} counted no received frames"

    with test.step("Show a bridge port in the CLI"):
        out = tssh.runsh(f'klish -c "show interface {tport1}"').stdout
        assert f"name               : {tport1}" in out, out

    test.succeed()
