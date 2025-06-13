from ..host import HOST
import json
import re

def wifi(ifname):
    data=HOST.run(tuple(f"wpa_cli -i {ifname} status".split()), default="")
    wifi_data={}

    if data != "":
        for line in data.splitlines():
            k,v = line.split("=")
            if k == "ssid":
                wifi_data["ssid"] = v
            if k == "wpa_state" and v == "DISCONNECTED": # wpa_suppicant has most likely restarted, restart scanning
                HOST.run(tuple(f"wpa_cli -i {ifname} scan".split()), default="")

        data=HOST.run(tuple(f"wpa_cli -i {ifname} signal_poll".split()), default="FAIL")

        # signal_poll return FAIL not connected
        if data.strip() != "FAIL":
            for line in data.splitlines():
                k,v = line.strip().split("=")
                if k == "RSSI":
                    wifi_data["rssi"]=int(v)
    data=HOST.run(tuple(f"wpa_cli -i {ifname} scan_result".split()), default="FAIL")

    if data != "FAIL":
        wifi_data["scan-results"] = parse_wpa_scan_result(data)

    return wifi_data


def parse_wpa_scan_result(scan_output):
    networks = {}
    lines = scan_output.strip().split('\n')

    # Skip header line and any empty lines
    for line in lines:
        line = line.strip()
        if not line or 'bssid / frequency' in line.lower():
            continue

        # Split by tabs or multiple spaces
        parts = re.split(r'\t+|\s{2,}', line)

        if len(parts) >= 5:
            bssid = parts[0].strip()
            frequency = int(parts[1].strip())
            rssi = int(parts[2].strip())
            flags = parts[3].strip()
            ssid = parts[4].strip() if len(parts) > 4 else ""

            # Skip hidden SSIDs (empty or whitespace only)
            if not ssid or ssid.isspace() or  '\\x00' in ssid:
                continue

            # Extract encryption information from flags
            encryption = extract_encryption(flags)

            # Convert frequency to channel
            channel = frequency_to_channel(frequency)

            # Keep only the network with best (highest) RSSI per SSID
            if ssid not in networks or rssi < networks[ssid]['rssi']:
                networks[ssid] = {
                    'bssid': bssid,
                    'ssid': ssid,
                    'rssi': rssi,
                    'encryption': encryption,
                    'channel': channel
                }
    # Convert to list and sort by RSSI (best first)
    result = list(networks.values())
    result.sort(key=lambda x: x['rssi'], reverse=False)

    return result

def frequency_to_channel(frequency):
    """Convert frequency (MHz) to WiFi channel number"""
    freq = int(frequency)

    # 2.4 GHz band (channels 1-14)
    if 2412 <= freq <= 2484: # Channel 14 is special
        if freq == 2484:
            return 14
        return (freq - 2412) // 5 + 1

    # 5 GHz band (channels 36-165)
    elif 5170 <= freq <= 5825:
        return (freq - 5000) // 5

    # 6 GHz band (channels 1-233)
    elif 5955 <= freq <= 7115:
        return (freq - 5950) // 5

    else:
        return f"Unknown ({freq} MHz)"

def extract_encryption(flags):
    """Extract detailed encryption information from flags string"""
    flags = flags.upper()
    encryption_info = {
        'protocols': [],
        'key_mgmt': [],
        'ciphers': [],
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
    if 'SAE' in flags:  # WPA3 Personal
        encryption_info['key_mgmt'].append('SAE')
        encryption_info['auth_type'] = 'Personal'
    if 'OWE' in flags:  # Enhanced Open (WPA3)
        encryption_info['key_mgmt'].append('OWE')
        encryption_info['auth_type'] = 'Enhanced Open'
    if 'FT' in flags:
        encryption_info['key_mgmt'].append('FT')

    # Extract cipher suites
    if 'CCMP' in flags:
        encryption_info['ciphers'].append('CCMP')
    if 'TKIP' in flags:
        encryption_info['ciphers'].append('TKIP')
    if 'GCMP' in flags:
        encryption_info['ciphers'].append('GCMP')

    # Handle special cases
    if 'WEP' in flags:
        return ['WEP']

    if not encryption_info['protocols'] and 'ESS' in flags:
        return ['Open']

    # Return array of supported protocols with auth type
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
