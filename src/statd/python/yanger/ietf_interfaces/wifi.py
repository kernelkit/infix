from ..host import HOST
import json
import re


def detect_wifi_mode(ifname):
    """Detect if interface is in AP or Station mode"""
    try:
        output = HOST.run(tuple(f"iw dev {ifname} info".split()), default="")
        for line in output.splitlines():
            if 'type' in line.lower():
                if 'ap' in line.lower():
                    return 'ap'
                else:
                    return 'station'
    except Exception:
        pass

    # Default to station mode
    return 'station'


def find_primary_interface_from_config(ifname):
    """Find primary interface by reading hostapd config files"""
    try:
        file_list = HOST.run(tuple("ls /etc/hostapd-*.conf".split()), default="")
        if not file_list:
            return None

        for config_file in file_list.splitlines():
            config_file = config_file.strip()
            if not config_file:
                continue

            try:
                content = HOST.run(tuple(f"cat {config_file}".split()), default="")
                if not content:
                    continue

                if f"interface={ifname}" in content or f"bss={ifname}" in content:
                    for line in content.splitlines():
                        if line.startswith("interface="):
                            return line.split("=", 1)[1].strip()
            except Exception:
                continue
    except Exception:
        pass
    return None


def wifi_ap(ifname):
    """Get operational data for AP mode using hostapd_cli"""
    ap_data = {}

    try:
        primary_if = find_primary_interface_from_config(ifname)
        if not primary_if:
            return {}

        data = HOST.run(tuple(f"hostapd_cli -i {primary_if} status".split()), default="")
        if not data:
            return {}

        # Find our interface's SSID, different for bss and primary, because it is
        if ifname == primary_if:
            # Primary interface - get ssid[0] or ssid
            for line in data.splitlines():
                if "=" in line:
                    try:
                        k, v = line.split("=", 1)
                        if k in ("ssid[0]", "ssid"):
                            ap_data["ssid"] = v
                            break
                    except ValueError:
                        continue
        else:
            # Secondary BSS - find in BSS array
            bss_idx = None
            for line in data.splitlines():
                if "=" in line:
                    try:
                        k, v = line.split("=", 1)
                        if v == ifname and k.startswith("bss["):
                            bss_idx = k[4:-1]  # Extract index from bss[N]
                            break
                    except ValueError:
                        continue

            if bss_idx:
                for line in data.splitlines():
                    if "=" in line:
                        try:
                            k, v = line.split("=", 1)
                            if k == f"ssid[{bss_idx}]":
                                ap_data["ssid"] = v
                                break
                        except ValueError:
                            continue

        stations_data = HOST.run(tuple(f"iw dev {ifname} station dump".split()), default="")
        stations = parse_iw_stations(stations_data)

        if stations:
            ap_data["stations"] = {
                "station": stations
            }

    except Exception:
        pass

    # Nest data inside access-point container to match YANG schema
    return {
        "access-point": ap_data
    } if ap_data else {}


def parse_iw_stations(output):
    """Parse iw station dump output to get connected stations"""
    stations = []
    current_station = None

    for line in output.splitlines():
        line = line.strip()

        # Station line: "Station aa:bb:cc:dd:ee:ff (on wifiX)"
        if line.startswith("Station "):
            if current_station:
                stations.append(current_station)
            # Extract MAC address
            parts = line.split()
            if len(parts) >= 2:
                current_station = {
                    "mac-address": parts[1].lower()
                }
        elif current_station:
            # Parse station attributes
            try:
                # Lines are in format "key: value" with tabs
                if ":" not in line:
                    continue

                parts = line.split(":", 1)
                key = parts[0].strip()
                value = parts[1].strip()

                if key == "signal":
                    # Format: "-42 dBm" or "-42 [-44] dBm"
                    rssi = int(value.split()[0])
                    current_station["rssi"] = rssi
                elif key == "connected time":
                    # Format: "123 seconds"
                    seconds = int(value.split()[0])
                    current_station["connected-time"] = seconds
                elif key == "rx packets":
                    current_station["rx-packets"] = int(value)
                elif key == "tx packets":
                    current_station["tx-packets"] = int(value)
                elif key == "rx bytes":
                    current_station["rx-bytes"] = int(value)
                elif key == "tx bytes":
                    current_station["tx-bytes"] = int(value)
                elif key == "tx bitrate":
                    # Format: "866.7 MBit/s ..." - extract speed and convert to 100kbit/s units
                    speed_mbps = float(value.split()[0])
                    current_station["tx-speed"] = int(speed_mbps * 10)
                elif key == "rx bitrate":
                    # Format: "780.0 MBit/s ..." - extract speed and convert to 100kbit/s units
                    speed_mbps = float(value.split()[0])
                    current_station["rx-speed"] = int(speed_mbps * 10)
            except (ValueError, KeyError, IndexError):
                # Skip invalid values
                continue

    # Add last station
    if current_station:
        stations.append(current_station)

    return stations


def wifi_station(ifname):
    """Get operational data for Station mode using wpa_cli"""
    station_data = {}

    try:
        data = HOST.run(tuple(f"wpa_cli -i {ifname} status".split()), default="")

        if data != "":
            for line in data.splitlines():
                try:
                    if "=" not in line:
                        continue
                    k, v = line.split("=", 1)
                    if k == "ssid":
                        station_data["ssid"] = v
                except ValueError:
                    # Skip malformed lines
                    continue

            try:
                data = HOST.run(tuple(f"wpa_cli -i {ifname} signal_poll".split()), default="FAIL")

                # signal_poll return FAIL if not connected
                if data.strip() != "FAIL":
                    for line in data.splitlines():
                        try:
                            if "=" not in line:
                                continue
                            k, v = line.strip().split("=", 1)
                            if k == "RSSI":
                                station_data["rssi"] = int(v)
                        except (ValueError, KeyError):
                            # Skip malformed lines or invalid integers
                            continue
            except Exception:
                # If signal_poll fails, continue without RSSI
                pass
    except Exception:
        # If status query fails entirely, continue with scan results
        pass

    try:
        data = HOST.run(tuple(f"wpa_cli -i {ifname} scan_result".split()), default="FAIL")
        if data != "FAIL":
            scan_results = parse_wpa_scan_result(data)
            if scan_results:
                station_data["scan-results"] = scan_results
    except Exception:
        # If scan results fail, just omit them
        pass

    # Always nest data inside station container to match YANG schema
    # In scan-only mode, this will be just scan-results with no ssid/rssi
    return {"station": station_data} if station_data else {}


def wifi(ifname):
    """Main entry point - detect mode and return appropriate data"""
    mode = detect_wifi_mode(ifname)

    if mode == 'ap':
        return wifi_ap(ifname)
    else:
        return wifi_station(ifname)


def parse_wpa_scan_result(scan_output):
    networks = {}
    lines = scan_output.strip().split('\n')

    # Skip header line and any empty lines
    for line in lines:
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
                    # Skip lines with invalid frequency or RSSI
                    continue

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
        except Exception:
            # Skip any malformed scan result lines
            continue

    # Convert to list and sort by RSSI (best first)
    result = list(networks.values())
    result.sort(key=lambda x: x['rssi'], reverse=True)

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
