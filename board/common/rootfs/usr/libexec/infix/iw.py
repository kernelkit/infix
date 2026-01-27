#!/usr/bin/env python3
"""
iw command wrapper that returns structured JSON data

Usage:
    iw.py list                  - List all PHY devices
    iw.py dev                   - List all interfaces grouped by PHY
    iw.py info <device>         - Get PHY or interface information
    iw.py survey <interface>    - Get channel survey data
"""

import sys
import json
import subprocess
import re
def decode_iw_ssid(ssid):
    """Decode iw escaped SSID (\\xHH) to UTF-8, stripping non-printable chars."""
    try:
        ssid = ssid.encode().decode('unicode_escape').encode('latin-1').decode('utf-8')
    except (UnicodeDecodeError, UnicodeEncodeError):
        return ssid
    return ''.join(c for c in ssid if c.isprintable())


def run_iw(*args):
    """Run iw command and return output"""
    try:
        result = subprocess.run(
            ['iw'] + list(args),
            capture_output=True,
            text=True,
            timeout=5
        )
        if result.returncode == 0:
            return result.stdout
        return None
    except Exception:
        return None


def normalize_phy_name(name):
    """
    Convert radioN to phyN or vice versa based on what exists in sysfs.
    Returns the actual phy name that exists.
    """
    import os

    # Try the name as-is first
    if os.path.exists(f'/sys/class/ieee80211/{name}'):
        return name

    # Try converting radioN <-> phyN
    if name.startswith('radio'):
        phy_name = 'phy' + name[5:]
        if os.path.exists(f'/sys/class/ieee80211/{phy_name}'):
            return phy_name
    elif name.startswith('phy'):
        radio_name = 'radio' + name[3:]
        if os.path.exists(f'/sys/class/ieee80211/{radio_name}'):
            return radio_name

    # Return original if nothing found
    return name


