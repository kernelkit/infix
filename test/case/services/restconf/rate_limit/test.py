#!/usr/bin/env python3
"""
RESTCONF rate limit

Verify that another host sending RESTCONF requests faster than the
configured rate limit gets HTTP 429, and that it gets none with the
limit turned off.
"""
import requests
import urllib3
from requests.auth import HTTPBasicAuth

import infamy
import infamy.util as util

urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)


def statuses(host, auth, count):
    url = f"https://[{host}]/restconf/data/ietf-system:system/hostname"
    session = requests.Session()
    result = []
    for _ in range(count):
        req = session.prepare_request(requests.Request(
            "GET", url, auth=auth,
            headers={"Accept": "application/yang-data+json"}))
        req.url = url
        result.append(session.send(req, verify=False, timeout=10).status_code)
    print(f"HTTP status codes: {result}")
    return result


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt", protocol="netconf")
        host = target.get_mgmt_ip()
        auth = HTTPBasicAuth("admin", env.get_password("target"))

    with test.step("Limit RESTCONF to 1 request a second with bursts of 5"):
        target.put_config_dicts({
            "infix-services": {
                "web": {
                    "restconf": {
                        "rate-limit": {
                            "enabled": True,
                            "rate": 1,
                            "burst": 5
                        }
                    }
                }
            }
        })

    with test.step("Verify 20 requests in a row from the host get HTTP 429"):
        util.until(lambda: 429 in statuses(host, auth, 20))

    with test.step("Turn the RESTCONF rate limit off"):
        target.put_config_dicts({
            "infix-services": {
                "web": {
                    "restconf": {
                        "rate-limit": {
                            "enabled": False
                        }
                    }
                }
            }
        })

    with test.step("Verify 20 requests in a row from the host all succeed"):
        util.until(lambda: statuses(host, auth, 20) == [200] * 20)

    test.succeed()
