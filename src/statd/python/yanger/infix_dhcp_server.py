#!/usr/bin/env python3
"""
Collect operational data for infix-dhcp-server.yang from dnsmasq
"""
from datetime import datetime, timezone
import json
import dbus


def leases(leases_file):
    """Populate DHCP leases table"""
    table = []
    try:
        with open(leases_file, 'r', encoding='utf-8') as fd:
            for line in fd:
                tokens = line.strip().split(" ")
                if len(tokens) != 5:
                    continue

                # Handle infinite lease time as specified in RFC 2131
                if tokens[0] == "0":
                    expires = "never"
                else:
                    dt = datetime.fromtimestamp(int(tokens[0]),
                                                tz=timezone.utc)
                    expires = dt.isoformat() + "+00:00"

                row = {
                    "expires": expires,
                    "address": tokens[2],
                    "phys-address": tokens[1],
                }

                if tokens[3] != '*':
                    row["hostname"] = tokens[3]
                else:
                    row["hostname"] = ""

                if tokens[4] != '*':
                    row["client-id"] = tokens[4]
                else:
                    row["client-id"] = ""

                table.append(row)
    except (IOError, OSError, ValueError):
        pass

    return table


def statistics():
    """Fetch DHCP server metrics over D-Bus"""
    try:
        bus = dbus.SystemBus()
        obj = bus.get_object("uk.org.thekelleys.dnsmasq",
                             "/uk/org/thekelleys/dnsmasq")
        srv = dbus.Interface(obj, "uk.org.thekelleys.dnsmasq")

        metrics = srv.GetMetrics()
    except dbus.exceptions.DBusException:
        metrics = {
            "dhcp_offer": 0,
            "dhcp_ack": 0,
            "dhcp_nak": 0,
            "dhcp_decline": 0,
            "dhcp_discover": 0,
            "dhcp_request": 0,
            "dhcp_release": 0,
            "dhcp_inform": 0
        }

    return {
        "sent": {
            "offer-count": metrics["dhcp_offer"],
            "ack-count":   metrics["dhcp_ack"],
            "nak-count":   metrics["dhcp_nak"]
        },
        "received": {
            "decline-count":  metrics["dhcp_decline"],
            "discover-count": metrics["dhcp_discover"],
            "request-count":  metrics["dhcp_request"],
            "release-count":  metrics["dhcp_release"],
            "inform-count":   metrics["dhcp_inform"]
        }
    }


def operational(leases_file="/var/lib/misc/dnsmasq.leases"):
    """Return operational status for DHCP server"""
    return {
        "infix-dhcp-server:dhcp-server": {
            "statistics": statistics(),
            "leases": {
                "lease": leases(leases_file)
            }
        }
    }


if __name__ == "__main__":
    print(json.dumps(operational(leases_file="mock.leases")))
