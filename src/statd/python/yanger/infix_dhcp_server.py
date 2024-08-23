from datetime import datetime
from .host import HOST


def lease_info(ifname):
    """Populate DHCP leases table"""
    leases = f"/var/run/dnsmasq-{ifname}.leases"
    hosts = []
    try:
        with open(leases, 'r', encoding='utf-8') as fd:
            for line in fd:
                tokens = line.strip().split(" ")
                if len(tokens) != 5:
                    continue

                host = {
                    "ip-address": tokens[2],
                    "hardware-address": tokens[1],
                }
                dt = datetime.utcfromtimestamp(int(tokens[0]))
                host["expires"] = dt.isoformat() + "+00:00"

                if tokens[3] != '*':
                    host["hostname"] = tokens[3]
                if tokens[4] != '*':
                    host["client-identifier"] = tokens[4]

                hosts.append(host)
    except (IOError, OSError, ValueError):
        pass

    return {
        "host-count": len(hosts),
        "host": hosts
    }


def status(servers):
    """Populate DHCP server status"""

    data = HOST.run_json(['/usr/libexec/statd/dhcp-server-status'], default=[])
    if data == []:
        return

    for entry in data:
        metrics = entry["metrics"]

        servers.append({
            "if-name": entry["if-name"],
            "packet-statistics": {
                "sent": {
                    "offer-count": metrics["dhcp_offer"],
                    "ack-count": metrics["dhcp_ack"],
                    "nak-count": metrics["dhcp_nak"]
                },
                "received": {
                    "decline-count": metrics["dhcp_decline"],
                    "discover-count": metrics["dhcp_discover"],
                    "request-count": metrics["dhcp_request"],
                    "release-count": metrics["dhcp_release"],
                    "inform-count": metrics["dhcp_inform"]
                }
            },
            "server": {
                "lease": lease_info(entry["if-name"])
            }
        })


def operational():
    """Return operational status for DHCP server"""
    out = {
        "infix-dhcp-server:dhcp-server": {
            "server-if": []
        }
    }
    status(out['infix-dhcp-server:dhcp-server']['server-if'])

    return out
