"""
Helpers for WiFi tests: generators for the config snippets every WiFi test
needs (radio components, keystore secrets, wifi interfaces) and readers for
the operational state under /ietf-interfaces.  See doc/wifi.md.
"""
import base64

import infamy.iface as _iface
from infamy.util import until


def radio(name, band=None, channel=None):
    """ietf-hardware component for a WiFi radio."""
    settings = {}
    if band:
        settings["band"] = band
    if channel is not None:
        settings["channel"] = channel
    return {
        "name": name,
        "class": "infix-hardware:wifi",
        "infix-hardware:wifi-radio": settings,
    }


def hardware(*radios, country="SE"):
    """ietf-hardware config: the radios plus the box-wide WiFi country code."""
    return {"hardware": {
        "infix-hardware:wifi": {"country-code": country},
        "component": list(radios),
    }}


def keystore(secrets):
    """ietf-keystore config with a passphrase entry per {name: psk}."""
    return {"keystore": {"symmetric-keys": {"symmetric-key": [
        {
            "name": name,
            "key-format": "infix-crypto-types:passphrase-key-format",
            "cleartext-symmetric-key": base64.b64encode(psk.encode()).decode(),
        } for name, psk in secrets.items()
    ]}}}


def iface(name, wifi, ipv4=None, bridge=None, pvid=None):
    """ietf-interfaces entry for a WiFi VIF.

    wifi is the infix-interfaces:wifi container: the radio plus one of
    access-point, station or mesh-point.  The address is left to the
    box, which gives every VIF one of its own; read it back with
    phys_address().
    """
    ifc = {
        "name": name,
        "type": "infix-if-type:wifi",
        "enabled": True,
        "infix-interfaces:wifi": wifi,
    }
    if ipv4:
        ifc["ietf-ip:ipv4"] = ipv4
    if bridge:
        ifc["infix-interfaces:bridge-port"] = {"bridge": bridge}
        if pvid is not None:
            ifc["infix-interfaces:bridge-port"]["pvid"] = pvid
    return ifc


def wds_link(name, ap, peer, bridge=None, pvid=None):
    """ietf-interfaces entry for a wds-link port of access point ap.

    The port inherits the AP's radio and MAC address, so unlike iface() it
    takes no radio.
    """
    ifc = {
        "name": name,
        "type": "infix-if-type:wifi",
        "enabled": True,
        "infix-interfaces:wifi": {
            "wds-link": {"access-point": ap, "peer-address": peer},
        },
    }
    if bridge:
        ifc["infix-interfaces:bridge-port"] = {"bridge": bridge}
        if pvid is not None:
            ifc["infix-interfaces:bridge-port"]["pvid"] = pvid
    return ifc


def channel_survey(target, radio="radio0", passive=False):
    """Run the channel-survey action on radio, return its channel list.

    Each entry has at least a frequency, the operating channel has
    in-use set.  Values are strings on NETCONF, compare with str().
    """
    xpath = f"/ietf-hardware:hardware/component[name='{radio}']" \
            "/infix-hardware:wifi-radio/channel-survey"
    output = target.call_action_output(xpath, {"passive": passive} if passive else None)
    return output.get("channel", [])


def scan(target, radio="radio0"):
    """Run the scan action on radio, return the networks heard.

    Each entry has a bssid, a frequency and, unless hidden, an ssid.
    Values are strings on NETCONF, compare with str().
    """
    xpath = f"/ietf-hardware:hardware/component[name='{radio}']" \
            "/infix-hardware:wifi-radio/scan"
    output = target.call_action_output(xpath, None)
    return output.get("network", [])


def in_use_frequency(channels):
    """Frequency in MHz of the channel marked in-use, or None."""
    for ch in channels:
        if str(ch.get("in-use")).lower() == "true":
            return int(ch["frequency"])
    return None


def skip_unless_supported(test, *targets):
    """Skip the test unless every target advertises the wifi feature."""
    for target in targets:
        if not target.has_feature("infix-interfaces", "wifi"):
            print("DUT does not advertise the 'wifi' feature -- skipping")
            test.skip()


def _wifi(ifc):
    return (ifc or {}).get("infix-interfaces:wifi") or (ifc or {}).get("wifi") or {}


def phys_address(target, ifname="wifi0"):
    """MAC address of ifname from operational data, lowercase, once it exists."""
    return until(lambda: _iface.get_phys_address(target, ifname),
                 attempts=30, interval=1).lower()


def chassis_address(target):
    """Chassis MAC of target, lowercase."""
    data = target.get_data("/ietf-hardware:hardware/component[name='mainboard']")
    return data["hardware"]["component"]["mainboard"]["phys-address"].lower()


def derived_address(chassis, pos, radio=0):
    """Address a box derives from chassis for the interface at position
    pos (from 1) on its radio number radio (from 0), see doc/wifi.md."""
    mac = [int(x, 16) for x in chassis.split(":")]
    mac[0] = 0x02 | ((pos & 0x3f) << 2)
    mac[1] = (mac[1] + radio) & 0xff
    return ":".join(f"{x:02x}" for x in mac)


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


bridge_vlan_members = _iface.bridge_vlan_members


def ap_ssid(target, ifname="wifi0"):
    """SSID the access point on ifname reports, None when not an AP."""
    return (_wifi(target.get_iface(ifname)).get("access-point") or {}).get("ssid")


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
