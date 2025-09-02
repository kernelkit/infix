#!/usr/bin/env python3
"""
Container environment variables

Verify that environment variables can be set in container configuration
and are available inside the running container. Tests the 'env' list
functionality by:

1. Creating a container with multiple environment variables
2. Using a custom script to extract env vars and serve them via HTTP
3. Fetching the served content to verify environment variables are set correctly

Uses the nftables container image with custom rc.local script.
"""
import infamy
from infamy.util import until, to_binary


with infamy.Test() as test:
    NAME = "web-env"
    DUTIP = "10.0.0.2"
    OURIP = "10.0.0.1"

    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

        if not target.has_model("infix-containers"):
            test.skip()

    with test.step("Configure data interface with static IPv4"):
        _, ifname = env.ltop.xlate("target", "data")
        target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [{
                    "name": f"{ifname}",
                    "ipv4": {
                        "address": [{
                            "ip": f"{DUTIP}",
                            "prefix-length": 24
                        }]
                    }
                }]
            }
        })

    with test.step("Create container with environment variables"):
        script = to_binary("""#!/bin/sh
# Create HTTP response with environment variables
printf "HTTP/1.1 200 OK\\r\\n" > /var/www/response.txt
printf "Content-Type: text/plain\\r\\n" >> /var/www/response.txt
printf "Connection: close\\r\\n\\r\\n" >> /var/www/response.txt

# Add environment variables using printf to control encoding
printf "TEST_VAR=\\"%s\\"\\n" "$TEST_VAR" >> /var/www/response.txt
printf "APP_PORT=%s\\n" "$APP_PORT" >> /var/www/response.txt
printf "DEBUG_MODE=\\"%s\\"\\n" "$DEBUG_MODE" >> /var/www/response.txt
printf "PATH_WITH_SPACES=\\"%s\\"\\n" "$PATH_WITH_SPACES" >> /var/www/response.txt

while true; do
    nc -l -p 8080 < /var/www/response.txt 2>>/var/www/debug.log || sleep 1
done
""")

        target.put_config_dict("infix-containers", {
            "containers": {
                "container": [
                    {
                        "name": f"{NAME}",
                        "image": f"oci-archive:{infamy.Container.NFTABLES_IMAGE}",
                        "env": [
                            {"key": "TEST_VAR", "value": "hello-world"},
                            {"key": "APP_PORT", "value": "8080"},
                            {"key": "DEBUG_MODE", "value": "true"},
                            {"key": "PATH_WITH_SPACES", "value": "/path with spaces/test"}
                        ],
                        "network": {
                            "host": True
                        },
                        "mount": [
                            {
                                "name": "rc.local",
                                "content": script,
                                "target": "/etc/rc.local",
                                "mode": "0755"
                            }
                        ],
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
        until(lambda: c.running(NAME), attempts=60)

    with test.step("Verify environment variables are available via HTTP"):
        _, hport = env.ltop.xlate("host", "data")
        url = infamy.Furl(f"http://{DUTIP}:8080/env.html")

        with infamy.IsolatedMacVlan(hport) as ns:
            ns.addip(OURIP)

            with test.step("Verify basic connectivity to data interface"):
                ns.must_reach(DUTIP)

            with test.step("Verify environment variables in HTTP response"):
                until(lambda: url.nscheck(ns, "TEST_VAR=\"hello-world\""), attempts=10)
                until(lambda: url.nscheck(ns, "APP_PORT=8080"), attempts=10)
                until(lambda: url.nscheck(ns, "DEBUG_MODE=\"true\""), attempts=10)
                until(lambda: url.nscheck(ns, "PATH_WITH_SPACES=\"/path with spaces/test\""), attempts=10)

    test.succeed()
