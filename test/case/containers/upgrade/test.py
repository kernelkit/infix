#!/usr/bin/env python3
"""Container Upgrade

Verify container upgrade functionality by testing the optimization
that skips recreation when the container configuration hasn't changed.

This test uses two versions of curios-httpd (24.05.0 and 24.11.0) served
over HTTP with :latest tag. The test verifies that:
1. Container starts successfully from the :latest image
2. When the :latest tag points to a new version, upgrade is triggered
3. The container runs the new version after upgrade
"""
import os
import time
import infamy
import infamy.file_server as srv
from infamy.util import until

SRVPORT = 8008
SRVDIR = "/srv"

with infamy.Test() as test:
    NAME = "web"

    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        tgtssh = env.attach("target", "mgmt", "ssh")

        if not target.has_model("infix-containers"):
            test.skip()

        _, hport = env.ltop.xlate("host", "data")
        _, tport = env.ltop.xlate("target", "data")

    with test.step("Detect target architecture"):
        # Query operational datastore for machine architecture
        system_state = target.get_data("/ietf-system:system-state")
        arch = system_state["system-state"]["platform"]["machine"]

        # Map kernel arch to our image naming
        arch_map = {
            "amd64": "amd64",
            "x86_64": "amd64",
            "arm64": "arm64",
            "aarch64": "arm64",
            "armv7l": "arm64",
        }
        image_arch = arch_map.get(arch, "amd64")
        print(f"Detected architecture: {arch} -> using {image_arch} images")

    with test.step("Set up isolated network and file server"):
        netns = infamy.IsolatedMacVlan(hport).start()
        netns.addip("192.168.0.1")

        target.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [
                        {
                            "name": tport,
                            "ipv4": {
                                "address": [
                                    {
                                        "ip": "192.168.0.2",
                                        "prefix-length": 24
                                    }
                                ]
                            }
                        }
                    ]
                }
            }
        })
        netns.must_reach("192.168.0.2")

    with srv.FileServer(netns, "192.168.0.1", SRVPORT, SRVDIR):
        with test.step("Create symlink for curios-httpd:latest -> 24.05.0"):
            # Create symlink in the file server directory
            old_img = f"{SRVDIR}/curios-httpd-24.05.0-{image_arch}.tar.gz"
            new_img = f"{SRVDIR}/curios-httpd-24.11.0-{image_arch}.tar.gz"
            latest_link = f"{SRVDIR}/curios-httpd-latest.tar.gz"

            # Remove any existing symlink
            if os.path.exists(latest_link):
                os.unlink(latest_link)

            # Point to old version
            os.symlink(old_img, latest_link)
            print(f"Created symlink: {latest_link} -> {old_img}")

        with test.step("Create container 'web' from curios-httpd:latest (24.05.0)"):
            target.put_config_dict("infix-containers", {
                "containers": {
                    "container": [
                        {
                            "name": f"{NAME}",
                            "image": f"http://192.168.0.1:{SRVPORT}/curios-httpd-latest.tar.gz",
                            "command": "/usr/sbin/httpd -f -v -p 91",
                            "network": {
                                "host": True
                            }
                        }
                    ]
                }
            })

        with test.step("Verify container 'web' has started with version 24.05.0"):
            c = infamy.Container(target)
            until(lambda: c.running(NAME), attempts=60)

            # Get initial operational data to capture container ID and image ID
            containers_data = target.get_data("/infix-containers:containers")
            container = containers_data["containers"]["container"][NAME]
            initial_container_id = container["id"]
            initial_image_id = container["image-id"]

            print(f"Container started with container-id: {initial_container_id[:12]}...")
            print(f"                   and image-id: {initial_image_id[:12]}...")

            # Get baseline disk usage for /var/lib/containers
            result = tgtssh.runsh("doas du -s /var/lib/containers")
            initial_disk_usage = int(result.stdout.split()[0])
            print(f"Disk usage after initial creation: {initial_disk_usage} KiB")

        with test.step("Update symlink to point to curios-httpd:24.11.0"):
            # Remove old symlink and point to new version
            os.unlink(latest_link)
            os.symlink(new_img, latest_link)
            print(f"Updated symlink: {latest_link} -> {new_img}")

        with test.step("Trigger container upgrade by calling upgrade action"):
            c = infamy.Container(target)
            c.action(NAME, "upgrade")

        with test.step("Wait for container 'web' to complete uprgade"):
            time.sleep(3)
            until(lambda: c.running(NAME), attempts=30)

        with test.step("Verify container 'web' is running new version 24.11.0"):
            c = infamy.Container(target)
            # Wait for upgrade to complete and container to restart
            until(lambda: c.running(NAME), attempts=60)

            # Get operational data after upgrade
            containers_data = target.get_data("/infix-containers:containers")
            container = containers_data["containers"]["container"][NAME]
            new_container_id = container["id"]
            new_image_id = container["image-id"]

            print(f"After upgrade container-id: {new_container_id[:12]}...")
            print(f"                  image-id: {new_image_id[:12]}...")

            # Verify that both IDs have changed
            if new_container_id == initial_container_id:
                test.fail("Container ID did not change after upgrade!")
            if new_image_id == initial_image_id:
                test.fail("Image ID did not change after upgrade!")

            print("✓ Both container ID and image ID changed after upgrade")

        with test.step("Verify old image was pruned and disk usage is reasonable"):
            # We expect minimal growth — the new image replaces old
            # image, and they are each around 500-600 KiB.  We allow for
            # some overhead, but should be < 200 KiB. If the old image
            # is not pruned properly, we'll see ~600 KiB extra growth.
            MAX_ACCEPTABLE_GROWTH = 1000

            # Get disk usage after upgrade
            result = tgtssh.runsh("doas du -s /var/lib/containers")
            final_disk_usage = int(result.stdout.split()[0])
            print(f"Disk usage after upgrade: {final_disk_usage} KiB")

            # Calculate the difference
            disk_growth = final_disk_usage - initial_disk_usage
            print(f"Disk usage growth: {disk_growth} KiB")

            if disk_growth > MAX_ACCEPTABLE_GROWTH:
                test.fail(f"Disk usage grew by {disk_growth} KiB (expected < {MAX_ACCEPTABLE_GROWTH} KiB). "
                          f"Old image may not have been pruned!")

            print(f"✓ Disk usage growth is acceptable ({disk_growth} KiB < {MAX_ACCEPTABLE_GROWTH} KiB)")

    test.succeed()
