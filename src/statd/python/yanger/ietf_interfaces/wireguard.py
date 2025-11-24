import subprocess
import json
from datetime import datetime, timezone

from ..host import HOST


def _parse_wg_show(ifname):
    """Parse `wg show <ifname> dump` output into structured data"""
    try:
        result = HOST.run(("wg", "show", ifname, "dump"), default="")
        if not result:
            return None

        lines = result.strip().split('\n')
        if len(lines) < 2:  # Need at least interface line + one peer
            return None

        peers = []
        # Skip first line (interface info), process peer lines
        for line in lines[1:]:
            parts = line.split('\t')
            if len(parts) < 8:
                continue

            public_key, preshared_key, endpoint, allowed_ips, \
                latest_handshake, rx_bytes, tx_bytes, persistent_keepalive = parts

            peer = {
                "public_key": public_key,
                "endpoint": endpoint if endpoint != "(none)" else None,
                "allowed_ips": allowed_ips.split(',') if allowed_ips else [],
                "latest_handshake": int(latest_handshake) if latest_handshake != "0" else None,
                "rx_bytes": int(rx_bytes),
                "tx_bytes": int(tx_bytes),
            }
            peers.append(peer)

        return peers
    except Exception:
        return None


def _format_timestamp(epoch_seconds):
    """Convert Unix timestamp to YANG date-and-time format"""
    if not epoch_seconds:
        return None
    dt = datetime.fromtimestamp(epoch_seconds, tz=timezone.utc)
    # YANG date-and-time requires timezone with colon: +00:00 not +0000
    timestamp = dt.strftime("%Y-%m-%dT%H:%M:%S%z")
    # Insert colon in timezone offset: +0000 -> +00:00
    return timestamp[:-2] + ':' + timestamp[-2:]


def _parse_endpoint(endpoint_str):
    """Parse endpoint string like '192.168.1.1:51820' or '[2001:db8::1]:51820'"""
    if not endpoint_str or endpoint_str == "(none)":
        return None, None

    # Handle IPv6 with brackets
    if endpoint_str.startswith('['):
        addr_end = endpoint_str.find(']')
        if addr_end == -1:
            return None, None
        addr = endpoint_str[1:addr_end]
        port_part = endpoint_str[addr_end+1:]
        port = int(port_part.lstrip(':')) if ':' in port_part else None
        return addr, port

    # Handle IPv4
    parts = endpoint_str.rsplit(':', 1)
    if len(parts) == 2:
        return parts[0], int(parts[1])
    return parts[0], None


def _connection_status(latest_handshake_epoch):
    """Determine connection status based on handshake time"""
    if not latest_handshake_epoch:
        return "down"

    # Consider connection up if handshake within last 3 minutes
    age = datetime.now(timezone.utc).timestamp() - latest_handshake_epoch
    return "up" if age < 180 else "down"


def wireguard(iplink):
    """Get WireGuard operational state data"""
    ifname = iplink.get("ifname")
    if not ifname:
        return None

    peers_data = _parse_wg_show(ifname)
    if not peers_data:
        return None

    peers = []
    for peer_data in peers_data:
        peer = {
            "public-key": peer_data["public_key"]
        }

        # Connection status (always include)
        if peer_data["latest_handshake"]:
            peer["latest-handshake"] = _format_timestamp(peer_data["latest_handshake"])
            peer["connection-status"] = _connection_status(peer_data["latest_handshake"])
        else:
            peer["connection-status"] = "down"

        # Parse endpoint
        if peer_data["endpoint"]:
            addr, port = _parse_endpoint(peer_data["endpoint"])
            if addr:
                peer["endpoint-address"] = addr
            if port:
                peer["endpoint-port"] = port

        # Transfer statistics
        if peer_data["tx_bytes"] or peer_data["rx_bytes"]:
            peer["transfer"] = {
                "tx-bytes": str(peer_data["tx_bytes"]),
                "rx-bytes": str(peer_data["rx_bytes"]),
            }

        peers.append(peer)

    return {"peer-status": {"peer": peers}} if peers else None
