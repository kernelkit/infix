#!/usr/bin/env python3
import json
import argparse
import sys
import re
import textwrap
import ipaddress
from collections import deque
from datetime import datetime, timezone

UNIT_TEST = False


def compress_interface_list(interfaces):
    """Converts interface list to compact range notation.

    Args:
        interfaces: List of interface names

    Returns:
        str: Compressed representation using ranges

    Algorithm:
        1. Extract prefix+number pairs via regex
        2. Group by prefix, sort numerically
        3. Find consecutive sequences
        4. Format as ranges (e1-e4) or singles (e1)
        5. Combine with non-numeric interfaces

    Examples:
        ['e1', 'e2', 'e3', 'e4'] -> 'e1-e4'
        ['e1', 'e2', 'e4', 'e5'] -> 'e1-e2, e4-e5'
        ['eth0', 'eth1', 'br0'] -> 'eth0-eth1, br0'
    """
    if not interfaces:
        return ""

    if len(interfaces) == 1:
        return interfaces[0]

    # Group interfaces by their prefix (e.g., 'e', 'eth', 'br')
    groups = {}
    standalone = []

    for iface in interfaces:
        # Extract prefix and number using regex
        match = re.match(r'^([a-zA-Z]+)(\d+)$', iface)
        if match:
            prefix = match.group(1)
            number = int(match.group(2))
            if prefix not in groups:
                groups[prefix] = []
            groups[prefix].append((number, iface))
        else:
            # Interface doesn't follow prefix+number pattern
            standalone.append(iface)

    # Process each group to find ranges
    result_parts = []

    for prefix in sorted(groups.keys()):
        # Sort by number
        numbers_and_ifaces = sorted(groups[prefix])
        ranges = []
        start = None
        end = None

        for number, iface in numbers_and_ifaces:
            if start is None:
                # Start new range
                start = number
                end = number
            elif number == end + 1:
                # Extend current range
                end = number
            else:
                # End current range and start new one
                if start == end:
                    ranges.append(f"{prefix}{start}")
                else:
                    ranges.append(f"{prefix}{start}-{prefix}{end}")
                start = number
                end = number

        # Add the final range
        if start is not None:
            if start == end:
                ranges.append(f"{prefix}{start}")
            else:
                ranges.append(f"{prefix}{start}-{prefix}{end}")

        result_parts.extend(ranges)

    # Add standalone interfaces
    result_parts.extend(sorted(standalone))

    return ", ".join(result_parts)


class Pad:
    iface = 16
    proto = 11
    state = 12
    data = 41


class PadMdb:
    bridge = 7
    vlan = 6
    group = 20
    ports = 45


class PadStpPort:
    port = 12
    id = 7
    state = 12
    role = 12
    edge = 6
    designated = 31

    total = 12 + 7 + 12 + 12 + 6 + 31


class PadRoute:
    dest = 30
    pref = 8
    next_hop = 30
    protocol = 6
    uptime = 9

    @staticmethod
    def set(ipv):
        """Set default padding based on the IP version ('ipv4' or 'ipv6')."""
        if ipv == 'ipv4':
            PadRoute.dest = 18
            PadRoute.next_hop = 15
        elif ipv == 'ipv6':
            PadRoute.dest = 43
            PadRoute.next_hop = 39
        else:
            raise ValueError(f"unknown IP version: {ipv}")


class PadSoftware:
    name = 10
    date = 25
    hash = 64
    state = 10
    version = 23


class PadDhcpServer:
    ip = 17
    mac = 19
    host = 21
    cid = 19
    exp = 10


class PadUsbPort:
    title = 30
    name = 20
    state = 10
    oper = 10

    @classmethod
    def table_width(cls):
        """Total width of USB port table"""
        return cls.name + cls.state + cls.oper


class PadSensor:
    name = 30
    value = 20
    status = 10

    @classmethod
    def table_width(cls):
        """Total width of sensor table (matches show system width)"""
        return cls.name + cls.value + cls.status


class PadNtpSource:
    address = 16
    mode = 13
    state = 13
    stratum = 11
    poll = 14


class PadService:
    name = 16
    status = 8
    pid = 8
    description = 40


class PadWifiScan:
    ssid  = 40
    encryption = 30
    signal = 9


class PadLldp:
    interface = 16
    rem_idx = 10
    time = 12
    chassis_id = 20
    port_id = 20


class PadDiskUsage:
    mount = 18
    size = 12
    used = 12
    avail = 12
    percent = 6

    @classmethod
    def table_width(cls):
        """Total width of disk usage table"""
        return cls.mount + cls.size + cls.used + cls.avail + cls.percent


class PadFirewall:
    zone_locked = 2
    zone_name = 21
    zone_type = 6
    zone_data = 34
    zone_services = 38

    zone_flow_to = 20
    zone_flow_action = 14
    zone_flow_policy = 20
    zone_flow_services = 45

    policy_locked = 2
    policy_name = 21
    policy_action = 9
    policy_ingress = 33
    policy_egress = 35

    service_name = 20
    service_ports = 69

    # Firewall log display formatting
    log_time = 15      # ISO format: MM-DD HH:MM:SS
    log_action = 6     # REJECT/DROP + small buffer
    log_iif = 11       # Input interface + small buffer
    log_src = 26       # IPv6 addresses (shortened) or IPv4
    log_dst = 26       # IPv6 addresses (shortened) or IPv4
    log_proto = 5      # TCP/UDP/ICMP + small buffer
    log_port = 5       # Port numbers + small buffer

    @classmethod
    def table_width(cls):
        """Table width for zones/policies tables, used to center matrix"""
        return cls.zone_locked + cls.zone_name + cls.zone_type + cls.zone_data \
            + cls.zone_services


class Decore():
    @staticmethod
    def decorate(sgr, txt, restore="0"):
        return f"\033[{sgr}m{txt}\033[{restore}m"

    @staticmethod
    def invert(txt):
        return Decore.decorate("7", txt)

    @staticmethod
    def bold(txt):
        return Decore.decorate("1", txt)

    @staticmethod
    def red(txt):
        return Decore.decorate("31", txt, "39")

    @staticmethod
    def green(txt):
        return Decore.decorate("32", txt, "39")

    @staticmethod
    def bright_green(txt):
        return Decore.decorate("1;32", txt, "39")

    @staticmethod
    def yellow(txt):
        return Decore.decorate("33", txt, "39")

    @staticmethod
    def bold_yellow(txt):
        return Decore.decorate("1;33", txt, "0")

    @staticmethod
    def flashing_red(txt):
        return Decore.decorate("5;31", txt, "0")

    @staticmethod
    def underline(txt):
        return Decore.decorate("4", txt, "24")

    @staticmethod
    def gray_bg(txt):
        return Decore.decorate("100", txt)

    @staticmethod
    def red_bg(txt):
        return Decore.decorate("41", txt, "49")

    @staticmethod
    def green_bg(txt):
        return Decore.decorate("42", txt, "49")

    @staticmethod
    def yellow_bg(txt):
        return Decore.decorate("43", txt, "49")

    @staticmethod
    def title(txt, len=None, bold=True):
        """Print section header with horizontal bar line above it
        Args:
            txt: The header text to display
            len: Length of horizontal bar line (defaults to len(txt))
            bold: Whether to make the text bold
        """
        length = len if len is not None else len(txt)
        underline = "─" * length
        print(underline)
        if bold:
            print(Decore.bold(txt))
        else:
            print(txt)


def rssi_to_status(rssi):
    if rssi <= -75:
        status = Decore.bright_green("excellent")
    elif rssi <= -65:
        status = Decore.green("good")
    elif rssi <= -50:
        status = Decore.yellow("poor")
    else:
        status = Decore.red("bad")

    return status


def datetime_now():
    if UNIT_TEST:
        return datetime(2023, 1, 1, 12, 0, 0, tzinfo=timezone.utc)
    return datetime.now(timezone.utc)


def get_json_data(default, indata, *args):
    data = indata
    for arg in args:
        if arg in data:
            data = data.get(arg)
        else:
            return default

    return data


def remove_yang_prefix(key):
    parts = key.split(":", 1)
    if len(parts) > 1:
        return parts[1]
    return key


def format_description(label, description, width=60):
    """Format description text with proper line wrapping"""
    if not description:
        return f"{label:<20}:"

    lines = textwrap.wrap(description, width=width)
    if not lines:
        return f"{label:<20}:"

    # First line with label
    result = f"{label:<20}: {lines[0]}"
    # Subsequent lines indented
    for line in lines[1:]:
        result += f"\n{'':<20}  {line}"

    return result


class Date(datetime):
    def _pretty_delta(delta):
        assert(delta.total_seconds() > 0)
        days = delta.days
        seconds = delta.seconds
        hours, remainder = divmod(seconds, 3600)
        minutes, seconds = divmod(remainder, 60)

        segments = (
            ("day", days),
            ("hour", hours),
            ("minute", minutes),
            ("second", seconds),
        )

        for i, seg in enumerate(segments):
            if not seg[1]:
                continue

            out = f"{seg[1]} {seg[0]}{'s' if seg[1] != 1 else ''}"
            if seg[0] == "second":
                return out

            seg = segments[i+1]
            out += f" and {seg[1]} {seg[0]}{'s' if seg[1] != 1 else ''}"
            return out

    def pretty(self):
        now = datetime_now()
        if self < now:
            delta = Date._pretty_delta(now - self)
            return f"{delta} ago"
        elif self > now:
            delta = Date._pretty_delta(self - now)
            return f"in {delta}"

        return "now"

    @classmethod
    def from_yang(cls, ydate):
        """Conver a YANG formatted date string into a Python datetime"""
        if not ydate:
            return None

        date, tz = ydate.split("+")
        tz = tz.replace(":", "")
        return cls.strptime(f"{date}+{tz}", "%Y-%m-%dT%H:%M:%S%z")

class Route:
    def __init__(self, data, ip):
        self.data = data
        self.ip = ip
        self.prefix = data.get(f'ietf-{ip}-unicast-routing:destination-prefix', '')
        self.protocol = data.get('source-protocol', '').split(':')[-1]
        self.last_updated = data.get('last-updated', '')
        self.active = data.get('active', False)
        self.pref = data.get('route-preference', '')
        self.metric = data.get('ietf-ospf:metric', 0)
        self.next_hop = []
        next_hop_list = get_json_data(None, self.data, 'next-hop', 'next-hop-list')
        if next_hop_list:
            for nh in next_hop_list["next-hop"]:
                if nh.get(f"ietf-{ip}-unicast-routing:address"):
                    hop = nh[f"ietf-{ip}-unicast-routing:address"]
                elif nh.get("outgoing-interface"):
                    hop = nh["outgoing-interface"]
                else:
                    hop = "unspecified"

                fib = nh.get('infix-routing:installed', False)
                self.next_hop.append((hop, fib))
        else:
            interface = get_json_data(None, self.data, 'next-hop', 'outgoing-interface')
            address = get_json_data(None, self.data, 'next-hop', f'ietf-{ip}-unicast-routing:next-hop-address')
            special = get_json_data(None, self.data, 'next-hop', 'special-next-hop')

            if address:
                self.next_hop.append(address)
            elif interface:
                self.next_hop.append(interface)
            elif special:
                self.next_hop.append(special)
            else:
                self.next_hop.append("unspecified")

    def get_distance_and_metric(self):
        if isinstance(self.pref, int):
            distance = self.pref
            metric = self.metric
        else:
            distance, metric = 0, 0

        return distance, metric

    def datetime2uptime(self):
        """Convert 'last-updated' string to uptime in AAhBBmCCs format."""
        ONE_DAY_SECOND = 60 * 60 * 24
        ONE_WEEK_SECOND = ONE_DAY_SECOND * 7

        if not self.last_updated:
            return "0h0m0s"

        # Replace the colon in the timezone offset (e.g., +00:00 -> +0000)
        pos = self.last_updated.rfind('+')
        if pos != -1:
            adjusted = self.last_updated[:pos] + self.last_updated[pos:].replace(':', '')
        else:
            adjusted = self.last_updated

        last_updated = datetime.strptime(adjusted, '%Y-%m-%dT%H:%M:%S%z')
        current_time = datetime_now()
        uptime_delta = current_time - last_updated

        total_seconds = int(uptime_delta.total_seconds())
        total_days = total_seconds // ONE_DAY_SECOND
        total_weeks = total_days // 7

        hours = (total_seconds % ONE_DAY_SECOND) // 3600
        minutes = (total_seconds % 3600) // 60
        seconds = total_seconds % 60

        if total_seconds < ONE_DAY_SECOND:
            return f"{hours:02}:{minutes:02}:{seconds:02}"
        elif total_seconds < ONE_WEEK_SECOND:
            return f"{total_days}d{hours:02}h{minutes:02}m"
        else:
            days_remaining = total_days % 7
            return f"{total_weeks:02}w{days_remaining}d{hours:02}h"

    def print(self):
        PadRoute.set(self.ip)
        distance, metric = self.get_distance_and_metric()
        uptime = self.datetime2uptime()
        pref = f"{distance}/{metric}"
        hop, fib = self.next_hop[0]

        row = ">" if self.active else " "
        row += "*" if fib else " "
        row += " "
        row += f"{self.prefix:<{PadRoute.dest}} "
        row += f"{pref:>{PadRoute.pref}} "
        row += f"{hop:<{PadRoute.next_hop}}  "
        row += f"{self.protocol:<{PadRoute.protocol}} "
        row += f"{uptime:>{PadRoute.uptime}}"
        print(row)
        for nh in self.next_hop[1:]:
            hop, fib = nh
            row = " "
            row += "*" if fib else " "
            row += " "
            row += f"{'':<{PadRoute.dest}} "
            row += f"{'':>{PadRoute.pref}} "
            row += f"{hop:<{PadRoute.next_hop}}  "
            print(row)


