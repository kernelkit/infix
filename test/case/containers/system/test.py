#!/usr/bin/env python3
"""Container without Image

Verify that a container without an image runs the software of the
system itself: the web server that Infix ships, serving a page from
the configuration as a content mount, with host networking on port 91.

The container starts from the root file system of the system image,
not from the running system.  It has the hostname of the container and
it reports no image, its image ID is the version of the system.
"""
import infamy
from infamy.util import until, curl, to_binary

with infamy.Test() as test:
    NAME = "web-system"
    MESG = "It works, from the system"

    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target, tgtssh = env.attach("target", "mgmt"), env.attach("target", "mgmt", "ssh")
        addr = target.get_mgmt_ip()

        if not target.has_model("infix-containers"):
            test.skip()

    with test.step("Create container 'web-system' without image, running /usr/sbin/httpd on port 91"):
        target.put_config_dicts({
            "infix-containers": {
                "containers": {
                    "container": [{
                        "name": NAME,
                        "command": "/usr/sbin/httpd -f -v -p 91 -h /var/www",
                        "mount": [{
                            "name": "index.html",
                            "content": to_binary(f"<html><body>{MESG}</body></html>\n"),
                            "target": "/var/www/index.html"
                        }],
                        "network": {
                            "host": True
                        }
                    }]
                }
            }
        })

    with test.step("Verify container 'web-system' has started"):
        c = infamy.Container(target)
        until(lambda: c.running(NAME), attempts=60)

    with test.step("Verify 'It works, from the system' on http://target:91/index.html"):
        url = f"http://[{addr}]:91/index.html"
        until(lambda: MESG in (curl(url, silent=True) or ""), attempts=10)

    with test.step("Verify the hostname in container 'web-system' is 'web-system'"):
        out = tgtssh.runsh(f"sudo ixcont exec {NAME} hostname").stdout
        assert out.strip() == NAME, f"hostname in container: {out!r}"

    with test.step("Verify container 'web-system' reports no image, and the system version as image ID"):
        data = target.get_data("/infix-containers:containers")
        web = data["containers"]["container"][NAME]
        version = target.get_data("/ietf-system:system-state")["system-state"]["platform"]["os-version"]

        assert "image" not in web, f"image: {web.get('image')}"
        assert web["image-id"] == version, f"image-id {web['image-id']}, system {version}"

    test.succeed()
