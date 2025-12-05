#!/usr/bin/env python3
"""
WiFi Channel Visualization Tool
Shows graphical representation of WiFi channel overlap and utilization
"""

import sys
import json
import argparse


class Colors:
    """ANSI color codes for terminal output"""
    RESET = '\033[0m'
    BOLD = '\033[1m'
    RED = '\033[91m'
    GREEN = '\033[92m'
    YELLOW = '\033[93m'
    BLUE = '\033[94m'
    MAGENTA = '\033[95m'
    CYAN = '\033[96m'
    GRAY = '\033[90m'
    BG_RED = '\033[101m'
    BG_GREEN = '\033[102m'
    BG_YELLOW = '\033[103m'
    BG_BLUE = '\033[104m'
    BG_GRAY = '\033[100m'


def freq_to_channel(freq):
    """Convert frequency (MHz) to WiFi channel number"""
    # 2.4 GHz band
    if 2412 <= freq <= 2484:
        if freq == 2484:
            return 14
        return (freq - 2412) // 5 + 1
    # 5 GHz band
    elif 5170 <= freq <= 5825:
        return (freq - 5000) // 5
    # 6 GHz band
    elif 5955 <= freq <= 7115:
        return (freq - 5950) // 5
    return None


def get_channel_frequency(channel, band='2.4'):
    """Get center frequency for a channel"""
    if band == '2.4':
        if channel == 14:
            return 2484
        return 2412 + (channel - 1) * 5
    elif band == '5':
        return 5000 + channel * 5
    return None


def get_busy_percentage(channel_data):
    """Calculate channel busy percentage"""
    active = channel_data.get('active-time', 0)
    busy = channel_data.get('busy-time', 0)
    if active > 0:
        return (busy / active) * 100
    return 0


def get_utilization_color(busy_pct):
    """Get color based on channel utilization"""
    if busy_pct >= 50:
        return Colors.RED
    elif busy_pct >= 25:
        return Colors.YELLOW
    elif busy_pct >= 10:
        return Colors.CYAN
    else:
        return Colors.GREEN


