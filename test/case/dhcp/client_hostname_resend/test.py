#!/usr/bin/env python3
"""DHCP Hostname Resend

Verify that updating the system hostname regenerates the DHCP client
Finit service file so subsequent DHCP requests advertise the current
hostname (option 12, RFC 2132).

Regression test for a bug where the DHCP client callback only reacts
on diffs in infix-dhcp-client, so a standalone change of
ietf-system:system/hostname leaves the previously written
/etc/finit.d/available/dhcp-client-<iface>.conf untouched and the
running udhcpc keeps announcing the old name.

"""

import infamy
from infamy.util import until


def finit_conf(ssh, ifname):
    """Return the contents of the generated DHCP client Finit service file."""
    path = f"/etc/finit.d/available/dhcp-client-{ifname}.conf"
    cmd = ssh.runsh(f"cat {path} 2>/dev/null")
    return cmd.stdout


def udhcpc_cmdline(ssh, ifname):
    """Return the NUL-separated argv of the running udhcpc for ifname."""
    pidfile = f"/run/dhcp-client-{ifname}.pid"
    cmd = ssh.runsh(
        f"p=$(cat {pidfile} 2>/dev/null); "
        f"[ -n \"$p\" ] && tr '\\0' ' ' < /proc/$p/cmdline"
    )
    return cmd.stdout


def running_hostname(ssh, ifname):
    """Extract the hostname udhcpc is currently announcing (-x hostname:<name>)."""
    for tok in udhcpc_cmdline(ssh, ifname).split():
        if tok.startswith("hostname:"):
            return tok.split(":", 1)[1]
    return None


with infamy.Test() as test:
    HOSTNM_A = "infix-resend-a"
    HOSTNM_B = "infix-resend-b"

    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        client = env.attach("client", "mgmt")
        clissh = env.attach("client", "mgmt", "ssh")
        _, port = env.ltop.xlate("client", "mgmt")

    with test.step(f"Configure initial hostname '{HOSTNM_A}'"):
        client.put_config_dict("ietf-system", {
            "system": {
                "hostname": HOSTNM_A
            }
        })
        until(lambda: client.get_data("/ietf-system:system")
              .get("system", {}).get("hostname") == HOSTNM_A)

    with test.step("Enable DHCP client sending hostname option"):
        client.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [{
                    "name": port,
                    "ipv4": {
                        "infix-dhcp-client:dhcp": {
                            "option": [
                                {"id": "vendor-class", "value": "infamy"},
                                {"id": "hostname", "value": "auto"},
                                {"id": "netmask"},
                                {"id": "router"}
                            ]
                        }
                    }
                }]
            }
        })

    with test.step(f"Verify running udhcpc announces hostname '{HOSTNM_A}'"):
        until(lambda: running_hostname(clissh, port) == HOSTNM_A)

    with test.step(f"Update system hostname to '{HOSTNM_B}'"):
        client.put_config_dict("ietf-system", {
            "system": {
                "hostname": HOSTNM_B
            }
        })
        until(lambda: client.get_data("/ietf-system:system")
              .get("system", {}).get("hostname") == HOSTNM_B)

    with test.step(f"Verify running udhcpc announces hostname '{HOSTNM_B}'"):
        try:
            until(lambda: running_hostname(clissh, port) == HOSTNM_B,
                  attempts=15)
        except Exception:
            cur = running_hostname(clissh, port)
            print(f"udhcpc still announcing hostname '{cur}', expected '{HOSTNM_B}'")
            test.fail()

    test.succeed()
