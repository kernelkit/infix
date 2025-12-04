import datetime
import os
import glob
import re

from .common import insert, YangDate
from .host import HOST


def vpd_vendor_extensions(data):
    vendor_extensions = []
    for ext in data:
        vendor_extension = {}
        vendor_extension["iana-enterprise-number"] = ext[0]
        vendor_extension["extension-data"] = ext[1]
        vendor_extensions.append(vendor_extension)
    return vendor_extensions


def vpd_component(vpd):
    component = {}
    component["name"] = vpd.get("board")
    component["infix-hardware:vpd-data"] = {}

    if vpd.get("data"):
        component["class"] = "infix-hardware:vpd"
        if vpd["data"].get("manufacture-date"):
            mfgdate = datetime.datetime.strptime(vpd["data"]["manufacture-date"],
                                                 "%m/%d/%Y %H:%M:%S")
            component["mfg-date"] = mfgdate.strftime("%Y-%m-%dT%H:%M:%SZ")
        if vpd["data"].get("manufacter"):
            component["mfg-name"] = vpd["data"]["manufacturer"]
        if vpd["data"].get("product-name"):
            component["model-name"] = vpd["data"]["product-name"]
        if vpd["data"].get("serial-number"):
            component["serial-num"] = vpd["data"]["serial-number"]

        # Set VPD-data entrys
        for k, v in vpd["data"].items():
            if vpd["data"].get(k):
                if k != "vendor-extension":
                    component["infix-hardware:vpd-data"][k] = v
                else:
                    vendor_extensions=vpd_vendor_extensions(v)
                    component["infix-hardware:vpd-data"]["infix-hardware:vendor-extension"] = vendor_extensions
    return component


def vpd_components(systemjson):
    return [vpd_component(vpd) for vpd in systemjson.get("vpd", {}).values()]


def usb_port_components(systemjson):
    usb_ports = systemjson.get("usb-ports", [])

    ports = []
    for usb_port in usb_ports:
        port = {}
        if usb_port.get("path"):
            # Path now points to the USB device directory, not the attribute
            base_path = usb_port["path"]
            authorized_default_path = os.path.join(base_path, "authorized_default")

            if HOST.exists(authorized_default_path):
                data = int(HOST.read(authorized_default_path))
                enabled = "unlocked" if data == 1 else "locked"
                port["state"] = {}
                port["state"]["admin-state"] = enabled
                port["name"] = usb_port["name"]
                port["class"] = "infix-hardware:usb"
                port["state"]["oper-state"] = "enabled"
                ports.append(port)

    return ports


def motherboard_component(systemjson):
    """
    Create a mainboard/chassis component from system.json data.
    This provides a standard ietf-hardware representation of the main board.
    """
    component = {
        "name": "mainboard",
        "class": "iana-hardware:chassis",
    }

    # Add manufacturer if available (from VPD or defaults)
    if systemjson.get("vendor"):
        component["mfg-name"] = systemjson["vendor"]

    # Add model name (from device tree or VPD)
    if systemjson.get("product-name"):
        component["model-name"] = systemjson["product-name"]

    # Add serial number if available (from VPD)
    if systemjson.get("serial-number"):
        component["serial-num"] = systemjson["serial-number"]

    # Add part number as hardware revision if available
    if systemjson.get("part-number"):
        component["hardware-rev"] = systemjson["part-number"]

    # Add chassis physical address (MAC) if available (from VPD or interface fallback)
    if systemjson.get("mac-address"):
        component["infix-hardware:phys-address"] = systemjson["mac-address"]

    # Set state - admin-state is "unknown" since chassis cannot be
    # administratively controlled (locked/unlocked)
    component["state"] = {
        "admin-state": "unknown",
        "oper-state": "enabled"
    }

    return [component]


