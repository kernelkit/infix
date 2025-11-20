#!/usr/bin/env python3
"""DHCP Hostname Priority

Verify deterministic hostname management: a DHCP acquired hostname takes
precedence over a configured hostname.  When a DHCP lease ends, or the
hostname option is removed, the system should revert to the configured
hostname.

"""

import infamy, infamy.dhcp
from infamy.util import until


def verify_hostname(node, expected):
    """Verify operational hostname matches expected value"""
    data = node.get_data("/ietf-system:system")
    return data["system"]["hostname"] == expected


with infamy.Test() as test:
    DHCP_HOSTNAME = "dhcp-assigned"
    CONF_HOSTNAME = "configured-host"

    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        client = env.attach("client", "mgmt")
        _, host = env.ltop.xlate("host", "mgmt")
        _, port = env.ltop.xlate("client", "mgmt")

    with test.step("Configure static system hostname"):
        client.put_config_dict("ietf-system", {
            "system": {
                "hostname": CONF_HOSTNAME
            }
        })

    with test.step("Verify configured hostname is set"):
        until(lambda: verify_hostname(client, CONF_HOSTNAME))

    with infamy.IsolatedMacVlan(host, mode="private") as netns:
        netns.addip("10.0.0.1")
        with infamy.dhcp.Server(netns, ip="10.0.0.42", hostname=DHCP_HOSTNAME):
            with test.step("Enable DHCP client requesting hostname option"):
                client.put_config_dict("ietf-interfaces", {
                    "interfaces": {
                        "interface": [{
                            "name": port,
                            "ipv4": {
                                "infix-dhcp-client:dhcp": {
                                    "option": [
                                        {"id": "vendor-class", "value": "infamy"},
                                        {"id": "hostname"},
                                        {"id": "netmask"},
                                        {"id": "router"}
                                    ]
                                }
                            }
                        }]
                    }
                })

            with test.step("Verify DHCP hostname takes precedence"):
                until(lambda: verify_hostname(client, DHCP_HOSTNAME))

            with test.step("Drop hostname option from client request"):
                path = f"/ietf-interfaces:interfaces/interface[name='{port}']" \
                     + "/ietf-ip:ipv4/infix-dhcp-client:dhcp/option[id='hostname']"
                client.delete_xpath(path)

            with test.step("Verify hostname reverts to configured value"):
                until(lambda: verify_hostname(client, CONF_HOSTNAME))

    test.succeed()
