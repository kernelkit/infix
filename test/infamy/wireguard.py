"""
Fetch WireGuard interface status from remote device.
"""
import json


def get_peers(target, iface):
    """Get all WireGuard peers from interface as a list"""
    interface = target.get_iface(iface)
    if interface is None:
        return []

    wg = interface.get("wireguard") or interface.get("infix-if-wireguard:wireguard")
    if wg is None:
        return []

    # Peers are under 'peer-status' in operational state
    peer_status = wg.get('peer-status')
    if peer_status is None:
        return []

    # The peer-status container has a 'peer' list inside it
    if isinstance(peer_status, dict):
        peer_list = peer_status.get('peer', [])
        if isinstance(peer_list, list):
            return peer_list
        # Single peer might not be in a list
        if peer_list:
            return [peer_list]
        return []

    # If peer_status is already a list
    if isinstance(peer_status, list):
        return peer_status

    return []


def get_peer_by_pubkey(target, iface, public_key):
    """Get specific WireGuard peer by public key"""
    peers = get_peers(target, iface)

    for peer in peers:
        if peer.get('public-key') == public_key:
            return peer

    return None


def is_peer_up(target, iface, public_key):
    """Check if WireGuard peer connection status is 'up'"""
    peer = get_peer_by_pubkey(target, iface, public_key)
    if peer is None:
        return False

    status = peer.get('connection-status')
    if status is None:
        return False

    return status.lower() == 'up'


def get_peer_endpoint(target, iface, public_key):
    """Get WireGuard peer endpoint"""
    peer = get_peer_by_pubkey(target, iface, public_key)
    if peer is None:
        return None

    return peer.get('endpoint')


def get_peer_transfer(target, iface, public_key):
    """Get WireGuard peer transfer statistics (tx, rx in bytes)"""
    peer = get_peer_by_pubkey(target, iface, public_key)
    if peer is None:
        return None, None

    tx = peer.get('transfer-tx')
    rx = peer.get('transfer-rx')

    return tx, rx


def get_peer_handshake(target, iface, public_key):
    """Get WireGuard peer latest handshake timestamp"""
    peer = get_peer_by_pubkey(target, iface, public_key)
    if peer is None:
        return None

    return peer.get('latest-handshake')