def normalize_sensor_name(name):
    """
    Normalize sensor names for cleaner display.

    Examples:
      sfp_2 -> sfp2
      mt7915_phy0 -> phy0
      marvell_alaska_tomte_phy7 -> phy7
      cpu_thermal -> cpu
      pwmfan -> pwmfan

    Strategy:
      1. Strip common suffixes like -thermal/_thermal
      2. Extract well-known sensor type names (phy, sfp, fan, etc.) from
         the end of the name, stripping any vendor/chipset prefix
      3. Remove underscores before trailing numbers (sfp_2 -> sfp2)
    """
    import re

    # Strip common suffixes
    name = name.replace("-thermal", "").replace("_thermal", "")

    # Extract well-known sensor types from end of name, stripping any prefix
    # This handles: mt7915_phy0 -> phy0, marvell_alaska_phy7 -> phy7, etc.
    sensor_types = r'(phy|sfp|fan|temp|sensor|psu|cpu|gpu|memory|disk)'
    match = re.search(rf'.*_({sensor_types}\d*)$', name)
    if match:
        name = match.group(1)

    # Remove underscores before trailing numbers (sfp_2 -> sfp2)
    name = re.sub(r'_(\d+)$', r'\1', name)

    return name


def get_wifi_phy_info():
    """
    Discover WiFi PHYs using iw list command.
    Returns dict: {phy_name: {band: str, iface: str, description: str}}

    Example: {"phy0": {"band": "2.4 GHz", "iface": "wlan0", "description": "WiFi Radio (2.4 GHz)"}}
    """
    phy_info = {}

    try:
        # Use iw to list all PHYs
        output = HOST.run(("iw", "list"), default="")
        if not output:
            return phy_info

        # Parse PHY names from 'Wiphy phy0' lines
        for line in output.splitlines():
            match = re.match(r'Wiphy (phy\d+)', line)
            if match:
                phy = match.group(1)
                info = {"band": "Unknown", "iface": None, "description": None}

                # Simple heuristic: phy0 is usually 2.4 GHz, phy1 is 5 GHz
                # This works for most MediaTek chips (mt7915, mt7921, etc.)
                if phy == "phy0":
                    info["band"] = "2.4 GHz"
                elif phy == "phy1":
                    info["band"] = "5 GHz"
                elif phy == "phy2":
                    info["band"] = "6 GHz"  # WiFi 6E

                phy_info[phy] = info

        # Find associated interfaces using iw dev
        dev_output = HOST.run(("iw", "dev"), default="")
        current_phy = None

        for line in dev_output.splitlines():
            # Match phy line
            if line.startswith("phy#"):
                current_phy = "phy" + line.replace("phy#", "").strip()
            elif current_phy and line.strip().startswith("Interface"):
                # Extract interface name
                parts = line.split()
                if len(parts) >= 2:
                    iface = parts[1]
                    if current_phy in phy_info and not phy_info[current_phy]["iface"]:
                        phy_info[current_phy]["iface"] = iface

        # Build descriptions
        for phy, info in phy_info.items():
            if info["iface"] and info["band"] != "Unknown":
                info["description"] = f"WiFi Radio {info['iface']} ({info['band']})"
            elif info["band"] != "Unknown":
                info["description"] = f"WiFi Radio ({info['band']})"
            elif info["iface"]:
                info["description"] = f"WiFi Radio {info['iface']}"
            else:
                info["description"] = "WiFi Radio"

    except Exception:
        pass

    return phy_info


