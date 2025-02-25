#!/usr/bin/env python3
"""Container Volume Persistence

Verify that a container created from a local OCI archive, with a volume
for persistent content, can be upgraded at runtime, without losing the
content in the volume on restart.

"""
import infamy
from infamy.util import until

with infamy.Test() as test:
    NAME = "web-volume"
    PORT = 8080
    MESG = "HEJ"

    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        tgtssh = env.attach("target", "mgmt", "ssh")
        addr = target.get_mgmt_ip()
        url = infamy.Furl(f"http://[{addr}]:{PORT}/index.html")

        if not target.has_model("infix-containers"):
            test.skip()

    with test.step("Create container with volume from bundled OCI image"):
        target.put_config_dict("infix-containers", {
            "containers": {
                "container": [
                    {
                        "name": f"{NAME}",
                        "image": f"oci-archive:{infamy.Container.HTTPD_IMAGE}",
                        "command": f"/usr/sbin/httpd -f -v -p {PORT}",
                        "network": {
                            "host": True
                        },
                        "volume": [{
                            "name": "www",
                            "target": "/var/www"
                        }]
                    }
                ]
            }
        })

    with test.step("Verify container has started"):
        c = infamy.Container(target)
        until(lambda: c.running(NAME), attempts=10)

    with test.step("Modify container volume content"):
        cmd = f"sudo container shell {NAME} 'echo {MESG} >/var/www/index.html'"
        tgtssh.runsh(cmd)

    with test.step("Verify container volume content"):
        until(lambda: url.check(MESG), attempts=10)

    with test.step("Upgrade container"):
        out = tgtssh.runsh(f"sudo container upgrade {NAME}")
        if ">> Done." not in out.stdout:
            msg = f"Failed upgrading container {NAME}:\n" \
                f"STDOUT:\n{out.stdout}\n" \
                f"STDERR:\n{out.stderr}"
            test.fail(msg)
        # else:
        #     print(f"Container {NAME} upgraded: {out.stdout}")

    with test.step("Verify container volume content survived upgrade"):
        until(lambda: url.check(MESG), attempts=10)

    test.succeed()
