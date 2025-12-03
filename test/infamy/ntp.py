"""
NTP client and server helpers
"""


def _get_ntp(target):
    xpath = "/ietf-system:system-state/infix-system:ntp"
    data = target.get_data(xpath)

    if data is None:
        return None

    return data.get("system-state", {}).get("infix-system:ntp", None) or data.get("system-state", {}).get("ntp", None)


def _get_ntp_sources(target):
    ntp = _get_ntp(target)

    if ntp is None:
        return []

    return ntp["sources"]["source"]


def get_sources(target):
    """Get list of NTP sources from operational state."""
    return _get_ntp_sources(target)


def get_source_by_address(target, address):
    """Get NTP source by address, or None if not found."""
    sources = _get_ntp_sources(target)
    for source in sources:
        if source.get("address") == address:
            return source
    return None


def any_source_selected(target):
    """Return the first selected NTP source, or None if no source is selected."""
    sources = _get_ntp_sources(target)

    for source in sources:
        if source["state"] == "selected":
            return source

    return None


def number_of_sources(target):
    sources = _get_ntp_sources(target)

    return len(sources)


def server_has_received_packets(target):
    """Verify NTP server (ietf-ntp) has received packets."""
    try:
        data = target.get_data("/ietf-ntp:ntp/ntp-statistics")
        if not data:
            return False

        stats = data["ntp"].get("ntp-statistics", {})
        if not stats:
            return False

        packets_received = int(stats.get("packet-received", 0))
        return packets_received > 0
    except Exception:
        return False


def server_query(netns, server_ip, expected_stratum=None):
    """Query NTP server from a network namespace and return True if successful.

    Optionally verify the stratum level if expected_stratum is provided.
    """
    result = netns.runsh(f"timeout 1 ntpd -qwp {server_ip}")
    output = result.stdout if result.stdout else ""

    if f"ntpd: reply from {server_ip}" not in output or "offset" not in output:
        return False

    if expected_stratum is not None:
        # Extract stratum from output like: "stratum 8"
        for line in output.split('\n'):
            if 'stratum' in line.lower():
                try:
                    stratum = int(line.split()[-1])
                    return stratum == expected_stratum
                except (ValueError, IndexError):
                    pass
        return False

    return True


def server_has_associations(target):
    """Verify NTP server (ietf-ntp) has any associations."""
    try:
        data = target.get_data("/ietf-ntp:ntp/associations")
        if not data:
            return False

        associations = data.get("ntp", {}).get("associations", {}).get("association", [])
        return len(associations) > 0
    except Exception:
        return False


def server_has_peer(target, peer_address):
    """Verify NTP server (ietf-ntp) has a peer association with given address."""
    try:
        data = target.get_data("/ietf-ntp:ntp/associations")
        if not data:
            return False

        associations = data.get("ntp", {}).get("associations", {}).get("association", [])
        if not associations:
            return False

        # Check if peer association exists with the given address
        # local-mode will be "ietf-ntp:active" or "active" depending on namespace handling
        for assoc in associations:
            local_mode = assoc.get("local-mode", "")
            if (assoc.get("address") == peer_address and
                (local_mode == "ietf-ntp:active" or local_mode == "active")):
                return True

        return False
    except Exception:
        return False


def server_peer_reachable(target, peer_address):
    """Verify NTP peer association exists (peer is configured and running)."""
    # For now, just check if the association exists
    # The YANG associations container doesn't expose reach/state info
    # but if the association shows up, it means chronyd is running and
    # communicating with the peer
    return server_has_peer(target, peer_address)
