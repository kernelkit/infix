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


def _stations_json(ifnames):
    """Shell command printing what yangerd reports for each interface"""
    names = ",".join(repr(n) for n in ifnames)
    return ("sudo yangerctl get /ietf-interfaces:interfaces | python3 -c "
            "\"import json,sys; d=json.load(sys.stdin); "
            "[print(i['name'], json.dumps(i.get('infix-interfaces:wifi'))[:1500]) "
            "for i in d['ietf-interfaces:interfaces']['interface'] "
            f"if i['name'] in ({names},)]\"")


def diagnose(env, ap=None, ap_ifaces=("wifi0",), clients=(), client_iface="wifi0"):
    """Print the AP and client WiFi state for a failed check.

    ap is one node name or several, when the client may be on any of them.

    Each side as the daemons and the kernel see it, next to what yangerd
    reports, so a failure tells apart WiFi that did not happen from WiFi
    operational data that missed it.  Never raises.
    """
    logs = "yangerd|hostapd|wpa_supplicant"
    probes = []
    for node in ((ap,) if isinstance(ap, str) else ap or ()):
        cmds = []
        for ifname in ap_ifaces:
            cmds += [f"sudo hostapd_cli -i {ifname} all_sta | grep -E '^[0-9a-f]{{2}}:'",
                     f"sudo iw dev {ifname} station dump | grep ^Station"]
        cmds += [_stations_json(ap_ifaces),
                 f"sudo grep -hE '{logs}' /var/log/syslog | "
                 "grep -iE 'wifi|attach|control socket|nl80211|peer|AP-STA|associated' | tail -40"]
        probes.append((node, cmds))
    for client in clients:
        probes.append((client, [
            f"sudo wpa_cli -i {client_iface} status | grep -E 'bssid|freq|ssid|wpa_state'",
            _stations_json((client_iface,)),
            f"sudo grep -hE '{logs}' /var/log/syslog | "
            "grep -iE 'associat|CTRL-EVENT|attach|control socket|peer' | tail -25"]))

    for node, cmds in probes:
        try:
            ssh = env.attach(node, "mgmt", "ssh")
        except Exception as err:
            print(f"diagnose {node}: cannot attach: {err}")
            continue
        for cmd in cmds:
            print(f"diagnose {node}: {cmd[:80]}")
            try:
                out = ssh.runsh(cmd)
            except Exception as err:
                print(f"  failed: {err}")
                continue
            for line in ((out.stdout or "") + (out.stderr or "")).splitlines():
                print(f"  {line}")


def until_diagnosed(env, fn, attempts=10, interval=1, **where):
    """infamy.util.until(), printing diagnose(env, **where) if it gives up"""
    from infamy.util import until
    try:
        return until(fn, attempts=attempts, interval=interval)
    except Exception:
        diagnose(env, **where)
        raise