class Software:
    """Software bundle class """
    def __init__(self, data):
        self.data = data
        self.name = data.get('bootname', '')
        self.size = data.get('size', '')
        self.type = data.get('class', '')
        self.hash = data.get('sha256', '')
        self.state = data.get('state', '')
        self.version = get_json_data('', self.data, 'bundle', 'version')
        self.date = get_json_data('', self.data, 'installed', 'datetime')

    def is_rootfs(self):
        """True if bundle type is 'rootfs'"""
        return self.type == "rootfs"

    def print(self):
        """Brief information about one bundle"""
        row  = f"{self.name:<{PadSoftware.name}}"
        row += f"{self.state:<{PadSoftware.state}}"
        row += f"{self.version:<{PadSoftware.version}}"
        row += f"{self.date:<{PadSoftware.date}}"
        print(row)

    def detail(self):
        """Detailed information about one bundle"""
        print(f"Name      : {self.name}")
        print(f"State     : {self.state}")
        print(f"Version   : {self.version}")
        print(f"Size      : {self.size}")
        print(f"SHA-256   : {self.hash}")
        print(f"Installed : {self.date}")


class USBport:
    def __init__(self, data):
        self.data = data
        self.name = data.get('name', '')
        self.state = get_json_data('', self.data, 'state', 'admin-state')
        self.oper = get_json_data('', self.data, 'state', 'oper-state')

    def print(self):
        row = f"{self.name:<{PadUsbPort.name}}"
        row += f"{self.state:<{PadUsbPort.state}}"
        row += f"{self.oper:<{PadUsbPort.oper}}"
        print(row)


class Sensor:
    def __init__(self, data):
        self.data = data
        self.name = data.get('name', 'unknown')
        self.description = data.get('description')  # Human-readable description
        self.parent = data.get('parent')  # Parent component name
        sensor_data = data.get('sensor-data', {})
        self.value_type = sensor_data.get('value-type', 'unknown')
        self.value = sensor_data.get('value', 0)
        self.value_scale = sensor_data.get('value-scale', 'units')
        self.oper_status = sensor_data.get('oper-status', 'unknown')

    def get_formatted_value(self):
        """Convert sensor value based on scale and type"""
        # Handle temperature sensors
        if self.value_type == 'celsius':
            if self.value_scale == 'milli':
                temp_celsius = self.value / 1000.0
                # Color code based on temperature thresholds
                if temp_celsius < 60:
                    return Decore.green(f"{temp_celsius:.1f} °C")
                elif temp_celsius < 75:
                    return Decore.yellow(f"{temp_celsius:.1f} °C")
                else:
                    return Decore.red(f"{temp_celsius:.1f} °C")
            else:
                return f"{self.value} °C"

        # Handle fan speed sensors
        elif self.value_type == 'rpm':
            return f"{self.value} RPM"

        # Handle voltage sensors
        elif self.value_type == 'volts-DC':
            if self.value_scale == 'milli':
                volts = self.value / 1000.0
                return f"{volts:.2f} VDC"
            else:
                return f"{self.value} VDC"

        # Handle current sensors
        elif self.value_type == 'amperes':
            if self.value_scale == 'milli':
                amps = self.value / 1000.0
                return f"{amps:.3f} A"
            else:
                return f"{self.value} A"

        # Handle power sensors
        elif self.value_type == 'watts':
            if self.value_scale == 'micro':
                watts = self.value / 1000000.0
                return f"{watts:.3f} W"
            elif self.value_scale == 'milli':
                watts = self.value / 1000.0
                return f"{watts:.2f} W"
            else:
                return f"{self.value} W"

        # For unknown sensor types, show raw value
        else:
            return f"{self.value} {self.value_type}"

    def print(self, indent=0):
        import re
        # Add indentation for child sensors
        indent_str = "  " * indent

        # Determine display name: prefer description, fallback to name
        if self.description:
            # Use description if available (e.g., "WiFi Radio wlan0 (2.4 GHz)")
            display_name = self.description
        elif indent > 0 and self.parent:
            # Child sensor: strip parent prefix from name
            # "sfp1-RX_power" -> "RX_power" -> "Rx Power"
            display_name = self.name
            if display_name.startswith(self.parent + "-"):
                display_name = display_name[len(self.parent) + 1:]
            # Format: "RX_power" -> "Rx Power"
            display_name = display_name.replace('_', ' ').replace('-', ' ').title()
        else:
            # Standalone sensor without description: use name as-is
            display_name = self.name

        row = f"{indent_str}{display_name:<{PadSensor.name - len(indent_str)}}"
        # For colored value, pad manually to account for ANSI codes
        value_str = self.get_formatted_value()
        # Count visible characters (strip ANSI codes for length calculation)
        visible_len = len(re.sub(r'\x1b\[[0-9;]*m', '', value_str))
        padding = PadSensor.value - visible_len
        row += value_str + (' ' * padding)
        row += f"{self.oper_status:<{PadSensor.status}}"
        print(row)


class STPBridgeID:
    def __init__(self, id):
        self.id = id

    def __str__(self):
        prio, sysid, addr = (
            self.id["priority"],
            self.id["system-id"],
            self.id["address"]
        )
        return f"{prio:1x}.{sysid:03x}.{addr}"


class STPPortID:
    def __init__(self, id):
        self.id = id

    def __str__(self):
        prio, pid = (
            self.id["priority"],
            self.id["port-id"],
        )
        return f"{prio:1x}.{pid:03x}"


class DhcpServer:
    def __init__(self, data):
        self.data = data
        self.leases = []
        now = datetime.now(timezone.utc)
        for lease in get_json_data([], self.data, 'leases', 'lease'):
            if lease["expires"] == "never":
                exp = " never"
            else:
                dt = datetime.strptime(lease['expires'], '%Y-%m-%dT%H:%M:%S%z')
                seconds = int((dt - now).total_seconds())
                exp = f" {self.format_duration(seconds)}"
            self.leases.append({
               "ip": lease["address"],
               "mac": lease["phys-address"],
               "cid": lease["client-id"],
               "host": lease["hostname"],
               "exp": exp
            })

        stats = get_json_data([], self.data, 'statistics')
        self.out_offers   = stats["out-offers"]
        self.out_acks     = stats["out-acks"]
        self.out_naks     = stats["out-naks"]
        self.in_declines  = stats["in-declines"]
        self.in_discovers = stats["in-discovers"]
        self.in_requests  = stats["in-requests"]
        self.in_releases  = stats["in-releases"]
        self.in_informs   = stats["in-informs"]

    def format_duration(self, seconds):
        """Convert seconds to DDdHHhMMmSSs format, omitting zero values"""
        if seconds < 0:
            return "expired"

        days, remainder = divmod(seconds, 86400)
        hours, remainder = divmod(remainder, 3600)
        minutes, seconds = divmod(remainder, 60)

        parts = []
        if days:
            parts.append(f"{days}d")
        if hours:
            parts.append(f"{hours}h")
        if minutes:
            parts.append(f"{minutes}m")
        if seconds or not parts:
            parts.append(f"{seconds}s")

        return "".join(parts)

    def print(self):
        for lease in self.leases:
            ip   = lease["ip"]
            mac  = lease["mac"]
            cid  = lease["cid"]
            exp  = lease['exp']
            host = lease["host"][:20]
            row  = f"{ip:<{PadDhcpServer.ip}}"
            row += f"{mac:<{PadDhcpServer.mac}}"
            row += f"{host:<{PadDhcpServer.host}}"
            row += f"{cid:<{PadDhcpServer.cid}}"
            row += f"{exp:>{PadDhcpServer.exp - 1}}"
            print(row)

    def print_stats(self):
        print(f"{'DHCP offers sent':<{32}}: {self.out_offers}")
        print(f"{'DHCP ACK messages sent':<{32}}: {self.out_acks}")
        print(f"{'DHCP NAK messages sent':<{32}}: {self.out_naks}")
        print(f"{'DHCP decline messages received':<{32}}: {self.in_declines}")
        print(f"{'DHCP discover messages received':<{32}}: {self.in_discovers}")
        print(f"{'DHCP request messages received':<{32}}: {self.in_requests}")
        print(f"{'DHCP release messages received':<{32}}: {self.in_discovers}")
        print(f"{'DHCP inform messages received':<{32}}: {self.in_discovers}")


