#!/usr/bin/env python3
"""
Container environment variables

Verify that environment variables can be set in container configuration
and are available inside the running container.  Also verify that
changing an environment variable triggers a container restart.

1  Set up a container config with multiple environment variables
2. Serve variables back to host using a CGI script in container
3. Verify served content against environment variables
4. Change an environment variable and verify the container restarts
"""
import infamy
from infamy.util import until, to_binary, curl


with infamy.Test() as test:
    ENV_VARS = [
        {"key": "TEST_VAR", "value": "hello-world"},
        {"key": "APP_PORT", "value": "8080"},
        {"key": "DEBUG_MODE", "value": "true"},
        {"key": "PATH_WITH_SPACES", "value": "/path with spaces/test"}
    ]
    NAME = "web-env"
    DUTIP = "10.0.0.2"
    OURIP = "10.0.0.1"
    URL =f"http://{DUTIP}:8080/cgi-bin/env.cgi"

    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, hport = env.ltop.xlate("host", "data")

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

    with test.step("Set up container with environment variables"):
        cgi = [
            '#!/bin/sh',
            '# CGI script to output environment variables',
            'echo "Content-Type: text/plain"',
            'echo ""'
        ]

        for var in ENV_VARS:
            cgi.append(f'echo "{var["key"]}=${var["key"]}"')

        target.put_config_dict("infix-containers", {
            "containers": {
                "container": [
                    {
                        "name": f"{NAME}",
                        "image": f"oci-archive:{infamy.Container.HTTPD_IMAGE}",
                        "command": "/usr/sbin/httpd -f -v -p 8080",
                        "env": ENV_VARS,
                        "network": {
                            "host": True
                        },
                        "mount": [
                            {
                                "name": "env.cgi",
                                "content": to_binary('\n'.join(cgi) + '\n'),
                                "target": "/var/www/cgi-bin/env.cgi",
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

    with infamy.IsolatedMacVlan(hport) as ns:
        ns.addip(OURIP)

        with test.step("Verify basic connectivity to data interface"):
            ns.must_reach(DUTIP)

        with test.step("Verify environment variables in CGI response"):
            expected_strings = []
            for var in ENV_VARS:
                expected_strings.append(f'{var["key"]}={var["value"]}')

            until(lambda: all(string in ns.call(lambda: curl(URL)) for string in expected_strings))

        with test.step("Change environment variable and verify container restarts"):
            UPDATED_ENV_VARS = [
                {"key": "TEST_VAR", "value": "updated-value"},
                {"key": "APP_PORT", "value": "8080"},
                {"key": "DEBUG_MODE", "value": "true"},
                {"key": "PATH_WITH_SPACES", "value": "/path with spaces/test"}
            ]

            target.put_config_dict("infix-containers", {
                "containers": {
                    "container": [{
                        "name": f"{NAME}",
                        "env": UPDATED_ENV_VARS,
                    }]
                }
            })

        with test.step("Verify container has restarted with updated env"):
            until(lambda: "TEST_VAR=updated-value" in ns.call(lambda: curl(URL)), attempts=60)

    test.succeed()
