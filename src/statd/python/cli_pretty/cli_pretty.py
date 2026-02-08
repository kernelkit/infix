#!/usr/bin/env python3
import base64
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
    flags = 2
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
    name = 11
    date = 25
    hash = 64
    state = 10
    version = 23


class PadDhcpServer:
    ip = 17
    mac = 19
    host = 21
    cid = 22
    exp = 10


class PadSensor:
    name = 30
    value = 22
    status = 10

    @classmethod
    def table_width(cls):
        """Total width of sensor table (matches show system width)"""
        return cls.name + cls.value + cls.status


class PadLldp:
    interface = 16
    rem_idx = 10
    time = 12
    chassis_id = 20
    port_id = 20


def format_memory_bytes(bytes_val):
    """Convert bytes to human-readable format"""
    if bytes_val == 0:
        return " "
    elif bytes_val < 1024:
        return f"{bytes_val}B"
    elif bytes_val < 1024 * 1024:
        return f"{bytes_val // 1024}K"
    elif bytes_val < 1024 * 1024 * 1024:
        return f"{bytes_val // (1024 * 1024):.1f}M"
    else:
        return f"{bytes_val // (1024 * 1024 * 1024):.1f}G"


def format_uptime_seconds(seconds):
    """Convert seconds to compact time format"""
    if seconds == 0:
        return " "
    elif seconds < 60:
        return f"{seconds}s"
    elif seconds < 3600:
        return f"{seconds // 60}m"
    elif seconds < 86400:
        return f"{seconds // 3600}h"
    else:
        return f"{seconds // 86400}d"


class Column:
    """Column definition for SimpleTable"""
    def __init__(self, name, align='left', formatter=None, flexible=False, min_width=None):
        self.name = name
        self.align = align
        self.formatter = formatter
        self.flexible = flexible
        self.min_width = min_width

class SimpleTable:
    """Simple table formatter that handles ANSI colors correctly and calculates dynamic column widths"""

    def __init__(self, columns, min_width=None):
        self.columns = columns
        self.rows = []
        self.min_width = min_width
        self._column_widths = None  # Cache calculated widths

    @staticmethod
    def visible_width(text):
        """Return visible character count, excluding ANSI escape sequences"""
        ansi_pattern = r'\x1b\[[0-9;]*m'
        clean_text = re.sub(ansi_pattern, '', str(text))
        return len(clean_text)

    def row(self, *values):
        """Store row data for later formatting"""
        if len(values) != len(self.columns):
            raise ValueError(f"Expected {len(self.columns)} values, got {len(values)}")
        self.rows.append(values)

    def width(self):
        """Calculate and return total table width"""
        if self._column_widths is None:
            self._column_widths = self._calculate_column_widths()

        # Sum column widths + 2-char separator between columns
        total = sum(self._column_widths)
        if self._column_widths:
            # Separators only between columns, not after the last one
            total += (len(self._column_widths) - 1) * 2

        return total

    def adjust_padding(self, width):
        """Distribute padding to width evenly across flexible columns"""
        if self._column_widths is None:
            self._column_widths = self._calculate_column_widths()

        current_width = self.width()
        extra_width = width - current_width

        if extra_width <= 0:
            return  # Already at or above target

        # Find flexible columns
        flex_indices = [i for i, col in enumerate(self.columns) if col.flexible]

        if not flex_indices:
            return  # No flexible columns to expand

        # Distribute evenly
        per_column = extra_width // len(flex_indices)
        remainder = extra_width % len(flex_indices)

        for i, idx in enumerate(flex_indices):
            self._column_widths[idx] += per_column
            # Give remainder to first columns
            if i < remainder:
                self._column_widths[idx] += 1

    def print(self, styled=True):
        """Calculate widths and print complete table"""
        if self._column_widths is None:
            self._column_widths = self._calculate_column_widths()

        # Apply minimum width if specified
        if self.min_width:
            self.adjust_padding(self.min_width)

        print(self._format_header(self._column_widths, styled))
        for row_data in self.rows:
            print(self._format_row(row_data, self._column_widths))

    def _calculate_column_widths(self):
        """Calculate maximum width needed for each column"""
        widths = [self.visible_width(col.name) for col in self.columns]

        for row_data in self.rows:
            for i, (value, column) in enumerate(zip(row_data, self.columns)):
                formatted_value = column.formatter(value) if column.formatter else value
                value_width = self.visible_width(str(formatted_value))
                widths[i] = max(widths[i], value_width)

        # Apply column minimum widths
        for i, column in enumerate(self.columns):
            if column.min_width:
                widths[i] = max(widths[i], column.min_width)

        return widths

    def _format_header(self, column_widths, styled=True):
        """Generate formatted header row"""
        header_parts = []
        for i, column in enumerate(self.columns):
            width = column_widths[i]
            # Add separator "  " only between columns, not after the last one
            is_last = (i == len(self.columns) - 1)
            separator = "" if is_last else "  "

            if column.align == 'right':
                header_parts.append(f"{column.name:>{width}}{separator}")
            else:
                header_parts.append(f"{column.name:{width}}{separator}")

        header_str = ''.join(header_parts)
        return Decore.invert(header_str) if styled else header_str

    def _format_row(self, row_data, column_widths):
        """Format a single data row"""
        row_parts = []
        for i, (value, column) in enumerate(zip(row_data, self.columns)):
            is_last = (i == len(self.columns) - 1)
            formatted_value = self._format_column_value(value, column, column_widths[i], is_last)
            row_parts.append(formatted_value)

        return ''.join(row_parts)

    def _format_column_value(self, value, column, width, is_last=False):
        """Format a single column value with proper alignment"""
        if column.formatter:
            value = column.formatter(value)

        value_str = str(value)
        visible_len = self.visible_width(value_str)

        # Add separator "  " only between columns, not after the last one
        separator = "" if is_last else "  "

        # Don't add trailing spaces to the last column
        if is_last:
            if column.align == 'right':
                padding = width - visible_len
                return ' ' * max(0, padding) + value_str
            return value_str
        elif column.align == 'right':
            padding = width - visible_len
            return ' ' * max(0, padding) + value_str + separator
        else:
            padding = width - visible_len
            return value_str + ' ' * max(0, padding) + separator


class Canvas:
    """Multi-item rendering canvas for unified width alignment across tables and content.

    Canvas coordinates the display of multiple SimpleTable instances and other content
    types (text, titles, spacing, pre-rendered content) with consistent width alignment.
    This creates professional-looking output where all table headers and content align
    to the same width, with flexible columns distributing extra space evenly.

    Purpose:
        - Achieve visual alignment across heterogeneous content
        - Minimize performance overhead with single-pass data traversal
        - Support mixed content: tables, text, titles, matrices, spacing

    How it works:
        1. Buffer items in sequence (add_text, add_table, add_title, etc.)
        2. Calculate maximum width from all SimpleTable instances
        3. Apply flex padding to narrower tables via SimpleTable.adjust_padding()
        4. Render all items sequentially with unified width

    Interaction with SimpleTable:
        - Calls SimpleTable.width() to determine natural table width
        - Calls SimpleTable.adjust_padding(max_width) to expand flexible columns
        - SimpleTable columns marked with flexible=True distribute extra width evenly
        - Preserves table alignment (left/right) and formatting

    Performance:
        - Single data traversal: tables built once, widths calculated once
        - Memory over CPU: buffers all items before rendering
        - Optimized for embedded systems (ARM Cortex-A7)

    Example:
        See show_firewall() for a complete usage example demonstrating:
        - Mixed content types (status text, matrix, tables)
        - Multiple tables with different flexible columns
        - Centered matrix using get_max_width()
        - Proper spacing and titles

    Args:
        min_width: Optional minimum width for all tables (default: None)
    """

    def __init__(self, min_width=None):
        self.min_width = min_width
        self.items = []  # List of (type, content) tuples

    def add_text(self, text):
        """Add a plain text line"""
        self.items.append(('text', text))

    def add_title(self, text):
        """Add a section title (will be centered to canvas width)"""
        self.items.append(('title', text))

    def add_raw(self, text):
        """Add pre-rendered multi-line text (like zone matrix)"""
        self.items.append(('raw', text))

    def add_spacing(self, lines=1):
        """Add blank lines for spacing"""
        self.items.append(('spacing', lines))

    def add_table(self, table):
        """Add a SimpleTable instance"""
        if not isinstance(table, SimpleTable):
            raise ValueError("Expected SimpleTable instance")
        self.items.append(('table', table))

    def insert(self, index, item_type, content):
        """Insert an item at a specific position"""
        self.items.insert(index, (item_type, content))

    def insert_text(self, index, text):
        """Insert a plain text line at position"""
        self.insert(index, 'text', text)

    def insert_title(self, index, text):
        """Insert a section title at position"""
        self.insert(index, 'title', text)

    def insert_raw(self, index, text):
        """Insert pre-rendered text at position"""
        self.insert(index, 'raw', text)

    def insert_spacing(self, index, lines=1):
        """Insert blank lines at position"""
        self.insert(index, 'spacing', lines)

    def insert_table(self, index, table):
        """Insert a SimpleTable at position"""
        if not isinstance(table, SimpleTable):
            raise ValueError("Expected SimpleTable instance")
        self.insert(index, 'table', table)

    def get_max_width(self):
        """Calculate and return the maximum width from all tables"""
        max_width = self.min_width or 0

        for item_type, content in self.items:
            if item_type == 'table':
                max_width = max(max_width, content.width())

        return max_width

    def render(self):
        """Calculate widths, apply padding, and output all items"""
        # First pass: find maximum width from all tables
        max_width = self.get_max_width()

        # Second pass: apply padding to all tables with flexible columns
        for item_type, content in self.items:
            if item_type == 'table':
                content.adjust_padding(max_width)

        # Third pass: render each item
        for i, (item_type, content) in enumerate(self.items):
            if item_type == 'text':
                print(content)
            elif item_type == 'title':
                # Title with underline matching canvas width
                underline = "─" * max_width
                print(underline)
                print(Decore.bold(content))
            elif item_type == 'raw':
                # Pre-rendered content (like matrix)
                print(content)
            elif item_type == 'spacing':
                for _ in range(content):
                    print()
            elif item_type == 'table':
                content.print()


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
        return Decore.decorate("1;32", txt, "0")

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
    def title(txt, width=None, bold=True):
        """Print section header with horizontal bar line above it
        Args:
            txt: The header text to display
            width: Length of horizontal bar line (defaults to len(txt))
            bold: Whether to make the text bold
        """
        length = width if width is not None else len(txt)
        underline = "─" * length
        print(underline)
        if bold:
            print(Decore.bold(txt))
        else:
            print(txt)