class Iface:
    def __init__(self, data):
        self.data = data
        self.name = data.get('name', '')
        self.type = data.get('type', '')
        self.index = data.get('if-index', '')
        self.oper_status = data.get('oper-status', '')
        self.autoneg = get_json_data('unknown', self.data, 'ieee802-ethernet-interface:ethernet',
                                          'auto-negotiation', 'enable')
        self.duplex = get_json_data('', self.data,'ieee802-ethernet-interface:ethernet','duplex')
        self.speed = get_json_data('', self.data, 'ieee802-ethernet-interface:ethernet', 'speed')
        self.phys_address = data.get('phys-address', '')

        self.br_mdb = get_json_data({}, self.data, 'infix-interfaces:bridge', 'multicast-filters')
        self.br_vlans = get_json_data({}, self.data, 'infix-interfaces:bridge', 'vlans', "vlan")
        self.bridge = get_json_data('', self.data, 'infix-interfaces:bridge-port', 'bridge')
        self.pvid = get_json_data('', self.data, 'infix-interfaces:bridge-port', 'pvid')
        self.stp_state = get_json_data('', self.data, 'infix-interfaces:bridge-port',
                                       'stp', 'cist', 'state')

        self.lag_mode = get_json_data('', self.data, 'infix-interfaces:lag', 'mode')
        if self.lag_mode:
            self.lag_type = get_json_data('', self.data, 'infix-interfaces:lag', 'static', 'mode')
            if self.lag_mode == "lacp":
                self.lag_hash = get_json_data('', self.data, 'infix-interfaces:lag', 'lacp', 'hash')
                self.lacp_id = get_json_data('', self.data, 'infix-interfaces:lag', 'lacp', 'aggregator-id')
                self.lacp_actor_key = get_json_data('', self.data, 'infix-interfaces:lag', 'lacp', 'actor-key')
                self.lacp_partner_key = get_json_data('', self.data, 'infix-interfaces:lag', 'lacp', 'partner-key')
                self.lacp_partner_mac = get_json_data('', self.data, 'infix-interfaces:lag', 'lacp', 'partner-mac')
                self.lacp_sys_prio = get_json_data('', self.data, 'infix-interfaces:lag', 'lacp', 'system-priority')
            else:
                self.lag_hash = get_json_data('', self.data, 'infix-interfaces:lag', 'static', 'hash')
            self.link_updelay = get_json_data('', self.data, 'infix-interfaces:lag', 'link-monitor', 'debounce', 'up')
            self.link_downdelay = get_json_data('', self.data, 'infix-interfaces:lag', 'link-monitor', 'debounce', 'down')

            self.lacp_mode = get_json_data('', self.data, 'infix-interfaces:lag', 'lacp', 'mode')
            rate = get_json_data('', self.data, 'infix-interfaces:lag', 'lacp', 'rate')
            self.lacp_rate = "fast (1s)" if rate == "fast" else "slow (30 sec)"

        self.lag = get_json_data('', self.data, 'infix-interfaces:lag-port', 'lag')
        if self.lag:
            self.lag_state = get_json_data('', self.data, 'infix-interfaces:lag-port', 'state')
            self.lacp_id = get_json_data('', self.data, 'infix-interfaces:lag-port', 'lacp', 'aggregator-id')
            self.lacp_state = get_json_data('', self.data, 'infix-interfaces:lag-port', 'lacp', 'actor-state')
            self.lacp_pstate = get_json_data('', self.data, 'infix-interfaces:lag-port', 'lacp', 'partner-state')
            self.link_failures = get_json_data('', self.data, 'infix-interfaces:lag-port', 'link-failures')

        self.containers = get_json_data('', self.data, 'infix-interfaces:container-network', 'containers')


        if data.get('statistics'):
            self.in_octets = data.get('statistics').get('in-octets', '')
            self.out_octets = data.get('statistics').get('out-octets', '')
        else:
            self.in_octets = ''
            self.out_octets = ''

        if self.data.get('ietf-ip:ipv4'):
            self.mtu = self.data.get('ietf-ip:ipv4').get('mtu', '')
            self.ipv4_addr = self.data.get('ietf-ip:ipv4').get('address', '')
        else:
            self.mtu = ''
            self.ipv4_addr = []

        if self.data.get('ietf-ip:ipv6'):
            self.ipv6_addr = self.data.get('ietf-ip:ipv6').get('address', '')
        else:
            self.ipv6_addr = []

        self.gre = self.data.get('infix-interfaces:gre')
        self.vxlan = self.data.get('infix-interfaces:vxlan')
        self.wifi = self.data.get('infix-interfaces:wifi')

        if self.data.get('infix-interfaces:vlan'):
            self.lower_if = self.data.get('infix-interfaces:vlan', None).get('lower-layer-if',None)
        else:
            self.lower_if = ''

    def is_wifi(self):
        return self.type == "infix-if-type:wifi"

    def is_vlan(self):
        return self.type == "infix-if-type:vlan"

    def is_in_container(self):
        # Return negative if cointainer isn't set or is an empty list
        return getattr(self, 'containers', None)

    def is_bridge(self):
        return self.type == "infix-if-type:bridge"

    def is_lag(self):
        return self.type == "infix-if-type:lag"

    def is_veth(self):
        return self.data['type'] == "infix-if-type:veth"

    def is_vxlan(self):
        return self.data['type'] == "infix-if-type:vxlan"

    def is_gre(self):
        return self.data['type'] == "infix-if-type:gre"

    def is_gretap(self):
        return self.data['type'] == "infix-if-type:gretap"

    def oper(self, detail=False):
        """Remap in brief overview to fit column widths."""
        if not detail and self.oper_status == "lower-layer-down":
            return "lower-down"
        return self.oper_status

    def pr_name(self, pipe=""):
        print(f"{pipe}{self.name:<{Pad.iface - len(pipe)}}", end="")

    def pr_proto_ipv4(self, pipe=''):
        for addr in self.ipv4_addr:
            origin = f"({addr['origin']})" if addr.get('origin') else ""

            row =  f"{pipe:<{Pad.iface}}"
            row += f"{'ipv4':<{Pad.proto}}"
            row += f"{'':<{Pad.state}}{addr['ip']}/{addr['prefix-length']} {origin}"
            print(row)

    def pr_proto_ipv6(self, pipe=''):
        for addr in self.ipv6_addr:
            origin = f"({addr['origin']})" if addr.get('origin') else ""

            row =  f"{pipe:<{Pad.iface}}"
            row += f"{'ipv6':<{Pad.proto}}"
            row += f"{'':<{Pad.state}}{addr['ip']}/{addr['prefix-length']} {origin}"
            print(row)

    def _pr_proto_common(self, name, phys_address, pipe=''):
        row = ""
        if len(pipe) > 0:
            row =  f"{pipe:<{Pad.iface}}"

        row += f"{name:<{Pad.proto}}"
        dec = Decore.green if self.oper() == "up" else Decore.red
        row += dec(f"{self.oper().upper():<{Pad.state}}")
        if phys_address:
            row += f"{self.phys_address:<{Pad.data}}"
        return row

    def pr_proto_eth(self, pipe=''):
        row = self._pr_proto_common("ethernet", True, pipe);
        print(row)

    def pr_proto_veth(self, pipe=''):
        row = self._pr_proto_common("veth", True, pipe);

        if self.lower_if:
            row = f"{'':<{Pad.iface}}"
            row += f"{'veth':<{Pad.proto}}"
            row += f"{'':<{Pad.state}}"
            row += f"peer:{self.lower_if}"

        print(row)

    def pr_proto_gretap(self, pipe=''):
        row = self._pr_proto_common("gretap", True, pipe);
        print(row)

    def pr_proto_gre(self, pipe=''):
        row = self._pr_proto_common("gre", False, pipe);
        print(row)

    def pr_proto_vxlan(self, pipe=''):
        row = self._pr_proto_common("vxlan", True, pipe);
        print(row)

    def pr_proto_loopack(self, pipe=''):
        row = self._pr_proto_common("loopback", False, pipe);
        print(row)

    def pr_wifi_ssids(self):
        hdr = (f"{'SSID':<{PadWifiScan.ssid}}"
               f"{'ENCRYPTION':<{PadWifiScan.encryption}}"
               f"{'SIGNAL':<{PadWifiScan.signal}}")

        print(Decore.invert(hdr))
        results = self.wifi.get("scan-results", {})
        for result in results:
            encstr = ", ".join(result["encryption"])
            status = rssi_to_status(result["rssi"])
            row = f"{result['ssid']:<{PadWifiScan.ssid}}"
            row += f"{encstr:<{PadWifiScan.encryption}}"
            row += f"{status:<{PadWifiScan.signal}}"

            print(row)


    def pr_proto_wifi(self, pipe=''):
        row = self._pr_proto_common("ethernet", True, pipe);
        print(row)
        ssid = None
        rssi = None

        if self.wifi:
            rssi=self.wifi.get("rssi")
            ssid=self.wifi.get("ssid")
        if ssid is None:
            ssid="------"

        if rssi is None:
            signal="------"
        else:
            signal=rssi_to_status(rssi)
        data_str = f"ssid: {ssid}, signal: {signal}"

        row =  f"{'':<{Pad.iface}}"
        row += f"{'wifi':<{Pad.proto}}"
        row += f"{'':<{Pad.state}}{data_str}"
        print(row)

    def pr_proto_br(self, br_vlans):
        data_str = ""

        row = f"{'bridge':<{Pad.proto}}"

        if self.oper() == "up":
            dec = Decore.green if self.stp_state == "forwarding" else Decore.yellow
            row += dec(f"{self.stp_state.upper():<{Pad.state}}")
        else:
            row += Decore.red(f"{self.oper().upper():<{Pad.state}}")

        for vlan in br_vlans:
            if 'tagged' in vlan:
                for tagged in vlan['tagged']:
                    if tagged == self.name:
                        if data_str:
                            data_str += f",{vlan['vid']}t"
                        else:
                            data_str += f"vlan:{vlan['vid']}t"
            if 'untagged' in vlan:
                for untagged in vlan['untagged']:
                    if untagged == self.name:
                        if data_str:
                            data_str += f",{vlan['vid']}u"
                        else:
                            data_str += f"vlan:{vlan['vid']}u"
        if self.pvid:
            data_str += f" pvid:{self.pvid}"

        if data_str:
            row += f"{data_str:<{Pad.data}}"

        print(row)

    def pr_bridge(self, _ifaces):
        self.pr_name(pipe="")
        self.pr_proto_br(self.br_vlans)

        lowers = []
        for _iface in [Iface(data) for data in _ifaces]:
            if _iface.bridge and _iface.bridge == self.name:
                lowers.append(_iface)

        if lowers:
            self.pr_proto_eth(pipe='│')
            self.pr_proto_ipv4(pipe='│')
            self.pr_proto_ipv6(pipe='│')
        else:
            self.pr_proto_eth(pipe=' ')
            self.pr_proto_ipv4()
            self.pr_proto_ipv6()

        for i, lower in enumerate(lowers):
            pipe = '└ ' if (i == len(lowers) -1)  else '├ '
            lower.pr_name(pipe)
            lower.pr_proto_br(self.br_vlans)

    def pr_loopback(self):
        self.pr_name(pipe="")
        self.pr_proto_loopack()
        self.pr_proto_ipv4()
        self.pr_proto_ipv6()

    def pr_proto_lag(self, member=True):
        data_str = ""

        row = f"{'lag':<{Pad.proto}}"
        if member:
            state = self.lag_state.upper()
            if self.oper() == "up":
                row += Decore.green(f"{state:<{Pad.state}}")
            else:
                row += Decore.yellow(f"{state:<{Pad.state}}")
            if self.lacp_state:
                lacp = ', '.join(self.lacp_state)
                data_str += lacp
        else:
            dec = Decore.green if self.oper() == "up" else Decore.yellow
            row += dec(f"{self.oper().upper():<{Pad.state}}")
            data_str += f"{self.lag_mode}"
            if self.lag_mode == "lacp":
                data_str += f": {self.lacp_mode}"
                data_str += f", rate: {self.lacp_rate}"
                data_str += f", hash: {self.lag_hash}"
            else:
                data_str += f": {self.lag_type}"
                data_str += f", hash: {self.lag_hash}"

        if data_str:
            row += f"{data_str:<{Pad.data}}"

        print(row)

    def pr_lag(self, _ifaces):
        self.pr_name(pipe="")
        self.pr_proto_lag(member=False)

        lowers = []
        for _iface in [Iface(data) for data in _ifaces]:
            if _iface.lag and _iface.lag == self.name:
                lowers.append(_iface)

        if lowers:
            self.pr_proto_eth(pipe='│')
            self.pr_proto_ipv4(pipe='│')
            self.pr_proto_ipv6(pipe='│')
        else:
            self.pr_proto_eth(pipe=' ')
            self.pr_proto_ipv4()
            self.pr_proto_ipv6()

        for i, lower in enumerate(lowers):
            pipe = '└ ' if (i == len(lowers) -1)  else '├ '
            lower.pr_name(pipe)
            lower.pr_proto_lag()

    def pr_veth(self):
        self.pr_name(pipe="")
        self.pr_proto_veth()
        self.pr_proto_ipv4()
        self.pr_proto_ipv6()

    def pr_gre(self):
        self.pr_name(pipe="")
        self.pr_proto_gre()
        self.pr_proto_ipv4()
        self.pr_proto_ipv6()

    def pr_gretap(self):
        self.pr_name(pipe="")
        self.pr_proto_gretap()
        self.pr_proto_ipv4()
        self.pr_proto_ipv6()

    def pr_vxlan(self):
        self.pr_name(pipe="")
        self.pr_proto_vxlan()
        self.pr_proto_ipv4()
        self.pr_proto_ipv6()

    def pr_wifi(self):
        self.pr_name(pipe="")
        self.pr_proto_wifi()
        self.pr_proto_ipv4()
        self.pr_proto_ipv6()

    def pr_vlan(self, _ifaces):
        self.pr_name(pipe="")
        self.pr_proto_eth()

        if self.lower_if:
            self.pr_proto_ipv4(pipe='│')
            self.pr_proto_ipv6(pipe='│')
        else:
            self.pr_proto_ipv4()
            self.pr_proto_ipv6()
            return

        parent = find_iface(_ifaces, self.lower_if)
        if not parent:
            print(f"Error, didn't find parent interface for vlan {self.name}")
            sys.exit(1)
        parent.pr_name(pipe='└ ')
        parent.pr_proto_eth()

    def pr_container(self):
        row = f"{self.name:<{Pad.iface}}"
        row += f"{'container':<{Pad.proto}}"
        row += f"{'':<{Pad.state}}"
        row += f"{', ' . join(self.containers):<{Pad.data}}"

        print(Decore.gray_bg(row))

    def pr_iface(self):
        if self.is_in_container():
            print(Decore.gray_bg(f"{'owned by container':<{20}}: {', ' . join(self.containers)}"))

        print(f"{'name':<{20}}: {self.name}")
        print(f"{'type':<{20}}: {self.type.split(':')[1]}")
        print(f"{'index':<{20}}: {self.index}")

        if self.mtu:
            print(f"{'mtu':<{20}}: {self.mtu}")
        if self.oper():
            print(f"{'operational status':<{20}}: {self.oper(detail=True)}")

        if self.lower_if:
            print(f"{'lower-layer-if':<{20}}: {self.lower_if}")

        if self.autoneg != 'unknown':
            val = "on" if self.autoneg else "off"
            print(f"{'auto-negotiation':<{20}}: {val}")

        if self.duplex:
            print(f"{'duplex':<{20}}: {self.duplex}")

        if self.speed:
            mbs = float(self.speed) * 1000
            print(f"{'speed':<{20}}: {int(mbs)}")

        if self.phys_address:
            print(f"{'physical address':<{20}}: {self.phys_address}")

        if self.lag_mode:
            print(f"{'lag mode':<{20}}: {self.lag_mode}")
            if self.lag_mode == "lacp":
                print(f"{'lag hash':<{20}}: {self.lag_hash}")
                print(f"{'lacp mode':<{20}}: {self.lacp_mode}")
                print(f"{'lacp rate':<{20}}: {self.lacp_rate}")
                print(f"{'lacp aggregate id':<{20}}: {self.lacp_id}")
                print(f"{'lacp system priority':<{20}}: {self.lacp_sys_prio}")
                print(f"{'lacp actor key':<{20}}: {self.lacp_actor_key}")
                print(f"{'lacp partner key':<{20}}: {self.lacp_partner_key}")
                print(f"{'lacp partner mac':<{20}}: {self.lacp_partner_mac}")
            else:
                print(f"{'lag type':<{20}}: {self.lag_type}")
                print(f"{'lag hash':<{20}}: {self.lag_hash}")
                print(f"{'link debounce up':<{20}}: {self.link_updelay} msec")
                print(f"{'link debounce down':<{20}}: {self.link_downdelay} msec")

        if self.lag:
            print(f"{'lag member':<{20}}: {self.lag}")
            print(f"{'lag member state':<{20}}: {self.lag_state}")
            if self.lacp_state:
                print(f"{'lacp aggregate id':<{20}}: {self.lacp_id}")
                print(f"{'lacp actor state':<{20}}: {', '.join(self.lacp_state)}")
                print(f"{'lacp partner state':<{20}}: {', '.join(self.lacp_pstate)}")
            print(f"{'link failure count':<{20}}: {self.link_failures}")

        if self.ipv4_addr:
            first = True
            for addr in self.ipv4_addr:
                origin = f"({addr['origin']})" if addr.get('origin') else ""
                key = 'ipv4 addresses' if first else ''
                colon = ':' if first else ' '
                row = f"{key:<{20}}{colon} "
                row += f"{addr['ip']}/{addr['prefix-length']} {origin}"
                print(row)
                first = False
        else:
                print(f"{'ipv4 addresses':<{20}}:")

        if self.ipv6_addr:
            first = True
            for addr in self.ipv6_addr:
                origin = f"({addr['origin']})" if addr.get('origin') else ""
                key = 'ipv6 addresses' if first else ''
                colon = ':' if first else ' '
                row = f"{key:<{20}}{colon} "
                row += f"{addr['ip']}/{addr['prefix-length']} {origin}"
                print(row)
                first = False
        else:
                print(f"{'ipv6 addresses':<{20}}:")

        if self.wifi:
            ssid=self.wifi.get('ssid', "----")
            rssi=self.wifi.get('rssi', "----")
            print(f"{'SSID':<{20}}: {ssid}")
            print(f"{'Signal':<{20}}: {rssi}")
            print("")
            self.pr_wifi_ssids()

        if self.gre:
            print(f"{'local address':<{20}}: {self.gre['local']}")
            print(f"{'remote address':<{20}}: {self.gre['remote']}")

        if self.vxlan:
            print(f"{'local address':<{20}}: {self.vxlan['local']}")
            print(f"{'remote address':<{20}}: {self.vxlan['remote']}")
            print(f"{'VxLAN id':<{20}}: {self.vxlan['vni']}")

        if self.in_octets and self.out_octets:
            print(f"{'in-octets':<{20}}: {self.in_octets}")
            print(f"{'out-octets':<{20}}: {self.out_octets}")

        frame = get_json_data([], self.data,'ieee802-ethernet-interface:ethernet',
                              'statistics', 'frame')
        if frame:
            print("")
            for key, val in frame.items():
                key = remove_yang_prefix(key)
                print(f"eth-{key:<{25}}: {val}")

    def pr_mdb(self, bridge):
        for group in self.br_mdb.get("multicast-filter", {}):
            row = f"{bridge:<{PadMdb.bridge}}"
            row += f"{'':<{PadMdb.vlan}}"
            row += f"{group['group']:<{PadMdb.group}}"
            if (group.get("ports")):
                ports = ", ".join(port_dict["port"] for port_dict in group["ports"])
            else:
                ports = ""
            row += f"{ports}"
            print(row)

    def pr_vlans_mdb(self, bridge):
        for vlan in self.br_vlans:
            filters=vlan.get("multicast-filters", {})
            for group in filters.get("multicast-filter", []):
                row = f"{bridge:<{PadMdb.bridge}}"
                row += f"{vlan['vid']:<{PadMdb.vlan}}"
                row += f"{group['group']:<{PadMdb.group}}"
                if (group.get("ports")):
                   ports = ", ".join(port_dict["port"] for port_dict in group["ports"])
                else:
                    ports = ""
                row += f"{ports}"
                print(row)

    def pr_stp(self):
        if not (stp := get_json_data({}, self.data, 'infix-interfaces:bridge', 'stp')):
            return

        if bid := stp["cist"].get("bridge-id"):
            bid = STPBridgeID(bid)
        else:
            bid = "UNKNOWN BRIDGE ID"

        if rid := stp["cist"].get("root-id"):
            rid = STPBridgeID(rid)
        else:
            rid = "none"

        print(f"{'bridge-id':<{20}}: {bid} ({self.name})")
        print(f"{'root-id':<{20}}: {rid}")
        print(f"{'protocol':<{20}}: {stp.get('force-protocol', 'UNKNOWN')}")
        print(f"{'hello time':<{20}}: {stp.get('hello-time', 'UNKNOWN'):2} seconds")
        print(f"{'forward delay':<{20}}: {stp.get('forward-delay', 'UNKNOWN'):2} seconds")
        print(f"{'max age':<{20}}: {stp.get('max-age', 'UNKNOWN'):2} seconds")
        print(f"{'transmit hold count':<{20}}: {stp.get('transmit-hold-count', 'UNKNOWN'):2}")
        print(f"{'max hops':<{20}}: {stp.get('max-hops', 'UNKNOWN'):2}")

        if tc := stp["cist"].get("topology-change"):
            print(f"{'topology change':<{20}}:")
            print(f"{'  count':<{20}}: {tc.get('count', 'UNKNOWN')}")
            print(f"{'  in progress':<{20}}: {'YES' if tc.get('in-progress') else 'no'}")
            print(f"{'  last change':<{20}}: {Date.from_yang(tc.get('time')).pretty()}")
            print(f"{'  port':<{20}}: {tc.get('port', 'UNKNOWN')}")