def parse_phy_info(phy_name):
    """
    Parse 'iw phy <name> info' output or 'iw <name> info' output
    Returns: {bands, driver, manufacturer, max_txpower, num_virtual_interfaces, interface_combinations}
    """
    # Normalize the phy name
    actual_phy = normalize_phy_name(phy_name)

    # Try 'iw phy <name> info' first
    output = run_iw('phy', actual_phy, 'info')

    # If that fails, try 'iw <name> info' (some systems support this)
    if not output:
        output = run_iw(actual_phy, 'info')

    if not output:
        return {}

    result = {
        'name': phy_name,
        'bands': [],
        'driver': None,
        'manufacturer': None,
        'max_txpower': None,
        'num_virtual_interfaces': 0,
        'interface_combinations': []
    }

    current_band = None
    band_num = 0
    in_combinations = False
    max_power = None

    for line in output.splitlines():
        stripped = line.strip()

        # Detect band sections
        if stripped.startswith('Band '):
            if current_band and current_band.get('frequencies'):
                result['bands'].append(current_band)
            band_num += 1
            current_band = {
                'band': band_num,
                'frequencies': [],
                'name': None,
                'ht_capable': False,
                'vht_capable': False,
                'he_capable': False
            }
            in_combinations = False

        # Parse frequencies (handle both "2412 MHz" and "2412.0 MHz" formats)
        elif current_band and not in_combinations:
            freq_match = re.match(r'\* ([0-9.]+) MHz.*?\(([0-9.]+) dBm\)', stripped)
            if freq_match:
                freq = int(float(freq_match.group(1)))  # Convert "2412.0" to 2412
                power = float(freq_match.group(2))

                current_band['frequencies'].append(freq)

                # Track max power
                if max_power is None or power > max_power:
                    max_power = power

            # Check capabilities
            if 'HT ' in stripped or 'High Throughput' in stripped:
                current_band['ht_capable'] = True
            if 'VHT' in stripped or 'Very High Throughput' in stripped:
                current_band['vht_capable'] = True
            if 'HE ' in stripped or 'High Efficiency' in stripped:
                current_band['he_capable'] = True

        # Detect interface combinations section
        if 'valid interface combinations:' in stripped.lower():
            in_combinations = True
            continue

        # Parse interface combinations
        if in_combinations:
            if stripped.startswith('*'):
                # Parse combination line
                comb_info = {'limits': []}

                # Parse limits: #{ type } <= max
                limit_matches = re.findall(r'#\{\s*([^}]+)\s*\}\s*<=\s*(\d+)', stripped)
                for types_str, max_val in limit_matches:
                    types = [t.strip() for t in types_str.split(',')]
                    comb_info['limits'].append({
                        'max': int(max_val),
                        'types': types
                    })

                # Parse total
                total_match = re.search(r'total\s*<=\s*(\d+)', stripped)
                if total_match:
                    comb_info['max_total'] = int(total_match.group(1))

                # Parse channels
                channels_match = re.search(r'#channels\s*<=\s*(\d+)', stripped)
                if channels_match:
                    comb_info['num_channels'] = int(channels_match.group(1))

                if comb_info.get('limits') or comb_info.get('max_total'):
                    result['interface_combinations'].append(comb_info)

            elif not stripped.startswith('#') and ':' in stripped and not stripped.startswith('*'):
                # End of combinations section
                in_combinations = False

    # Add last band
    if current_band and current_band.get('frequencies'):
        result['bands'].append(current_band)

    # Determine band names and assign band numbers
    for band in result['bands']:
        if band['frequencies']:
            freq = band['frequencies'][0]
            if 2400 <= freq <= 2500:
                band['name'] = '2.4GHz'
                band['band'] = 1
            elif 5150 <= freq <= 5900:
                band['name'] = '5GHz'
                band['band'] = 2
            elif 5955 <= freq <= 7115:
                band['name'] = '6GHz'
                band['band'] = 3

    # Set max TX power
    if max_power is not None:
        result['max_txpower'] = int(max_power)

    # Get driver and manufacturer from sysfs
    try:
        driver_link = subprocess.run(
            ['readlink', '-f', f'/sys/class/ieee80211/{actual_phy}/device/driver'],
            capture_output=True, text=True, timeout=1
        ).stdout.strip()

        if driver_link:
            driver_name = driver_link.split('/')[-1]
            result['driver'] = driver_name

            # Map driver to manufacturer
            driver_lower = driver_name.lower()
            if 'mt' in driver_lower or 'mediatek' in driver_lower:
                result['manufacturer'] = 'MediaTek Inc.'
            elif 'rtw' in driver_lower or 'realtek' in driver_lower:
                result['manufacturer'] = 'Realtek Semiconductor Corp.'
            elif 'ath' in driver_lower or 'qca' in driver_lower:
                result['manufacturer'] = 'Qualcomm Atheros'
            elif 'iwl' in driver_lower or 'intel' in driver_lower:
                result['manufacturer'] = 'Intel Corporation'
            elif 'brcm' in driver_lower or 'broadcom' in driver_lower:
                result['manufacturer'] = 'Broadcom Inc.'
    except Exception:
        pass

    # Count virtual interfaces
    dev_output = run_iw('dev')
    if dev_output:
        # Extract phy number from actual phy name
        phy_num = None
        if actual_phy.startswith('radio'):
            phy_num = actual_phy[5:]
        elif actual_phy.startswith('phy'):
            phy_num = actual_phy[3:]

        if phy_num:
            count = 0
            current_phy = None
            for line in dev_output.splitlines():
                if line.startswith('phy#'):
                    current_phy = line.replace('phy#', '').strip()
                elif current_phy == phy_num and 'Interface' in line:
                    count += 1
            result['num_virtual_interfaces'] = count

    return result


def parse_interface_info(ifname):
    """
    Parse 'iw dev <name> info' output
    Returns: {ifname, iftype, mac, ssid, frequency, channel, txpower, channel_width}
    """
    output = run_iw('dev', ifname, 'info')
    if not output:
        return {}

    result = {'ifname': ifname}

    for line in output.splitlines():
        stripped = line.strip()

        # Interface type
        if stripped.startswith('type '):
            result['iftype'] = stripped.split()[1]

        # MAC address
        elif stripped.startswith('addr '):
            result['mac'] = stripped.split()[1]

        # SSID
        elif stripped.startswith('ssid '):
            result['ssid'] = decode_iw_ssid(' '.join(stripped.split()[1:]))

        # Channel/frequency
        elif stripped.startswith('channel '):
            parts = stripped.split()
            if len(parts) >= 2:
                result['channel'] = int(parts[1])
            if 'MHz' in stripped:
                freq_match = re.search(r'\((\d+) MHz', stripped)
                if freq_match:
                    result['frequency'] = int(freq_match.group(1))
            # Channel width
            if 'width:' in stripped:
                width_match = re.search(r'width:\s*(\d+)\s*MHz', stripped)
                if width_match:
                    result['channel_width'] = f"{width_match.group(1)} MHz"

        # TX power
        elif stripped.startswith('txpower '):
            power_match = re.search(r'([0-9.]+) dBm', stripped)
            if power_match:
                result['txpower'] = float(power_match.group(1))

    return result