def hwmon_sensor_components():
    """
    Discover hwmon sensors and create sensor components with parent/child relationships.
    Returns a list of hardware components with sensor-data for temperature,
    fan, voltage, current, and power sensors.

    For devices with multiple sensors (like SFP modules), creates:
    - A parent component representing the device (class: module/container)
    - Child sensor components that reference the parent via "parent" field

    For simple devices with only one sensor, creates standalone sensor components.
    """
    components = []
    device_sensors = {}  # Track {device_base_name: [list of sensor components]}

    def add_sensor(base_name, sensor_component):
        """Helper to track sensors per device"""
        if base_name not in device_sensors:
            device_sensors[base_name] = []
        device_sensors[base_name].append(sensor_component)

    try:
        hwmon_devices = glob.glob("/sys/class/hwmon/hwmon*")

        for hwmon_path in hwmon_devices:
            try:
                name_path = os.path.join(hwmon_path, "name")
                if not HOST.exists(name_path):
                    continue

                device_name = HOST.read(name_path).strip()
                base_name = normalize_sensor_name(device_name)

                # Helper to create sensor component with human-readable description
                def create_sensor(sensor_name, value, value_type, value_scale, label=None):
                    component = {
                        "name": sensor_name,
                        "class": "iana-hardware:sensor",
                        "sensor-data": {
                            "value": value,
                            "value-type": value_type,
                            "value-scale": value_scale,
                            "value-precision": 0,
                            "value-timestamp": str(YangDate()),
                            "oper-status": "ok"
                        }
                    }
                    # Add human-readable description if we have a label
                    if label:
                        # Format label nicely: "RX_power" -> "RX Power", "VCC" -> "VCC"
                        desc = label.replace('_', ' ').title()
                        component["description"] = desc
                    return component

                # Temperature sensors
                for temp_file in glob.glob(os.path.join(hwmon_path, "temp*_input")):
                    try:
                        sensor_num = os.path.basename(temp_file).split('_')[0].replace('temp', '')
                        value = int(HOST.read(temp_file).strip())
                        label_file = os.path.join(hwmon_path, f"temp{sensor_num}_label")
                        raw_label = None
                        if HOST.exists(label_file):
                            raw_label = HOST.read(label_file).strip()
                            label = normalize_sensor_name(raw_label)
                            sensor_name = f"{base_name}-{label}"
                        else:
                            sensor_name = base_name if sensor_num == '1' else f"{base_name}{sensor_num}"
                        add_sensor(base_name, create_sensor(sensor_name, value, "celsius", "milli", raw_label))
                    except (FileNotFoundError, ValueError, IOError):
                        continue

                # Fan sensors (RPM from tachometer)
                for fan_file in glob.glob(os.path.join(hwmon_path, "fan*_input")):
                    try:
                        sensor_num = os.path.basename(fan_file).split('_')[0].replace('fan', '')
                        value = int(HOST.read(fan_file).strip())
                        label_file = os.path.join(hwmon_path, f"fan{sensor_num}_label")
                        raw_label = None
                        if HOST.exists(label_file):
                            raw_label = HOST.read(label_file).strip()
                            label = normalize_sensor_name(raw_label)
                            sensor_name = f"{base_name}-{label}"
                        else:
                            sensor_name = base_name if sensor_num == '1' else f"{base_name}{sensor_num}"
                        add_sensor(base_name, create_sensor(sensor_name, value, "rpm", "units", raw_label))
                    except (FileNotFoundError, ValueError, IOError):
                        continue

                # PWM fan sensors (duty cycle percentage)
                # Only add if no fan*_input exists for this device (avoid duplicates)
                has_rpm_sensor = bool(glob.glob(os.path.join(hwmon_path, "fan*_input")))
                if not has_rpm_sensor:
                    for pwm_file in glob.glob(os.path.join(hwmon_path, "pwm[0-9]*")):
                        # Skip pwm*_enable, pwm*_mode, etc. - only process pwm1, pwm2, etc.
                        pwm_basename = os.path.basename(pwm_file)
                        if not pwm_basename.replace('pwm', '').isdigit():
                            continue
                        try:
                            sensor_num = pwm_basename.replace('pwm', '')
                            pwm_raw = int(HOST.read(pwm_file).strip())
                            # Convert PWM duty cycle (0-255) to percentage (0-100)
                            # Note: Some devices are inverted (255=off, 0=max), but we report as-is
                            # The value represents duty cycle, not necessarily fan speed
                            # Use "other" value-type since PWM duty cycle isn't a standard IETF type
                            value = int((pwm_raw / 255.0) * 100 * 1000)  # Convert to milli-percent (0-100000)
                            label_file = os.path.join(hwmon_path, f"pwm{sensor_num}_label")
                            raw_label = None
                            if HOST.exists(label_file):
                                raw_label = HOST.read(label_file).strip()
                                label = normalize_sensor_name(raw_label)
                                sensor_name = f"{base_name}-{label}"
                            else:
                                sensor_name = base_name if sensor_num == '1' else f"{base_name}{sensor_num}"
                            # Use "PWM Fan" as description so it displays nicely in show hardware
                            add_sensor(base_name, create_sensor(sensor_name, value, "other", "milli", raw_label or "PWM Fan"))
                        except (FileNotFoundError, ValueError, IOError):
                            continue

                # Voltage sensors
                for voltage_file in glob.glob(os.path.join(hwmon_path, "in*_input")):
                    try:
                        sensor_num = os.path.basename(voltage_file).split('_')[0].replace('in', '')
                        value = int(HOST.read(voltage_file).strip())
                        label_file = os.path.join(hwmon_path, f"in{sensor_num}_label")
                        raw_label = None
                        if HOST.exists(label_file):
                            raw_label = HOST.read(label_file).strip()
                            label = normalize_sensor_name(raw_label)
                            sensor_name = f"{base_name}-{label}"
                        else:
                            raw_label = "voltage"
                            sensor_name = f"{base_name}-voltage" if sensor_num == '0' else f"{base_name}-voltage{sensor_num}"
                        add_sensor(base_name, create_sensor(sensor_name, value, "volts-DC", "milli", raw_label))
                    except (FileNotFoundError, ValueError, IOError):
                        continue

                # Current sensors
                for current_file in glob.glob(os.path.join(hwmon_path, "curr*_input")):
                    try:
                        sensor_num = os.path.basename(current_file).split('_')[0].replace('curr', '')
                        value = int(HOST.read(current_file).strip())
                        label_file = os.path.join(hwmon_path, f"curr{sensor_num}_label")
                        raw_label = None
                        if HOST.exists(label_file):
                            raw_label = HOST.read(label_file).strip()
                            label = normalize_sensor_name(raw_label)
                            sensor_name = f"{base_name}-{label}"
                        else:
                            raw_label = "current"
                            sensor_name = f"{base_name}-current" if sensor_num == '1' else f"{base_name}-current{sensor_num}"
                        add_sensor(base_name, create_sensor(sensor_name, value, "amperes", "milli", raw_label))
                    except (FileNotFoundError, ValueError, IOError):
                        continue

                # Power sensors
                for power_file in glob.glob(os.path.join(hwmon_path, "power*_input")):
                    try:
                        sensor_num = os.path.basename(power_file).split('_')[0].replace('power', '')
                        value = int(HOST.read(power_file).strip())
                        label_file = os.path.join(hwmon_path, f"power{sensor_num}_label")
                        raw_label = None
                        if HOST.exists(label_file):
                            raw_label = HOST.read(label_file).strip()
                            label = normalize_sensor_name(raw_label)
                            sensor_name = f"{base_name}-{label}"
                        else:
                            raw_label = "power"
                            sensor_name = f"{base_name}-power" if sensor_num == '1' else f"{base_name}-power{sensor_num}"
                        add_sensor(base_name, create_sensor(sensor_name, value, "watts", "micro", raw_label))
                    except (FileNotFoundError, ValueError, IOError):
                        continue

            except (FileNotFoundError, ValueError, IOError):
                continue

    except Exception:
        pass

    # Now create parent/child relationships
    for base_name, sensors in device_sensors.items():
        if len(sensors) > 1:
            # Multiple sensors: create parent component
            parent = {
                "name": base_name,
                "class": "iana-hardware:module",  # Use "module" for multi-sensor devices like SFP
            }
            components.append(parent)

            # Add parent reference to all child sensors
            for sensor in sensors:
                sensor["parent"] = base_name
                components.append(sensor)
        else:
            # Single sensor: add without parent
            components.extend(sensors)

    # Enrich WiFi PHY sensors with descriptive information
    wifi_info = get_wifi_phy_info()
    for component in components:
        name = component.get("name", "")
        # Match phy0, phy1, etc. sensors
        if name.startswith("phy") and name in wifi_info:
            phy = wifi_info[name]
            # Add WiFi-specific description
            component["description"] = phy["description"]
            # Optionally change class to wifi for WiFi PHY sensors
            if component.get("class") == "iana-hardware:sensor":
                # Keep as sensor but we could create a parent WiFi component later if needed
                pass

    return components


