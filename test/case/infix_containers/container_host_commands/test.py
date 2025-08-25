#!/usr/bin/env python3
r"""Host Command Execution from Container

This test verifies that a container running on Infix can execute commands
that affect the host system. Specifically, it confirms that the container
can change the hostname of the host.
"""

import infamy
from infamy.util import until, to_binary

with infamy.Test() as test:
    cont_image = f"oci-archive:{infamy.Container.NFTABLES_IMAGE}"
    cont_name = "cont0"
    hostname_init = "container-host"
    hostname_new = "coffee"

    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

        if not target.has_model("infix-containers"):
            test.skip()

    with test.step("Set initial hostname"):
        target.put_config_dict("ietf-system", {
            "system": {
                "hostname": hostname_init
                }
            })

    with test.step("Verify initial hostname in operational"):
        oper = target.get_data("/ietf-system:system")
        name = oper["system"]["hostname"]

        if name != hostname_init:
            print(f"Expected hostname: {hostname_init}, actual hostname: {name}")
            test.fail()

    with test.step("Include script in OCI image to modify host hostname"):
        commands = to_binary(f"""#!/bin/sh
nsenter -m/1/ns/mnt -u/1/ns/uts -i/1/ns/ipc -n/1/ns/net hostname {hostname_new}
""")

        target.put_config_dict("infix-containers", {
            "containers": {
                "container": [
                    {
                        "name": cont_name,
                        "image": cont_image,
                        "network": {
                            "host": True
                        },
                        "mount": [
                            {
                              "name": "rc.local",
                              "content": commands,
                              "target": "/etc/rc.local",
                              "mode": "0755"
                            },
                            {
                              "name": "proc1ns",
                              "source": "/proc/1/ns",
                              "target": "/1/ns",
                            }
                        ],
                        "privileged": True
                    }
                ]
            }
        })

    with test.step("Verify container has started"):
        c = infamy.Container(target)
        until(lambda: c.running(cont_name), attempts=10)

    with test.step("Verify the new hostname set by the container"):
        until(lambda: c.running(cont_name) != target.get_data("/ietf-system:system")["system"]["hostname"], attempts=10)
    test.succeed()