class LldpNeighbor:
    def __init__(self, iface, data):
        self.interface = iface
        self.remote_index = data.get("remote-index", 0)
        self.time_mark = data.get("time-mark", 0)
        self.chassis_id = data.get("chassis-id", "unknown")
        self.port_id = data.get("port-id", "unknown")

    def print(self):
        row = (
            f"{self.interface:<{PadLldp.interface}}"
            f"{self.remote_index:<{PadLldp.rem_idx}}"
            f"{self.time_mark:<{PadLldp.time}}"
            f"{self.chassis_id:<{PadLldp.chassis_id}}"
            f"{self.port_id:<{PadLldp.port_id}}"
        )
        print(row)


def find_iface(_ifaces, name):
    for _iface in [Iface(data) for data in _ifaces]:
        if _iface.name == name:
            return _iface

    return False


def version_sort(s):
    return [int(x) if x.isdigit() else x for x in re.split(r'(\d+)', s)]


def ifname_sort(iface):
    return version_sort(iface["name"])


def brport_sort(iface):
    brname = iface.get("infix-interfaces:bridge-port", {}).get("bridge", "")
    return version_sort(brname) + version_sort(iface["name"])


def print_interface(iface):
    iface.pr_name()
    iface.pr_proto_eth()
    iface.pr_proto_ipv4()
    iface.pr_proto_ipv6()


def pr_interface_list(json):
    hdr = (f"{'INTERFACE':<{Pad.iface}}"
           f"{'PROTOCOL':<{Pad.proto}}"
           f"{'STATE':<{Pad.state}}"
           f"{'DATA':<{Pad.data}}")

    print(Decore.invert(hdr))

    ifaces = sorted(json["ietf-interfaces:interfaces"]["interface"],
                    key=ifname_sort)
    iface = find_iface(ifaces, "lo")
    if iface:
        iface.pr_loopback()

    for iface in [Iface(data) for data in ifaces]:
        if iface.name == "lo":
            continue

        if iface.is_in_container():
            iface.pr_container()
            continue

        if iface.is_bridge():
            iface.pr_bridge(ifaces)
            continue

        if iface.is_lag():
            iface.pr_lag(ifaces)
            continue

        if iface.is_veth():
            iface.pr_veth()
            continue

        if iface.is_gre():
            iface.pr_gre()
            continue

        if iface.is_gretap():
            iface.pr_gretap()
            continue

        if iface.is_vxlan():
            iface.pr_vxlan()
            continue

        if iface.is_wifi():
            iface.pr_wifi()
            continue

        if iface.is_vlan():
            iface.pr_vlan(ifaces)
            continue

        # These interfaces are printed by their parent, such as bridge
        if iface.lower_if:
            continue
        if iface.bridge:
            continue
        if iface.lag:
            continue

        print_interface(iface)


def show_interfaces(json, name):
    if name:
        if not json.get("ietf-interfaces:interfaces"):
            print(f"No interface data found for \"{name}\"")
            sys.exit(1)
        iface = find_iface(json["ietf-interfaces:interfaces"]["interface"], name)
        if not iface:
            print(f"Interface \"{name}\" not found")
            sys.exit(1)
        else:
            iface.pr_iface()
    else:
        if not json.get("ietf-interfaces:interfaces"):
            print("Error, top level \"ietf-interfaces:interfaces\" missing")
            sys.exit(1)
        pr_interface_list(json)


def show_bridge_mdb(json):
    header_printed = False
    if not json.get("ietf-interfaces:interfaces"):
        print("Error, top level \"ietf-interfaces:interface\" missing")
        sys.exit(1)

    ifaces = sorted(json["ietf-interfaces:interfaces"]["interface"],
                    key=ifname_sort)
    for iface in [Iface(data) for data in ifaces]:
        if iface.type != "infix-if-type:bridge":
            continue
        if not header_printed:
            hdr = (f"{'BRIDGE':<{PadMdb.bridge}}"
                   f"{'VID':<{PadMdb.vlan}}"
                   f"{'GROUP':<{PadMdb.group}}"
                   f"{'PORTS':<{PadMdb.ports}}")
            print(Decore.invert(hdr))
            header_printed = True
        iface.pr_mdb(iface.name)
        iface.pr_vlans_mdb(iface.name)


