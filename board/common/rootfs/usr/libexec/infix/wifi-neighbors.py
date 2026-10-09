#!/usr/bin/env python3
"""
Tell the other nodes about our access points, learn about theirs.

Usage: wifi-neighbors.py <hostapd config>...

A client asked to move needs to know where to, and a node knows nothing
about the other nodes' access points.  Every 30 seconds each node sends
one frame per network its access points are bridged to, listing them:
BSSID, frequency, PHY type and SSID.  The network is the one the 802.11r
key exchange uses, ft_iface in the hostapd config, so the announcements
reach exactly the nodes a client can roam to.  Bridges flood the frames
over the backhaul like any multicast, no IP address is needed and no
radio ever leaves its channel.

Anything on that network can send such a frame, so every line carries a
tag made with the 802.11r key of its SSID, which the nodes serving the
SSID already share.  A line whose tag does not check out, or that names
an SSID this node does not serve, is dropped.  An access point without
an 802.11r key is neither announced nor learned.

The first line of a frame is 'infix-wifi <version> <hostname>'.  Nodes
only listen to their own version, a frame of another version is logged
once per sender and dropped, so a mixed set of nodes shows up in the
log instead of as a silent lack of neighbors.  Bump VERSION when the
frame format changes.

What is heard is kept for 90 seconds and handed to hostapd as 802.11k
neighbors of every access point with the same SSID, for its neighbor
reports and for the transition requests sent by hostapd.sh and
wifi-steer.sh, which read the candidate list of a BSS from
/run/wifi-neighbors/<bss>, one bss_tm_req neighbor= argument per line.
"""
import hmac
import hashlib
import os
import re
import select
import socket
import struct
import subprocess
import sys
import time

ETHERTYPE = 0x88B5                      # IEEE 802 local experimental 1
GROUP = bytes.fromhex('034b4b000001')   # locally administered group address
MAGIC = b'infix-wifi'
VERSION = 2
PERIOD = 30
EXPIRE = 95
MAX_NEIGHBORS = 64
DIR = '/run/wifi-neighbors'
MAC = re.compile(r'^([0-9a-f]{2}:){5}[0-9a-f]{2}$')


def log(msg):
    subprocess.run(['logger', '-t', 'hostapd', '-p', 'daemon.notice', msg],
                   capture_output=True)


def hostapd_cli(bss, *args):
    try:
        res = subprocess.run(['hostapd_cli', '-i', bss, *args],
                             capture_output=True, text=True, timeout=5)
        return res.stdout if res.returncode == 0 else ''
    except (OSError, subprocess.SubprocessError):
        return ''


def parse_configs(paths):
    """{bss: (ft_iface, key)} for every BSS, both None without 802.11r."""
    bsses = {}
    for path in paths:
        if not path.endswith('.conf'):
            continue
        cur = None
        try:
            lines = open(path).read().splitlines()
        except OSError:
            continue
        for line in lines:
            if line.startswith(('interface=', 'bss=')):
                cur = line.split('=', 1)[1].strip()
                bsses.setdefault(cur, [None, None])
            elif line.startswith('ft_iface=') and cur:
                bsses[cur][0] = line.split('=', 1)[1].strip()
            elif line.startswith('r0kh=') and cur:
                # r0kh=<mac> <nas id> <key>, the key is what every node derives
                # from the mobility domain and the secret
                bsses[cur][1] = line.split()[-1].encode()
    return {bss: tuple(v) for bss, v in bsses.items()}


def opclass(freq):
    """20 MHz operating class of a frequency."""
    if freq >= 5925:
        return 131
    if freq >= 5745:
        return 124
    if freq >= 5500:
        return 121
    if freq >= 5260:
        return 118
    if freq >= 5180:
        return 115
    return 81


def channel(freq):
    if freq == 5935:
        return 2
    if freq >= 5955:
        return (freq - 5950) // 5
    if freq >= 5000:
        return (freq - 5000) // 5
    if freq == 2484:
        return 14
    return (freq - 2407) // 5


def valid(bssid, freq, phy, ssid):
    """A line is only used if every field fits in a neighbor report."""
    return (MAC.match(bssid) is not None
            and (2412 <= freq <= 2484 or 5180 <= freq <= 5885
                 or freq == 5935 or 5955 <= freq <= 7115)
            and 1 <= channel(freq) <= 233
            and 0 <= phy <= 255
            and 1 <= len(ssid.encode()) <= 32)


def tag(key, bssid, freq, phy, ssid):
    msg = f'{bssid} {freq} {phy} {ssid}'.encode()
    return hmac.new(key, msg, hashlib.sha256).hexdigest()[:32]


def own_bsses(bsses):
    """{bss: (bssid, freq, phy, ssid)} from hostapd, for the keyed BSSes up."""
    out = {}
    for bss, (_, key) in bsses.items():
        if not key:
            continue
        cfg = hostapd_cli(bss, 'get_config')
        status = hostapd_cli(bss, 'status')
        bssid = re.search(r'^bssid=(\S+)', cfg, re.M)
        ssid = re.search(r'^ssid=(.*)$', cfg, re.M)
        freq = re.search(r'^freq=(\d+)', status, re.M)
        if not (bssid and ssid and freq):
            continue
        # dot11PHYType: OFDM, HT, VHT, HE
        if 'ieee80211ax=1' in status:
            phy = 14
        elif 'ieee80211ac=1' in status:
            phy = 9
        elif 'ieee80211n=1' in status:
            phy = 7
        else:
            phy = 4
        entry = (bssid.group(1).lower(), int(freq.group(1)), phy, ssid.group(1))
        if valid(*entry):
            out[bss] = entry
    return out