def draw_channel_graph_2_4ghz(survey_data):
    """Draw channel overlap graph for 2.4 GHz band"""
    # Parse survey data
    channels = {}
    in_use_channel = None

    for ch_data in survey_data:
        freq = ch_data.get('frequency')
        ch_num = freq_to_channel(freq)
        if ch_num and 1 <= ch_num <= 14:
            busy_pct = get_busy_percentage(ch_data)
            channels[ch_num] = {
                'freq': freq,
                'noise': ch_data.get('noise', -100),
                'busy': busy_pct,
                'in_use': ch_data.get('in-use', False),
                'active_time': ch_data.get('active-time', 0),
                'busy_time': ch_data.get('busy-time', 0)
            }
            if ch_data.get('in-use'):
                in_use_channel = ch_num

    if not channels:
        print("No 2.4 GHz channel data available")
        return

    print(f"\n{Colors.BOLD}2.4 GHz WiFi Channel Overlap Visualization{Colors.RESET}")
    print("=" * 80)
    print(f"Channel width: 20 MHz | Channel spacing: 5 MHz")
    print(f"Non-overlapping channels: 1, 6, 11 (shown in {Colors.GREEN}green{Colors.RESET})")
    print()

    # Draw frequency scale
    print("Frequency (MHz):")
    print("2400        2420        2440        2460        2480")
    print("|-----------|-----------|-----------|-----------|")

    # Draw each channel as a bar showing its 20 MHz width
    # Each channel occupies ~4 adjacent channels worth of space
    for ch in range(1, 14):
        if ch not in channels:
            continue

        data = channels[ch]
        busy_pct = data['busy']
        is_in_use = data['in_use']
        noise = data['noise']

        # Determine color based on status
        if is_in_use:
            color = Colors.BG_BLUE
            marker = '█'
        elif busy_pct >= 50:
            color = Colors.RED
            marker = '▓'
        elif busy_pct >= 25:
            color = Colors.YELLOW
            marker = '▒'
        elif busy_pct > 0:
            color = Colors.CYAN
            marker = '░'
        else:
            color = Colors.GRAY
            marker = '·'

        # Non-overlapping channels get green color
        if ch in [1, 6, 11] and not is_in_use and busy_pct < 10:
            color = Colors.GREEN

        # Calculate position (each channel is offset by 5 MHz = 1 position)
        # Channel 1 is at 2412 MHz, base is 2400
        offset = ((data['freq'] - 2400) // 5)

        # Draw channel bar (20 MHz = 4 positions wide)
        line = ' ' * 80
        line_arr = list(line)

        # Mark the channel span (20 MHz width)
        for i in range(4):
            pos = offset + i - 2  # Center the 20 MHz around channel
            if 0 <= pos < len(line_arr):
                line_arr[pos] = marker

        # Add channel label
        label_pos = offset
        if 0 <= label_pos < len(line_arr) - 5:
            # Clear space for label
            for i in range(5):
                if label_pos + i < len(line_arr):
                    line_arr[label_pos + i] = ' '

        line = ''.join(line_arr)

        # Status indicators
        status = ""
        if is_in_use:
            status = f" {Colors.BOLD}[IN USE]{Colors.RESET}"

        busy_color = get_utilization_color(busy_pct)

        print(f"{color}Ch{ch:2d}{Colors.RESET} {color}{line}{Colors.RESET} "
              f"{busy_color}{busy_pct:5.1f}%{Colors.RESET} "
              f"{noise:4d}dBm{status}")

    print("\n" + "=" * 80)
    print(f"\n{Colors.BOLD}Legend:{Colors.RESET}")
    print(f"  {Colors.BG_BLUE}██{Colors.RESET} In use (your network)")
    print(f"  {Colors.RED}▓▓{Colors.RESET} High usage (>50%)")
    print(f"  {Colors.YELLOW}▒▒{Colors.RESET} Medium usage (25-50%)")
    print(f"  {Colors.CYAN}░░{Colors.RESET} Low usage (1-25%)")
    print(f"  {Colors.GRAY}··{Colors.RESET} Idle (<1%)")
    print()


def draw_channel_list(survey_data):
    """Draw a simple channel list with utilization bars"""
    print(f"\n{Colors.BOLD}Channel Utilization{Colors.RESET}")
    print("=" * 80)
    print(f"{'Ch':<4} {'Freq':<6} {'Noise':<8} {'Busy%':<8} {'Utilization Bar':<40}")
    print("-" * 80)

    for ch_data in sorted(survey_data, key=lambda x: x.get('frequency', 0)):
        freq = ch_data.get('frequency')
        ch_num = freq_to_channel(freq)
        if not ch_num:
            continue

        noise = ch_data.get('noise', -100)
        busy_pct = get_busy_percentage(ch_data)
        is_in_use = ch_data.get('in-use', False)

        # Create utilization bar (40 chars wide = 100%)
        bar_length = int(busy_pct * 40 / 100)
        bar_color = get_utilization_color(busy_pct)

        if is_in_use:
            bar = f"{Colors.BG_BLUE}{'█' * bar_length}{Colors.RESET}"
            marker = f" {Colors.BOLD}◀ IN USE{Colors.RESET}"
        else:
            bar = f"{bar_color}{'█' * bar_length}{Colors.RESET}"
            marker = ""

        empty = '░' * (40 - bar_length)

        print(f"{ch_num:<4} {freq:<6} {noise:<8} {busy_pct:5.1f}%  {bar}{Colors.GRAY}{empty}{Colors.RESET}{marker}")

    print()


def draw_overlap_pie(survey_data):
    """Draw a pie-style visualization of channel group utilization"""
    # Parse channel data into 3 non-overlapping groups
    # Group 1: channels 1-5 (centered on ch 1)
    # Group 2: channels 4-8 (centered on ch 6)
    # Group 3: channels 9-13 (centered on ch 11)

    channels = {}
    in_use_channel = None

    for ch_data in survey_data:
        freq = ch_data.get('frequency')
        ch_num = freq_to_channel(freq)
        if ch_num and 1 <= ch_num <= 13:
            busy_pct = get_busy_percentage(ch_data)
            channels[ch_num] = {
                'busy': busy_pct,
                'noise': ch_data.get('noise', -100),
                'in_use': ch_data.get('in-use', False)
            }
            if ch_data.get('in-use'):
                in_use_channel = ch_num

    if not channels:
        return

    # Calculate group utilization (average of channels in each group)
    groups = [
        {'name': 'Ch 1', 'channels': [1, 2, 3, 4, 5], 'center': 1},
        {'name': 'Ch 6', 'channels': [4, 5, 6, 7, 8], 'center': 6},
        {'name': 'Ch 11', 'channels': [9, 10, 11, 12, 13], 'center': 11},
    ]

    for group in groups:
        busy_values = [channels.get(ch, {}).get('busy', 0) for ch in group['channels'] if ch in channels]
        group['avg_busy'] = sum(busy_values) / len(busy_values) if busy_values else 0
        group['center_busy'] = channels.get(group['center'], {}).get('busy', 0)
        group['in_use'] = in_use_channel in group['channels'] if in_use_channel else False

    total_busy = sum(g['avg_busy'] for g in groups)

    print(f"\n{Colors.BOLD}Channel Group Utilization (2.4 GHz){Colors.RESET}")
    print("=" * 60)
    print("Non-overlapping channel groups with their overlap zones:\n")

    # Draw ASCII donut/pie
    pie_width = 50

    # Calculate proportions
    if total_busy > 0:
        for group in groups:
            group['proportion'] = group['avg_busy'] / total_busy
            group['width'] = max(1, int(group['proportion'] * pie_width))
    else:
        for group in groups:
            group['proportion'] = 1/3
            group['width'] = pie_width // 3

    # Adjust to exactly fill pie_width
    total_width = sum(g['width'] for g in groups)
    if total_width < pie_width:
        groups[0]['width'] += pie_width - total_width

    # Draw the pie bar
    pie_chars = ['█', '▓', '░']
    colors = [Colors.GREEN, Colors.YELLOW, Colors.CYAN]

    pie_line = ""
    for i, group in enumerate(groups):
        if group['in_use']:
            color = Colors.BG_BLUE
        elif group['avg_busy'] >= 50:
            color = Colors.RED
        elif group['avg_busy'] >= 25:
            color = Colors.YELLOW
        else:
            color = Colors.GREEN

        char = pie_chars[i % len(pie_chars)]
        pie_line += f"{color}{char * group['width']}{Colors.RESET}"

    # Draw centered pie
    print(f"    ┌{'─' * pie_width}┐")
    print(f"    │{pie_line}│")
    print(f"    └{'─' * pie_width}┘")

    # Legend with percentages
    print()
    for i, group in enumerate(groups):
        char = pie_chars[i % len(pie_chars)]

        if group['in_use']:
            color = Colors.BG_BLUE
            marker = " ◀ IN USE"
        elif group['avg_busy'] >= 50:
            color = Colors.RED
            marker = ""
        elif group['avg_busy'] >= 25:
            color = Colors.YELLOW
            marker = ""
        else:
            color = Colors.GREEN
            marker = ""

        pct_of_total = group['proportion'] * 100
        print(f"    {color}{char * 3}{Colors.RESET} {group['name']:>5}: "
              f"{group['center_busy']:5.1f}% busy (center), "
              f"{group['avg_busy']:5.1f}% avg in overlap zone, "
              f"{pct_of_total:4.1f}% of total{marker}")

    # Draw overlap diagram
    print(f"\n{Colors.BOLD}Channel Overlap Diagram:{Colors.RESET}")
    print("    Ch:  1   2   3   4   5   6   7   8   9  10  11  12  13")
    print("        ╔═══════════════════╗")
    print("    G1: ║ 1 ─ 2 ─ 3 ─ 4 ─ 5 ║         (centered on ch 1)")
    print("        ╚═══════╦═══════════╝")
    print("                ╔═══════════════════╗")
    print("    G2:         ║ 4 ─ 5 ─ 6 ─ 7 ─ 8 ║ (centered on ch 6)")
    print("                ╚═══════════╦═══════╝")
    print("                        ╔═══════════════════════╗")
    print("    G3:                 ║ 9 ─10 ─11 ─12 ─13 ║   (centered on ch 11)")
    print("                        ╚═══════════════════════╝")
    print()


def generate_svg(survey_data, output_file=None):
    """Generate SVG image(s) showing channel overlap and utilization for both bands"""
    # Separate channels by band
    channels_2_4 = {}
    channels_5 = {}
    in_use_2_4 = None
    in_use_5 = None

    for ch_data in survey_data:
        freq = ch_data.get('frequency')
        ch_num = freq_to_channel(freq)
        if not ch_num:
            continue

        busy_pct = get_busy_percentage(ch_data)
        ch_info = {
            'freq': freq,
            'busy': busy_pct,
            'noise': ch_data.get('noise', -100),
            'in_use': ch_data.get('in-use', False)
        }

        if 2400 <= freq <= 2500:
            channels_2_4[ch_num] = ch_info
            if ch_info['in_use']:
                in_use_2_4 = ch_num
        elif 5100 <= freq <= 5900:
            channels_5[ch_num] = ch_info
            if ch_info['in_use']:
                in_use_5 = ch_num

    def busy_to_color(busy_pct, is_in_use=False):
        if is_in_use:
            return "#3b82f6"  # Blue
        elif busy_pct >= 50:
            return "#ef4444"  # Red
        elif busy_pct >= 25:
            return "#f59e0b"  # Yellow/Orange
        elif busy_pct >= 10:
            return "#06b6d4"  # Cyan
        else:
            return "#22c55e"  # Green

    def generate_band_svg(channels, band, freq_min, freq_max, title, non_overlap_channels=None):
        if not channels:
            return None

        width = 900
        height = 400
        margin_left = 60
        margin_right = 40
        margin_top = 60
        margin_bottom = 80
        chart_width = width - margin_left - margin_right
        chart_height = height - margin_top - margin_bottom

        def freq_to_x(freq):
            return margin_left + (freq - freq_min) / (freq_max - freq_min) * chart_width

        def busy_to_height(busy_pct):
            return (busy_pct / 100) * chart_height

        svg_parts = []

        # SVG header
        svg_parts.append(f'''<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {width} {height}">
  <defs>
    <style>
      .title {{ font: bold 18px sans-serif; fill: #333; }}
      .label {{ font: 12px sans-serif; fill: #666; }}
      .axis {{ font: 10px sans-serif; fill: #333; }}
      .channel-label {{ font: bold 10px sans-serif; fill: #333; }}
      .legend {{ font: 12px sans-serif; fill: #333; }}
    </style>
  </defs>

  <!-- Background -->
  <rect width="{width}" height="{height}" fill="#fafafa"/>

  <!-- Title -->
  <text x="{width/2}" y="30" text-anchor="middle" class="title">{title}</text>

  <!-- Chart area -->
  <rect x="{margin_left}" y="{margin_top}" width="{chart_width}" height="{chart_height}" fill="#fff" stroke="#ddd"/>
''')

        # Draw grid lines
        for pct in [25, 50, 75, 100]:
            y = margin_top + chart_height - busy_to_height(pct)
            svg_parts.append(f'  <line x1="{margin_left}" y1="{y}" x2="{margin_left + chart_width}" y2="{y}" stroke="#eee" stroke-dasharray="4,4"/>')
            svg_parts.append(f'  <text x="{margin_left - 5}" y="{y + 4}" text-anchor="end" class="axis">{pct}%</text>')

        # Y-axis label
        svg_parts.append(f'  <text x="15" y="{margin_top + chart_height/2}" text-anchor="middle" transform="rotate(-90, 15, {margin_top + chart_height/2})" class="label">Channel Busy %</text>')

        # Draw channels as bars
        for ch_num, data in sorted(channels.items()):
            center_freq = data['freq']
            busy_pct = data['busy']
            is_in_use = data['in_use']

            # 20 MHz width: ±10 MHz from center
            x1 = freq_to_x(center_freq - 10)
            x2 = freq_to_x(center_freq + 10)
            bar_width = x2 - x1
            bar_height = busy_to_height(busy_pct)
            bar_y = margin_top + chart_height - bar_height

            color = busy_to_color(busy_pct, is_in_use)
            opacity = 0.6 if not is_in_use else 0.8

            # Draw the channel bar
            svg_parts.append(f'  <rect x="{x1}" y="{bar_y}" width="{bar_width}" height="{bar_height}" fill="{color}" opacity="{opacity}" stroke="{color}" stroke-width="1"/>')

            # Channel label at bottom
            label_x = freq_to_x(center_freq)
            svg_parts.append(f'  <text x="{label_x}" y="{margin_top + chart_height + 15}" text-anchor="middle" class="channel-label">{ch_num}</text>')

            # Frequency label (only for some channels to avoid clutter)
            if band == '2.4' or ch_num in [36, 52, 100, 149, 165]:
                svg_parts.append(f'  <text x="{label_x}" y="{margin_top + chart_height + 28}" text-anchor="middle" class="axis">{center_freq}</text>')

            # Busy percentage on top of bar (if tall enough)
            if bar_height > 20:
                svg_parts.append(f'  <text x="{label_x}" y="{bar_y + 15}" text-anchor="middle" class="channel-label" fill="white">{busy_pct:.0f}%</text>')

        # X-axis labels
        svg_parts.append(f'  <text x="{margin_left + chart_width/2}" y="{height - 15}" text-anchor="middle" class="label">Channel (Center Frequency MHz)</text>')

        # Legend
        legend_y = margin_top + 10
        legend_x = margin_left + chart_width - 150
        legend_items = [
            ("#3b82f6", "In Use"),
            ("#ef4444", "High (>50%)"),
            ("#f59e0b", "Medium (25-50%)"),
            ("#06b6d4", "Low (10-25%)"),
            ("#22c55e", "Idle (<10%)"),
        ]

        svg_parts.append(f'  <rect x="{legend_x - 10}" y="{legend_y - 5}" width="160" height="{len(legend_items) * 18 + 10}" fill="white" stroke="#ddd" rx="4"/>')

        for i, (color, label) in enumerate(legend_items):
            y = legend_y + 10 + i * 18
            svg_parts.append(f'  <rect x="{legend_x}" y="{y - 8}" width="12" height="12" fill="{color}" opacity="0.7"/>')
            svg_parts.append(f'  <text x="{legend_x + 18}" y="{y + 2}" class="legend">{label}</text>')

        # Non-overlapping channels note (for 2.4 GHz)
        if non_overlap_channels:
            svg_parts.append(f'  <text x="{margin_left + 5}" y="{margin_top + chart_height + 50}" class="label">Non-overlapping: Ch {", ".join(map(str, non_overlap_channels))}</text>')
            for ch in non_overlap_channels:
                if ch in channels:
                    center_freq = channels[ch]['freq']
                    x = freq_to_x(center_freq)
                    svg_parts.append(f'  <circle cx="{x}" cy="{margin_top + chart_height + 40}" r="8" fill="none" stroke="#22c55e" stroke-width="2"/>')
                    svg_parts.append(f'  <text x="{x}" y="{margin_top + chart_height + 44}" text-anchor="middle" class="channel-label" fill="#22c55e">{ch}</text>')
        else:
            svg_parts.append(f'  <text x="{margin_left + 5}" y="{margin_top + chart_height + 50}" class="label">All channels non-overlapping (20 MHz spacing)</text>')

        svg_parts.append('</svg>')
        return '\n'.join(svg_parts)

    # Generate SVGs for each band
    svg_2_4 = generate_band_svg(channels_2_4, '2.4', 2400, 2485,
                                 "2.4 GHz WiFi Channel Overlap &amp; Utilization",
                                 [1, 6, 11])
    svg_5 = generate_band_svg(channels_5, '5', 5150, 5850,
                               "5 GHz WiFi Channel Utilization",
                               None)

    # Combine or output separately
    if svg_2_4 and svg_5:
        # Combine into one SVG with both bands stacked
        combined = f'''<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 900 820">
  <g transform="translate(0, 0)">
{svg_2_4.replace('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 900 400">', '').replace('</svg>', '')}
  </g>
  <g transform="translate(0, 410)">
{svg_5.replace('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 900 400">', '').replace('</svg>', '')}
  </g>
</svg>'''
        svg_content = combined
    elif svg_2_4:
        svg_content = svg_2_4
    elif svg_5:
        svg_content = svg_5
    else:
        return None

    if output_file:
        with open(output_file, 'w') as f:
            f.write(svg_content)
        print(f"SVG written to: {output_file}")
    else:
        print(svg_content)

    return svg_content


def draw_recommendations(survey_data, json_output=False):
    """Analyze channels and provide recommendations"""
    # Parse channel data
    channels = {}
    in_use_channel = None

    for ch_data in survey_data:
        freq = ch_data.get('frequency')
        ch_num = freq_to_channel(freq)
        if ch_num:
            busy_pct = get_busy_percentage(ch_data)
            channels[ch_num] = {
                'busy': busy_pct,
                'noise': ch_data.get('noise', -100),
                'in_use': ch_data.get('in-use', False)
            }
            if ch_data.get('in-use'):
                in_use_channel = ch_num

    # Find least congested non-overlapping channels
    best_channels = []
    for ch in [1, 6, 11]:
        if ch in channels:
            best_channels.append((ch, channels[ch]['busy']))

    best_channels.sort(key=lambda x: x[1])

    # JSON output
    if json_output:
        output = {
            "recommended_channels": [
                {"channel": ch, "busy_percent": round(busy, 1)}
                for ch, busy in best_channels
            ]
        }
        if in_use_channel:
            output["current_channel"] = in_use_channel
            output["current_busy_percent"] = round(channels.get(in_use_channel, {}).get('busy', 0), 1)

        print(json.dumps(output, indent=2))
        return

    # Text output
    print(f"\n{Colors.BOLD}Channel Recommendations{Colors.RESET}")
    print("=" * 80)

    if in_use_channel:
        print(f"Current channel: {Colors.BOLD}{in_use_channel}{Colors.RESET}")
        current_busy = channels.get(in_use_channel, {}).get('busy', 0)
        if current_busy > 50:
            print(f"  {Colors.RED}⚠{Colors.RESET}  High congestion detected ({current_busy:.1f}% busy)")
        elif current_busy > 25:
            print(f"  {Colors.YELLOW}⚠{Colors.RESET}  Moderate congestion ({current_busy:.1f}% busy)")
        else:
            print(f"  {Colors.GREEN}✓{Colors.RESET}  Good channel utilization ({current_busy:.1f}% busy)")

    print(f"\nRecommended non-overlapping channels (2.4 GHz):")
    for i, (ch, busy) in enumerate(best_channels[:3], 1):
        color = get_utilization_color(busy)
        marker = "★" if i == 1 else " "
        print(f"  {marker} Channel {ch:2d}: {color}{busy:5.1f}% busy{Colors.RESET}")

    print()


def main():
    parser = argparse.ArgumentParser(
        description='Visualize WiFi channel overlap and utilization',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog='''
Examples:
  # Read from yanger output (show all sections)
  yanger -x "ixll -A ssh host sudo" ietf-hardware | %(prog)s

  # Read from file
  %(prog)s survey_data.json

  # Show only list view
  %(prog)s --list survey_data.json

  # Show only specific sections
  %(prog)s --overlap survey_data.json
  %(prog)s --pie survey_data.json
  %(prog)s --utilization survey_data.json
  %(prog)s --recommendations survey_data.json
  %(prog)s --overlap --pie survey_data.json

  # Output recommendations as JSON
  %(prog)s --recommendations --json survey_data.json

  # Generate SVG image
  %(prog)s --svg /tmp/wifi-channels.svg survey_data.json
  %(prog)s --svg - survey_data.json > output.svg
        '''
    )
    parser.add_argument('file', nargs='?', help='JSON file with hardware data (default: stdin)')
    parser.add_argument('--list', action='store_true', help='Show simple list view instead of overlap graph')
    parser.add_argument('--no-color', action='store_true', help='Disable colors')
    parser.add_argument('--json', action='store_true', help='Output recommendations in JSON format')
    parser.add_argument('--svg', metavar='FILE', help='Generate SVG image to FILE (use - for stdout)')

    # Section filters
    parser.add_argument('--overlap', action='store_true', help='Show only channel overlap visualization (2.4 GHz)')
    parser.add_argument('--pie', action='store_true', help='Show only channel group pie chart (2.4 GHz)')
    parser.add_argument('--utilization', action='store_true', help='Show only channel utilization list')
    parser.add_argument('--recommendations', action='store_true', help='Show only channel recommendations')

    args = parser.parse_args()

    # Disable colors if requested
    if args.no_color:
        for attr in dir(Colors):
            if not attr.startswith('_'):
                setattr(Colors, attr, '')

    # Read input
    if args.file:
        with open(args.file, 'r') as f:
            data = json.load(f)
    else:
        data = json.load(sys.stdin)

    # Extract survey data from hardware components
    survey_data = []
    hardware = data.get('ietf-hardware:hardware', {})
    components = hardware.get('component', [])

    for component in components:
        if component.get('class') == 'infix-hardware:wifi':
            wifi_radio = component.get('infix-hardware:wifi-radio', {})
            survey = wifi_radio.get('survey', {})
            channels = survey.get('channel', [])
            if channels:
                survey_data.extend(channels)

    if not survey_data:
        print("No WiFi survey data found in input", file=sys.stderr)
        print("Expected format: yanger ietf-hardware output with wifi-radio survey data", file=sys.stderr)
        sys.exit(1)

    # Generate SVG if requested (exclusive mode)
    if args.svg:
        output_file = None if args.svg == '-' else args.svg
        generate_svg(survey_data, output_file)
        return

    # Determine which sections to show
    # If no section flags are set, show all sections
    show_all = not (args.overlap or args.pie or args.utilization or args.recommendations)

    show_overlap = show_all or args.overlap
    show_pie = show_all or args.pie
    show_utilization = show_all or args.utilization
    show_recommendations_section = show_all or args.recommendations

    # Draw visualization
    freqs = [ch.get('frequency', 0) for ch in survey_data]
    has_2_4ghz = any(2400 <= f <= 2500 for f in freqs)

    if show_overlap and has_2_4ghz and not args.list:
        draw_channel_graph_2_4ghz(survey_data)

    if show_pie and has_2_4ghz:
        draw_overlap_pie(survey_data)

    if show_utilization:
        draw_channel_list(survey_data)

    if show_recommendations_section:
        draw_recommendations(survey_data, json_output=args.json)


if __name__ == '__main__':
    main()