def signal_to_status(signal):
    if signal >= -50:
        status = Decore.bright_green("excellent")
    elif signal >= -60:
        status = Decore.green("good")
    elif signal >= -70:
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

        # Handle PWM fan sensors (reported as "other" type with milli scale)
        # PWM duty cycle is reported as percentage (0-100)
        elif self.value_type == 'other' and self.value_scale == 'milli':
            # Check if this is likely a PWM sensor based on description or name
            name_lower = self.name.lower()
            desc_lower = (self.description or "").lower()
            if 'pwm' in desc_lower or 'fan' in name_lower or 'fan' in desc_lower:
                percent = self.value / 1000.0
                return f"{percent:.1f}%"
            # Fall through for other "other" type sensors

        # For unknown sensor types, show raw value
        if self.value_type in ['other', 'unknown']:
            return f"{self.value}"
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
                exp = "never"
            else:
                dt = datetime.strptime(lease['expires'], '%Y-%m-%dT%H:%M:%S%z')
                seconds = int((dt - now).total_seconds())
                exp = self.format_duration(seconds)
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
            row += f"{exp:>{PadDhcpServer.exp}}"
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
    # Class variable to hold routing-enabled interfaces for the current display session
    _routing_ifaces = set()
    _keystore = {}

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
        self.wireguard = self.data.get('infix-interfaces:wireguard')

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

    def is_wireguard(self):
        return self.data['type'] == "infix-if-type:wireguard"

    def oper(self, detail=False):
        """Remap in brief overview to fit column widths."""
        if not detail and self.oper_status == "lower-layer-down":
            return "lower-down"
        return self.oper_status

    def pr_name(self, pipe=""):
        flag = "⇅" if self.name in Iface._routing_ifaces else " "
        print(f"{flag:<{Pad.flags}}", end="")
        print(f"{pipe}{self.name:<{Pad.iface - len(pipe)}}", end="")

    def pr_proto_ipv4(self, pipe=''):
        for addr in self.ipv4_addr:
            origin = f"({addr['origin']})" if addr.get('origin') else ""

            row =  f"{'':<{Pad.flags}}"
            row += f"{pipe:<{Pad.iface}}"
            row += f"{'ipv4':<{Pad.proto}}"
            row += f"{'':<{Pad.state}}{addr['ip']}/{addr['prefix-length']} {origin}"
            print(row)

    def pr_proto_ipv6(self, pipe=''):
        for addr in self.ipv6_addr:
            origin = f"({addr['origin']})" if addr.get('origin') else ""

            row =  f"{'':<{Pad.flags}}"
            row += f"{pipe:<{Pad.iface}}"
            row += f"{'ipv6':<{Pad.proto}}"
            row += f"{'':<{Pad.state}}{addr['ip']}/{addr['prefix-length']} {origin}"
            print(row)

    def _pr_proto_common(self, name, phys_address, pipe=''):
        row = ""
        if len(pipe) > 0:
            row =  f"{'':<{Pad.flags}}"
            row += f"{pipe:<{Pad.iface}}"

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

    def pr_proto_wireguard(self, pipe=''):
        row = self._pr_proto_common("wireguard", False, pipe)

        if self.wireguard:
            peer_status = self.wireguard.get('peer-status', {})
            peers = peer_status.get('peer', [])
            total_peers = len(peers)
            up_peers = sum(1 for p in peers if p.get('connection-status') == 'up')

            if total_peers > 0:
                row += f"{total_peers} peer"
                if total_peers != 1:
                    row += "s"
                row += f" ({up_peers} up)"

        print(row)

    def pr_proto_loopack(self, pipe=''):
        row = self._pr_proto_common("loopback", False, pipe);
        print(row)

    def pr_wifi_ssids(self):
        width = 70
        Decore.title("Available Networks", width)
        ssid_table = SimpleTable([
            Column('SSID', flexible=True),
            Column('BSSID'),
            Column('SECURITY'),
            Column('SIGNAL'),
            Column('CHANNEL', 'right')
        ], min_width=width)

        station = self.wifi.get("station", {})
        results = station.get("scan-results", {})
        for result in results:
            ssid = result.get('ssid', 'Hidden')
            bssid = result.get("bssid", "unknown")
            encstr = ", ".join(result.get("encryption", ["Unknown"]))
            status = signal_to_status(result.get("signal-strength", -100))
            channel = result.get("channel", "?")

            ssid_table.row(ssid, bssid, encstr, status, channel)
        ssid_table.print()

    def pr_wifi_stations(self):
        """Display connected stations for AP mode"""
        if not self.wifi:
            return

        # Get stations from access-point container
        ap = self.wifi.get("access-point", {})
        stations_data = ap.get("stations", {})
        stations = stations_data.get("station", [])

        if not stations:
            return

        print("\nCONNECTED STATIONS:")
        stations_table = SimpleTable([
            Column('MAC'),
            Column('SIGNAL'),
            Column('TIME'),
            Column('RX PKT'),
            Column('TX PKT'),
            Column('RX BYTES'),
            Column('TX BYTES'),
            Column('RX SPEED'),
            Column('TX SPEED')
        ])

        for station in stations:
            mac = station.get("mac-address", "unknown")
            signal = station.get("signal-strength")
            signal_str = signal_to_status(signal) if signal is not None else "------"

            conn_time = station.get("connected-time", 0)
            time_str = f"{conn_time}s"

            rx_pkt = station.get("rx-packets", 0)
            tx_pkt = station.get("tx-packets", 0)
            rx_bytes = station.get("rx-bytes", 0)
            tx_bytes = station.get("tx-bytes", 0)

            # Speed in 100 kbps units, convert to Mbps for display
            rx_speed = station.get("rx-speed", 0)
            tx_speed = station.get("tx-speed", 0)
            rx_speed_str = f"{rx_speed / 10:.1f}" if rx_speed else "-"
            tx_speed_str = f"{tx_speed / 10:.1f}" if tx_speed else "-"

            stations_table.row(mac, signal_str, time_str, rx_pkt, tx_pkt,
                             rx_bytes, tx_bytes, rx_speed_str, tx_speed_str)

        stations_table.print()


    def pr_proto_wifi(self, pipe=''):
        row = self._pr_proto_common("ethernet", True, pipe);
        print(row)
        ssid = None
        signal = None
        mode = None

        if self.wifi:
            # Detect mode: AP has "stations", Station has "signal-strength" or "scan-results"
            ap=self.wifi.get("access-point", {})
            if ap:
                ssid = ap.get("ssid", "------")
                mode = "AP"
                stations_data = ap.get("stations", {})
                stations = stations_data.get("station", [])
                station_count = len(stations)
                data_str = f"{mode}, ssid: {ssid}, stations: {station_count}"
            else:
                station=self.wifi.get("station", {})
                ssid = station.get("ssid", "------")
                signal = station.get("signal-strength")
                mode = "Station"
                if signal is not None:
                    signal_str = signal_to_status(signal)
                    data_str = f"{mode}, ssid: {ssid}, signal: {signal_str}"
                else:
                    data_str = f"{mode}, ssid: {ssid}"
        else:
            data_str = "ssid: ------"

        row =  f"{'':<{Pad.flags}}"
        row += f"{pipe:<{Pad.iface}}"
        row =  f"{'':<{Pad.flags}}"
        row += f"{pipe:<{Pad.iface}}"
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

    def pr_wireguard(self):
        self.pr_name(pipe="")
        self.pr_proto_wireguard()
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
        # Add ⇅ flag for interfaces with IP forwarding enabled
        flag = "⇅" if self.name in Iface._routing_ifaces else " "
        # Flag column is outside the gray background
        print(f"{flag:<{Pad.flags}}", end="")

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

        forwarding = "enabled" if self.name in Iface._routing_ifaces else "disabled"
        print(f"{'ip forwarding':<{20}}: {forwarding}")

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

        if self.in_octets and self.out_octets:
            print(f"{'in-octets':<{20}}: {self.in_octets}")
            print(f"{'out-octets':<{20}}: {self.out_octets}")

        if self.wifi:
            # Detect mode: AP has "stations", Station has "signal-strength" or "scan-results"
            ap = self.wifi.get('access-point')
            if ap:
                mode = "access-point"
                ssid = ap.get('ssid', "----")
                stations_data = ap.get("stations", {})
                stations = stations_data.get("station", [])
                print(f"{'mode':<{20}}: {mode}")
                print(f"{'ssid':<{20}}: {ssid}")
                print(f"{'connected stations':<{20}}: {len(stations)}")
                self.pr_wifi_stations()
            else:
                mode = "station"
                station = self.wifi.get('station', {})
                signal = station.get('signal-strength')
                ssid = station.get('ssid', "----")
                print(f"{'mode':<{20}}: {mode}")
                print(f"{'ssid':<{20}}: {ssid}")
                if signal is not None:
                    signal_status = signal_to_status(signal)
                    print(f"{'signal':<{20}}: {signal} dBm ({signal_status})")
                rx_speed = station.get('rx-speed')
                tx_speed = station.get('tx-speed')
                if rx_speed is not None:
                    print(f"{'rx bitrate':<{20}}: {rx_speed / 10:.1f} Mbps")
                if tx_speed is not None:
                    print(f"{'tx bitrate':<{20}}: {tx_speed / 10:.1f} Mbps")
                if "scan-results" in station:
                    self.pr_wifi_ssids()

        if self.gre:
            print(f"{'local address':<{20}}: {self.gre['local']}")
            print(f"{'remote address':<{20}}: {self.gre['remote']}")

        if self.vxlan:
            print(f"{'local address':<{20}}: {self.vxlan['local']}")
            print(f"{'remote address':<{20}}: {self.vxlan['remote']}")
            print(f"{'VxLAN id':<{20}}: {self.vxlan['vni']}")

        if self.wireguard:
            peer_status = self.wireguard.get('peer-status', {})
            peers = peer_status.get('peer', [])
            if peers:
                print(f"{'peers':<{20}}: {len(peers)}")
                for idx, peer in enumerate(peers, 1):
                    print(f"\n  Peer {idx}:")

                    # Public key (always 44 chars: 43 + '=')
                    if pubkey := peer.get('public-key'):
                        print(f"    {'public key':<{18}}: {pubkey}")

                    # Connection status with color
                    status = peer.get('connection-status', 'unknown')
                    if status == 'up':
                        status_str = Decore.green(status.upper())
                    else:
                        status_str = Decore.red(status.upper())
                    print(f"    {'status':<{18}}: {status_str}")

                    # Endpoint information
                    if endpoint := peer.get('endpoint-address'):
                        port = peer.get('endpoint-port', '')
                        endpoint_str = f"{endpoint}:{port}" if port else endpoint
                        print(f"    {'endpoint':<{18}}: {endpoint_str}")

                    # Latest handshake
                    if handshake := peer.get('latest-handshake'):
                        print(f"    {'latest handshake':<{18}}: {handshake}")

                    # Transfer statistics
                    if transfer := peer.get('transfer'):
                        tx = transfer.get('tx-bytes', '0')
                        rx = transfer.get('rx-bytes', '0')
                        print(f"    {'transfer tx':<{18}}: {tx} bytes")
                        print(f"    {'transfer rx':<{18}}: {rx} bytes")


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
    hdr = (f"{'⚑':<{Pad.flags}}"
           f"{'INTERFACE':<{Pad.iface}}"
           f"{'PROTOCOL':<{Pad.proto}}"
           f"{'STATE':<{Pad.state}}"
           f"{'DATA':<{Pad.data}}")

    print(Decore.invert(hdr))

    # Set class variable with routing-enabled interfaces (with IP forwarding)
    if "ietf-routing:routing" in json:
        routing_data = json["ietf-routing:routing"].get("interfaces", {})
        Iface._routing_ifaces = set(routing_data.get("interface", []))
    else:
        Iface._routing_ifaces = set()

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

        if iface.is_wireguard():
            iface.pr_wireguard()
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
    # Set class variable with routing-enabled interfaces (with IP forwarding)
    if "ietf-routing:routing" in json:
        routing_data = json["ietf-routing:routing"].get("interfaces", {})
        Iface._routing_ifaces = set(routing_data.get("interface", []))
    else:
        Iface._routing_ifaces = set()

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
        order = ""
        for boot in boot_order:
            order += f"{boot.strip()} "
        print(f"Boot order : {order}")
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

    service_table = SimpleTable([
        Column('NAME'),
        Column('STATUS'),
        Column('PID', 'right'),
        Column('MEM', 'right'),
        Column('UP', 'right'),
        Column('RST', 'right'),
        Column('DESCRIPTION')
    ])

    for svc in services:
        name = svc.get('name', '')
        status = svc.get('status', '')
        pid = svc.get('pid', 0)
        description = svc.get('description', '')
        stats = svc.get('statistics', {})

        if status in ('running', 'active', 'done'):
            status_str = Decore.green(status)
        elif status in ('crashed', 'failed', 'halted', 'missing', 'dead', 'conflict'):
            status_str = Decore.red(status)
        else:
            status_str = Decore.yellow(status)

        pid_str = str(pid) if pid > 0 else ' '

        memory_bytes = int(stats.get('memory-usage', 0))
        uptime_secs = int(stats.get('uptime', 0))
        restart_count = stats.get('restart-count', 0)

        memory_str = format_memory_bytes(memory_bytes)
        uptime_str = format_uptime_seconds(uptime_secs)

        service_table.row(name, status_str, pid_str, memory_str,
                          uptime_str, restart_count, description)

    service_table.print()


def show_hardware(json):
    if not json.get("ietf-hardware:hardware"):
        print("Error, top level \"ietf-hardware:component\" missing")
        sys.exit(1)

    components = get_json_data({}, json, "ietf-hardware:hardware", "component")

    motherboard = [c for c in components if c.get("class") == "iana-hardware:chassis"]
    usb_ports = [c for c in components if c.get("class") == "infix-hardware:usb"]
    sensors = [c for c in components if c.get("class") == "iana-hardware:sensor"]
    wifi_radios = [c for c in components if c.get("class") == "infix-hardware:wifi"]
    gps_receivers = [c for c in components if c.get("class") == "infix-hardware:gps"]

    width = max(PadSensor.table_width(), 62)

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
        if board.get("infix-hardware:phys-address"):
            print(f"Base MAC Address    : {board['infix-hardware:phys-address']}")
        if board.get("hardware-rev"):
            print(f"Hardware Revision   : {board['hardware-rev']}")

    if wifi_radios:
        Decore.title("WiFi radios", width)

        radios_table = SimpleTable([
            Column('NAME', flexible=True),
            Column('MANUFACTURER', flexible=True),
            Column('BANDS', 'right'),
            Column('STANDARDS', 'right'),
            Column('MAX AP', 'right')
        ], min_width=width)

        for component in wifi_radios:
            phy = component.get("name", "")
            manufacturer = component.get("mfg-name", "Unknown")

            radio_data = component.get("infix-hardware:wifi-radio", {})

            bands = radio_data.get("bands", [])
            band_names = []
            has_ht = False
            has_vht = False
            has_he = False

            for band in bands:
                if band.get("name"):
                    band_names.append(band["name"])
                if band.get("ht-capable"):
                    has_ht = True
                if band.get("vht-capable"):
                    has_vht = True
                if band.get("he-capable"):
                    has_he = True

            bands_str = "/".join(band_names) if band_names else "Unknown"

            standards = []
            if has_ht:
                standards.append("11n")
            if has_vht:
                standards.append("11ac")
            if has_he:
                standards.append("11ax")
            standard_str = "/".join(standards) if standards else "Unknown"

            max_if = radio_data.get("max-interfaces", {})
            max_ap = max_if.get('ap', 'N/A') if max_if else 'N/A'

            radios_table.row(phy, manufacturer, bands_str, standard_str, max_ap)
        radios_table.print()

    if gps_receivers:
        Decore.title("GPS/GNSS Receivers", width)

        for component in gps_receivers:
            gps = component.get("infix-hardware:gps-receiver", {})
            name = component.get("name", "unknown")
            device = gps.get("device", "N/A")
            driver = gps.get("driver", "Unknown")
            fix = gps.get("fix-mode", "none")
            activated = gps.get("activated", False)

            print(f"{'Name':<20}: {name}")
            print(f"{'Device':<20}: {device}")
            print(f"{'Driver':<20}: {driver}")
            print(f"{'Status':<20}: {'Active' if activated else 'Inactive'}")
            print(f"{'Fix':<20}: {fix.upper()}")

            sat_vis = gps.get("satellites-visible")
            sat_used = gps.get("satellites-used")
            if sat_vis is not None:
                print(f"{'Satellites':<20}: {sat_used}/{sat_vis} (used/visible)")

            lat = gps.get("latitude")
            lon = gps.get("longitude")
            alt = gps.get("altitude")
            if lat is not None and lon is not None:
                lat_f = float(lat)
                lon_f = float(lon)
                lat_dir = "N" if lat_f >= 0 else "S"
                lon_dir = "E" if lon_f >= 0 else "W"
                pos = f"{abs(lat_f):.6f}{lat_dir} {abs(lon_f):.6f}{lon_dir}"
                if alt is not None:
                    pos += f" {alt}m"
                print(f"{'Position':<20}: {pos}")

            pps = gps.get("pps-available", False)
            print(f"{'PPS':<20}: {'Available' if pps else 'Not available'}")

    if usb_ports:
        Decore.title("USB Ports", width)

        usb_table = SimpleTable([
            Column('NAME', flexible=True),
            Column('STATE'),
            Column('OPER')
        ], min_width=width)

        for component in usb_ports:
            port = USBport(component)
            usb_table.row(port.name, port.state, port.oper)

        usb_table.print()

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