def thermal_sensor_components():
    """
    Discover thermal zones and create sensor components.
    Returns a list of hardware components with sensor-data.
    """
    components = []

    try:
        # Find all thermal zones
        thermal_zones = glob.glob("/sys/class/thermal/thermal_zone*")

        for zone_path in thermal_zones:
            try:
                # Read zone type (e.g., "cpu-thermal", "gpu-thermal")
                type_path = os.path.join(zone_path, "type")
                if not HOST.exists(type_path):
                    continue

                zone_type = HOST.read(type_path).strip()

                # Read temperature in millidegrees Celsius
                temp_path = os.path.join(zone_path, "temp")
                if not HOST.exists(temp_path):
                    continue

                temp_millidegrees = int(HOST.read(temp_path).strip())

                # Create component with sensor-data
                # Component name: strip "-thermal" suffix for cleaner display
                component_name = normalize_sensor_name(zone_type)

                component = {
                    "name": component_name,
                    "class": "iana-hardware:sensor",
                    "sensor-data": {
                        "value": temp_millidegrees,
                        "value-type": "celsius",
                        "value-scale": "milli",
                        "value-precision": 0,
                        "value-timestamp": str(YangDate()),
                        "oper-status": "ok"
                    }
                }

                components.append(component)

            except (FileNotFoundError, ValueError, IOError):
                # Skip this thermal zone if we can't read it
                continue

    except Exception:
        # If we can't access /sys/class/thermal at all, just return empty list
        pass

    return components


