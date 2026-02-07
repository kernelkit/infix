"""
WiFi operational state provider using iw.py for interface data.
Scanning still uses wpa_supplicant for better compatibility.
"""
import json
import re

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

    # Get connected stations
    stations = get_iw_stations(ifname)
    if stations:
        ap_data['stations'] = {'station': stations}

    return {'access-point': ap_data} if ap_data else {}


def wifi_station(ifname):
    """Get operational data for Station mode using iw + wpa_cli for scanning"""
    station_data = {}

    # Get link info (includes SSID and signal strength when connected)
    link = get_iw_link(ifname)

    if link.get('connected'):
        if link.get('ssid'):
            station_data['ssid'] = link['ssid']
        if link.get('signal-strength') is not None:
            station_data['signal-strength'] = link['signal-strength']
        if link.get('rx-speed') is not None:
            station_data['rx-speed'] = link['rx-speed']
        if link.get('tx-speed') is not None:
            station_data['tx-speed'] = link['tx-speed']

    # Get scan results from wpa_supplicant (better scan support)
    try:
        data = HOST.run(('wpa_cli', '-i', ifname, 'scan_result'), default='FAIL')
        if data and data != 'FAIL':
            scan_results = parse_wpa_scan_result(data)
            if scan_results:
                station_data['scan-results'] = scan_results
    except Exception:
        pass

    return {'station': station_data} if station_data else {}


def wifi(ifname):
    """Main entry point - detect mode and return appropriate data"""
    info = get_iw_info(ifname)
    mode = info.get('iftype', '').lower()

    result = {}

    if info.get('phy'):
        result['radio'] = info['phy']

    if mode == 'ap':
        result.update(wifi_ap(ifname))
    else:
        result.update(wifi_station(ifname))

    return result


def parse_wpa_scan_result(scan_output):
    """Parse wpa_cli scan_result output"""
    networks = {}

    for line in scan_output.strip().split('\n'):
        try:
            line = line.strip()
            if not line or 'bssid / frequency' in line.lower():
                continue

            # Split by tabs or multiple spaces
            parts = re.split(r'\t+|\s{2,}', line)

            if len(parts) >= 5:
                bssid = parts[0].strip()
                try:
                    frequency = int(parts[1].strip())
                    rssi = int(parts[2].strip())
                except ValueError:
                    continue

                flags = parts[3].strip()
                ssid = parts[4].strip() if len(parts) > 4 else ""
                try:
                    ssid = ssid.encode().decode('unicode_escape').encode('latin-1').decode('utf-8')
                except (UnicodeDecodeError, UnicodeEncodeError):
                    pass
                # Strip control chars (terminal injection risk from rogue APs)
                ssid = ''.join(c for c in ssid if c.isprintable())

                # Skip hidden SSIDs (empty or null-filled)
                if not ssid or ssid.isspace():
                    continue

                encryption = extract_encryption(flags)
                channel = frequency_to_channel(frequency)

                # Keep best signal per SSID
                if ssid not in networks or rssi > networks[ssid]['signal-strength']:
                    networks[ssid] = {
                        'bssid': bssid,
                        'ssid': ssid,
                        'signal-strength': rssi,
                        'encryption': encryption,
                        'channel': channel
                    }
        except Exception:
            continue

    # Sort by signal strength (best first)
    result = list(networks.values())
    result.sort(key=lambda x: x['signal-strength'], reverse=True)

    return result


def frequency_to_channel(frequency):
    """Convert frequency (MHz) to WiFi channel number"""
    freq = int(frequency)

    # 2.4 GHz band (channels 1-14)
    if 2412 <= freq <= 2484:
        if freq == 2484:
            return 14
        return (freq - 2412) // 5 + 1

    # 5 GHz band (channels 36-165)
    elif 5170 <= freq <= 5825:
        return (freq - 5000) // 5

    # 6 GHz band (channels 1-233)
    elif 5955 <= freq <= 7115:
        return (freq - 5950) // 5

    return 0


def extract_encryption(flags):
    """Extract encryption information from flags string"""
    flags = flags.upper()
    encryption_info = {
        'protocols': [],
        'key_mgmt': [],
        'auth_type': 'Unknown'
    }

    # Extract WPA protocols
    if 'WPA3' in flags:
        encryption_info['protocols'].append('WPA3')
    if 'WPA2' in flags:
        encryption_info['protocols'].append('WPA2')
    if 'WPA-' in flags and 'WPA2' not in flags and 'WPA3' not in flags:
        encryption_info['protocols'].append('WPA')

    # Extract key management methods
    if 'PSK' in flags:
        encryption_info['key_mgmt'].append('PSK')
        encryption_info['auth_type'] = 'Personal'
    if 'EAP' in flags:
        encryption_info['key_mgmt'].append('EAP')
        encryption_info['auth_type'] = 'Enterprise'
    if 'SAE' in flags:
        encryption_info['key_mgmt'].append('SAE')
        encryption_info['auth_type'] = 'Personal'
    if 'OWE' in flags:
        encryption_info['key_mgmt'].append('OWE')
        encryption_info['auth_type'] = 'Enhanced Open'

    # Handle special cases
    if 'WEP' in flags:
        return ['WEP']

    if not encryption_info['protocols'] and 'ESS' in flags:
        return ['Open']

    # Return protocols with auth type
    result = []
    for protocol in encryption_info['protocols']:
        if encryption_info['auth_type'] == 'Enterprise':
            result.append(f"{protocol}-Enterprise")
        elif encryption_info['auth_type'] == 'Personal':
            result.append(f"{protocol}-Personal")
        elif encryption_info['auth_type'] == 'Enhanced Open':
            result.append(f"{protocol}-Enhanced-Open")
        else:
            result.append(protocol)

    return result if result else ['Unknown']
