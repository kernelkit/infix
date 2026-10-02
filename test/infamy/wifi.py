"""
Helpers for WiFi tests: generators for the config snippets every WiFi test
needs (radio components, keystore secrets, wifi interfaces) and readers for
the operational state under /ietf-interfaces.  See doc/wifi.md.
"""
import base64


def radio(name, country="SE", band=None, channel=None):
    """ietf-hardware component for a WiFi radio."""
    settings = {"country-code": country}
    if band:
        settings["band"] = band
    if channel is not None:
        settings["channel"] = channel
    return {
        "name": name,
        "class": "infix-hardware:wifi",
        "infix-hardware:wifi-radio": settings,
    }


def keystore(secrets):
    """ietf-keystore config with a passphrase entry per {name: psk}."""
    return {"keystore": {"symmetric-keys": {"symmetric-key": [
        {
            "name": name,
            "key-format": "infix-crypto-types:passphrase-key-format",
            "cleartext-symmetric-key": base64.b64encode(psk.encode()).decode(),
        } for name, psk in secrets.items()
    ]}}}


def iface(name, mac, wifi, ipv4=None, bridge=None):
    """ietf-interfaces entry for a WiFi VIF.

    wifi is the infix-interfaces:wifi container: the radio plus one of
    access-point, station or mesh-point.
    """
    ifc = {
        "name": name,
        "type": "infix-if-type:wifi",
        "enabled": True,
        "infix-interfaces:custom-phys-address": {"static": mac},
        "infix-interfaces:wifi": wifi,
    }
    if ipv4:
        ifc["ietf-ip:ipv4"] = ipv4
    if bridge:
        ifc["infix-interfaces:bridge-port"] = {"bridge": bridge}
    return ifc


def wds_link(name, radio, ap, peer, bridge=None, pvid=None):
    """ietf-interfaces entry for a wds-link port of access point ap.

    The port inherits the AP's MAC address, so unlike iface() it takes no
    custom-phys-address.
    """
    ifc = {
        "name": name,
        "type": "infix-if-type:wifi",
        "enabled": True,
        "infix-interfaces:wifi": {
            "radio": radio,
            "wds-link": {"access-point": ap, "peer-address": peer},
        },
    }
    if bridge:
        ifc["infix-interfaces:bridge-port"] = {"bridge": bridge}
        if pvid is not None:
            ifc["infix-interfaces:bridge-port"]["pvid"] = pvid
    return ifc


def skip_unless_supported(test, *targets):
    """Skip the test unless every target advertises the wifi feature."""
    for target in targets:
        if not target.has_feature("infix-interfaces", "wifi"):
            print("DUT does not advertise the 'wifi' feature -- skipping")
            test.skip()


def _wifi(ifc):
    return (ifc or {}).get("infix-interfaces:wifi") or (ifc or {}).get("wifi") or {}


def station(target, ifname="wifi0"):
    """Operational station state on ifname, {} when not associated."""
    return _wifi(target.get_iface(ifname)).get("station", {})


def associated(target, ssid, ifname="wifi0"):
    """True once the station on ifname is associated to ssid.

    A signal-strength is only reported for an established association,
    so requiring it filters out a station that is merely configured
    with the SSID but not (yet) associated.
    """
    sta = station(target, ifname)
    return (sta.get("ssid") == ssid) and (sta.get("signal-strength") is not None)


def station_bssid(target, ifname="wifi0"):
    """BSSID the station on ifname is associated to, lowercase."""
    return (station(target, ifname).get("bssid") or "").lower()


def wds_connected(target, ifname):
    """True once the station of the wds-link port ifname is bound to it."""
    return _wifi(target.get_iface(ifname)).get("wds-link", {}).get("connected") is True


def bridge_vlan_members(target, bridge, vid, tagging="untagged"):
    """Ports listed as tagged/untagged members of vid on bridge, from operational."""
    ifc = target.get_iface(bridge) or {}
    br = ifc.get("bridge") or ifc.get("infix-interfaces:bridge") or {}
    for vlan in (br.get("vlans") or {}).get("vlan") or []:
        if vlan.get("vid") == vid:
            return set(vlan.get(tagging) or [])
    return set()


def ap_stations(target, ifname="wifi0"):
    """MACs of the stations currently associated to this AP BSS, lowercase."""
    ap = _wifi(target.get_iface(ifname)).get("access-point") or {}
    stations = (ap.get("stations") or {}).get("station") or []
    return {sta.get("mac-address", "").lower() for sta in stations}


def mesh_point(target, ifname="wifi0"):
    """Operational mesh-point container of ifname, empty if not a mesh point."""
    return _wifi(target.get_iface(ifname)).get("mesh-point") or {}


def mesh_peers(target, ifname="wifi0"):
    """Peers of the mesh point on ifname."""
    return (mesh_point(target, ifname).get("peers") or {}).get("peer") or []


def mesh_peer_macs(target, ifname="wifi0"):
    """MACs of the mesh peers on ifname, lowercase."""
    return {peer.get("mac-address", "").lower() for peer in mesh_peers(target, ifname)}
