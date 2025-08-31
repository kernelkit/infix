#!/usr/bin/env python3
"""
Container enabled/disabled

Verify that a container can be enabled and disabled via configuration.
Tests the 'enabled' leaf functionality by:

1. Creating an enabled container and verifying it starts
2. Disabling the container and verifying it stops
3. Re-enabling the container and verifying it starts again

Uses operational datastore to verify container running status.
"""
import infamy
from infamy.util import until


def set_container_enabled(target, name, enabled):
    """Helper function to set container enabled state and verify the change"""
    target.put_config_dict("infix-containers", {
        "containers": {
            "container": [
                {
                    "name": name,
                    "enabled": enabled
                }
            ]
        }
    })


with infamy.Test() as test:
    NAME = "web-enabled"

    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

        if not target.has_model("infix-containers"):
            test.skip()

    with test.step("Set hostname to 'container-host'"):
        target.put_config_dict("ietf-system", {
            "system": {
                "hostname": "container-host"
                }
            })

    with test.step("Create enabled container from bundled OCI image"):
        target.put_config_dict("infix-containers", {
            "containers": {
                "container": [
                    {
                        "name": f"{NAME}",
                        "enabled": True,
                        "image": f"oci-archive:{infamy.Container.HTTPD_IMAGE}",
                        "command": "/usr/sbin/httpd -f -v -p 91",
                        "network": {
                            "host": True
                        }
                    }
                ]
            }
        })

    with test.step("Verify container has started"):
        c = infamy.Container(target)
        until(lambda: c.running(NAME), attempts=60)

    with test.step("Disable container"):
        set_container_enabled(target, NAME, False)

    with test.step("Verify container has stopped"):
        until(lambda: not c.running(NAME), attempts=60)

    with test.step("Re-enable container"):
        set_container_enabled(target, NAME, True)

    with test.step("Verify container has started again"):
        until(lambda: c.running(NAME), attempts=60)

    test.succeed()