def resolve_container_network(network, all_ifaces):
    """Resolve container network to bridge names or network type.

    Args:
        network: Container network dict from operational data
        all_ifaces: List of all interface dicts

    Returns:
        str: Network description (bridge name, "host", interface name, or "-")
    """
    if network.get('host'):
        return "host"

    if 'interface' not in network:
        return "-"

    interfaces = network.get('interface', [])
    if not interfaces:
        return "-"

    network_names = []
    for iface in interfaces:
        iface_name = iface.get('name', '')
        iface_data = next((i for i in all_ifaces if i.get('name') == iface_name), None)

        bridge_found = False
        if iface_data:
            # Check if this is a VETH with a peer (host-side veth)
            veth = iface_data.get('infix-interfaces:veth', {})
            peer_name = veth.get('peer')

            if peer_name:
                # Find the peer interface and check if it's a bridge port
                peer_data = next((i for i in all_ifaces if i.get('name') == peer_name), None)
                if peer_data:
                    bridge_port = peer_data.get('infix-interfaces:bridge-port', {})
                    bridge_name = bridge_port.get('bridge')
                    if bridge_name:
                        network_names.append(bridge_name)
                        bridge_found = True

        # If not found via veth peer, try reverse lookup
        if not bridge_found:
            for host_iface in all_ifaces:
                container_net = host_iface.get('infix-interfaces:container-network', {})
                containers_list = container_net.get('containers', [])

                if iface_name in containers_list:
                    bridge_port = host_iface.get('infix-interfaces:bridge-port', {})
                    bridge_name = bridge_port.get('bridge')

                    if bridge_name:
                        network_names.append(bridge_name)
                        bridge_found = True
                        break

        if not bridge_found:
            network_names.append(iface_name)

    return ', '.join(network_names)


def show_container(json):
    """Display container table view with resource usage."""
    containers_data = json.get("infix-containers:containers", {})
    containers = containers_data.get("container", [])

    if not containers:
        print("No containers configured.")
        return

    # Get interface data for network resolution
    all_ifaces_data = json.get('ietf-interfaces:interfaces', {})
    all_ifaces = all_ifaces_data.get('interface', [])

    # Create table with column definitions
    container_table = SimpleTable([
        Column('NAME'),
        Column('STATUS'),
        Column('NETWORK'),
        Column('MEMORY (KiB)', 'right'),
        Column('CPU%', 'right')
    ])

    for container in containers:
        name = container.get("name", "")
        status = container.get("status", "")

        # Get network information
        network = container.get('network', {})
        network_str = resolve_container_network(network, all_ifaces)

        # Get resource limits and usage
        resource_limit = container.get("resource-limit", {})
        resource_usage = container.get("resource-usage", {})

        mem_limit = resource_limit.get("memory")
        mem_usage = resource_usage.get("memory")
        cpu_usage = resource_usage.get("cpu", "0.0")

        # Format memory display
        if mem_usage is not None and mem_limit is not None:
            memory_str = f"{mem_usage}/{mem_limit}"
        elif mem_usage is not None:
            memory_str = str(mem_usage)
        else:
            memory_str = "-"

        # Format CPU display
        cpu_str = str(cpu_usage)

        # Color code status like in show_services
        if status in ('running', 'active'):
            status_str = Decore.green(status)
        elif status in ('exited', 'stopped', 'created'):
            status_str = Decore.yellow(status)
        elif status in ('error', 'dead'):
            status_str = Decore.red(status)
        else:
            status_str = status

        container_table.row(name, status_str, network_str, memory_str, cpu_str)

    container_table.print()


def show_container_detail(json, name):
    """Display detailed container view with full resource information."""
    containers_data = json.get("infix-containers:containers", {})
    containers = containers_data.get("container", [])

    container = None
    for c in containers:
        if c.get("name") == name:
            container = c
            break

    if not container:
        print(f"Container '{name}' not found.")
        return

    print(f"Name            : {container.get('name', '-')}")
    print(f"Container ID    : {container.get('id', '-')}")
    print(f"Image           : {container.get('image', '-')}")
    print(f"Image ID        : {container.get('image-id', '')}")

    command = container.get('command')
    if command:
        print(f"Command         : {command}")

    # Get network information
    network = container.get('network', {})
    all_ifaces_data = json.get('ietf-interfaces:interfaces', {})
    all_ifaces = all_ifaces_data.get('interface', [])
    network_str = resolve_container_network(network, all_ifaces)
    print(f"Network         : {network_str}")

    print(f"Status          : {container.get('status', '-')}")
    print(f"Running         : {'yes' if container.get('running') else 'no'}")

    resource_limit = container.get("resource-limit", {})
    resource_usage = container.get("resource-usage", {})

    pids = resource_usage.get("pids")
    if pids is not None:
        print(f"Processes       : {pids}")

    mem_limit = resource_limit.get("memory")
    if mem_limit is not None:
        print(f"Memory Limit    : {mem_limit} KiB")

    mem_usage = resource_usage.get("memory")
    if mem_usage is not None:
        mem_usage_int = int(mem_usage)
        if mem_limit:
            mem_limit_int = int(mem_limit)
            percent = (mem_usage_int / mem_limit_int) * 100
            print(f"Memory Usage    : {mem_usage_int} KiB ({percent:.1f}%)")
        else:
            print(f"Memory Usage    : {mem_usage_int} KiB")

    cpu_limit = resource_limit.get("cpu")
    if cpu_limit is not None:
        cpu_limit_int = int(cpu_limit)
        cores = cpu_limit_int / 1000.0
        print(f"CPU Limit       : {cpu_limit_int} millicores ({cores:.1f} cores)")

    cpu_usage = resource_usage.get("cpu")
    if cpu_usage is not None:
        print(f"CPU Usage       : {cpu_usage}%")


def show_ntp_source_detail_single(source, is_association=False):
    """Display detailed information for a single NTP source (auto-detected single source)"""
    print(f"{'Server address':<20}: {source.get('address', 'N/A')}")

    # State
    if is_association:
        prefer = source.get("prefer", False)
        state_desc = "Selected sync source" if prefer else "Candidate"
    else:
        state = source.get('state', 'unknown')
        state_desc = {
            'selected': 'Selected sync source',
            'candidate': 'Candidate',
            'unreach': 'Unreachable',
            'not-combined': 'Not combined'
        }.get(state, state)
    print(f"{'State':<20}: {state_desc}")

    # Stratum
    stratum = source.get('stratum')
    if stratum is not None:
        print(f"{'Stratum':<20}: {stratum}")

    # Poll interval
    poll = source.get('poll')
    if poll is not None:
        poll_seconds = 2 ** poll
        print(f"{'Poll interval':<20}: {poll} (2^{poll} seconds = {poll_seconds}s)")

def show_ntp(json, address=None):
    """Unified NTP status display for both client and server modes"""
    ntp_data = json.get("ietf-ntp:ntp", {})
    port = ntp_data.get("port")
    is_server = port is not None

    sources = []
    if is_server:
        associations = ntp_data.get("associations", {}).get("association", [])
        sources = associations
    else:
        system_state = json.get("ietf-system:system-state", {})
        if system_state:
            sources = get_json_data({}, json, 'ietf-system:system-state', 'infix-system:ntp', 'sources', 'source')

    if address:
        matching = [s for s in sources if s.get('address') == address]
        if not matching:
            print(f"No NTP source found with address: {address}")
            return
        if is_server:
            show_ntp_association_detail(matching[0])
        else:
            show_ntp_source_detail_single(matching[0], False)
        return

    # Check for GPS/GNSS hardware reference clocks
    hw_components = json.get("ietf-hardware:hardware", {}).get("component", [])
    gps_sources = [c for c in hw_components if c.get("class") == "infix-hardware:gps"]

    if is_server:
        if sources:
            print(f"{'Mode':<20}: Relay (no local reference clock)")
        elif gps_sources:
            gps_names = ", ".join(c.get("name", "?") for c in gps_sources)
            print(f"{'Mode':<20}: Server (GPS reference clock: {gps_names})")
        else:
            print(f"{'Mode':<20}: Server (local reference clock)")
        print(f"{'Port':<20}: {port}")

        # Show operational stratum
        refclock = ntp_data.get("refclock-master")
        if refclock:
            stratum = refclock.get("master-stratum")
            if stratum is not None:
                print(f"{'Stratum':<20}: {stratum}")

        # Show reference time
        clock_state = ntp_data.get("clock-state", {}).get("system-status", {})
        ref_time = clock_state.get("reference-time")
        if ref_time:
            from datetime import datetime
            try:
                dt = datetime.fromisoformat(ref_time.replace("Z", "+00:00"))
                ref_time_str = dt.strftime("%a %b %d %H:%M:%S %Y")
                print(f"{'Ref time (UTC)':<20}: {ref_time_str}")
            except (ValueError, AttributeError):
                pass

        interfaces = ntp_data.get("interfaces", {}).get("interface", [])
        if interfaces:
            print(f"{'Interfaces':<20}: {', '.join([iface.get('name', '?') for iface in interfaces])}")
        else:
            print(f"{'Interfaces':<20}: All")

        stats = ntp_data.get("ntp-statistics")
        if stats:
            print(f"{'Packets Received':<20}: {stats.get('packet-received', 0):,}")
            print(f"{'Packets Sent':<20}: {stats.get('packet-sent', 0):,}")
            print(f"{'Packets Dropped':<20}: {stats.get('packet-dropped', 0):,}")
            print(f"{'Send Failures':<20}: {stats.get('packet-sent-fail', 0):,}")
    else:
        print(f"{'Mode':<20}: Client")

        # Show local clock state in client mode
        clock_state = ntp_data.get("clock-state", {}).get("system-status", {})

        # Show local operational stratum
        stratum = clock_state.get("clock-stratum")
        if stratum is not None:
            print(f"{'Stratum':<20}: {stratum}")

        # Show reference time
        ref_time = clock_state.get("reference-time")
        if ref_time:
            from datetime import datetime
            try:
                dt = datetime.fromisoformat(ref_time.replace("Z", "+00:00"))
                ref_time_str = dt.strftime("%a %b %d %H:%M:%S %Y")
                print(f"{'Ref time (UTC)':<20}: {ref_time_str}")
            except (ValueError, AttributeError):
                pass

    if len(sources) == 0:
        return
    if len(sources) == 1:
        show_ntp_source_detail_single(sources[0], is_server)
        return
    print()
    table = SimpleTable([
        Column('ADDRESS'),
        Column('MODE'),
        Column('STATE'),
        Column('STRATUM', 'right'),
        Column('POLL', 'right')
    ])

    for source in sources:
        # Extract fields - handle both association and ietf-system format
        address = source.get('address', 'N/A')

        if is_server:
            # Association format
            local_mode = source.get("local-mode", "")
            if ":" in local_mode:
                local_mode = local_mode.split(":")[-1]
            mode_str = local_mode
            prefer = source.get("prefer", False)
            state_str = "selected" if prefer else "candidate"
        else:
            # ietf-system format
            mode_str = source.get('mode', 'unknown')
            state_str = source.get('state', 'unknown')
            if state_str == 'not-combined':
                state_str = 'not combined'

        stratum = source.get('stratum', 0)
        poll = source.get('poll', 0)
        poll_str = f"{2**poll}s" if poll else "-"

        table.row(address, mode_str, state_str, stratum, poll_str)

    table.print()


def show_ntp_tracking(json):
    """Display NTP clock tracking state using YANG operational data"""
    ntp_data = json.get("ietf-ntp:ntp")
    if not ntp_data:
        print("NTP server not enabled.")
        return

    clock_state = ntp_data.get("clock-state", {}).get("system-status", {})
    if not clock_state:
        print("No clock state data available.")
        return

    # Reference ID
    refid = clock_state.get("clock-refid", "N/A")
    print(f"{'Reference ID':<20}: {refid}")

    # Stratum
    stratum = clock_state.get("clock-stratum", 16)
    print(f"{'Stratum':<20}: {stratum}")

    # Reference time (show epoch if not set)
    ref_time = clock_state.get("reference-time")
    if ref_time:
        from datetime import datetime
        try:
            dt = datetime.fromisoformat(ref_time.replace("Z", "+00:00"))
            ref_time_str = dt.strftime("%a %b %d %H:%M:%S %Y")
        except (ValueError, AttributeError):
            ref_time_str = ref_time
    else:
        ref_time_str = "Thu Jan 01 00:00:00 1970"
    print(f"{'Ref time (UTC)':<20}: {ref_time_str}")

    # System time offset (3 fraction-digits in ms = microsecond precision)
    offset = clock_state.get("clock-offset")
    if offset is not None:
        offset_sec = float(offset) / 1000.0
        sign = "slow" if offset_sec >= 0 else "fast"
        print(f"{'System time':<20}: {abs(offset_sec):.6f} seconds {sign} of NTP time")

    # Last offset (infix-ntp augment)
    last_offset = clock_state.get("infix-ntp:last-offset")
    if last_offset is not None:
        last_offset_sec = float(last_offset)
        sign = "+" if last_offset_sec >= 0 else ""
        print(f"{'Last offset':<20}: {sign}{last_offset_sec:.9f} seconds")
    else:
        print(f"{'Last offset':<20}: N/A")

    # RMS offset (infix-ntp augment)
    rms_offset = clock_state.get("infix-ntp:rms-offset")
    if rms_offset is not None:
        print(f"{'RMS offset':<20}: {float(rms_offset):.9f} seconds")
    else:
        print(f"{'RMS offset':<20}: N/A")

    # Frequency (convert from Hz difference to ppm)
    nominal_freq = clock_state.get("nominal-freq")
    actual_freq = clock_state.get("actual-freq")
    if nominal_freq and actual_freq:
        freq_diff_hz = float(actual_freq) - float(nominal_freq)
        freq_ppm = (freq_diff_hz / float(nominal_freq)) * 1000000.0
        if freq_ppm == 0.0:
            direction = "slow"
        else:
            direction = "slow" if freq_ppm < 0 else "fast"
        print(f"{'Frequency':<20}: {abs(freq_ppm):.3f} ppm {direction}")

    # Residual freq (infix-ntp augment)
    residual_freq = clock_state.get("infix-ntp:residual-freq")
    if residual_freq is not None:
        residual_freq_val = float(residual_freq)
        sign = "+" if residual_freq_val >= 0 else ""
        print(f"{'Residual freq':<20}: {sign}{residual_freq_val:.3f} ppm")
    else:
        print(f"{'Residual freq':<20}: N/A")

    # Skew (infix-ntp augment)
    skew = clock_state.get("infix-ntp:skew")
    if skew is not None:
        print(f"{'Skew':<20}: {float(skew):.3f} ppm")
    else:
        print(f"{'Skew':<20}: N/A")

    # Root delay (3 fraction-digits in ms = microsecond precision)
    root_delay = clock_state.get("root-delay")
    if root_delay is not None:
        root_delay_sec = float(root_delay) / 1000.0
        print(f"{'Root delay':<20}: {root_delay_sec:.6f} seconds")

    # Root dispersion (3 fraction-digits in ms = microsecond precision)
    root_disp = clock_state.get("root-dispersion")
    if root_disp is not None:
        root_disp_sec = float(root_disp) / 1000.0
        print(f"{'Root dispersion':<20}: {root_disp_sec:.6f} seconds")

    # Update interval (infix-ntp augment)
    update_interval = clock_state.get("infix-ntp:update-interval")
    if update_interval is not None:
        print(f"{'Update interval':<20}: {float(update_interval):.1f} seconds")
    else:
        print(f"{'Update interval':<20}: N/A")

    # Leap status
    sync_state = clock_state.get("sync-state", "")
    if "clock-synchronized" in sync_state:
        leap_status = "Normal"
    elif "clock-never-set" in sync_state:
        leap_status = "Not synchronised"
    else:
        leap_status = "Unknown"
    print(f"{'Leap status':<20}: {leap_status}")


