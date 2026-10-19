"""
WiFi operational state provider using iw.py for interface data.
Scanning still uses wpa_supplicant for better compatibility.
"""
import json

from ..host import HOST


def get_iw_info(ifname):
    """Get interface info via iw.py on target"""
    try:
        data = HOST.run(('/usr/libexec/infix/iw.py', 'info', ifname), default='{}')
        return json.loads(data)
    except Exception:
        pass
    return {}


def get_iw_stations(ifname):
    """Get connected stations via iw.py (AP mode)"""
    try:
        data = HOST.run(('/usr/libexec/infix/iw.py', 'station', ifname), default='[]')
        return json.loads(data)
    except Exception:
        pass
    return []


def get_iw_wds_ports(ifname):
    """Get the WDS ports of an AP via iw.py"""
    try:
        data = HOST.run(('/usr/libexec/infix/iw.py', 'wds', ifname), default='[]')
        return json.loads(data)
    except Exception:
        pass
    return []


def get_iw_mesh_param(ifname):
    """Get mesh parameters via iw.py (mesh point mode)"""
    try:
        data = HOST.run(('/usr/libexec/infix/iw.py', 'mesh', ifname), default='{}')
        return json.loads(data)
    except Exception:
        pass
    return {}


def get_iw_link(ifname):
    """Get link info via iw.py (station mode)"""
    try:
        data = HOST.run(('/usr/libexec/infix/iw.py', 'link', ifname), default='{}')
        result = json.loads(data)
        if result:
            return result
    except Exception:
        pass
    return {'connected': False}


def wifi_ap(ifname):
    """Get operational data for AP mode using iw"""
    ap_data = {}

    # Get interface info (includes SSID for AP mode)
    info = get_iw_info(ifname)

    if info.get('ssid'):
        ap_data['ssid'] = info['ssid']

    # Connected stations, including the 4-address ones the kernel lists
    # under the AP's WDS ports rather than under the AP itself
    stations = get_iw_stations(ifname)
    for port in get_iw_wds_ports(ifname):
        stations += get_iw_stations(port)
    if stations:
        ap_data['stations'] = {'station': stations}

    return {'access-point': ap_data} if ap_data else {}


def decode_wpa_ssid(ssid):
    """Decode an SSID as wpa_supplicant prints it

    wpa_supplicant runs SSIDs through printf_encode(), so anything
    outside printable ASCII arrives as \\xHH.  Control characters are
    dropped, a rogue AP must not get to write escape sequences to a
    terminal.
    """
    try:
        ssid = ssid.encode().decode('unicode_escape').encode('latin-1').decode('utf-8')
    except (UnicodeDecodeError, UnicodeEncodeError):
        pass
    return ''.join(c for c in ssid if c.isprintable())


def get_wpa_status(ifname):
    """Get wpa_supplicant status as a dict of key=value lines"""
    data = HOST.run(('wpa_cli', '-i', ifname, 'status'), default='FAIL')
    if not data or data == 'FAIL':
        return {}

    status = {}
    for line in data.splitlines():
        key, sep, val = line.partition('=')
        if sep:
            status[key.strip()] = val.strip()

    return status


def wifi_mesh(ifname, info=None):
    """Get operational data for mesh point mode using iw"""
    mesh_data = {}

    if info is None:
        info = get_iw_info(ifname)
    # 'iw dev info' reports no SSID for a mesh interface, so ask
    # wpa_supplicant, which is the one that joined the mesh.  Absent
    # until it has, which is the honest answer: no mesh, no mesh id.
    mesh_id = info.get('ssid') or get_wpa_status(ifname).get('ssid')
    if mesh_id:
        mesh_data['mesh-id'] = decode_wpa_ssid(mesh_id)

    fwding = get_iw_mesh_param(ifname).get('mesh_fwding')
    if fwding is not None:
        mesh_data['forwarding'] = bool(fwding)

    peers = get_iw_stations(ifname)
    if peers:
        mesh_data['peers'] = {'peer': peers}

    return {'mesh-point': mesh_data}

def wifi_station(ifname):
    """Get operational data for Station mode"""
    station_data = {}

    # Get link info (includes SSID and signal strength when connected)
    link = get_iw_link(ifname)

    if link.get('connected'):
        if link.get('ssid'):
            station_data['ssid'] = link['ssid']
        if link.get('bssid'):
            station_data['bssid'] = link['bssid']
        if link.get('signal-strength') is not None:
            station_data['signal-strength'] = link['signal-strength']
        if link.get('rx-speed') is not None:
            station_data['rx-speed'] = link['rx-speed']
        if link.get('tx-speed') is not None:
            station_data['tx-speed'] = link['tx-speed']

    return {'station': station_data} if station_data else {}


def wifi_wds(ifname):
    """Operational data for a wds-link port (AP_VLAN).

    The station bound to the port is the only entry in its station dump,
    so that is both the connected flag and the link quality.
    """
    stations = get_iw_stations(ifname)
    if not stations:
        return {'wds-link': {'connected': False}}

    sta = stations[0]
    data = {'connected': True}
    for key in ('signal-strength', 'rx-speed', 'tx-speed'):
        if sta.get(key) is not None:
            data[key] = sta[key]

    return {'wds-link': data}


def wifi(ifname):
    """Main entry point - detect mode and return appropriate data"""
    info = get_iw_info(ifname)
    mode = info.get('iftype', '').lower()

    result = {}

    if info.get('phy'):
        result['radio'] = info['phy']

    if mode == 'ap':
        result.update(wifi_ap(ifname))
    elif mode == 'ap/vlan':
        result.update(wifi_wds(ifname))
    elif mode == 'mesh point':
        result.update(wifi_mesh(ifname, info))
    else:
        result.update(wifi_station(ifname))

    return result