def show_bridge_stp_port(ifname, brport):
    stp = brport["stp"]

    state = stp["cist"]["state"]
    if state == "forwarding":
        state = Decore.green(f"{state.upper():<{PadStpPort.state}}")
    else:
        state = Decore.yellow(f"{state.upper():<{PadStpPort.state}}")

    role = stp["cist"]["role"]
    if role == "root":
        role = Decore.bold(f"{role:<{PadStpPort.role}}")
    else:
        role = f"{role:<{PadStpPort.role}}"

    designated = "unknown"
    if cdesbr := stp["cist"].get("designated", {}).get("bridge-id"):
        brid = str(STPBridgeID(cdesbr))

        cdesport = stp["cist"]["designated"].get("port-id")
        port = str(STPPortID(cdesport)) if cdesport else "UNKNOWN"
        designated = f"{brid} ({port})"

    row = (
        f"{ifname:<{PadStpPort.port}}"
        f"{str(STPPortID(stp['cist']['port-id'])):<{PadStpPort.id}}"
        f"{state}"
        f"{role}"
        f"{'yes' if stp['edge'] else 'no':<{PadStpPort.edge}}"
        f"{designated:<{PadStpPort.designated}}"
    )
    print(row)


def show_bridge_stp(json):
    if not json.get("ietf-interfaces:interfaces"):
        print("Error, top level \"ietf-interfaces:interface\" missing")
        sys.exit(1)

    brs = sorted(filter(lambda i: i.get("type") == "infix-if-type:bridge",
                        json["ietf-interfaces:interfaces"].get("interface",[])),
                 key=ifname_sort)

    for i, br in enumerate(brs):
        if i:
            print()
        Iface(br).pr_stp()

    ports = sorted(filter(lambda i: i.get("infix-interfaces:bridge-port"),
                          json["ietf-interfaces:interfaces"].get("interface",[])),
                   key=brport_sort)
    if not ports:
        return

    print()
    hdr = (
        f"{'PORT':<{PadStpPort.port}}"
        f"{'ID':<{PadStpPort.id}}"
        f"{'STATE':<{PadStpPort.state}}"
        f"{'ROLE':<{PadStpPort.role}}"
        f"{'EDGE':<{PadStpPort.edge}}"
        f"{'DESIGNATED BRIDGE':<{PadStpPort.designated}}"
    )
    print(Decore.invert(hdr))

    lastbr = None
    for port in ports:
        brport = port["infix-interfaces:bridge-port"]
        if not brport.get("stp"):
            continue

        if brport["bridge"] != lastbr:
            lastbr = brport["bridge"]
            separator = f"{'bridge:'+lastbr:<{PadStpPort.total}}"
            print(Decore.gray_bg(separator))

        show_bridge_stp_port(port["name"], brport)


def show_routing_table(json, ip):
    if not json.get("ietf-routing:routing"):
        print("Error, top level \"ietf-routing:routing\" missing")
        sys.exit(1)

    PadRoute.set(ip)
    hdr = (f"   {'DESTINATION':<{PadRoute.dest}} "
           f"{'PREF':>{PadRoute.pref}} "
           f"{'NEXT-HOP':<{PadRoute.next_hop}}  "
           f"{'PROTO':<{PadRoute.protocol}} "
           f"{'UPTIME':>{PadRoute.uptime}}")

    print(Decore.invert(hdr))
    for rib in get_json_data({}, json, 'ietf-routing:routing', 'ribs', 'rib'):
        if rib["name"] != ip:
            continue

        routes = get_json_data(None, rib, "routes", "route")
        if routes:
            for r in routes:
                route = Route(r, ip)
                route.print()


def find_slot(_slots, name):
    for _slot in [Software(data) for data in _slots]:
        if _slot.name == name:
            return _slot

    return False


def show_software(json, name):
    if not json.get("ietf-system:system-state", "infix-system:software"):
        print("Error, cannot find infix-system:software")
        sys.exit(1)

    software = get_json_data({}, json, 'ietf-system:system-state', 'infix-system:software')
    slots = software.get("slot")
    boot_order = software.get("boot-order", ["Unknown"])
    if name:
        slot = find_slot(slots, name)
        if slot:
            slot.detail()
    else:
        print(Decore.invert("BOOT ORDER"))
        order=""
        for boot in boot_order:
            order+=f"{boot.strip()} "
        print(order)
        print("")

        hdr = (f"{'NAME':<{PadSoftware.name}}"
               f"{'STATE':<{PadSoftware.state}}"
               f"{'VERSION':<{PadSoftware.version}}"
               f"{'DATE':<{PadSoftware.date}}")
        print(Decore.invert(hdr))
        for _s in reversed(slots):
            slot = Software(_s)
            if slot.is_rootfs():
                slot.print()


def show_services(json):
    if not json.get("ietf-system:system-state", "infix-system:services"):
        print("Error, cannot find infix-system:services")
        sys.exit(1)

    services_data = get_json_data({}, json, 'ietf-system:system-state', 'infix-system:services')
    services = services_data.get("service", [])

    hdr = (f"{'NAME':<{PadService.name}}"
           f"{'STATUS':<{PadService.status}}"
           f"{'PID':>{PadService.pid -1}}"
           f"  {'DESCRIPTION'}")
    print(Decore.invert(hdr))

    for svc in services:
        name = svc.get('name', '')
        status = svc.get('status', '')
        pid = svc.get('pid', 0)
        description = svc.get('description', '')

        if status in ('running', 'active', 'done'):
            status_str = Decore.green(status)
        elif status in ('crashed', 'failed', 'halted', 'missing', 'dead', 'conflict'):
            status_str = Decore.red(status)
        else:
            status_str = Decore.yellow(status)

        pid_str = str(pid) if pid > 0 else '-'

        row  = f"{name:<{PadService.name}}"
        row += f"{status_str:<{PadService.status + 9}}"
        row += f"{pid_str:>{PadService.pid}}"
        row += f"  {description}"
        print(row)


def show_hardware(json):
    if not json.get("ietf-hardware:hardware"):
        print("Error, top level \"ietf-hardware:component\" missing")
        sys.exit(1)

    components = get_json_data({}, json, "ietf-hardware:hardware", "component")

    motherboard = [c for c in components if c.get("class") == "iana-hardware:chassis"]
    usb_ports = [c for c in components if c.get("class") == "infix-hardware:usb"]
    sensors = [c for c in components if c.get("class") == "iana-hardware:sensor"]

    # Determine overall width (use the wider of the two sections)
    width = max(PadUsbPort.table_width(), PadSensor.table_width())

    # Display full-width inverted heading
    print(Decore.invert(f"{'HARDWARE COMPONENTS':<{width}}"))

    if motherboard:
        board = motherboard[0]  # Should only be one
        Decore.title("Board Information", width)

        if board.get("model-name"):
            print(f"Model               : {board['model-name']}")
        if board.get("mfg-name"):
            print(f"Manufacturer        : {board['mfg-name']}")
        if board.get("serial-num"):
            print(f"Serial Number       : {board['serial-num']}")
        if board.get("hardware-rev"):
            print(f"Hardware Revision   : {board['hardware-rev']}")

    if usb_ports:
        Decore.title("USB Ports", width)
        hdr = (f"{'NAME':<{PadUsbPort.name}}"
               f"{'STATE':<{PadUsbPort.state}}"
               f"{'OPER':<{PadUsbPort.oper}}")
        # Pad header to full width
        hdr = f"{hdr:<{width}}"
        print(Decore.invert(hdr))

        for component in usb_ports:
            port = USBport(component)
            port.print()

    if sensors:
        Decore.title("Sensors", width)

        # Print header
        hdr = (f"{'NAME':<{PadSensor.name}}"
               f"{'VALUE':<{PadSensor.value}}"
               f"{'STATUS':<{PadSensor.status}}")
        print(Decore.invert(hdr))

        # Build parent-child map
        children = {}  # parent_name -> [child_components]
        standalone = []  # components without parents

        for component in sensors:
            parent = component.get("parent")
            if parent:
                if parent not in children:
                    children[parent] = []
                children[parent].append(component)
            else:
                standalone.append(component)

        # Get all parent modules (non-sensor components)
        modules = [c for c in components if c.get("class") == "iana-hardware:module"]

        # Display modules with their child sensors (indented)
        for module in sorted(modules, key=lambda m: m.get("name", "")):
            module_name = module.get("name", "unknown")
            print(f"\n{module_name}:")

            if module_name in children:
                for child in sorted(children[module_name], key=lambda c: c.get("name", "")):
                    sensor = Sensor(child)
                    sensor.print(indent=1)

        # Display standalone sensors (no parent)
        if standalone:
            if modules:
                print()  # Add blank line between modules and standalone
            for component in sorted(standalone, key=lambda c: c.get("name", "")):
                sensor = Sensor(component)
                sensor.print()


def show_ntp(json):
    if not json.get("ietf-system:system-state"):
        print("NTP client not enabled.")
        return
    hdr =  (f"{'ADDRESS':<{PadNtpSource.address}}"
            f"{'MODE':<{PadNtpSource.mode}}"
            f"{'STATE':<{PadNtpSource.state}}"
            f"{'STRATUM':>{PadNtpSource.stratum}}"
            f"{'POLL-INTERVAL':>{PadNtpSource.poll}}"
            )
    print(Decore.invert(hdr))
    sources = get_json_data({}, json, 'ietf-system:system-state', 'infix-system:ntp', 'sources', 'source')
    for source in sources:
        row = f"{source['address']:<{PadNtpSource.address}}"
        row += f"{source['mode']:<{PadNtpSource.mode}}"
        row += f"{source['state'] if source['state'] != 'not-combined' else 'not combined':<{PadNtpSource.state}}"
        row += f"{source['stratum']:>{PadNtpSource.stratum}}"
        row += f"{source['poll']:>{PadNtpSource.poll}}"
        print(row)