def parse_stations(ifname):
    """
    Parse 'iw dev <name> station dump' output
    Returns: list of connected stations with stats
    """
    output = run_iw('dev', ifname, 'station', 'dump')
    if not output:
        return []

    stations = []
    current = None

    for line in output.splitlines():
        stripped = line.strip()

        # New station entry: "Station aa:bb:cc:dd:ee:ff (on wifiX)"
        if stripped.startswith('Station '):
            if current:
                stations.append(current)
            parts = stripped.split()
            if len(parts) >= 2:
                current = {'mac-address': parts[1].lower()}
            else:
                current = None
            continue

        if not current or ':' not in stripped:
            continue

        key, _, value = stripped.partition(':')
        key = key.strip()
        value = value.strip()

        try:
            if key == 'signal':
                # Format: "-42 dBm" or "-42 [-44, -45] dBm"
                current['signal-strength'] = int(value.split()[0])
            elif key == 'connected time':
                # Format: "123 seconds"
                current['connected-time'] = int(value.split()[0])
            elif key == 'rx bytes':
                current['rx-bytes'] = value  # counter64: string-encoded
            elif key == 'tx bytes':
                current['tx-bytes'] = value  # counter64: string-encoded
            elif key == 'rx packets':
                current['rx-packets'] = value  # counter64: string-encoded
            elif key == 'tx packets':
                current['tx-packets'] = value  # counter64: string-encoded
            elif key == 'tx bitrate':
                # Format: "866.7 MBit/s ..." - convert to 100kbit/s units
                speed_mbps = float(value.split()[0])
                current['tx-speed'] = int(speed_mbps * 10)
            elif key == 'rx bitrate':
                speed_mbps = float(value.split()[0])
                current['rx-speed'] = int(speed_mbps * 10)
            elif key == 'inactive time':
                # Format: "1234 ms"
                current['inactive-time'] = int(value.split()[0])
        except (ValueError, IndexError):
            continue

    if current:
        stations.append(current)

    return stations


def parse_survey(ifname):
    """
    Parse 'iw dev <name> survey dump' output
    Returns: list of {frequency, in_use, noise, active_time, busy_time, receive_time, transmit_time}
    """
    output = run_iw('dev', ifname, 'survey', 'dump')
    if not output:
        return []

    channels = []
    current_channel = None

    for line in output.splitlines():
        stripped = line.strip()

        # New survey entry
        if stripped.startswith('Survey data from'):
            if current_channel:
                channels.append(current_channel)
            current_channel = None

        # Frequency
        elif stripped.startswith('frequency:'):
            parts = stripped.split()
            if len(parts) >= 2:
                freq = int(parts[1])
                in_use = '[in use]' in stripped
                current_channel = {
                    'frequency': freq,
                    'in_use': in_use
                }

        # Channel metrics
        elif current_channel:
            if stripped.startswith('noise:'):
                noise_match = re.search(r'(-?\d+) dBm', stripped)
                if noise_match:
                    current_channel['noise'] = int(noise_match.group(1))

            elif stripped.startswith('channel active time:'):
                time_match = re.search(r'(\d+) ms', stripped)
                if time_match:
                    current_channel['active_time'] = int(time_match.group(1))

            elif stripped.startswith('channel busy time:'):
                time_match = re.search(r'(\d+) ms', stripped)
                if time_match:
                    current_channel['busy_time'] = int(time_match.group(1))

            elif stripped.startswith('channel receive time:'):
                time_match = re.search(r'(\d+) ms', stripped)
                if time_match:
                    current_channel['receive_time'] = int(time_match.group(1))

            elif stripped.startswith('channel transmit time:'):
                time_match = re.search(r'(\d+) ms', stripped)
                if time_match:
                    current_channel['transmit_time'] = int(time_match.group(1))

    # Add last channel
    if current_channel:
        channels.append(current_channel)

    return channels


def parse_list():
    """
    Parse 'iw list' output
    Returns: list of PHY names
    """
    output = run_iw('list')
    if not output:
        return []

    phys = []
    for line in output.splitlines():
        match = re.match(r'Wiphy (phy\d+|radio\d+)', line)
        if match:
            phys.append(match.group(1))

    return phys