def parse_survey_dump(output):
    """Parse iw survey dump output to get channel utilization data"""
    channels = []
    current_channel = None

    for line in output.splitlines():
        line = line.strip()

        # Start of new survey entry
        if line.startswith("Survey data from"):
            if current_channel:
                channels.append(current_channel)
            current_channel = None

        # Frequency line with optional [in use] marker
        elif line.startswith("frequency:"):
            parts = line.split()
            try:
                freq = int(parts[1])
                in_use = "[in use]" in line

                current_channel = {
                    "frequency": freq,
                    "in-use": in_use
                }
            except (ValueError, IndexError):
                continue

        # Parse channel metrics
        elif current_channel:
            try:
                if line.startswith("noise:"):
                    # Extract noise value: "noise: -95 dBm"
                    noise_str = line.split()[1]
                    current_channel["noise"] = int(noise_str)

                elif line.startswith("channel active time:"):
                    # Extract time: "channel active time: 1234 ms"
                    time_str = line.split()[3]
                    current_channel["active-time"] = int(time_str)

                elif line.startswith("channel busy time:"):
                    time_str = line.split()[3]
                    current_channel["busy-time"] = int(time_str)

                elif line.startswith("channel receive time:"):
                    time_str = line.split()[3]
                    current_channel["receive-time"] = int(time_str)

                elif line.startswith("channel transmit time:"):
                    time_str = line.split()[3]
                    current_channel["transmit-time"] = int(time_str)

            except (ValueError, IndexError):
                # Skip malformed lines
                continue

    # Add last channel
    if current_channel:
        channels.append(current_channel)

    return channels