def show_system(json):
    """System information overivew"""
    if not json.get("ietf-system:system-state"):
        print("Error: No system data available.")
        sys.exit(1)

    system_state = json["ietf-system:system-state"]
    platform = system_state.get("platform", {})
    clock = system_state.get("clock", {})
    software = system_state.get("infix-system:software", {})
    runtime = json.get("runtime", {})

    # Calculate uptime
    uptime_str = "Unknown"
    if clock.get("current-datetime") and clock.get("boot-datetime"):
        try:
            current = datetime.fromisoformat(clock["current-datetime"].replace("Z", "+00:00"))
            boot = datetime.fromisoformat(clock["boot-datetime"].replace("Z", "+00:00"))
            uptime = current - boot
            days = uptime.days
            hours, remainder = divmod(uptime.seconds, 3600)
            minutes, seconds = divmod(remainder, 60)
            uptime_str = f"{days}d {hours:02d}:{minutes:02d}:{seconds:02d}"
        except (ValueError, KeyError):
            pass

    width = PadDiskUsage.table_width()
    print(Decore.invert(f"{'SYSTEM INFORMATION':<{width}}"))
    print(f"{'OS Name':<20}: {platform.get('os-name', 'Unknown')}")
    print(f"{'OS Version':<20}: {platform.get('os-version', 'Unknown')}")
    print(f"{'Architecture':<20}: {platform.get('machine', 'Unknown')}")

    booted = software.get("booted", "Unknown")
    slots = software.get("slot", [])
    booted_slot = None
    for slot in slots:
        if slot.get("state") == "booted":
            booted_slot = slot
            break

    if booted_slot:
        bundle = booted_slot.get("bundle", {})
        print(f"{'Boot Partition':<20}: {booted} ({bundle.get('version', 'Unknown')})")
    else:
        print(f"{'Boot Partition':<20}: {booted}")

    # Format current time more readably: "2025-10-18 13:23:47 +00:00"
    current_time = clock.get('current-datetime', 'Unknown')
    if current_time != 'Unknown':
        try:
            dt = datetime.fromisoformat(current_time.replace("Z", "+00:00"))
            # Format as "YYYY-MM-DD HH:MM:SS +HH:MM" (keep UTC offset)
            current_time = dt.strftime('%Y-%m-%d %H:%M:%S %z')
            # Insert colon in timezone offset: +0000 -> +00:00
            if len(current_time) >= 5 and current_time[-5] in ['+', '-']:
                current_time = current_time[:-2] + ':' + current_time[-2:]
        except (ValueError, AttributeError):
            pass

    print(f"{'Current Time':<20}: {current_time}")
    print(f"{'Uptime':<20}: {uptime_str}")

    Decore.title("Status", width)

    load = runtime.get("load", {})
    if load:
        print(f"{'Load Average':<20}: {load.get('1min', '?')}, {load.get('5min', '?')}, {load.get('15min', '?')}")

    memory = runtime.get("memory", {})
    if memory:
        total = memory.get("MemTotal", 0)
        available = memory.get("MemAvailable", 0)
        used = total - available
        percent = int((used / total * 100)) if total > 0 else 0
        print(f"{'Memory':<20}: {used} / {total} MB ({percent}% used)")

    # Show CPU temperature and fan speed on one line
    cpu_temp = runtime.get("cpu_temp")
    fan_rpm = runtime.get("fan_rpm")

    if cpu_temp is not None or fan_rpm is not None:
        status_line = ""

        # Add CPU temperature with color coding
        if cpu_temp is not None:
            temp_str = f"{cpu_temp:.1f} °C"
            if cpu_temp < 60:
                temp_colored = Decore.green(temp_str)
            elif cpu_temp < 75:
                temp_colored = Decore.yellow(temp_str)
            else:
                temp_colored = Decore.red(temp_str)
            status_line = f"CPU: {temp_colored}"

        # Add fan speed if available
        if fan_rpm is not None:
            if status_line:
                status_line += f", Fan: {fan_rpm} RPM"
            else:
                status_line = f"Fan: {fan_rpm} RPM"

        if status_line:
            print(f"{'Hardware':<20}: {status_line}")

    disk = runtime.get("disk", [])
    # Filter out root partition (/) - it's read-only and shows confusing 100% usage
    disk_filtered = [d for d in disk if d.get("mount") != "/"]
    if disk_filtered:
        Decore.title("Disk Usage", width)
        hdr = (f"{'MOUNTPOINT':<{PadDiskUsage.mount}}"
               f"{'SIZE':>{PadDiskUsage.size}}"
               f"{'USED':>{PadDiskUsage.used}}"
               f"{'AVAIL':>{PadDiskUsage.avail}}"
               f"{'USE%':>{PadDiskUsage.percent}}")
        print(Decore.invert(hdr))
        for d in disk_filtered:
            mount = d.get("mount", "?")
            size = d.get("size", "?")
            used = d.get("used", "?")
            avail = d.get("available", "?")
            percent = d.get("percent", "?")
            print(f"{mount:<{PadDiskUsage.mount}}"
                  f"{size:>{PadDiskUsage.size}}"
                  f"{used:>{PadDiskUsage.used}}"
                  f"{avail:>{PadDiskUsage.avail}}"
                  f"{percent:>{PadDiskUsage.percent}}")


def show_dhcp_server(json, stats):
    data = json.get("infix-dhcp-server:dhcp-server")
    if not data:
        print("DHCP server not enabled.")
        return

    server = DhcpServer(data)

    if stats:
        server.print_stats()
    else:
        hdr = (f"{'IP ADDRESS':<{PadDhcpServer.ip}}"
               f"{'MAC':<{PadDhcpServer.mac}}"
               f"{'HOSTNAME':<{PadDhcpServer.host}}"
               f"{'CLIENT ID':<{PadDhcpServer.cid}}"
               f"{'EXPIRES':>{PadDhcpServer.exp}}")
        print(Decore.invert(hdr))
        server.print()


def show_lldp(json):
    if not json.get("ieee802-dot1ab-lldp:lldp"):
        print("Error: No LLDP data available.")
        sys.exit(1)

    lldp_ports = json["ieee802-dot1ab-lldp:lldp"].get("port", [])

    if not lldp_ports:
        print("No LLDP neighbors found.")
        return

    header = (
        f"{'INTERFACE':<{PadLldp.interface}}"
        f"{'REM-IDX':<{PadLldp.rem_idx}}"
        f"{'TIME':<{PadLldp.time}}"
        f"{'CHASSIS-ID':<{PadLldp.chassis_id}}"
        f"{'PORT-ID':<{PadLldp.port_id}}"
    )

    print(Decore.invert(header))

    for port_data in lldp_ports:
        port_name = port_data["name"]
        neighbors = port_data.get("remote-systems-data", [])

        for neighbor in neighbors:
            entry = LldpNeighbor(port_name, neighbor)
            entry.print()


def parse_firewall_log_line(line):
    """Parse a single firewall log line into structured data"""

    # Look for kernel logs with netfilter IN=/OUT= fields
    if not ('kernel' in line and 'IN=' in line and 'OUT=' in line):
        return None

    # Extract timestamp from syslog format: Aug 17 12:34:56
    timestamp_match = re.match(r'^(\w{3}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2})', line.strip())
    if not timestamp_match:
        return None

    timestamp = timestamp_match.group(1)

    # Look for action indicator in the log line
    action = 'DROP'
    if 'REJECT' in line:
        action = 'REJECT'

    # Extract key fields from netfilter log
    patterns = {
        'in_iface': r'IN=([^\s]*)',
        'out_iface': r'OUT=([^\s]*)',
        'src': r'SRC=([^\s]+)',
        'dst': r'DST=([^\s]+)',
        'proto': r'PROTO=([^\s]+)',
        'spt': r'SPT=([^\s]+)',
        'dpt': r'DPT=([^\s]+)',
    }

    parsed = {'timestamp': timestamp, 'action': action}

    for key, pattern in patterns.items():
        match = re.search(pattern, line)
        value = match.group(1) if match else ''

        # Compress any IPv6 addresses for src and dst
        if key in ['src', 'dst'] and value:
            try:
                ip = ipaddress.ip_address(value)
                if isinstance(ip, ipaddress.IPv6Address):
                    value = str(ip.compressed)
            except ValueError:
                # Not a valid IP address, keep original value
                pass

        parsed[key] = value

    return parsed


def show_firewall_logs(limit=10):
    """Show recent firewall log entries, tail -N equivalent"""
    try:
        hdr = (f"{'TIME':<{PadFirewall.log_time}} "
               f"{'ACTION':>{PadFirewall.log_action}} "
               f"{'IIF':>{PadFirewall.log_iif}} "
               f"{'SOURCE':<{PadFirewall.log_src}} "
               f"{'DEST':<{PadFirewall.log_dst}} "
               f"{'PROTO':<{PadFirewall.log_proto}} "
               f"{'PORT':>{PadFirewall.log_port}}")

        Decore.title(f"Log (last {limit})", len(hdr))

        with open('/var/log/firewall.log', 'r', encoding='utf-8') as f:
            lines = deque(f, maxlen=limit)

        if not lines:
            raise FileNotFoundError

        print(Decore.invert(hdr))
        for line in lines:
            parsed = parse_firewall_log_line(line)
            if not parsed:
                continue

            time_str = ''
            if parsed['timestamp']:
                try:
                    ts = parsed['timestamp'].strip()
                    if 'T' in ts:   # ISO format
                        dt = datetime.fromisoformat(ts)
                        time_str = dt.strftime("%b %d %H:%M:%S")
                    else:           # syslog format
                        dt = datetime.strptime(ts, "%b %d %H:%M:%S")
                        time_str = dt.strftime("%b %d %H:%M:%S")
                except Exception:
                    time_str = parsed['timestamp'][:PadFirewall.log_time-1]

            if parsed['action'] == 'REJECT':
                action_color = Decore.red
            else:
                action_color = Decore.yellow
            action = action_color(parsed['action'])

            print(f"{time_str:<{PadFirewall.log_time}} "
                  f"{action:>{PadFirewall.log_action + 10}} "
                  f"{parsed['in_iface']:>{PadFirewall.log_iif}} "
                  f"{parsed['src']:<{PadFirewall.log_src}} "
                  f"{parsed['dst']:<{PadFirewall.log_dst}} "
                  f"{parsed['proto']:<{PadFirewall.log_proto}} "
                  f"{parsed['dpt']:>{PadFirewall.log_port}}")

    except FileNotFoundError:
        print("No logs found (may be disabled or no denied traffic)")
    except Exception as e:
        print(f"Error reading firewall logs: {e}")


def show_firewall(json):
    """Main firewall display orchestrator.

    Args:
        json: Complete firewall configuration dict

    Displays:
        - Status with lockdown/logging alerts
        - Zone-to-zone traffic matrix
        - Zone and policy tables
        - Firewall logs (if logging enabled)
    """
    fw = json.get('infix-firewall:firewall', {})
    if not fw:
        print("Firewall disabled.")
        return

    # Build firewall status with contextual alerts
    lockdown_state = fw.get('lockdown', False)
    logging_enabled = fw.get('logging', 'off') != 'off'

    firewall_status = "active"
    if lockdown_state:  # Lockdown mode takes priority
        firewall_status += f" [ {Decore.flashing_red('LOCKDOWN MODE')} ]"
    elif logging_enabled:
        firewall_status += f" [ {Decore.bold_yellow('MONITORING')} ]"

    # Adjust 20 + 8, where 8 is len(bold)
    print(f"{Decore.bold('Firewall'):<28}: {firewall_status}")

    lockdown_display = "active" if lockdown_state else "inactive"
    print(f"{Decore.bold('Lockdown mode'):<28}: {lockdown_display}")

    print(f"{Decore.bold('Default zone'):<28}: {fw.get('default', 'unknown')}")
    print(f"{Decore.bold('Log denied traffic'):<28}: {fw.get('logging', 'off')}")

    show_firewall_matrix(fw)
    show_firewall_zone(json)
    show_firewall_policy(json)

    # Add firewall logs at the bottom if logging is enabled
    if fw.get('logging', 'off') != 'off':
        show_firewall_logs()


def build_policy_map(policies):
    """Creates optimized (ingress,egress) -> policy_info lookup map.

    Args:
        policies: List of policy dicts

    Returns:
        dict: {(from_zone, to_zone): {
            'allow': bool,        # Full access (action=accept, no restrictions)
            'conditional': bool,  # Limited access (services/port-forwards)
            'services': set,      # Allowed service names
            'policies': list      # Contributing policy names
        }}

    Logic:
        - action=accept + no services/port-forwards → allow=True
        - action=accept + services/port-forwards → conditional=True
        - action=reject + services → conditional=True
        - action=reject + no services → allow=False, conditional=False

    Excludes: Global ANY-to-ANY policies (handled separately)
    """
    policy_map = {}

    for policy in policies:
        ingress_zones = policy.get('ingress', [])
        egress_zones = policy.get('egress', [])
        services = policy.get('service', [])
        action = policy.get('action', 'reject')
        policy_name = policy.get('name', 'unknown')

        for ing in ingress_zones:
            for egr in egress_zones:
                # Handle specific zone-to-zone flows (excluding global ANY-to-ANY)
                if not (ing == 'ANY' and egr == 'ANY'):
                    key = (ing, egr)

                    if key not in policy_map:
                        policy_map[key] = {
                            'allow': False,
                            'conditional': False,
                            'services': set(),
                            'policies': []
                        }

                    if action in ['accept', 'continue']:
                        if services:
                            # Accept policy with specific services - conditional only
                            policy_map[key]['conditional'] = True
                            policy_map[key]['services'].update(services)
                            # Don't set allow=True - this is conditional, not full allow
                        else:
                            # Accept policy with no restrictions - allow all
                            policy_map[key]['allow'] = True
                    else:
                        # Reject/drop policies
                        if services:
                            # Reject with service exceptions - conditional access only
                            policy_map[key]['conditional'] = True
                            policy_map[key]['services'].update(services)
                            # Don't set allow=True - this is conditional, not full allow
                        # else: reject with no services - stays deny (allow=False)

                    policy_map[key]['policies'].append(policy_name)
    return policy_map


