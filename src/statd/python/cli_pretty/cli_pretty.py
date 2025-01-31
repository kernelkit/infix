#!/usr/bin/env python3
import json
import argparse
import sys
import re
from datetime import datetime, timezone

UNIT_TEST = False

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

class PadNtpSource:
    address = 16
    mode = 13
    state = 13
    stratum = 11
    poll = 14

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
    def yellow(txt):
        return Decore.decorate("33", txt, "39")

    @staticmethod
    def underline(txt):
        return Decore.decorate("4", txt, "24")

    @staticmethod
    def gray_bg(txt):
        return Decore.decorate("100", txt)

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

    def print(self):
        #print(self.name)
        row = f"{self.name:<{PadUsbPort.name}}"
        row += f"{self.state:<{PadUsbPort.state}}"
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

        if self.data.get('infix-interfaces:vlan'):
            self.lower_if = self.data.get('infix-interfaces:vlan', None).get('lower-layer-if',None)
        else:
            self.lower_if = ''

    def is_vlan(self):
        return self.type == "infix-if-type:vlan"

    def is_in_container(self):
        # Return negative if cointainer isn't set or is an empty list
        return getattr(self, 'containers', None)

    def is_bridge(self):
        return self.type == "infix-if-type:bridge"

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

    def pr_veth(self, _ifaces):
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

        if iface.is_veth():
            iface.pr_veth(ifaces)
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

        if iface.is_vlan():
            iface.pr_vlan(ifaces)
            continue

        # These interfaces are printed by there parent, such as bridge
        if iface.lower_if:
            continue
        if iface.bridge:
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
        for _s in slots:
            slot = Software(_s)
            if slot.is_rootfs():
                slot.print()

def show_hardware(json):
    if not json.get("ietf-hardware:hardware"):
       print(f"Error, top level \"ietf-hardware:component\" missing")
       sys.exit(1)

    hdr = (f"{'USB PORTS':<{PadUsbPort.title}}")
    print(Decore.invert(hdr))
    hdr =  (f"{'NAME':<{PadUsbPort.name}}"
            f"{'STATE':<{PadUsbPort.state}}")
    print(Decore.invert(hdr))

    components = get_json_data({}, json, "ietf-hardware:hardware", "component")

    for component in components:
        if component.get("class") == "infix-hardware:usb":
            port = USBport(component)
            port.print()

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

    parser_show_routing_table = subparsers.add_parser('show-routing-table', help='Show the routing table')
    parser_show_routing_table.add_argument('-i', '--ip', required=True, help='IPv4 or IPv6 address')

    parser_show_interfaces = subparsers.add_parser('show-interfaces', help='Show interfaces')
    parser_show_interfaces.add_argument('-n', '--name', help='Interface name')

    parser_show_bridge_mdb = subparsers.add_parser('show-bridge-mdb', help='Show bridge MDB')
    parser_show_bridge_stp = subparsers.add_parser('show-bridge-stp',
                                                   help='Show spanning tree state')

    parser_show_software = subparsers.add_parser('show-software', help='Show software versions')
    parser_show_software.add_argument('-n', '--name', help='Slotname')

    parser_show_hardware = subparsers.add_parser('show-hardware', help='Show USB ports')

    parser_show_ntp_sources = subparsers.add_parser('show-ntp', help='Show NTP sources')

    parser_show_boot_order = subparsers.add_parser('show-boot-order', help='Show NTP sources')

    parser_dhcp_srv = subparsers.add_parser('show-dhcp-server', help='Show DHCP server')
    parser_dhcp_srv.add_argument("-s", "--stats", action="store_true", help="Show server statistics")

    args = parser.parse_args()
    UNIT_TEST = args.test

    if args.command == "show-interfaces":
        show_interfaces(json_data, args.name)
    elif args.command == "show-routing-table":
        show_routing_table(json_data, args.ip)
    elif args.command == "show-software":
        show_software(json_data, args.name)
    elif args.command == "show-bridge-mdb":
        show_bridge_mdb(json_data)
    elif args.command == "show-bridge-stp":
        show_bridge_stp(json_data)
    elif args.command == "show-hardware":
        show_hardware(json_data)
    elif args.command == "show-ntp":
        show_ntp(json_data)
    elif args.command == "show-dhcp-server":
        show_dhcp_server(json_data, args.stats)
    else:
        print(f"Error, unknown command '{args.command}'")
        sys.exit(1)


if __name__ == "__main__":
    main()