def parse_dev():
    """
    Parse 'iw dev' output
    Returns: dict mapping PHY numbers to list of interfaces
    """
    output = run_iw('dev')
    if not output:
        return {}

    result = {}
    current_phy = None

    for line in output.splitlines():
        # PHY line: "phy#0" or "phy#1"
        if line.startswith('phy#'):
            current_phy = line.replace('phy#', '').strip()
            if current_phy not in result:
                result[current_phy] = []
        # Interface line: "    Interface wlan0"
        elif current_phy and 'Interface' in line:
            ifname = line.split('Interface')[1].strip()
            result[current_phy].append(ifname)

    return result


def parse_link(ifname):
    """
    Parse 'iw dev <name> link' output for station mode
    Returns: {connected, bssid, ssid, frequency, signal, tx_bitrate, rx_bitrate}
    """
    output = run_iw('dev', ifname, 'link')
    if not output:
        return {'connected': False}

    if 'Not connected' in output:
        return {'connected': False}

    result = {'connected': True}

    for line in output.splitlines():
        stripped = line.strip()

        # Connected to aa:bb:cc:dd:ee:ff
        if stripped.startswith('Connected to '):
            parts = stripped.split()
            if len(parts) >= 3:
                result['bssid'] = parts[2].lower()

        # SSID: NetworkName
        elif stripped.startswith('SSID: '):
            result['ssid'] = decode_iw_ssid(stripped[6:])

        # freq: 5180
        elif stripped.startswith('freq: '):
            try:
                result['frequency'] = int(stripped[6:])
            except ValueError:
                pass

        # signal: -42 dBm
        elif stripped.startswith('signal: '):
            try:
                result['signal-strength'] = int(stripped.split()[1])
            except (ValueError, IndexError):
                pass

        # tx bitrate: 866.7 MBit/s ...
        elif stripped.startswith('tx bitrate: '):
            try:
                speed = float(stripped.split()[2])
                result['tx-speed'] = int(speed * 10)  # 100kbit/s units
            except (ValueError, IndexError):
                pass

        # rx bitrate: 780.0 MBit/s ...
        elif stripped.startswith('rx bitrate: '):
            try:
                speed = float(stripped.split()[2])
                result['rx-speed'] = int(speed * 10)
            except (ValueError, IndexError):
                pass

    return result


def main():
    if len(sys.argv) < 2:
        print(json.dumps({
            'error': 'Usage: iw.py <command> [device]',
            'commands': {
                'list': 'List all PHY devices',
                'dev': 'List all interfaces grouped by PHY',
                'info': 'Get PHY or interface information (requires device)',
                'survey': 'Get channel survey data (requires interface)',
                'station': 'Get connected stations in AP mode (requires interface)',
                'link': 'Get link info in station mode (requires interface)'
            },
            'examples': [
                'iw.py list',
                'iw.py dev',
                'iw.py info radio0',
                'iw.py info wlan0',
                'iw.py station wifi0',
                'iw.py link wlan0',
                'iw.py survey wlan0'
            ]
        }, indent=2))
        sys.exit(1)

    command = sys.argv[1]

    try:
        if command == 'list':
            data = parse_list()
        elif command == 'dev':
            data = parse_dev()
        elif command == 'info':
            if len(sys.argv) < 3:
                data = {'error': 'info command requires device argument'}
            else:
                device = sys.argv[2]
                # Auto-detect if device is a PHY (phy*/radio*) or interface
                if device.startswith('phy') or device.startswith('radio'):
                    data = parse_phy_info(device)
                else:
                    data = parse_interface_info(device)
        elif command == 'station':
            if len(sys.argv) < 3:
                data = {'error': 'station command requires interface argument'}
            else:
                data = parse_stations(sys.argv[2])
        elif command == 'link':
            if len(sys.argv) < 3:
                data = {'error': 'link command requires interface argument'}
            else:
                data = parse_link(sys.argv[2])
        elif command == 'survey':
            if len(sys.argv) < 3:
                data = {'error': 'survey command requires interface argument'}
            else:
                data = parse_survey(sys.argv[2])
        else:
            data = {'error': f'Unknown command: {command}'}

        print(json.dumps(data, indent=2, ensure_ascii=False))

    except Exception as e:
        print(json.dumps({'error': str(e)}))
        sys.exit(1)


if __name__ == '__main__':
    main()