def format_port_forwards(port_forwards, max_length=35):
    """Format port-forward rules for compact display.

    Args:
        port_forwards: List of port-forward dicts
        max_length: Maximum string length before truncation

    Returns:
        str: Formatted port-forward string like ":80/tcp→192.168.1.10:8080, :443/tcp→..."
    """
    if not port_forwards:
        return ""

    formatted_rules = []
    for pf in port_forwards:
        lower = pf.get('lower')
        upper = pf.get('upper')
        proto = pf.get('proto', 'tcp')
        to_addr = pf.get('to', {}).get('addr')
        to_port = pf.get('to', {}).get('port', lower)

        if lower:
            # Format port range or single port
            if upper and upper != lower:
                port_spec = f":{lower}-{upper}/{proto}"
            else:
                port_spec = f":{lower}/{proto}"

            # Format destination
            if to_addr:
                if to_port:
                    dest = f"{to_addr}:{to_port}"
                else:
                    dest = to_addr
                formatted_rules.append(f"{port_spec} → {dest}")

    result = ", ".join(formatted_rules)
    if len(result) > max_length:
        # Truncate and add indicator
        truncated = result[:max_length-4] + "..."
        return truncated
    return result


def find_zone_by_ip(ip_str, zones):
    """Find which zone contains the given IP address.

    Args:
        ip_str: IP address string to lookup
        zones: List of zone configurations

    Returns:
        str: Zone name if found, None otherwise
    """
    try:
        target_ip = ipaddress.ip_address(ip_str)
        for zone in zones:
            networks = zone.get('network', [])
            for network_str in networks:
                try:
                    network = ipaddress.ip_network(network_str, strict=False)
                    if target_ip in network:
                        return zone['name']
                except ValueError:
                    continue
    except ValueError:
        pass
    return None


def traffic_flow(from_zone, to_zone, policy_map, zones, policies, cell_width):
    """Core zone-to-zone traffic analysis with colored cell output.

    Args:
        from_zone, to_zone: Zone names for traffic direction
        policy_map: Pre-computed policy lookup from build_policy_map()
        zones: Zone configuration list
        policies: Not used (kept for compatibility)
        cell_width: Matrix cell width for formatting

    Returns:
        str: Colored terminal cell with symbol (✓/✗/⚠/—)

    Logic:
        - HOST↔HOST: Gray — (not applicable)
        - HOST→zone: Green ✓ (firewall can reach zones)
        - zone→HOST: Based on zone input config (action/services)
        - zone→zone: Based on explicit policy only
        - Intra-zone: Requires explicit policy
        - No policy: Red ✗ (default deny)

    Symbols: ✓=allow all, ✗=deny, ⚠=conditional, —=n/a
    """
    def make_cell(symbol, bg_func):
        # Create full-width colored cell
        return bg_func(f" {symbol:^{cell_width}} ")

    # Handle HOST zone specially
    if from_zone == "HOST" and to_zone == "HOST":
        # HOST-to-HOST communication (localhost) - not applicable
        return make_cell("—", Decore.gray_bg)

    # HOST-to-zone traffic: firewall input rules control this
    if from_zone == "HOST":
        # Traffic from firewall device to zones - typically allowed
        return make_cell("✓", Decore.green_bg)

    # zone-to-HOST traffic: controlled by zone input configuration
    if to_zone == "HOST":
        # Find the zone configuration for this source zone
        zone_config = None
        for zone in zones:
            if zone.get('name') == from_zone:
                zone_config = zone
                break

        if zone_config:
            action = zone_config.get('action', 'reject')
            services = zone_config.get('service', [])
            port_forwards = zone_config.get('port-forward', [])

            if action == 'accept':
                # Zone allows all traffic to HOST
                return make_cell("✓", Decore.green_bg)
            elif services or port_forwards:
                # Zone has service exceptions or port forwards (conditional access)
                return make_cell("⚠", Decore.yellow_bg)
            else:
                # Zone blocks all traffic to HOST
                return make_cell("✗", Decore.red_bg)

        # Zone not found - default deny
        return make_cell("✗", Decore.red_bg)

    # Intra-zone communication now requires explicit policies
    if from_zone == to_zone:
        # Look for explicit intra-zone policy (e.g., "lan-to-lan")
        key = (from_zone, to_zone)
        policy = policy_map.get(key)
        if policy and policy['allow']:
            return make_cell("✓", Decore.green_bg)
        return make_cell("✗", Decore.red_bg)

    # Check for explicit policy between these zones
    key = (from_zone, to_zone)
    policy = policy_map.get(key)

    if policy:
        if policy['allow']:
            # Policy explicitly allows all traffic
            return make_cell("✓", Decore.green_bg)
        elif policy['conditional']:
            # Policy has service restrictions or port forwards
            return make_cell("⚠", Decore.yellow_bg)
        else:
            # Policy explicitly denies
            return make_cell("✗", Decore.red_bg)

    # Check for port-forwards from source zone to target zone via IP mapping
    source_zone_config = None
    for zone in zones:
        if zone.get('name') == from_zone:
            source_zone_config = zone
            break

    if source_zone_config:
        port_forwards = source_zone_config.get('port-forward', [])
        for pf in port_forwards:
            to_addr = pf.get('to', {}).get('addr')
            if to_addr:
                target_zone = find_zone_by_ip(to_addr, zones)
                if target_zone == to_zone:
                    # Port forward creates conditional access to target zone
                    return make_cell("⚠", Decore.yellow_bg)

    # No explicit policy - default deny
    return make_cell("✗", Decore.red_bg)