def show_ntp_association_detail(assoc):
    """Display detailed information for a single NTP association"""
    print(f"{'Address':<20}: {assoc.get('address', 'N/A')}")

    # Mode
    local_mode = assoc.get("local-mode", "")
    if ":" in local_mode:
        local_mode = local_mode.split(":")[-1]

    mode_desc = {
        'client': 'Server (client mode) [^]',
        'active': 'Peer (symmetric active) [=]',
        'broadcast-client': 'Broadcast/Local refclock [#]'
    }.get(local_mode, local_mode)
    print(f"{'Mode':<20}: {mode_desc}")

    # State/Prefer
    prefer = assoc.get("prefer", False)
    state_desc = "Selected sync source [*]" if prefer else "Candidate [+]"
    print(f"{'State':<20}: {state_desc}")

    # Configured
    isconfigured = assoc.get("isconfigured", False)
    print(f"{'Configured':<20}: {'Yes' if isconfigured else 'No (dynamic)'}")

    # Stratum
    stratum = assoc.get("stratum")
    if stratum is not None:
        print(f"{'Stratum':<20}: {stratum}")

    # Poll interval
    poll = assoc.get("poll")
    if poll is not None:
        print(f"{'Poll interval':<20}: {poll} (2^{poll} seconds = {2**poll}s)")

    # Reachability
    reach = assoc.get("reach")
    if reach is not None:
        print(f"{'Reachability':<20}: {reach:03o} (octal) = {reach:08b}b")

    # Time since last packet
    now = assoc.get("now")
    if now is not None:
        print(f"{'Last RX':<20}: {now}s ago")

    # Offset
    offset = assoc.get("offset")
    if offset is not None:
        offset_ms = float(offset)
        if abs(offset_ms) < 1.0:
            offset_str = f"{offset_ms * 1000.0:+.1f}us ({offset_ms:+.6f}ms)"
        else:
            offset_str = f"{offset_ms:+.3f}ms ({offset_ms / 1000.0:+.6f}s)"
        print(f"{'Offset':<20}: {offset_str}")

    # Delay
    delay = assoc.get("delay")
    if delay is not None:
        delay_ms = float(delay)
        if abs(delay_ms) < 1.0:
            delay_str = f"{delay_ms * 1000.0:.1f}us ({delay_ms:.6f}ms)"
        else:
            delay_str = f"{delay_ms:.3f}ms ({delay_ms / 1000.0:.6f}s)"
        print(f"{'Delay':<20}: {delay_str}")

    # Dispersion
    dispersion = assoc.get("dispersion")
    if dispersion is not None:
        disp_ms = float(dispersion)
        if abs(disp_ms) < 1.0:
            disp_str = f"{disp_ms * 1000.0:.1f}us ({disp_ms:.6f}ms)"
        else:
            disp_str = f"{disp_ms:.3f}ms ({disp_ms / 1000.0:.6f}s)"
        print(f"{'Dispersion':<20}: {disp_str}")

def show_ntp_source(json, address=None):
    """Display NTP associations/sources"""
    ntp_data = json.get("ietf-ntp:ntp")
    if not ntp_data:
        print("NTP server not enabled.")
        return

    associations = ntp_data.get("associations", {}).get("association", [])

    # Check for GPS/GNSS reference clock sources from hardware data
    hw_components = json.get("ietf-hardware:hardware", {}).get("component", [])
    gps_sources = [c for c in hw_components if c.get("class") == "infix-hardware:gps"]

    # Show GPS reference clocks
    if gps_sources:
        clock_state = ntp_data.get("clock-state", {}).get("system-status", {})
        clock_refid = clock_state.get("clock-refid", "").strip()

        for gps in gps_sources:
            gps_data = gps.get("infix-hardware:gps-receiver", {})
            name = gps.get("name", "unknown")
            driver = gps_data.get("driver", "Unknown")
            fix = gps_data.get("fix-mode", "none")
            activated = gps_data.get("activated", False)
            sat_used = gps_data.get("satellites-used", 0)
            sat_vis = gps_data.get("satellites-visible", 0)

            # Determine if this GPS is the current sync source
            is_synced = clock_refid in ("GPS", "PPS", "GLO", "GAL", "BDS", "GNSS")

            state = "selected" if is_synced else ("active" if activated else "inactive")
            print(f"{'Reference Clock':<20}: {name} ({driver})")
            print(f"{'Status':<20}: {state}")
            print(f"{'Fix Mode':<20}: {fix.upper()}")
            if sat_vis:
                print(f"{'Satellites':<20}: {sat_used}/{sat_vis} (used/visible)")
            print()

    if not associations and not gps_sources:
        print("No NTP associations found.")
        return

    if not associations:
        return

    # If address specified, show detailed view for that association
    if address:
        matching = [a for a in associations if a.get('address') == address]
        if not matching:
            print(f"No NTP association found with address: {address}")
            return
        show_ntp_association_detail(matching[0])
        return

    # If single association, show detailed view automatically
    if len(associations) == 1:
        show_ntp_association_detail(associations[0])
        return

    # Table with chronyc-style columns
    table = SimpleTable([
        Column('MS'),
        Column('Name/IP address', flexible=True),
        Column('Stratum', 'right'),
        Column('Poll', 'right'),
        Column('Reach', 'right'),
        Column('LastRx', 'right'),
        Column('Last sample', 'right')
    ])

    # Build rows
    for assoc in associations:
        # State indicator: * = prefer (sync source), + = candidate
        prefer = assoc.get("prefer", False)
        state = "*" if prefer else "+"

        # Mode indicator
        local_mode = assoc.get("local-mode", "")
        if ":" in local_mode:
            local_mode = local_mode.split(":")[-1]
        # Map to chronyc-style mode indicators
        mode_indicator = "^"  # Default to server mode
        if local_mode == "active":
            mode_indicator = "="
        elif local_mode == "broadcast-client":
            mode_indicator = "#"

        ms_col = f"{mode_indicator}{state}"
        address = assoc.get("address", "N/A")
        stratum = assoc.get("stratum", 0)

        # Poll interval (log2 seconds)
        poll = assoc.get("poll")
        poll_str = str(poll) if poll is not None else "-"

        # Reachability register (display as octal)
        reach = assoc.get("reach")
        if reach is not None:
            reach_str = f"{reach:03o}"
        else:
            reach_str = "-"

        # Time since last packet (LastRx)
        now = assoc.get("now")
        now_str = str(now) if now is not None else "-"

        # Offset (in milliseconds, convert to microseconds for display)
        offset = assoc.get("offset")
        if offset is not None:
            offset_ms = float(offset)
            if abs(offset_ms) < 1.0:
                # Show in microseconds if less than 1ms
                offset_str = f"{offset_ms * 1000.0:+.0f}us"
            else:
                # Show in milliseconds
                offset_str = f"{offset_ms:+.3f}ms"
        else:
            offset_str = "-"

        # Delay (in milliseconds) - show as +/- similar to chronyc
        delay = assoc.get("delay")
        if delay is not None:
            delay_ms = float(delay)
            if abs(delay_ms) < 1.0:
                delay_str = f"+/- {delay_ms * 1000.0:.0f}us"
            else:
                delay_str = f"+/- {delay_ms:.3f}ms"
        else:
            delay_str = ""

        # Last sample column combines offset and delay like chronyc
        if offset_str != "-":
            last_sample = f"{offset_str} {delay_str}"
        else:
            last_sample = "-"

        table.row(ms_col, address, stratum, poll_str, reach_str, now_str, last_sample)

    table.print()


def _analyze_group_permissions(nacm):
    """Analyze NACM rules to determine effective permissions per group.

    NACM rule evaluation order:
    1. Rule-lists are processed sequentially in configuration order
    2. Within each rule-list, rules are evaluated sequentially
    3. First matching rule wins - no further rules are evaluated
    4. If no rule matches, global defaults apply
    5. permit-all (module-name=*, access-operations=*) bypasses everything
    """
    read_default = nacm.get('read-default', 'permit') == 'permit'
    write_default = nacm.get('write-default', 'permit') == 'permit'
    exec_default = nacm.get('exec-default', 'permit') == 'permit'

    rule_lists = nacm.get('rule-list', [])
    groups_config = nacm.get('groups', {}).get('group', [])

    # Collect all deny rules that apply to "*" (all groups)
    # These create restrictions for groups without permit-all
    global_denials = []
    for rule_list in rule_lists:
        if '*' in rule_list.get('group', []):
            for rule in rule_list.get('rule', []):
                if rule.get('action') == 'deny':
                    global_denials.append(rule)

    results = []

    for group in groups_config:
        group_name = group.get('name', '')

        # Defaults
        can_read = read_default
        can_write = write_default
        can_exec = exec_default
        restrictions = []
        has_permit_all = False

        # Process rule-lists in order (first match wins)
        for rule_list in rule_lists:
            list_groups = rule_list.get('group', [])

            # Check if this rule-list applies to this group specifically
            if group_name not in list_groups:
                continue

            for rule in rule_list.get('rule', []):
                action = rule.get('action')
                access_ops = rule.get('access-operations', '')
                module_name = rule.get('module-name', '')

                # permit-all pattern: bypasses ALL other rules including global denials
                if action == 'permit' and module_name == '*' and access_ops == '*':
                    has_permit_all = True
                    break

                # deny pattern for write+exec (like guest)
                if action == 'deny' and module_name == '*':
                    ops = access_ops.lower()
                    if all(op in ops for op in ['create', 'update', 'delete']):
                        can_write = False
                    if 'exec' in ops:
                        can_exec = False

            if has_permit_all:
                break

        # If not permit-all, global denials create restrictions
        # These affect READ too (access-operations: "*" includes read)
        if not has_permit_all:
            for rule in global_denials:
                path = rule.get('path', '')
                module = rule.get('module-name', '')

                if path:
                    # Extract meaningful part: /ietf-system:system/.../password -> password
                    restriction = path.rstrip('/').split('/')[-1]
                elif module:
                    # Remove ietf- prefix for brevity: ietf-keystore -> keystore
                    restriction = module.replace('ietf-', '')
                else:
                    continue

                if restriction and restriction not in restrictions:
                    restrictions.append(restriction)

        results.append({
            'group': group_name,
            'read': can_read,
            'write': can_write,
            'exec': can_exec,
            'restrictions': restrictions,
            'has_permit_all': has_permit_all
        })

    return results