def parse_phy_info(phy_name):
    """Parse 'iw phy <name> info' output to extract operational data"""
    radio_data = {}

    try:
        output = HOST.run(tuple(f"iw phy {phy_name} info".split()), default="")
        if not output:
            return radio_data

        lines = output.splitlines()
        supported_channels = []
        max_txpower = None
        current_freq = None
        current_phy_mode = None

        for i, line in enumerate(lines):
            line = line.strip()

            # Parse frequencies/channels
            # Example: "* 2412 MHz [1] (20.0 dBm)"
            freq_match = re.match(r'\* (\d+) MHz \[(\d+)\].*\(([0-9.]+) dBm\)', line)
            if freq_match:
                freq_mhz = int(freq_match.group(1))
                channel = int(freq_match.group(2))
                power_dbm = float(freq_match.group(3))

                supported_channels.append(channel)

                # Track maximum power across all channels
                if max_txpower is None or power_dbm > max_txpower:
                    max_txpower = int(power_dbm)

            # Parse current channel/frequency (from survey data or interface info)
            # This might not be available if no interface is active
            channel_match = re.search(r'channel (\d+).*\((\d+) MHz\)', line)
            if channel_match:
                current_freq = int(channel_match.group(2))

            # Detect PHY modes from capabilities
            if 'HE RX/TX' in line or '802.11ax' in line:
                current_phy_mode = 'ieee80211ax'
            elif 'VHT Capabilities' in line or '802.11ac' in line:
                current_phy_mode = 'ieee80211ac'
            elif 'HT Capabilities' in line or '802.11n' in line:
                if current_phy_mode != 'ieee80211ax' and \
                   current_phy_mode != 'ieee80211ac':
                    current_phy_mode = 'ieee80211n'

        # Add parsed data
        if supported_channels:
            radio_data['supported-channels'] = sorted(set(supported_channels))

        if max_txpower is not None:
            radio_data['max-txpower'] = max_txpower

        if current_freq is not None:
            radio_data['frequency'] = current_freq

        if current_phy_mode:
            radio_data['current-phy-mode'] = current_phy_mode

    except Exception:
        # If parsing fails, return partial data
        pass

    return radio_data


def count_virtual_interfaces(phy_name):
    """Count number of virtual interfaces on this radio"""
    try:
        output = HOST.run(("iw", "dev"), default="")
        phy_num = phy_name[3:]  # Remove 'phy' prefix
        count = 0

        current_phy = None
        for line in output.splitlines():
            # Match "phy#0" or "wiphy 0"
            phy_match = re.match(r'(phy|wiphy)\s*#?(\d+)', line)
            if phy_match:
                current_phy = phy_match.group(2)

            # Count interfaces on this PHY
            if current_phy == phy_num and 'Interface' in line:
                count += 1

        return count
    except Exception:
        return 0


def wifi_radio_components():
    """
    Create WiFi radio components with complete operational data.
    Returns a list of hardware components for WiFi radios.
    """
    components = []
    wifi_info = get_wifi_phy_info()

    for phy_name, phy_data in wifi_info.items():
        component = {
            "name": phy_name,
            "class": "infix-hardware:wifi",
            "description": phy_data.get("description", "WiFi Radio")
        }

        # Initialize wifi-radio data structure
        wifi_radio_data = {}

        # Get PHY info (supported channels, max TX power, PHY mode, etc.)
        phy_info = parse_phy_info(phy_name)
        wifi_radio_data.update(phy_info)

        # Count virtual interfaces
        num_ifaces = count_virtual_interfaces(phy_name)
        wifi_radio_data['num-virtual-interfaces'] = num_ifaces

        # Get survey data if we have an interface
        iface = phy_data.get("iface")
        if iface:
            try:
                survey_output = HOST.run(tuple(f"iw dev {iface} survey dump".split()), default="")
                channels = parse_survey_dump(survey_output)

                if channels:
                    wifi_radio_data["survey"] = {
                        "channel": channels
                    }
            except Exception:
                # If survey fails, continue without survey data
                pass

        # Add wifi-radio data to component
        if wifi_radio_data:
            component["infix-hardware:wifi-radio"] = wifi_radio_data

        components.append(component)

    return components


def operational():
    systemjson = HOST.read_json("/run/system.json")

    return {
        "ietf-hardware:hardware": {
            "component":
            motherboard_component(systemjson) +
            vpd_components(systemjson) +
            usb_port_components(systemjson) +
            hwmon_sensor_components() +
            thermal_sensor_components() +
            wifi_radio_components() +
            [],
        },
    }