def show_firewall_matrix(fw):
    """Renders visual zone-to-zone traffic flow matrix.

    Args:
        fw: Firewall config dict with zones/policies

    Algorithm:
        1. Collect zones with interfaces/networks + implicit HOST
        2. Build policy lookup map via build_policy_map()
        3. Generate matrix cells using traffic_flow() logic
        4. Render with box-drawing chars and colored symbols

    Symbols: ✓=allow, ✗=deny, ⚠=conditional, —=n/a
    """
    zones = fw.get('zone', [])
    policies = fw.get('policy', [])

    # Build zone list - include zones with interfaces OR networks
    zone_names = []
    for z in zones:
        interfaces = z.get('interface', [])
        networks = z.get('network', [])
        # Include if zone has interfaces OR networks (non-empty lists)
        if interfaces or networks:
            zone_names.append(z['name'])

    # Always add the implicit HOST zone
    zone_names.insert(0, "HOST")

    if len(zone_names) <= 1:
        return None

    # Build enhanced policy lookup map
    policy_map = build_policy_map(policies)

    max_zone_len = max(len(zone) for zone in zone_names)
    col_width = max(max_zone_len, 1)  # At least 1 char for symbols
    left_col_width = max_zone_len

    # Box drawing characters for proper borders, '+ 2' is for spacing
    top_border = "┌" + "─" * left_col_width + "──" + "┬"
    for _ in zone_names:
        top_border += "─" * (col_width + 2) + "┬"
    top_border = top_border[:-1] + "┐"  # Replace last ┬ with ┐

    middle_border = "├" + "─" * left_col_width + "──" + "┼"
    for _ in zone_names:
        middle_border += "─" * (col_width + 2) + "┼"
    middle_border = middle_border[:-1] + "┤"  # Replace last ┼ with ┤

    bottom_border = "└" + "─" * left_col_width + "──" + "┴"
    for _ in zone_names:
        bottom_border += "─" * (col_width + 2) + "┴"
    bottom_border = bottom_border[:-1] + "┘"  # Replace last ┴ with ┘

    # Header with arrow in top-left cell
    hdr = f"│ {'→':^{left_col_width}} │"
    for zone in zone_names:
        hdr += f" {zone:^{col_width}} │"

    # Calculate centering relative to zones/policies table width
    matrix_width = len(top_border)
    target_width = PadFirewall.table_width()
    padding = max(0, (target_width - matrix_width) // 2)
    indent = " " * padding

    # Center the title underline to match table width
    title_padding = max(0, (target_width - len("Zone Matrix")) // 2)
    title_underline = "─" * target_width

    print(title_underline)
    print(f"{'':<{title_padding}}{Decore.bold('Zone Matrix')}")
    print(f"{indent}{top_border}")
    print(f"{indent}{hdr}")
    print(f"{indent}{middle_border}")

    for from_zone in zone_names:
        row = f"│ {from_zone:>{left_col_width}} │"
        for to_zone in zone_names:
            # Find symbol for this cell based on traffic flow
            symbol = traffic_flow(from_zone, to_zone, policy_map,
                                  zones, policies, col_width)
            row += f"{symbol}│"
        print(f"{indent}{row}")
    print(f"{indent}{bottom_border}")

    # Center the legend - define parts first for length calculation
    legend_data = [
        ("✓ Allow", Decore.green_bg),
        ("✗ Deny", Decore.red_bg),
        ("⚠ Conditional", Decore.yellow_bg)
    ]

    # Calculate visible length, then colorize the parts
    visible_parts = [f" {text} " for text, _ in legend_data]
    visible_legend = "   ".join(visible_parts)
    colorized_parts = [bg_func(f" {text} ") for text, bg_func in legend_data]
    legend = "   ".join(colorized_parts)
    # Depending on taste and number of zones, but +1 works for me
    legend_padding = max(0, (target_width - len(visible_legend)) // 2) + 1
    print(f"{' ' * legend_padding}{legend}")


def show_firewall_zone(json, zone_name=None):
    """Displays zone configuration table or detailed zone analysis.

    Args:
        json: Complete firewall config
        zone_name: Optional specific zone name for detail view

    Table mode (zone_name=None):
        - Zone name, type (ifaces/networks), members, HOST services
        - Uses compress_interface_list() for clean display
        - Shows immutable lock indicators

    Detail mode (zone_name specified):
        - Complete zone configuration
        - Traffic flow analysis to all other zones
        - Policy-based access determination
    """
    fw = json.get('infix-firewall:firewall', {})
    zones = fw.get('zone', [])
    policies = fw.get('policy', [])

    if zone_name:
        zone = next((z for z in zones if z.get('name') == zone_name), None)
        if not zone:
            print(f"Zone '{zone_name}' not found")
            return

        description = zone.get('description', '')
        interfaces = zone.get('interface', [])
        if not interfaces:
            interfaces = ""
        networks = zone.get('network', [])
        if not networks:
            networks = ""
        services = zone.get('service', [])
        action = zone.get('action', 'reject')

        if action == 'accept':
            services_display = "ANY"
        elif services:
            services_display = ", ".join(services)
        else:
            services_display = "(none)"

        print(format_description('description', description))
        print(f"{'name':<20}: {zone_name}")
        print(f"{'action':<20}: {action}")
        print(f"{'interface':<20}: {compress_interface_list(interfaces)}")
        print(f"{'networks':<20}: {', '.join(networks)}")
        print(f"{'services (to HOST)':<20}: {services_display}")

        # Show port forwards if any
        port_forwards = zone.get('port-forward', [])
        if port_forwards:
            print(f"{'port forwards':<20}: {len(port_forwards)} rule(s)")
            for fwd in port_forwards:
                lower = fwd.get('lower')
                upper = fwd.get('upper')
                proto = fwd.get('proto', '')

                if upper:
                    from_port = f"{lower}-{upper}/{proto}"
                else:
                    from_port = f"{lower}/{proto}"

                to_info = fwd.get('to', {})
                to_addr = to_info.get('addr', '')
                to_port = to_info.get('port', lower)  # Default to source port

                if upper and to_port:
                    # Calculate upper destination port for ranges
                    port_diff = upper - lower
                    to_upper = to_port + port_diff
                    to_port_str = f"{to_port}-{to_upper}"
                else:
                    to_port_str = str(to_port)

                to_display = f"{to_addr}:{to_port_str}"
                print(f"{'  - ' + from_port:<18} → {to_display}")

        hdr = (f"{'TO ZONE':<{PadFirewall.zone_flow_to}}"
               f"{'ACTION':<{PadFirewall.zone_flow_action}}"
               f"{'POLICY':<{PadFirewall.zone_flow_policy}}"
               f"{'SERVICES':<{PadFirewall.zone_flow_services}}")
        Decore.title(f"Traffic Flows: {zone_name} →", len(hdr))
        print(Decore.invert(hdr))

        # Add HOST zone first
        current_zone = next((z for z in zones if z.get('name') == zone_name), None)
        if current_zone:
            # Zone-to-HOST traffic logic
            action = current_zone.get('action', 'reject')
            services = current_zone.get('service', [])

            if action == 'accept':
                host_action = "✓ allow"
                host_services = "(any)"
            elif services:
                host_action = "⚠ conditional"
                host_services = ", ".join(services)
            else:
                host_action = "✗ deny"
                host_services = "(none)"

            print(f"{'HOST':<{PadFirewall.zone_flow_to}}"
                  f"{host_action:<{PadFirewall.zone_flow_action}}"
                  f"{'(services)':<{PadFirewall.zone_flow_policy}}"
                  f"{host_services}")

        # Add other zones
        for other_zone in zones:
            if other_zone.get('name') == zone_name:
                continue

            # Check if there's a policy allowing this flow
            other_name = other_zone.get('name')
            policy_name = "(none)"
            action = "✗ deny"
            services_display = "(none)"

            for policy in policies:
                if zone_name in policy.get('ingress', []) and other_name \
                   in policy.get('egress', []):
                    policy_name = policy.get('name', 'unknown')
                    policy_action = policy.get('action', 'reject')
                    policy_services = policy.get('service', [])

                    if policy_action == 'accept':
                        action = "✓ allow"
                        services_display = "(any)"
                    elif policy_services:
                        action = "⚠ conditional"
                        services_display = ", ".join(policy_services)
                    else:
                        action = "✗ deny"
                        services_display = "(none)"
                    break

            print(f"{other_name:<{PadFirewall.zone_flow_to}}"
                  f"{action:<{PadFirewall.zone_flow_action}}"
                  f"{policy_name:<{PadFirewall.zone_flow_policy}}"
                  f"{services_display}")
    else:
        hdr = (f"{'':<{PadFirewall.zone_locked}}"
               f"{'NAME':<{PadFirewall.zone_name}}"
               f"{'TYPE':<{PadFirewall.zone_type}}"
               f"{'DATA':<{PadFirewall.zone_data}}"
               f"{'ALLOWED HOST SERVICES':<{PadFirewall.zone_services}}")
        Decore.title("Zones", len(hdr))
        print(Decore.invert(hdr))

        for zone in zones:
            name = zone.get('name', '')
            action = zone.get('action', 'reject')
            interface_list = zone.get('interface', [])
            network_list = zone.get('network', [])
            port_forwards = zone.get('port-forward', [])
            services = zone.get('service', [])

            if action == 'accept':
                services_display = "ANY"
            elif services:
                services_display = ", ".join(services)
            else:
                services_display = "(none)"

            immutable = zone.get('immutable', False)
            locked = "⚷" if immutable else " "

            # Build configuration strings
            config_lines = []

            # Interfaces
            if interface_list:
                interfaces = compress_interface_list(interface_list)
                config_lines.append(("iif", interfaces))
            else:
                config_lines.append(("iif", "(none)"))

            # Networks
            if network_list:
                networks = ", ".join(network_list)
                config_lines.append(("net", networks))

            # Port forwards
            if port_forwards:
                pf_display = format_port_forwards(port_forwards)
                config_lines.append(("fwd", pf_display))

            # Print first line with zone name and services
            if config_lines:
                first_type, first_data = config_lines[0]
                print(f"{locked:<{PadFirewall.zone_locked}}"
                      f"{name:<{PadFirewall.zone_name}}"
                      f"{first_type:<{PadFirewall.zone_type}}"
                      f"{first_data:<{PadFirewall.zone_data}}"
                      f"{services_display}")

                # Print additional configuration lines
                for config_type, config_data in config_lines[1:]:
                    print(f"{'':<{PadFirewall.zone_locked}}"
                          f"{'':<{PadFirewall.zone_name}}"
                          f"{config_type:<{PadFirewall.zone_type}}"
                          f"{config_data}")

            # if zone != zones[-1]:  # Don't add line after last zone
            #     print()


def show_firewall_policy(json, policy_name=None):
    """Displays policy configuration table or detailed policy analysis.

    Args:
        json: Complete firewall config
        policy_name: Optional specific policy name for detail view

    Table mode (policy_name=None):
        - Policy name, action, ingress/egress zones
        - Sorted by priority (lower = higher precedence)
        - Shows immutable lock indicators

    Detail mode (policy_name specified):
        - Complete policy configuration
        - Custom ICMP filters with type/family details
        - Port forwarding rules with range mapping
        - Service restrictions and masquerade settings
    """
    fw = json.get('infix-firewall:firewall', {})
    policies = fw.get('policy', [])

    if policy_name:
        policy = next((p for p in policies if p.get('name') == policy_name), None)
        if not policy:
            print(f"Policy '{policy_name}' not found")
            return

        ingress = policy.get('ingress', [])
        egress = policy.get('egress', [])
        action = policy.get('action', 'reject')
        masquerade = "yes" if policy.get('masquerade') else "no"
        description = policy.get('description', '')
        services = policy.get('service', [])
        custom = policy.get('custom', {})
        custom_filters = custom.get('filter', [])

        if policy == 'accept':
            services_display = "(all)"
        else:
            services_display = ", ".join(services) if services else "(none)"

        print(format_description('description', description))
        print(f"{'name':<20}: {policy_name}")
        print(f"{'ingress':<20}: {', '.join(ingress) if ingress else '(none)'}")
        print(f"{'egress':<20}: {', '.join(egress) if egress else '(none)'}")
        print(f"{'action':<20}: {action}")
        print(f"{'masquerade':<20}: {masquerade}")
        print(f"{'services':<20}: {services_display}")

        if custom_filters:
            print(f"{'custom filters':<20}: {len(custom_filters)} filter(s)")

            sorted_filters = sorted(custom_filters, key=lambda f: f.get('priority', 32767))
            for _, filter_entry in enumerate(sorted_filters):
                action = filter_entry.get('action', 'accept')
                family = filter_entry.get('family', 'both')

                icmp = filter_entry.get('icmp')
                if icmp:
                    icmp_type = icmp.get('type', 'unknown')
                    print(f"{'  - ' + action:<6} {family} icmp-type {icmp_type}")
                else:
                    print(f"{'  - ' + action:<6} {family} (unknown type)")
    else:
        hdr = (f"{'':<{PadFirewall.policy_locked}}"
               f"{'NAME':<{PadFirewall.policy_name}}"
               f"{'ACTION':<{PadFirewall.policy_action}}"
               f"{'INGRESS':<{PadFirewall.policy_ingress}}"
               f"{'EGRESS':<{PadFirewall.policy_egress}}")
        Decore.title("Policies", len(hdr))
        print(Decore.invert(hdr))

        sorted_policies = sorted(policies, key=lambda p: p.get('priority', 32767))
        for policy in sorted_policies:
            name = policy.get('name', '')
            ingress = ", ".join(policy.get('ingress', []))
            egress = ", ".join(policy.get('egress', []))
            action = policy.get('action', 'reject')

            # Check for custom filters
            # custom = policy.get('custom', {})
            # custom_filters = custom.get('filter', [])
            # if custom_filters:
            #     name += f" ({len(custom_filters)} filter(s))"

            immutable = policy.get('immutable', False)
            locked = "⚷" if immutable else " "

            print(f"{locked:<{PadFirewall.policy_locked}}"
                  f"{name:<{PadFirewall.policy_name}}"
                  f"{action:<{PadFirewall.policy_action}}"
                  f"{ingress:<{PadFirewall.policy_ingress}}"
                  f"{egress:<{PadFirewall.policy_egress}}")


def format_port_list(ports):
    """Format port list from YANG data"""
    if not ports:
        return "(none)"

    formatted = []
    for port in ports:
        proto = port.get('proto', 'tcp')
        lower = port.get('lower')
        upper = port.get('upper')

        if upper and upper != lower:
            formatted.append(f"{lower}-{upper}/{proto}")
        else:
            formatted.append(f"{lower}/{proto}")

    return ", ".join(formatted)


def show_firewall_service(json, name=None):
    """Show firewall services table or specific service details"""
    fw = json.get('infix-firewall:firewall', {})
    services = fw.get('service', [])

    if name:
        service = next((s for s in services if s.get('name') == name), None)
        if not service:
            print(f"Service '{name}' not found")
            return

        ports = format_port_list(service.get('port', []))
        description = service.get('description', '')

        print(f"{'name':<20}: {name}")
        print(f"{'ports':<20}: {ports}")
        print(format_description('description', description))
    else:
        hdr = (f"{'NAME':<{PadFirewall.service_name}}"
               f"{'PORTS':<{PadFirewall.service_ports}}")
        print(Decore.invert(hdr))
        for service in services:
            name = service.get('name', '')
            ports = format_port_list(service.get('port', []))

            print(f"{name:<{PadFirewall.service_name}}"
                  f"{ports:<{PadFirewall.service_ports}}")


def main():
    global UNIT_TEST

    try:
        json_data = json.load(sys.stdin)
    except json.JSONDecodeError:
        print("Error, invalid JSON input")
        sys.exit(1)
    except Exception as e:
        print("Error, unexpected error parsing JSON")
        sys.exit(1)

    parser = argparse.ArgumentParser(description="JSON CLI Pretty Printer")
    subparsers = parser.add_subparsers(dest='command', help='Commands')

    parser.add_argument('-t', '--test', action='store_true', help='Enable unit test mode')

    subparsers.add_parser('show-boot-order', help='Show NTP sources')

    subparsers.add_parser('show-bridge-mdb', help='Show bridge MDB')

    subparsers.add_parser('show-bridge-stp', help='Show spanning tree state')

    subparsers.add_parser('show-dhcp-server', help='Show DHCP server') \
              .add_argument("-s", "--stats", action="store_true", help="Show server statistics")

    subparsers.add_parser('show-hardware', help='Show USB ports')

    subparsers.add_parser('show-interfaces', help='Show interfaces') \
              .add_argument('-n', '--name', help='Interface name')

    subparsers.add_parser('show-lldp', help='Show LLDP neighbors')
    subparsers.add_parser('show-firewall', help='Show firewall overview')
    subparsers.add_parser('show-firewall-zone', help='Show firewall zones') \
              .add_argument('name', nargs='?', help='Zone name')
    subparsers.add_parser('show-firewall-policy', help='Show firewall policies') \
              .add_argument('name', nargs='?', help='Policy name')
    subparsers.add_parser('show-firewall-service', help='Show firewall services') \
              .add_argument('name', nargs='?', help='Service name')

    subparsers.add_parser('show-ntp', help='Show NTP sources')

    subparsers.add_parser('show-routing-table', help='Show the routing table') \
              .add_argument('-i', '--ip', required=True, help='IPv4 or IPv6 address')

    subparsers.add_parser('show-services', help='Show system services')

    subparsers.add_parser('show-software', help='Show software versions') \
              .add_argument('-n', '--name', help='Slotname')

    subparsers.add_parser('show-system', help='Show system overview')

    args = parser.parse_args()
    UNIT_TEST = args.test

    if args.command == "show-bridge-mdb":
        show_bridge_mdb(json_data)
    elif args.command == "show-bridge-stp":
        show_bridge_stp(json_data)
    elif args.command == "show-dhcp-server":
        show_dhcp_server(json_data, args.stats)
    elif args.command == "show-hardware":
        show_hardware(json_data)
    elif args.command == "show-interfaces":
        show_interfaces(json_data, args.name)
    elif args.command == "show-lldp":
        show_lldp(json_data)
    elif args.command == "show-firewall":
        show_firewall(json_data)
    elif args.command == "show-firewall-zone":
        show_firewall_zone(json_data, args.name)
    elif args.command == "show-firewall-policy":
        show_firewall_policy(json_data, args.name)
    elif args.command == "show-firewall-service":
        show_firewall_service(json_data, args.name)
    elif args.command == "show-ntp":
        show_ntp(json_data)
    elif args.command == "show-routing-table":
        show_routing_table(json_data, args.ip)
    elif args.command == "show-software":
        show_software(json_data, args.name)
    elif args.command == "show-services":
        show_services(json_data)
    elif args.command == "show-system":
        show_system(json_data)

    else:
        print(f"Error, unknown command '{args.command}'")
        sys.exit(1)


if __name__ == "__main__":
    main()
