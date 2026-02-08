import datetime
import os
import re
import sys

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

    Example: {"radio0": {"band": "2.4 GHz", "iface": "wlan0", "description": "WiFi Radio (2.4 GHz)"}}
    """
    phy_info = {}

    try:
        # Use iw.py to list all PHYs
        phys = HOST.run_json(("/usr/libexec/infix/iw.py", "list"), default=[])
        if not phys:
            return phy_info

        # Initialize PHY info for each PHY
        for phy in phys:
            phy_info[phy] = {"band": "Unknown", "iface": None, "description": None}

        # Create a mapping from PHY number to PHY name
        phy_num_to_name = {}
        for phy_name in phy_info.keys():
            # Extract number from radio/phy name (e.g., "0" from "radio0" or "phy0")
            num_match = re.search(r'(\d+)$', phy_name)
            if num_match:
                phy_num = num_match.group(1)
                phy_num_to_name[phy_num] = phy_name

        # Find associated virtual interfaces using iw.py dev
        dev_map = HOST.run_json(("/usr/libexec/infix/iw.py", "dev"), default={})

        # dev_map is a dict mapping PHY numbers to list of interfaces
        for phy_num, interfaces in dev_map.items():
            phy_name = phy_num_to_name.get(phy_num)
            if phy_name and phy_name in phy_info and interfaces:
                # Use the first interface
                phy_info[phy_name]["iface"] = interfaces[0]

        # Build descriptions
        for phy, info in phy_info.items():
            if info["iface"] and info["band"] != "Unknown":
                info["description"] = f"WiFi Radio {phy}"
            elif info["band"] != "Unknown":
                info["description"] = f"WiFi Radio ({info['band']})"
            elif info["iface"]:
                info["description"] = f"WiFi Radio {phy}"
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
        hwmon_entries = HOST.run(("ls", "/sys/class/hwmon"), default="").split()
        hwmon_devices = [os.path.join("/sys/class/hwmon", entry) for entry in hwmon_entries if entry.startswith("hwmon")]

        for hwmon_path in hwmon_devices:
            try:
                name_path = os.path.join(hwmon_path, "name")
                if not HOST.exists(name_path):
                    continue

                device_name = HOST.read(name_path).strip()

                # Check if device/name exists (e.g., for WiFi radios) and use that instead
                device_name_path = os.path.join(hwmon_path, "device", "name")
                if HOST.exists(device_name_path):
                    device_name = HOST.read(device_name_path).strip()

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
                temp_entries = HOST.run(("ls", hwmon_path), default="").split()
                temp_files = [os.path.join(hwmon_path, e) for e in temp_entries if e.startswith("temp") and e.endswith("_input")]
                for temp_file in temp_files:
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
                fan_entries = HOST.run(("ls", hwmon_path), default="").split()
                fan_files = [os.path.join(hwmon_path, e) for e in fan_entries if e.startswith("fan") and e.endswith("_input")]
                for fan_file in fan_files:
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
                has_rpm_sensor = bool(fan_files)
                if not has_rpm_sensor:
                    pwm_entries = HOST.run(("ls", hwmon_path), default="").split()
                    pwm_files = [os.path.join(hwmon_path, e) for e in pwm_entries if e.startswith("pwm") and e[3:].replace('_', '').isdigit() if len(e) > 3]
                    for pwm_file in pwm_files:
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
                voltage_entries = HOST.run(("ls", hwmon_path), default="").split()
                voltage_files = [os.path.join(hwmon_path, e) for e in voltage_entries if e.startswith("in") and e.endswith("_input")]
                for voltage_file in voltage_files:
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
                current_entries = HOST.run(("ls", hwmon_path), default="").split()
                current_files = [os.path.join(hwmon_path, e) for e in current_entries if e.startswith("curr") and e.endswith("_input")]
                for current_file in current_files:
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
                power_entries = HOST.run(("ls", hwmon_path), default="").split()
                power_files = [os.path.join(hwmon_path, e) for e in power_entries if e.startswith("power") and e.endswith("_input")]
                for power_file in power_files:
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
        # Match radio0, radio1, etc. sensors
        if name.startswith("radio") and name in wifi_info:
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
        thermal_entries = HOST.run(("ls", "/sys/class/thermal"), default="").split()
        thermal_zones = [os.path.join("/sys/class/thermal", entry) for entry in thermal_entries if entry.startswith("thermal_zone")]

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


def get_survey_data(ifname):
    """Get channel survey data using iw.py script"""
    channels = []

    try:
        survey_data = HOST.run_json(("/usr/libexec/infix/iw.py", "survey", ifname), default=[])

        for entry in survey_data:
            channel = {
                "frequency": entry.get("frequency"),
                "in-use": entry.get("in_use", False)
            }

            # Add optional fields if present
            if "noise" in entry:
                channel["noise"] = entry["noise"]
            if "active_time" in entry:
                channel["active-time"] = entry["active_time"]
            if "busy_time" in entry:
                channel["busy-time"] = entry["busy_time"]
            if "receive_time" in entry:
                channel["receive-time"] = entry["receive_time"]
            if "transmit_time" in entry:
                channel["transmit-time"] = entry["transmit_time"]

            channels.append(channel)

    except Exception:
        pass

    return channels


def get_phy_info(phy_name):
    """Get complete PHY information using iw.py script"""
    try:
        return HOST.run_json(("/usr/libexec/infix/iw.py", "info", phy_name), default={})
    except Exception:
        return {}


def convert_iw_phy_info_for_yanger(phy_info):
    """
    Convert iw.py phy_info format to yanger format.
    Input: iw.py format with 'bands', 'driver', 'manufacturer', 'interface_combinations'
    Output: yanger format with renamed/restructured fields
    """
    result = {"bands": [], "driver": None, "manufacturer": "Unknown", "max-interfaces": {}}

    # Convert bands - iw.py already uses snake_case for capabilities
    for band in phy_info.get("bands", []):
        band_data = {
            "band": str(band.get("band", 0)),
            "name": band.get("name", "Unknown")
        }

        # Add capability flags (iw.py uses snake_case: ht_capable, vht_capable, he_capable)
        if band.get("ht_capable"):
            band_data["ht-capable"] = True
        if band.get("vht_capable"):
            band_data["vht-capable"] = True
        if band.get("he_capable"):
            band_data["he-capable"] = True

        result["bands"].append(band_data)

    # Copy driver and manufacturer
    if phy_info.get("driver"):
        result["driver"] = phy_info["driver"]
    if phy_info.get("manufacturer"):
        result["manufacturer"] = phy_info["manufacturer"]

    # Convert interface combinations to max-interfaces
    # Find max AP interfaces from combinations
    for comb in phy_info.get("interface_combinations", []):
        for limit in comb.get("limits", []):
            if "AP" in limit.get("types", []):
                ap_max = limit.get("max", 0)
                if "ap" not in result["max-interfaces"] or ap_max > result["max-interfaces"]["ap"]:
                    result["max-interfaces"]["ap"] = ap_max

    return result


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

        # Get complete PHY information from iw.py script
        iw_info = get_phy_info(phy_name)

        # Convert iw.py format to yanger format
        phy_details = convert_iw_phy_info_for_yanger(iw_info)

        # Add manufacturer to component
        if phy_details.get("manufacturer") and phy_details["manufacturer"] != "Unknown":
            component["mfg-name"] = phy_details["manufacturer"]

        # Add bands
        if phy_details.get("bands"):
            wifi_radio_data["bands"] = phy_details["bands"]

        # Add driver
        if phy_details.get("driver"):
            wifi_radio_data["driver"] = phy_details["driver"]

        # Add max-interfaces
        if phy_details.get("max-interfaces"):
            wifi_radio_data["max-interfaces"] = phy_details["max-interfaces"]

        # Add max TX power from iw info
        if iw_info.get("max_txpower"):
            wifi_radio_data["max-txpower"] = iw_info["max_txpower"]

        # Add supported channels from band frequencies
        supported_channels = []
        for band in iw_info.get("bands", []):
            for freq in band.get("frequencies", []):
                # Convert frequency to channel number
                if 2412 <= freq <= 2484:
                    channel = (freq - 2407) // 5
                elif 5170 <= freq <= 5825:
                    channel = (freq - 5000) // 5
                elif 5955 <= freq <= 7115:
                    channel = (freq - 5950) // 5
                else:
                    continue
                supported_channels.append(channel)

        if supported_channels:
            wifi_radio_data['supported-channels'] = sorted(set(supported_channels))

        # Count virtual interfaces from iw info
        num_ifaces = iw_info.get('num_virtual_interfaces', 0)
        wifi_radio_data['num-virtual-interfaces'] = num_ifaces

        # Get survey data if we have an interface
        iface = phy_data.get("iface")
        if iface:
            try:
                channels = get_survey_data(iface)

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


def gps_receiver_components():
    """Discover GPS/GNSS receivers and populate operational state.

    GPS devices are discovered via /dev/gps* symlinks (created by udev rules).
    Status is read from /run/gps-status.json, a cache maintained by statd's
    background GPS monitor (gpsd.c) which streams data from gpsd without
    blocking the operational datastore.
    """
    components = []

    # Discover GPS devices via /dev/gps* symlinks (created by udev rules)
    gps_devices = {}
    for i in range(4):
        dev_path = f"/dev/gps{i}"
        if not HOST.exists(dev_path):
            continue
        # Resolve symlink to actual device (for matching gpsd cache keys)
        actual = HOST.run(("readlink", "-f", dev_path), default="").strip()
        gps_devices[actual] = {
            "name": f"gps{i}",
            "symlink": dev_path,
        }

    if not gps_devices:
        return components

    # Read cached GPS status from statd background monitor
    cache = HOST.read_json("/run/gps-status.json", {})

    # Build hardware components for each discovered GPS device
    for actual_path, dev in gps_devices.items():
        name = dev["name"]
        component = {
            "name": name,
            "class": "infix-hardware:gps",
            "description": "GPS/GNSS Receiver"
        }

        gps_data = {}
        gps_data["device"] = dev["symlink"]

        # Look up cached status by actual device path
        info = cache.get(actual_path, {})

        if info.get("driver"):
            gps_data["driver"] = info["driver"]
        gps_data["activated"] = bool(info.get("activated"))

        mode = info.get("mode", 0)
        if mode == 2:
            gps_data["fix-mode"] = "2d"
        elif mode == 3:
            gps_data["fix-mode"] = "3d"
        else:
            gps_data["fix-mode"] = "none"

        if "lat" in info:
            gps_data["latitude"] = f"{float(info['lat']):.6f}"
        if "lon" in info:
            gps_data["longitude"] = f"{float(info['lon']):.6f}"
        if "altHAE" in info:
            gps_data["altitude"] = f"{float(info['altHAE']):.1f}"

        if "satellites_visible" in info:
            gps_data["satellites-visible"] = int(info["satellites_visible"])
            gps_data["satellites-used"] = int(info.get("satellites_used", 0))

        # Check for PPS device availability
        pps_path = f"/dev/pps{name.replace('gps', '')}"
        gps_data["pps-available"] = HOST.exists(pps_path)

        if gps_data:
            component["infix-hardware:gps-receiver"] = gps_data

        components.append(component)

    return components


def operational():
    systemjson = HOST.read_json("/run/system.json", {})

    return {
        "ietf-hardware:hardware": {
            "component":
            motherboard_component(systemjson) +
            vpd_components(systemjson) +
            usb_port_components(systemjson) +
            hwmon_sensor_components() +
            thermal_sensor_components() +
            wifi_radio_components() +
            gps_receiver_components() +
            [],
        },
    }