def announcement(host, own, bsses):
    lines = [b'%s %d %s' % (MAGIC, VERSION, host.encode())]
    for bss, (bssid, freq, phy, ssid) in own.items():
        key = bsses[bss][1]
        lines.append(f'{bssid} {freq} {phy} {tag(key, bssid, freq, phy, ssid)} {ssid}'.encode())
    return b'\n'.join(lines) + b'\n'


def parse_announcement(data, keys, seen):
    """[(bssid, freq, phy, ssid)] for the lines tagged with a key of ours."""
    try:
        text = data.decode()
    except UnicodeDecodeError:
        return []
    lines = text.split('\n')
    head = lines[0].split(' ', 2) if lines else []
    if len(head) != 3 or head[0] != MAGIC.decode():
        return []
    if head[1] != str(VERSION):
        if head[2] not in seen:
            seen.add(head[2])
            log(f'wifi-neighbors: {head[2][:64]} announces version {head[1][:8]}, '
                f'ours is {VERSION}, ignoring it')
        return []
    out = []
    for line in lines[1:]:
        parts = line.split(' ', 4)
        if len(parts) < 5:
            continue
        bssid, ssid = parts[0].lower(), parts[4]
        try:
            freq, phy = int(parts[1]), int(parts[2])
        except ValueError:
            continue
        key = keys.get(ssid)
        if not key or not valid(bssid, freq, phy, ssid):
            continue
        if not hmac.compare_digest(parts[3], tag(key, bssid, freq, phy, ssid)):
            continue
        out.append((bssid, freq, phy, ssid))
    return out


def candidate(bssid, freq, phy):
    # BSSID information: AP reachable, same security and key scope
    return f'{bssid},1151,{opclass(freq)},{channel(freq)},{phy}'


def nr_hex(bssid, freq, phy):
    info = 1151
    return (bssid.replace(':', '') + struct.pack('<I', info).hex()
            + bytes([opclass(freq), channel(freq), phy]).hex())


def publish(own, neighbors, published):
    """Write the candidate files and sync hostapd's neighbor database."""
    for bss, (mybssid, _, _, myssid) in own.items():
        mine = {n[0]: n for n in neighbors.values()
                if n[3] == myssid and n[0] != mybssid}
        if published.get(bss) == mine:
            continue
        with open(f'{DIR}/{bss}.tmp', 'w') as f:
            for bssid, freq, phy, _ in mine.values():
                f.write(candidate(bssid, freq, phy) + '\n')
        os.replace(f'{DIR}/{bss}.tmp', f'{DIR}/{bss}')
        for bssid in set(published.get(bss, {})) - set(mine):
            hostapd_cli(bss, 'remove_neighbor', bssid)
        for bssid, freq, phy, ssid in mine.values():
            if published.get(bss, {}).get(bssid) == mine[bssid]:
                continue
            hostapd_cli(bss, 'set_neighbor', bssid, f'ssid="{ssid}"',
                        f'nr={nr_hex(bssid, freq, phy)}')
        published[bss] = mine
        log(f"{bss}: {len(mine)} neighbor(s) for '{myssid}'")


def main():
    os.makedirs(DIR, exist_ok=True)
    bsses = parse_configs(sys.argv[1:])
    ifaces = sorted({ifc for ifc, key in bsses.values() if ifc and key})
    if not ifaces:
        return
    host = socket.gethostname()

    socks = {}
    for ifc in ifaces:
        for _ in range(50):
            try:
                s = socket.socket(socket.AF_PACKET, socket.SOCK_RAW, socket.htons(ETHERTYPE))
                s.bind((ifc, 0))
                socks[s] = ifc
                break
            except OSError:
                time.sleep(0.2)
    if not socks:
        log('wifi-neighbors: no interface to announce on')
        return

    neighbors = {}      # bssid -> (bssid, freq, phy, ssid, last seen)
    published = {}
    next_send = 0.0
    own = {}
    keys = {}           # ssid -> 802.11r key, for the SSIDs we serve
    other = set()       # nodes heard announcing another version
    while True:
        now = time.monotonic()
        if now >= next_send:
            own = own_bsses(bsses)
            keys = {ssid: bsses[bss][1] for bss, (_, _, _, ssid) in own.items()}
            payload = struct.pack('!H', ETHERTYPE) + announcement(host, own, bsses)
            for s, ifc in socks.items():
                try:
                    s.send(GROUP + s.getsockname()[4] + payload)
                except OSError:
                    pass
            next_send = now + PERIOD

        ready, _, _ = select.select(list(socks), [], [], max(0.1, next_send - now))
        for s in ready:
            try:
                data = s.recv(2048)
            except OSError:
                continue
            stamp = time.monotonic()
            for bssid, freq, phy, ssid in parse_announcement(data[14:], keys, other):
                if any(bssid == o[0] for o in own.values()):
                    continue
                if bssid not in neighbors and len(neighbors) >= MAX_NEIGHBORS:
                    continue
                neighbors[bssid] = (bssid, freq, phy, ssid, stamp)

        cutoff = time.monotonic() - EXPIRE
        for bssid in [b for b, n in neighbors.items() if n[4] < cutoff]:
            del neighbors[bssid]
        if own:
            try:
                publish(own, {b: n[:4] for b, n in neighbors.items()}, published)
            except OSError as err:
                log(f'wifi-neighbors: {err}')


if __name__ == '__main__':
    try:
        main()
    except KeyboardInterrupt:
        pass