def _nacm_permissions_matrix(nacm, width=None):
    """Render NACM group permissions as a colored matrix.

    Symbols: ✓=full access, ⚠=restricted, ✗=denied

    Returns:
        Multi-line string containing the rendered matrix, or None if no groups
    """
    permissions = _analyze_group_permissions(nacm)
    if not permissions:
        return None

    # Column widths
    group_col_width = max(len(p['group']) for p in permissions)
    group_col_width = max(group_col_width, 5)  # Minimum for "GROUP"
    cell_width = 7  # Width for READ/WRITE/EXEC columns

    # Box drawing
    top_border = "┌" + "─" * (group_col_width + 2) + "┬"
    top_border += "─" * (cell_width + 2) + "┬"
    top_border += "─" * (cell_width + 2) + "┬"
    top_border += "─" * (cell_width + 2) + "┐"

    header_border = "├" + "─" * (group_col_width + 2) + "┼"
    header_border += "─" * (cell_width + 2) + "┼"
    header_border += "─" * (cell_width + 2) + "┼"
    header_border += "─" * (cell_width + 2) + "┤"

    bottom_border = "└" + "─" * (group_col_width + 2) + "┴"
    bottom_border += "─" * (cell_width + 2) + "┴"
    bottom_border += "─" * (cell_width + 2) + "┴"
    bottom_border += "─" * (cell_width + 2) + "┘"

    # Header row
    header = f"│ {'GROUP':<{group_col_width}} │"
    header += f" {'READ':^{cell_width}} │"
    header += f" {'WRITE':^{cell_width}} │"
    header += f" {'EXEC':^{cell_width}} │"

    # Calculate centering
    matrix_width = len(top_border)
    if width and width > matrix_width:
        padding = (width - matrix_width) // 2
        indent = " " * padding
    else:
        indent = ""

    lines = []
    lines.append(indent + top_border)
    lines.append(indent + header)
    lines.append(indent + header_border)

    # Data rows
    for perm in permissions:
        group_name = perm['group']
        has_restrictions = bool(perm['restrictions'])

        # Determine symbols and colors for each cell
        def get_cell(has_access, has_restrictions):
            if not has_access:
                return Decore.red_bg(f" {'✗':^{cell_width}} ")
            elif has_restrictions:
                return Decore.yellow_bg(f" {'⚠':^{cell_width}} ")
            else:
                return Decore.green_bg(f" {'✓':^{cell_width}} ")

        read_cell = get_cell(perm['read'], has_restrictions)
        write_cell = get_cell(perm['write'], has_restrictions)
        exec_cell = get_cell(perm['exec'], has_restrictions)

        row = f"│ {group_name:<{group_col_width}} │{read_cell}│{write_cell}│{exec_cell}│"
        lines.append(indent + row)

    lines.append(indent + bottom_border)

    # Legend
    legend_data = [
        ("✓ Full", Decore.green_bg),
        ("⚠ Restricted", Decore.yellow_bg),
        ("✗ Denied", Decore.red_bg)
    ]
    colorized_parts = [bg_func(f" {text} ") for text, bg_func in legend_data]
    legend = "  ".join(colorized_parts)

    if width:
        visible_legend = "  ".join([f" {text} " for text, _ in legend_data])
        legend_padding = max(0, (width - len(visible_legend)) // 2)
        lines.append(" " * legend_padding + legend)
    else:
        lines.append(indent + legend)

    return "\n".join(lines)


def show_nacm(json):
    """Display users and NACM (Network Configuration Access Control) groups"""
    min_width = 62

    nacm = json.get("ietf-netconf-acm:nacm", {})
    if not nacm:
        print("NACM not configured.")
        print()

    if nacm:
        enabled = "yes" if nacm.get("enable-nacm", True) else "no"
        print(f"{'enabled':<21}: {enabled}")
        print(f"{'default read access':<21}: {nacm.get('read-default', 'permit')}")
        print(f"{'default write access':<21}: {nacm.get('write-default', 'deny')}")
        print(f"{'default exec access':<21}: {nacm.get('exec-default', 'permit')}")
        print(f"{'denied operations':<21}: {nacm.get('denied-operations', 0)}")
        print(f"{'denied data writes':<21}: {nacm.get('denied-data-writes', 0)}")
        print(f"{'denied notifications':<21}: {nacm.get('denied-notifications', 0)}")
        print()


    # Group permissions matrix
    if nacm:
        matrix = _nacm_permissions_matrix(nacm, min_width)
        if matrix:
            print(matrix)
            print()

    # Users table
    system = json.get("ietf-system:system", {})
    users = system.get("authentication", {}).get("user", [])
    if users:
        user_table = SimpleTable([
            Column('USER', min_width=12),
            Column('SHELL'),
            Column('LOGIN', flexible=True)
        ], min_width=min_width)

        for user in users:
            name = user.get("name", "")
            shell_data = user.get("infix-system:shell", "false")
            shell = shell_data.split(":")[-1] if ":" in shell_data else shell_data

            has_password = bool(user.get("password"))
            has_keys = bool(user.get("authorized-key"))
            if has_password and has_keys:
                login = "password+key"
            elif has_password:
                login = "password"
            elif has_keys:
                login = "key"
            else:
                login = "-"

            user_table.row(name, shell, login)

        user_table.print()
        print()

    # Groups table
    groups = nacm.get("groups", {}).get("group", []) if nacm else []
    if groups:
        group_table = SimpleTable([
            Column('GROUP', min_width=12),
            Column('USERS', flexible=True)
        ], min_width=min_width)

        for group in groups:
            name = group.get("name", "")
            members = " ".join(group.get("user-name", []))
            group_table.row(name, members)

        group_table.print()
        print()


def show_nacm_group(json):
    """Display detailed information about a specific NACM group."""
    group_name = json.get('_group_name')
    nacm = json.get("ietf-netconf-acm:nacm", {})

    if not nacm:
        print("NACM not configured.")
        return

    # Find the group
    groups = nacm.get("groups", {}).get("group", [])
    group = None
    for g in groups:
        if g.get("name") == group_name:
            group = g
            break

    if not group:
        print(f"Group '{group_name}' not found.")
        return

    # Group info
    members = group.get("user-name", [])
    members_str = ", ".join(members) if members else "(none)"
    print(f"{'members':<17}: {members_str}")

    # Analyze permissions
    permissions = _analyze_group_permissions(nacm)
    group_perm = None
    for p in permissions:
        if p['group'] == group_name:
            group_perm = p
            break

    if group_perm:
        def perm_str(has_access, has_restrictions):
            if not has_access:
                return Decore.red("no")
            elif has_restrictions:
                return Decore.yellow("restricted")
            else:
                return Decore.green("yes")

        has_restrictions = bool(group_perm['restrictions'])
        print(f"{'read permission':<17}: {perm_str(group_perm['read'], has_restrictions)}")
        print(f"{'write permission':<17}: {perm_str(group_perm['write'], has_restrictions)}")
        print(f"{'exec permission':<17}: {perm_str(group_perm['exec'], has_restrictions)}")

    # Find applicable rules, respecting NACM evaluation order
    # Rule-lists are evaluated in order; first matching rule wins
    # If a permit-all rule is found, no further rules would be evaluated
    rule_lists = nacm.get("rule-list", [])
    applicable_rules = []
    has_permit_all = group_perm.get('has_permit_all', False) if group_perm else False

    for rule_list in rule_lists:
        list_groups = rule_list.get("group", [])

        # Check if this rule-list applies to this group specifically (not via "*")
        if group_name in list_groups:
            list_name = rule_list.get("name", "")
            for rule in rule_list.get("rule", []):
                applicable_rules.append({
                    'list': list_name,
                    'rule': rule,
                    'applies_via': group_name
                })

                # Check if this is a permit-all rule
                action = rule.get('action')
                module_name = rule.get('module-name', '')
                access_ops = rule.get('access-operations', '')
                if action == 'permit' and module_name == '*' and access_ops == '*':
                    # permit-all matches everything, stop here
                    break

            if has_permit_all:
                # Don't process any more rule-lists
                break

    # Only add rules from "*" group if we don't have permit-all
    if not has_permit_all:
        for rule_list in rule_lists:
            list_groups = rule_list.get("group", [])
            if "*" in list_groups and group_name not in list_groups:
                list_name = rule_list.get("name", "")
                for rule in rule_list.get("rule", []):
                    applicable_rules.append({
                        'list': list_name,
                        'rule': rule,
                        'applies_via': "*"
                    })

    if applicable_rules:
        print(f"{'applicable rules':<17}: {len(applicable_rules)}")

        for item in applicable_rules:
            rule = item['rule']
            rule_name = rule.get('name', '(unnamed)')
            action = rule.get('action', 'permit')
            access_ops = rule.get('access-operations', '*')
            module_name = rule.get('module-name', '')
            path = rule.get('path', '')
            rpc_name = rule.get('rpc-name', '')
            comment = rule.get('comment', '')

            # Format action with color
            if action == 'permit':
                action_str = Decore.green("permit")
            else:
                action_str = Decore.red("deny")

            # Build target description
            target = ""
            if path:
                target = path
            elif module_name:
                target = module_name
                if rpc_name:
                    target += f" (rpc: {rpc_name})"

            applies_note = f" (via '*')" if item['applies_via'] == '*' else ""

            Decore.title(f"{rule_name}{applies_note}", width=70)
            print(f"{'  action':<13}: {action_str}")
            print(f"{'  operations':<13}: {access_ops}")
            if target:
                print(f"{'  target':<13}: {target}")
            if comment:
                print(f"{'  comment':<13}: {comment}")
            print()


def show_nacm_user(json):
    """Display detailed information about a specific user's NACM permissions."""
    user_name = json.get('_user_name')
    nacm = json.get("ietf-netconf-acm:nacm", {})
    system = json.get("ietf-system:system", {})

    # Find the user
    users = system.get("authentication", {}).get("user", [])
    user = None
    for u in users:
        if u.get("name") == user_name:
            user = u
            break

    if not user:
        print(f"User '{user_name}' not found.")
        return

    # User details
    shell_data = user.get("infix-system:shell", "false")
    shell = shell_data.split(":")[-1] if ":" in shell_data else shell_data
    print(f"{'shell':<17}: {shell}")

    has_password = bool(user.get("password"))
    has_keys = bool(user.get("authorized-key"))
    if has_password and has_keys:
        login = "password + SSH key"
    elif has_password:
        login = "password"
    elif has_keys:
        login = "SSH key"
    else:
        login = "(none configured)"
    print(f"{'login':<17}: {login}")

    # Find which NACM groups this user belongs to
    if not nacm:
        print("NACM not configured.")
        return

    groups = nacm.get("groups", {}).get("group", [])
    user_groups = []
    for group in groups:
        if user_name in group.get("user-name", []):
            user_groups.append(group.get("name"))

    groups_str = ", ".join(user_groups) if user_groups else "(none)"
    print(f"{'nacm group':<17}: {groups_str}")

    # Show effective permissions for each group
    if user_groups:
        permissions = _analyze_group_permissions(nacm)

        for group_name in user_groups:
            for p in permissions:
                if p['group'] == group_name:
                    def perm_str(has_access, has_restrictions):
                        if not has_access:
                            return Decore.red("no")
                        elif has_restrictions:
                            return Decore.yellow("restricted")
                        else:
                            return Decore.green("yes")

                    has_restrictions = bool(p['restrictions'])
                    print(f"{'read permission':<17}: {perm_str(p['read'], has_restrictions)}")
                    print(f"{'write permission':<17}: {perm_str(p['write'], has_restrictions)}")
                    print(f"{'exec permission':<17}: {perm_str(p['exec'], has_restrictions)}")
                    break

    print()
    print("For detailed rules, use: show nacm group <name>")


def _keystore_format_name(key_format):
    """Simplify key-format to a short display name."""
    fmt = key_format.split(':')[-1] if ':' in key_format else key_format
    return fmt.replace('-key-format', '')


def _keystore_decode_symmetric(key):
    """Decode a symmetric key value for display."""
    key_format = key.get('key-format', '')
    fmt = _keystore_format_name(key_format)
    b64val = key.get('cleartext-symmetric-key', '')

    if fmt == 'passphrase' and b64val:
        try:
            return base64.b64decode(b64val).decode('utf-8')
        except Exception:
            return b64val
    return b64val if b64val else '-'


def _keystore_find_key(keystore, kind, name):
    """Find a key by type and name in the keystore."""
    if kind == 'symmetric':
        keys = keystore.get('symmetric-keys', {}).get('symmetric-key', [])
    else:
        keys = keystore.get('asymmetric-keys', {}).get('asymmetric-key', [])
    for key in keys:
        if key.get('name') == name:
            return key
    return None


def _keystore_asym_type(key):
    """Derive asymmetric key algorithm from key format fields."""
    for field in ('private-key-format', 'public-key-format'):
        fmt = key.get(field, '')
        name = fmt.split(':')[-1] if ':' in fmt else fmt
        name = name.replace('-private-key-format', '').replace('-public-key-format', '')
        if name:
            return name
    return ''


def show_keystore_detail(keystore, kind, name):
    """Display detailed information about a specific key."""
    key = _keystore_find_key(keystore, kind, name)
    if not key:
        print(f'{kind.capitalize()} key "{name}" not found.')
        return

    print(f"{'name':<{20}}: {name}")
    if kind == 'symmetric':
        fmt = _keystore_format_name(key.get('key-format', ''))
        value = _keystore_decode_symmetric(key)
        print(f"{'format':<{20}}: {fmt}")
        print(f"{'value':<{20}}: {value}")
    else:
        ktype = _keystore_asym_type(key)
        if ktype:
            print(f"{'algorithm':<{20}}: {ktype}")
        pub_fmt = _keystore_format_name(key.get('public-key-format', ''))
        if pub_fmt:
            print(f"{'public key format':<{20}}: {pub_fmt}")
        pub_key = key.get('public-key', '')
        if pub_key:
            print(f"{'public key':<{20}}: {pub_key}")


def show_keystore(json, kind=None, name=None):
    """Display keystore keys overview or detail for a specific key."""
    keystore = json.get("ietf-keystore:keystore", {})
    if not keystore:
        print("Keystore is empty.")
        return

    if kind and name:
        show_keystore_detail(keystore, kind, name)
        return

    TABLE_WIDTH = 72

    # Symmetric keys
    sym_keys_data = keystore.get('symmetric-keys', {}).get('symmetric-key', [])
    if sym_keys_data:
        Decore.title("Symmetric Keys", TABLE_WIDTH)
        table = SimpleTable([
            Column('NAME', flexible=True),
            Column('FORMAT'),
            Column('VALUE', flexible=True)
        ], min_width=TABLE_WIDTH)

        for key in sym_keys_data:
            name = key.get('name', '')
            fmt = _keystore_format_name(key.get('key-format', ''))
            value = _keystore_decode_symmetric(key)
            table.row(name, fmt, value)

        table.print()

    # Asymmetric keys
    asym_keys_data = keystore.get('asymmetric-keys', {}).get('asymmetric-key', [])
    if asym_keys_data:
        Decore.title("Asymmetric Keys", TABLE_WIDTH)
        table = SimpleTable([
            Column('NAME', flexible=True),
            Column('TYPE'),
            Column('PUBLIC KEY', flexible=True)
        ], min_width=TABLE_WIDTH)

        for key in asym_keys_data:
            name = key.get('name', '')
            ktype = _keystore_asym_type(key)

            pub_key = key.get('public-key', '')
            if len(pub_key) > 40:
                pub_key = pub_key[:37] + '...'

            table.row(name, ktype, pub_key)

        table.print()

    if not sym_keys_data and not asym_keys_data:
        print("Keystore is empty.")


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

    width = 62
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
        disk_table = SimpleTable([
            Column('MOUNTPOINT', flexible=True),
            Column('SIZE', 'right'),
            Column('USED', 'right'),
            Column('AVAIL', 'right'),
            Column('USE%', 'right')
        ], min_width=width)

        for d in disk_filtered:
            disk_table.row(
                d.get("mount", "?"),
                d.get("size", "?"),
                d.get("used", "?"),
                d.get("available", "?"),
                d.get("percent", "?")
            )
        disk_table.print()


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


def firewall_log_table(limit=None):
    """Create firewall log table (returns None if no logs available)"""
    try:
        with open('/var/log/firewall.log', 'r', encoding='utf-8') as f:
            lines = deque(f, maxlen=limit)

        if not lines:
            return None

        # Create table with column definitions - mark flexible columns
        log_table = SimpleTable([
            Column('TIME'),
            Column('ACTION', 'right'),
            Column('IIF', 'right'),
            Column('SOURCE', flexible=True),
            Column('DEST', flexible=True),
            Column('PROTO'),
            Column('PORT', 'right')
        ])

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
                    time_str = parsed['timestamp'][:14]  # Truncate long timestamps

            if parsed['action'] == 'REJECT':
                action_str = Decore.red(parsed['action'])
            else:
                action_str = Decore.yellow(parsed['action'])

            log_table.row(time_str, action_str, parsed['in_iface'],
                         parsed['src'], parsed['dst'], parsed['proto'],
                         parsed['dpt'])

        return log_table

    except (FileNotFoundError, Exception):
        return None


def show_firewall_logs(limit=None):
    """Show recent firewall log entries, tail -N equivalent (legacy)"""
    log_table = firewall_log_table(limit)
    if log_table:
        log_table.print()
    else:
        print("No logs found (may be disabled or no denied traffic)")


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

    # Create Canvas instance
    canvas = Canvas()

    # Build firewall status with contextual alerts
    lockdown_state = fw.get('lockdown', False)
    logging_enabled = fw.get('logging', 'off') != 'off'

    firewall_status = "active"
    if lockdown_state:  # Lockdown mode takes priority
        firewall_status += f" [ {Decore.flashing_red('LOCKDOWN MODE')} ]"
    elif logging_enabled:
        firewall_status += f" [ {Decore.bold_yellow('MONITORING')} ]"

    # Add status information
    canvas.add_text(f"{Decore.bold('Firewall'):<28}: {firewall_status}")

    lockdown_display = "active" if lockdown_state else "inactive"
    canvas.add_text(f"{Decore.bold('Lockdown mode'):<28}: {lockdown_display}")
    canvas.add_text(f"{Decore.bold('Default zone'):<28}: {fw.get('default', 'unknown')}")
    canvas.add_text(f"{Decore.bold('Log denied traffic'):<28}: {fw.get('logging', 'off')}")

    canvas.add_spacing()

    # Create tables
    zone_table = firewall_zone_table(json)
    policy_table = firewall_policy_table(json)

    # Add zone table
    if zone_table:
        canvas.add_title("Zones")
        canvas.add_table(zone_table)
        canvas.add_spacing()

    # Add policy table
    if policy_table:
        canvas.add_title("Policies")
        canvas.add_table(policy_table)
        canvas.add_spacing()

    # Add firewall logs if logging is enabled
    if logging_enabled:
        log_table = firewall_log_table(limit=10)
        if log_table:
            canvas.add_title("Log (last 10)")
            canvas.add_table(log_table)
            canvas.add_spacing()

    # Get max width for matrix centering
    max_width = canvas.get_max_width()

    # Render matrix centered to max table width
    matrix_text = firewall_matrix(fw, width=max_width)
    if matrix_text:
        # Insert matrix after status lines and first spacing (index 5)
        # Status: 4 lines + 1 spacing = index 5
        canvas.insert_title(5, "Zone Matrix")
        canvas.insert_raw(6, matrix_text)
        canvas.insert_spacing(7)

    # Render the canvas
    canvas.render()


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


def firewall_matrix(fw, width=None):
    """Render zone-to-zone traffic flow matrix as a multi-line string.

    Args:
        fw:    Firewall config dict with zones/policies
        width: Optional target width for centering (from Canvas)

    Symbols: ✓=allow, ✗=deny, ⚠=conditional, —=n/a

    Returns:
        Multi-line string containing the rendered matrix, or None if < 2 zones
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

    # Build matrix rows
    lines = []

    # Calculate centering if width is provided
    matrix_width = len(top_border)
    if width and width > matrix_width:
        padding = (width - matrix_width) // 2
        indent = " " * padding
    else:
        indent = ""

    lines.append(indent + top_border)
    lines.append(indent + hdr)
    lines.append(indent + middle_border)

    for from_zone in zone_names:
        row = f"│ {from_zone:>{left_col_width}} │"
        for to_zone in zone_names:
            # Find symbol for this cell based on traffic flow
            symbol = traffic_flow(from_zone, to_zone, policy_map,
                                  zones, policies, col_width)
            row += f"{symbol}│"
        lines.append(indent + row)
    lines.append(indent + bottom_border)

    # Add legend
    legend_data = [
        ("✓ Allow", Decore.green_bg),
        ("✗ Deny", Decore.red_bg),
        ("⚠ Conditional", Decore.yellow_bg)
    ]

    colorized_parts = [bg_func(f" {text} ") for text, bg_func in legend_data]
    legend = "   ".join(colorized_parts)

    if width:
        # Calculate legend centering
        # Use visible width (without ANSI codes) for calculation
        visible_legend = "   ".join([f" {text} " for text, _ in legend_data])
        legend_padding = max(0, (width - len(visible_legend)) // 2)
        lines.append(" " * legend_padding + legend)
    else:
        lines.append(indent + legend)

    return "\n".join(lines)


def show_firewall_matrix(json):
    """Renders visual zone-to-zone traffic flow matrix."""
    fw = json.get('infix-firewall:firewall', {})
    if fw:
        print(firewall_matrix(fw))


def firewall_zone_table(json):
    """Create firewall zones table (returns SimpleTable or None)"""
    fw = json.get('infix-firewall:firewall', {})
    zones = fw.get('zone', [])

    if not zones:
        return None

    # Create zones table with flexible columns
    zone_table = SimpleTable([
        Column(''),  # Lock icon
        Column('NAME', flexible=True),
        Column('TYPE'),
        Column('DATA', flexible=True),
        Column('ALLOWED HOST SERVICES', flexible=True)
    ])

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

        # Add first line with zone name and services
        if config_lines:
            first_type, first_data = config_lines[0]
            zone_table.row(locked, name, first_type, first_data, services_display)

            # Add additional configuration lines as separate rows
            for config_type, config_data in config_lines[1:]:
                zone_table.row('', '', config_type, config_data, '')

    return zone_table


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

        # Create traffic flows table
        flow_table = SimpleTable([
            Column('TO ZONE'),
            Column('ACTION'),
            Column('POLICY'),
            Column('SERVICES')
        ])

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

            flow_table.row('HOST', host_action, '(services)', host_services)

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

            flow_table.row(other_name, action, policy_name, services_display)

        Decore.title(f"Traffic Flows: {zone_name} →")
        flow_table.print()
    else:
        # Use helper to create and display zones table
        zone_table = firewall_zone_table(json)
        if zone_table:
            zone_table.min_width = 72
            zone_table.print()


def firewall_policy_table(json):
    """Create firewall policies table (returns SimpleTable or None)"""
    fw = json.get('infix-firewall:firewall', {})
    policies = fw.get('policy', [])

    if not policies:
        return None

    # Create policies table with flexible columns
    policy_table = SimpleTable([
        Column(''),  # Lock icon
        Column('NAME', flexible=True),
        Column('ACTION'),
        Column('INGRESS', flexible=True),
        Column('EGRESS', flexible=True)
    ])

    sorted_policies = sorted(policies, key=lambda p: p.get('priority', 32767))
    for policy in sorted_policies:
        name = policy.get('name', '')
        ingress = ", ".join(policy.get('ingress', []))
        egress = ", ".join(policy.get('egress', []))
        action = policy.get('action', 'reject')

        immutable = policy.get('immutable', False)
        locked = "⚷" if immutable else " "

        policy_table.row(locked, name, action, ingress, egress)

    return policy_table


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
        # Use helper to create and display policies table
        policy_table = firewall_policy_table(json)
        if policy_table:
            policy_table.min_width = 72
            policy_table.print()


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


def firewall_service_table(json):
    """Create firewall services table (returns SimpleTable or None)"""
    fw = json.get('infix-firewall:firewall', {})
    services = fw.get('service', [])

    if not services:
        return None

    # Create services table with flexible columns
    service_table = SimpleTable([
        Column('NAME'),
        Column('PORTS', flexible=True)
    ])

    for service in services:
        name = service.get('name', '')
        ports = format_port_list(service.get('port', []))
        service_table.row(name, ports)

    return service_table


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
        # Use helper to create and display services table
        service_table = firewall_service_table(json)
        if service_table:
            service_table.min_width = 72
            service_table.print()


def show_ospf(json_data):
    """Show OSPF general instance information"""
    routing = json_data.get('ietf-routing:routing', {})
    protocols = routing.get('control-plane-protocols', {}).get('control-plane-protocol', [])

    ospf_instance = None
    for protocol in protocols:
        if 'ietf-ospf:ospf' in protocol:
            ospf_instance = protocol
            break

    if not ospf_instance:
        print("OSPF is not configured or running")
        return

    ospf = ospf_instance.get('ietf-ospf:ospf', {})
    router_id = ospf.get('router-id', '0.0.0.0')
    areas = ospf.get('areas', {}).get('area', [])

    # OSPF Process header
    print(f" OSPF Routing Process, Router ID: {router_id}")
    print(f" Number of areas attached to this router: {len(areas)}")
    print()

    if not areas:
        print("No areas configured")
        return

    # Display detailed area information
    for area in areas:
        area_id = area.get('area-id', '0.0.0.0')
        area_type = area.get('area-type', 'ietf-ospf:normal-area')

        # Determine area type display
        if area_id == "0.0.0.0":
            area_label = f" Area ID: {area_id} (Backbone)"
        else:
            if 'nssa' in area_type.lower():
                area_label = f" Area ID: {area_id} (NSSA)"
            elif 'stub' in area_type.lower():
                area_label = f" Area ID: {area_id} (Stub)"
            else:
                area_label = f" Area ID: {area_id}"

        print(area_label)

        interfaces = area.get('interfaces', {}).get('interface', [])
        interface_count = len(interfaces)

        # Count active interfaces and neighbors
        active_count = 0
        neighbor_count = 0
        fully_adjacent = 0

        for iface in interfaces:
            # Count active interfaces (enabled and up)
            if iface.get('enabled', False):
                state = iface.get('state', 'down')
                if state != 'down':
                    active_count += 1

            # Count neighbors
            neighbors = iface.get('neighbors', {}).get('neighbor', [])
            neighbor_count += len(neighbors)

            # Count fully adjacent neighbors (full state)
            for neighbor in neighbors:
                if neighbor.get('state', '') == 'full':
                    fully_adjacent += 1

        print(f"   Number of interfaces in this area: Total: {interface_count}, Active: {active_count}")
        print(f"   Number of fully adjacent neighbors in this area: {fully_adjacent}")
        print()


def show_ospf_interfaces(json_data):
    """Show OSPF interface information"""
    routing = json_data.get('ietf-routing:routing', {})
    protocols = routing.get('control-plane-protocols', {}).get('control-plane-protocol', [])

    ospf_instance = None
    for protocol in protocols:
        if 'ietf-ospf:ospf' in protocol:
            ospf_instance = protocol
            break

    if not ospf_instance:
        print("OSPF is not configured or running")
        return

    ospf = ospf_instance.get('ietf-ospf:ospf', {})
    router_id = ospf.get('router-id', '0.0.0.0')
    areas = ospf.get('areas', {}).get('area', [])

    if not areas:
        print("No OSPF interfaces configured")
        return

    # Check if specific interface requested
    requested_ifname = json_data.get('_ifname')

    # Collect all interfaces from all areas
    all_interfaces = []
    for area in areas:
        area_id = area.get('area-id', '0.0.0.0')
        interfaces = area.get('interfaces', {}).get('interface', [])
        for iface in interfaces:
            iface['_area_id'] = area_id  # Add area context
            all_interfaces.append(iface)

    if not all_interfaces:
        print("No OSPF interfaces found")
        return

    # If specific interface requested, show detailed view
    if requested_ifname:
        target_iface = None
        for iface in all_interfaces:
            if iface.get('name') == requested_ifname:
                target_iface = iface
                break

        if not target_iface:
            print(f"Interface {requested_ifname} not found in OSPF")
            return

        # Display detailed interface information (vtysh-style)
        name = target_iface.get('name', 'unknown')
        area_id = target_iface.get('_area_id', '0.0.0.0')
        state = target_iface.get('state', 'down')
        cost = target_iface.get('cost', 0)
        priority = target_iface.get('priority', 1)
        iface_type = target_iface.get('interface-type', 'unknown')
        hello_interval = target_iface.get('hello-interval', 10)
        dead_interval = target_iface.get('dead-interval', 40)
        retransmit_interval = target_iface.get('retransmit-interval', 5)
        transmit_delay = target_iface.get('transmit-delay', 1)
        dr_id = target_iface.get('dr-router-id', '-')
        dr_addr = target_iface.get('dr-ip-addr', '-')
        bdr_id = target_iface.get('bdr-router-id', '-')
        bdr_addr = target_iface.get('bdr-ip-addr', '-')
        neighbors = target_iface.get('neighbors', {}).get('neighbor', [])

        # Get interface IP address from interfaces data
        ip_address = None
        broadcast = None
        if json_data.get('_interfaces'):
            interfaces = json_data['_interfaces'].get('ietf-interfaces:interfaces', {}).get('interface', [])
            for iface in interfaces:
                if iface.get('name') == name:
                    # Get IPv4 address
                    ipv4 = iface.get('ietf-ip:ipv4', {})
                    addresses = ipv4.get('address', [])
                    if addresses:
                        addr = addresses[0]
                        ip = addr.get('ip', '')
                        prefix_len = addr.get('prefix-length', 0)
                        if ip and prefix_len:
                            ip_address = f"{ip}/{prefix_len}"
                            # Calculate broadcast (simple approximation for /30 networks)
                            import ipaddress
                            try:
                                net = ipaddress.IPv4Network(ip_address, strict=False)
                                broadcast = str(net.broadcast_address)
                            except:
                                broadcast = None
                    break

        # Get BFD information from routing data
        bfd_detect_mult = None
        bfd_rx_interval = None
        bfd_tx_interval = None
        bfd_state = None
        if json_data.get('_routing'):
            routing = json_data['_routing'].get('ietf-routing:routing', {})
            protocols = routing.get('control-plane-protocols', {}).get('control-plane-protocol', [])
            for proto in protocols:
                if proto.get('type') == 'infix-routing:bfdv1':
                    bfd = proto.get('ietf-bfd:bfd', {})
                    ip_sh = bfd.get('ietf-bfd-ip-sh:ip-sh', {})
                    sessions_container = ip_sh.get('sessions', {})
                    sessions = sessions_container.get('session', [])
                    # Find BFD session for this interface
                    for session in sessions:
                        if session.get('interface') == name:
                            session_running = session.get('session-running', {})
                            # Get session state
                            bfd_state = session_running.get('local-state', 'down')
                            # Get detection multiplier and intervals
                            rx_interval = session_running.get('negotiated-rx-interval', 0)
                            tx_interval = session_running.get('negotiated-tx-interval', 0)
                            detection_time = session_running.get('detection-time', 0)
                            if rx_interval and detection_time:
                                bfd_detect_mult = detection_time // rx_interval
                            bfd_rx_interval = rx_interval // 1000 if rx_interval else 0  # Convert μs to ms
                            bfd_tx_interval = tx_interval // 1000 if tx_interval else 0
                            break
                    break

        # State display
        state_display = state.upper() if state in ['dr', 'bdr'] else state.capitalize()
        if state == 'dr-other':
            state_display = 'DROther'

        # Network type display (match FRR/Cisco format)
        network_type_map = {
            'point-to-point': 'POINTOPOINT',
            'broadcast': 'BROADCAST',
            'non-broadcast': 'NBMA'
        }
        network_type = network_type_map.get(iface_type, iface_type.upper())

        print(f"{name} is up")
        if ip_address:
            broadcast_str = f", Broadcast {broadcast}" if broadcast else ""
            print(f"  Internet Address {ip_address}{broadcast_str}, Area {area_id}")
        else:
            print(f"  Internet Address (not available), Area {area_id}")
        print(f"  Router ID {router_id}, Network Type {network_type}, Cost: {cost}")
        print(f"  Transmit Delay is {transmit_delay} sec, State {state_display}, Priority {priority}")

        if dr_id != '-':
            print(f"  Designated Router (ID) {dr_id}, Interface Address {dr_addr}")
        if bdr_id != '-':
            print(f"  Backup Designated Router (ID) {bdr_id}, Interface Address {bdr_addr}")

        print(f"  Timer intervals configured, Hello {hello_interval}s, Dead {dead_interval}s, Retransmit {retransmit_interval}")

        # Count adjacent neighbors
        adjacent_count = sum(1 for n in neighbors if n.get('state') == 'full')
        print(f"  Neighbor Count is {len(neighbors)}, Adjacent neighbor count is {adjacent_count}")

        # Show BFD status if available
        if bfd_detect_mult is not None and bfd_rx_interval is not None and bfd_tx_interval is not None:
            # Format state for display
            state_display_map = {
                'up': 'Up (two-way connection established)',
                'down': 'Down (no connection)',
                'init': 'Init (initializing, no two-way yet)',
                'adminDown': 'AdminDown (administratively disabled)'
            }
            bfd_state_display = state_display_map.get(bfd_state, bfd_state.capitalize() if bfd_state else 'Unknown')
            print(f"  BFD: Status: {bfd_state_display}")
            print(f"       Detect Multiplier: {bfd_detect_mult}, Min Rx interval: {bfd_rx_interval}, Min Tx interval: {bfd_tx_interval}")
        else:
            print(f"  BFD: Disabled")

        return

    # Display table view (no specific interface)
    hdr = f"{'INTERFACE':<12} {'AREA':<12} {'STATE':<10} {'COST':<6} {'PRI':<4} {'DR':<15} {'BDR':<15} {'NBRS':<5}"
    print(Decore.invert(hdr))

    for iface in all_interfaces:
        name = iface.get('name', 'unknown')
        area_id = iface.get('_area_id', '0.0.0.0')
        state = iface.get('state', 'down')
        cost = iface.get('cost', 0)
        priority = iface.get('priority', 1)
        dr_id = iface.get('dr-router-id', '-')
        bdr_id = iface.get('bdr-router-id', '-')
        neighbors = iface.get('neighbors', {}).get('neighbor', [])
        nbr_count = len(neighbors)

        # Capitalize state nicely
        state_display = state.upper() if state in ['dr', 'bdr'] else state.capitalize()
        if state == 'dr-other':
            state_display = 'DROther'

        # Shorten router IDs for display
        dr_display = dr_id if dr_id != '-' else '-'
        bdr_display = bdr_id if bdr_id != '-' else '-'

        print(f"{name:<12} {area_id:<12} {state_display:<10} {cost:<6} {priority:<4} {dr_display:<15} {bdr_display:<15} {nbr_count:<5}")


def show_ospf_neighbor(json_data):
    """Show OSPF neighbor information"""
    routing = json_data.get('ietf-routing:routing', {})
    protocols = routing.get('control-plane-protocols', {}).get('control-plane-protocol', [])

    ospf_instance = None
    for protocol in protocols:
        if 'ietf-ospf:ospf' in protocol:
            ospf_instance = protocol
            break

    if not ospf_instance:
        print("OSPF is not configured or running")
        return

    ospf = ospf_instance.get('ietf-ospf:ospf', {})
    areas = ospf.get('areas', {}).get('area', [])

    if not areas:
        print("No OSPF areas configured")
        return

    # Collect all neighbors from all interfaces in all areas
    all_neighbors = []
    for area in areas:
        area_id = area.get('area-id', '0.0.0.0')
        interfaces = area.get('interfaces', {}).get('interface', [])
        for iface in interfaces:
            iface_name = iface.get('name', 'unknown')
            neighbors = iface.get('neighbors', {}).get('neighbor', [])
            for neighbor in neighbors:
                neighbor['_interface'] = iface_name
                neighbor['_area_id'] = area_id
                all_neighbors.append(neighbor)

    if not all_neighbors:
        print("No OSPF neighbors found")
        return

    # Display table header with PRI and UPTIME columns
    hdr = f"{'NEIGHBOR ID':<16} {'PRI':<4} {'STATE':<12} {'UPTIME':<10} {'DEAD TIME':<10} {'ADDRESS':<16} {'INTERFACE':<18} {'AREA':<12}"
    print(Decore.invert(hdr))

    for neighbor in all_neighbors:
        neighbor_id = neighbor.get('neighbor-router-id', '0.0.0.0')
        address = neighbor.get('address', '0.0.0.0')
        state = neighbor.get('state', 'down')
        priority = neighbor.get('priority', 1)  # Default priority is 1
        uptime = neighbor.get('infix-routing:uptime', 0)
        dead_timer = neighbor.get('dead-timer', 0)
        # Use interface-name (e.g., "e5:10.0.23.1") if available, fallback to interface name
        interface = neighbor.get('infix-routing:interface-name', neighbor.get('_interface', 'unknown'))
        area_id = neighbor.get('_area_id', '0.0.0.0')
        role = neighbor.get('infix-routing:role', '')

        # Capitalize state and add role if present
        state_base = state.capitalize() if state != '2-way' else '2-Way'
        if role and state_base == 'Full':
            state_display = f"{state_base}/{role}"
        else:
            state_display = state_base

        # Format uptime (convert seconds to human-readable format)
        if uptime > 0:
            days = uptime // 86400
            hours = (uptime % 86400) // 3600
            minutes = (uptime % 3600) // 60
            seconds = uptime % 60

            if days > 0:
                uptime_display = f"{days}d{hours:02d}h{minutes:02d}m"
            elif hours > 0:
                uptime_display = f"{hours}h{minutes:02d}m{seconds:02d}s"
            elif minutes > 0:
                uptime_display = f"{minutes}m{seconds:02d}s"
            else:
                uptime_display = f"{seconds}s"
        else:
            uptime_display = "-"

        # Format dead timer
        dead_display = f"{dead_timer}s" if dead_timer > 0 else "-"

        print(f"{neighbor_id:<16} {priority:<4} {state_display:<12} {uptime_display:<10} {dead_display:<10} {address:<16} {interface:<18} {area_id:<12}")


def show_ospf_routes(json_data):
    """Show OSPF routing table (local-rib)"""
    routing = json_data.get('ietf-routing:routing', {})
    protocols = routing.get('control-plane-protocols', {}).get('control-plane-protocol', [])

    ospf_instance = None
    for protocol in protocols:
        if 'ietf-ospf:ospf' in protocol:
            ospf_instance = protocol
            break

    if not ospf_instance:
        print("OSPF is not configured or running")
        return

    ospf = ospf_instance.get('ietf-ospf:ospf', {})
    local_rib = ospf.get('local-rib', {})
    routes = local_rib.get('route', [])

    if not routes:
        print("No OSPF routes in local RIB")
        return

    # Display table header (AREA column from infix-routing augmentation)
    hdr = f"{'DESTINATION':<20} {'TYPE':<10} {'AREA':<12} {'METRIC':<8} {'NEXT HOP':<16} {'INTERFACE':<12}"
    print(Decore.invert(hdr))

    for route in routes:
        prefix = route.get('prefix', '0.0.0.0/0')
        route_type = route.get('route-type', 'unknown')
        # Check for area-id with infix-routing prefix (augmented field)
        area_id = route.get('infix-routing:area-id', route.get('area-id', '-'))
        metric = route.get('metric', 0)

        # Simplify route type display
        type_map = {
            'intra-area': 'Intra',
            'inter-area': 'Inter',
            'external-1': 'Ext-1',
            'external-2': 'Ext-2',
            'nssa-1': 'NSSA-1',
            'nssa-2': 'NSSA-2'
        }
        type_display = type_map.get(route_type, route_type)

        # Get next hops
        next_hops_data = route.get('next-hops', {})
        next_hops = next_hops_data.get('next-hop', [])

        if not next_hops:
            print(f"{prefix:<20} {type_display:<10} {area_id:<12} {metric:<8} {'-':<16} {'-':<12}")
        else:
            # Display first next hop on same line as route
            first_hop = next_hops[0]
            next_hop_addr = first_hop.get('next-hop', '-')
            outgoing_iface = first_hop.get('outgoing-interface', '-')

            print(f"{prefix:<20} {type_display:<10} {area_id:<12} {metric:<8} {next_hop_addr:<16} {outgoing_iface:<12}")

            # Display additional next hops indented
            for hop in next_hops[1:]:
                next_hop_addr = hop.get('next-hop', '-')
                outgoing_iface = hop.get('outgoing-interface', '-')
                print(f"{'':<52} {next_hop_addr:<16} {outgoing_iface:<12}")


def show_rip(json_data):
    """Show RIP general instance information"""
    routing = json_data.get('ietf-routing:routing', {})
    protocols = routing.get('control-plane-protocols', {}).get('control-plane-protocol', [])

    rip_instance = None
    for protocol in protocols:
        if 'ietf-rip:rip' in protocol:
            rip_instance = protocol
            break

    if not rip_instance:
        print("RIP is not configured or running")
        return

    rip = rip_instance.get('ietf-rip:rip', {})

    # Display RIP configuration
    print(" RIP Routing Process")
    print()

    distance = rip.get('distance', 120)
    default_metric = rip.get('default-metric', 1)
    num_routes = rip.get('num-of-routes', 0)

    print(f" Administrative distance: {distance}")
    print(f" Default metric: {default_metric}")
    print(f" Number of RIP routes: {num_routes}")
    print()

    # Display timers if available
    timers = rip.get('timers', {})
    if timers:
        update = timers.get('update-interval', 30)
        invalid = timers.get('invalid-interval', 180)
        flush = timers.get('flush-interval', 240)
        print(" Timers:")
        print(f"   Update interval:  {update} seconds")
        print(f"   Invalid interval: {invalid} seconds")
        print(f"   Flush interval:   {flush} seconds")
        print()

    # Display interfaces
    interfaces = rip.get('interfaces', {}).get('interface', [])
    if interfaces:
        print(f" Number of interfaces: {len(interfaces)}")
        for iface in interfaces:
            iface_name = iface.get('interface', 'unknown')
            print(f"   {iface_name}")
        print()


def show_rip_routes(json_data):
    """Show RIP routing table"""
    routing = json_data.get('ietf-routing:routing', {})
    protocols = routing.get('control-plane-protocols', {}).get('control-plane-protocol', [])

    rip_instance = None
    for protocol in protocols:
        if 'ietf-rip:rip' in protocol:
            rip_instance = protocol
            break

    if not rip_instance:
        print("RIP is not configured or running")
        return

    rip = rip_instance.get('ietf-rip:rip', {})
    routes = rip.get('ipv4', {}).get('routes', {}).get('route', [])

    if not routes:
        print("No RIP routes")
        return

    # Header
    hdr = f"{'PREFIX':<20} {'METRIC':<8} {'NEXT-HOP':<16} {'INTERFACE':<12}"
    print(Decore.invert(hdr))

    for route in routes:
        prefix = route.get('ipv4-prefix', 'unknown')
        metric = route.get('metric', 0)
        next_hop = route.get('next-hop', '-')
        interface = route.get('interface', '-')

        print(f"{prefix:<20} {metric:<8} {next_hop:<16} {interface:<12}")


def show_rip_interfaces(json_data):
    """Show RIP interface information"""
    routing = json_data.get('ietf-routing:routing', {})
    protocols = routing.get('control-plane-protocols', {}).get('control-plane-protocol', [])

    rip_instance = None
    for protocol in protocols:
        if 'ietf-rip:rip' in protocol:
            rip_instance = protocol
            break

    if not rip_instance:
        print("RIP is not configured or running")
        return

    rip = rip_instance.get('ietf-rip:rip', {})
    interfaces = rip.get('interfaces', {}).get('interface', [])

    if not interfaces:
        print("No RIP interfaces")
        return

    # Header
    hdr = f"{'INTERFACE':<12} {'STATUS':<10} {'SEND':<6} {'RECV':<6} {'SPLIT-HORIZON':<20} {'PASSIVE':<10}"
    print(Decore.invert(hdr))

    for iface in interfaces:
        iface_name = iface.get('interface', 'unknown')
        oper_status = iface.get('oper-status', 'down')
        send_ver = iface.get('send-version', '-')
        recv_ver = iface.get('receive-version', '-')
        split_horizon = iface.get('split-horizon', 'simple')
        passive = 'Yes' if iface.get('passive', False) else 'No'

        print(f"{iface_name:<12} {oper_status:<10} {send_ver:<6} {recv_ver:<6} {split_horizon:<20} {passive:<10}")


def show_rip_neighbors(json_data):
    """Show RIP neighbor information"""
    routing = json_data.get('ietf-routing:routing', {})
    protocols = routing.get('control-plane-protocols', {}).get('control-plane-protocol', [])

    rip_instance = None
    for protocol in protocols:
        if 'ietf-rip:rip' in protocol:
            rip_instance = protocol
            break

    if not rip_instance:
        print("RIP is not configured or running")
        return

    rip = rip_instance.get('ietf-rip:rip', {})
    ipv4 = rip.get('ipv4', {})
    neighbors = ipv4.get('neighbors', {}).get('neighbor', [])

    if not neighbors:
        print("No RIP neighbors")
        return

    # Header
    hdr = f"{'ADDRESS':<16} {'BAD-PACKETS':<14} {'BAD-ROUTES':<12}"
    print(Decore.invert(hdr))

    for neighbor in neighbors:
        address = neighbor.get('ipv4-address', 'unknown')
        bad_packets = neighbor.get('bad-packets-rcvd', 0)
        bad_routes = neighbor.get('bad-routes-rcvd', 0)

        print(f"{address:<16} {bad_packets:<14} {bad_routes:<12}")


def show_bfd_status(json_data):
    """Show BFD status summary"""
    routing = json_data.get('ietf-routing:routing', {})
    protocols = routing.get('control-plane-protocols', {})
    protocol_list = protocols.get('control-plane-protocol', [])

    bfd_protocol = None
    for proto in protocol_list:
        if proto.get('type') == 'infix-routing:bfdv1':
            bfd_protocol = proto
            break

    if not bfd_protocol:
        print("BFD is not enabled")
        return

    print("Is enabled, single-hop (ospf)")


def show_bfd_peers(json_data):
    """Show BFD peer sessions in detailed format"""
    routing = json_data.get('ietf-routing:routing', {})
    protocols = routing.get('control-plane-protocols', {})
    protocol_list = protocols.get('control-plane-protocol', [])

    bfd_protocol = None
    for proto in protocol_list:
        if proto.get('type') == 'infix-routing:bfdv1':
            bfd_protocol = proto
            break

    if not bfd_protocol:
        print("BFD is not enabled")
        return

    bfd = bfd_protocol.get('ietf-bfd:bfd', {})
    ip_sh = bfd.get('ietf-bfd-ip-sh:ip-sh', {})
    sessions_container = ip_sh.get('sessions', {})
    sessions = sessions_container.get('session', [])

    if not sessions:
        print("No BFD sessions found")
        return

    print("BFD Peers:")
    for session in sessions:
        peer = session.get('dest-addr', 'unknown')
        interface = session.get('interface', 'unknown')
        local_disc = session.get('local-discriminator', 0)
        remote_disc = session.get('remote-discriminator', 0)

        # Get session running state
        session_running = session.get('session-running', {})
        local_state = session_running.get('local-state', 'unknown')
        remote_state = session_running.get('remote-state', 'unknown')
        local_diag = session_running.get('local-diagnostic', 'unknown')
        detection_mode = session_running.get('detection-mode', 'unknown')
        detection_time = session_running.get('detection-time', 0)
        rx_interval = session_running.get('negotiated-rx-interval', 0)
        tx_interval = session_running.get('negotiated-tx-interval', 0)

        # Convert microseconds to milliseconds for display
        rx_ms = rx_interval // 1000 if rx_interval else 0
        tx_ms = tx_interval // 1000 if tx_interval else 0
        detect_mult = detection_time // rx_interval if rx_interval and detection_time else 0

        print(f"        peer {peer}")
        print(f"                ID: {local_disc}")
        print(f"                Remote ID: {remote_disc}")
        print(f"                Status: {local_state}")
        print(f"                Diagnostics: {local_diag}")
        print(f"                Remote diagnostics: {local_diag}")
        print(f"                Peer Type: dynamic")
        print(f"                Local timers:")
        print(f"                        Detect-multiplier: {detect_mult}")
        print(f"                        Receive interval: {rx_ms}ms")
        print(f"                        Transmission interval: {tx_ms}ms")
        print(f"                        Echo transmission interval: disabled")
        print(f"                Remote timers:")
        print(f"                        Detect-multiplier: {detect_mult}")
        print(f"                        Receive interval: {rx_ms}ms")
        print(f"                        Transmission interval: {tx_ms}ms")
        print(f"                        Echo transmission interval: disabled")
        print()


def show_bfd_peers_brief(json_data):
    """Show BFD peers in brief table format"""
    routing = json_data.get('ietf-routing:routing', {})
    protocols = routing.get('control-plane-protocols', {})
    protocol_list = protocols.get('control-plane-protocol', [])

    # Find BFD protocol instance
    bfd_protocol = None
    for proto in protocol_list:
        if proto.get('type') == 'infix-routing:bfdv1':
            bfd_protocol = proto
            break

    if not bfd_protocol:
        print("BFD is not enabled")
        return

    bfd = bfd_protocol.get('ietf-bfd:bfd', {})
    ip_sh = bfd.get('ietf-bfd-ip-sh:ip-sh', {})
    sessions_container = ip_sh.get('sessions', {})
    sessions = sessions_container.get('session', [])

    if not sessions:
        print("No BFD sessions found")
        return

    # Build interface to IP mapping by looking up from peer address
    # We need to extract the local IP from the same subnet as the peer
    ifaces_data = json_data.get('_interfaces', {})
    if_ip_map = {}

    if ifaces_data:
        interfaces = ifaces_data.get('ietf-interfaces:interfaces', {}).get('interface', [])
        for iface in interfaces:
            ifname = iface.get('name', '')
            ipv4 = iface.get('ietf-ip:ipv4', {})
            addresses = ipv4.get('address', [])
            for addr_entry in addresses:
                ip = addr_entry.get('ip', '')
                prefix_len = addr_entry.get('prefix-length', 0)
                if ip and ifname:
                    if_ip_map[ifname] = ip

    print(f"Session count: {len(sessions)}")
    hdr = f"{'SESSION ID':<16} {'LOCAL':<22} {'PEER':<22} {'STATE':<10}"
    print(Decore.invert(hdr))

    for session in sessions:
        local_disc = session.get('local-discriminator', 0)
        interface = session.get('interface', '-')
        peer = session.get('dest-addr', '-')

        # Get session running state
        session_running = session.get('session-running', {})
        local_state = session_running.get('local-state', 'unknown')

        # Format local address as interface:ip
        local_ip = if_ip_map.get(interface, '')
        if local_ip:
            local_addr = f"{interface}:{local_ip}"
        else:
            local_addr = interface

        print(f"{local_disc:<16} {local_addr:<22} {peer:<22} {local_state:<10}")


def show_bfd_peer(json_data):
    """Show specific BFD peer details"""
    peer_addr = json_data.get('_peer_addr')
    if not peer_addr:
        print("Error: No peer address specified")
        return

    routing = json_data.get('ietf-routing:routing', {})
    protocols = routing.get('control-plane-protocols', {})
    protocol_list = protocols.get('control-plane-protocol', [])

    # Find BFD protocol instance
    bfd_protocol = None
    for proto in protocol_list:
        if proto.get('type') == 'infix-routing:bfdv1':
            bfd_protocol = proto
            break

    if not bfd_protocol:
        print("BFD is not enabled")
        return

    bfd = bfd_protocol.get('ietf-bfd:bfd', {})
    ip_sh = bfd.get('ietf-bfd-ip-sh:ip-sh', {})
    sessions_container = ip_sh.get('sessions', {})
    sessions = sessions_container.get('session', [])

    # Find the specific peer
    target_session = None
    for session in sessions:
        if session.get('dest-addr') == peer_addr:
            target_session = session
            break

    if not target_session:
        print(f"BFD peer {peer_addr} not found")
        return

    # Display single peer details
    peer = target_session.get('dest-addr', 'unknown')
    interface = target_session.get('interface', 'unknown')
    local_disc = target_session.get('local-discriminator', 0)
    remote_disc = target_session.get('remote-discriminator', 0)

    # Get session running state
    session_running = target_session.get('session-running', {})
    local_state = session_running.get('local-state', 'unknown')
    remote_state = session_running.get('remote-state', 'unknown')
    local_diag = session_running.get('local-diagnostic', 'unknown')
    detection_mode = session_running.get('detection-mode', 'unknown')
    detection_time = session_running.get('detection-time', 0)
    rx_interval = session_running.get('negotiated-rx-interval', 0)
    tx_interval = session_running.get('negotiated-tx-interval', 0)

    # Convert microseconds to milliseconds for display
    rx_ms = rx_interval // 1000 if rx_interval else 0
    tx_ms = tx_interval // 1000 if tx_interval else 0
    detect_mult = detection_time // rx_interval if rx_interval and detection_time else 0

    print("BFD Peer:")
    print(f"            peer {peer}")
    print(f"                ID: {local_disc}")
    print(f"                Remote ID: {remote_disc}")
    print(f"                Status: {local_state}")
    print(f"                Diagnostics: {local_diag}")
    print(f"                Remote diagnostics: {local_diag}")
    print(f"                Peer Type: dynamic")
    print(f"                Local timers:")
    print(f"                        Detect-multiplier: {detect_mult}")
    print(f"                        Receive interval: {rx_ms}ms")
    print(f"                        Transmission interval: {tx_ms}ms")
    print(f"                        Echo transmission interval: disabled")
    print(f"                Remote timers:")
    print(f"                        Detect-multiplier: {detect_mult}")
    print(f"                        Receive interval: {rx_ms}ms")
    print(f"                        Transmission interval: {tx_ms}ms")
    print(f"                        Echo transmission interval: disabled")


def show_bfd(json_data):
    """Legacy function - redirect to show_bfd_peers_brief for backward compatibility"""
    show_bfd_peers_brief(json_data)


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

    subparsers.add_parser('show-container', help='Show containers table')
    subparsers.add_parser('show-container-detail', help='Show container details') \
              .add_argument('name', help='Container name')

    subparsers.add_parser('show-hardware', help='Show USB ports')

    subparsers.add_parser('show-interfaces', help='Show interfaces') \
              .add_argument('-n', '--name', help='Interface name')

    subparsers.add_parser('show-lldp', help='Show LLDP neighbors')

    subparsers.add_parser('show-firewall', help='Show firewall overview')
    subparsers.add_parser('show-firewall-matrix', help='Show firewall matrix')
    subparsers.add_parser('show-firewall-zone', help='Show firewall zones') \
              .add_argument('name', nargs='?', help='Zone name')
    subparsers.add_parser('show-firewall-policy', help='Show firewall policies') \
              .add_argument('name', nargs='?', help='Policy name')
    subparsers.add_parser('show-firewall-service', help='Show firewall services') \
              .add_argument('name', nargs='?', help='Service name')
    subparsers.add_parser('show-firewall-log', help='Show firewall log') \
              .add_argument('limit', nargs='?', help='Last N lines, default: all')

    subparsers.add_parser('show-nacm', help='Show NACM status and groups')
    subparsers.add_parser('show-nacm-group', help='Show NACM group details')
    subparsers.add_parser('show-nacm-user', help='Show NACM user details')

    ks_parser = subparsers.add_parser('show-keystore', help='Show keystore keys')
    ks_parser.add_argument('-t', '--type', help='Key type (symmetric or asymmetric)')
    ks_parser.add_argument('-n', '--name', help='Key name')

    subparsers.add_parser('show-ntp', help='Show NTP status') \
              .add_argument('-a', '--address', help='Show details for specific address')
    subparsers.add_parser('show-ntp-tracking', help='Show NTP tracking status')
    subparsers.add_parser('show-ntp-source', help='Show NTP associations/sources') \
              .add_argument('-a', '--address', help='Show details for specific source')

    subparsers.add_parser('show-bfd', help='Show BFD sessions')
    subparsers.add_parser('show-bfd-status', help='Show BFD status')
    subparsers.add_parser('show-bfd-peers', help='Show BFD peers')
    subparsers.add_parser('show-bfd-peers-brief', help='Show BFD peers brief')
    subparsers.add_parser('show-bfd-peer', help='Show BFD peer')

    subparsers.add_parser('show-ospf', help='Show OSPF instance information')
    subparsers.add_parser('show-ospf-interfaces', help='Show OSPF interfaces')
    subparsers.add_parser('show-ospf-neighbor', help='Show OSPF neighbors')
    subparsers.add_parser('show-ospf-routes', help='Show OSPF routing table')

    subparsers.add_parser('show-rip', help='Show RIP instance information')
    subparsers.add_parser('show-rip-routes', help='Show RIP routing table')
    subparsers.add_parser('show-rip-interfaces', help='Show RIP interfaces')
    subparsers.add_parser('show-rip-neighbors', help='Show RIP neighbors')

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
    elif args.command == "show-container":
        show_container(json_data)
    elif args.command == "show-container-detail":
        show_container_detail(json_data, args.name)
    elif args.command == "show-hardware":
        show_hardware(json_data)
    elif args.command == "show-interfaces":
        show_interfaces(json_data, args.name)
    elif args.command == "show-lldp":
        show_lldp(json_data)
    elif args.command == "show-firewall":
        show_firewall(json_data)
    elif args.command == "show-firewall-matrix":
        show_firewall_matrix(json_data)
    elif args.command == "show-firewall-zone":
        show_firewall_zone(json_data, args.name)
    elif args.command == "show-firewall-policy":
        show_firewall_policy(json_data, args.name)
    elif args.command == "show-firewall-service":
        show_firewall_service(json_data, args.name)
    elif args.command == "show-firewall-log":
        show_firewall_logs(args.limit)
    elif args.command == "show-nacm":
        show_nacm(json_data)
    elif args.command == "show-nacm-group":
        show_nacm_group(json_data)
    elif args.command == "show-nacm-user":
        show_nacm_user(json_data)
    elif args.command == "show-keystore":
        show_keystore(json_data, getattr(args, 'type', None), args.name)
    elif args.command == "show-ntp":
        show_ntp(json_data, args.address)
    elif args.command == "show-ntp-tracking":
        show_ntp_tracking(json_data)
    elif args.command == "show-ntp-source":
        show_ntp_source(json_data, args.address)
    elif args.command == "show-wifi-radio":
        show_wifi_radio(json_data)
    elif args.command == "show-bfd":
        show_bfd(json_data)
    elif args.command == "show-bfd-status":
        show_bfd_status(json_data)
    elif args.command == "show-bfd-peers":
        show_bfd_peers(json_data)
    elif args.command == "show-bfd-peers-brief":
        show_bfd_peers_brief(json_data)
    elif args.command == "show-bfd-peer":
        show_bfd_peer(json_data)
    elif args.command == "show-ospf":
        show_ospf(json_data)
    elif args.command == "show-ospf-interfaces":
        show_ospf_interfaces(json_data)
    elif args.command == "show-ospf-neighbor":
        show_ospf_neighbor(json_data)
    elif args.command == "show-ospf-routes":
        show_ospf_routes(json_data)
    elif args.command == "show-rip":
        show_rip(json_data)
    elif args.command == "show-rip-routes":
        show_rip_routes(json_data)
    elif args.command == "show-rip-interfaces":
        show_rip_interfaces(json_data)
    elif args.command == "show-rip-neighbors":
        show_rip_neighbors(json_data)
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
