"""Minimal PPPoE access concentrator for testing PPPoE clients

A real PPPoE server needs /dev/ppp and PPP support in the kernel the
test container runs on, which a rootless container never gets.  This
one speaks just enough PPPoE and PPP from userspace, over a raw socket
in an isolated network namespace, for a client to bring up a session:

  - PPPoE discovery: PADI/PADO, PADR/PADS, and PADT both ways
  - LCP: configuration, echo, and termination
  - PAP or CHAP-MD5 authentication against one user name and password
  - IPCP: the client's address and primary DNS server
  - IPv4: answers ICMP echo requests sent to the server's address, and
    records the MSS of TCP SYNs, see Server.syn_mss()

Anything else in a session is rejected with an LCP Protocol-Reject.

Usage, in a test:

    with infamy.IsolatedMacVlan(port) as ns:
        with infamy.pppoe.Server(ns, user="user", password="secret"):
            ...

The server serves one session at a time, a new PADR replaces it.
"""
import argparse
import hashlib
import os
import signal
import socket
import subprocess
import sys
import tempfile

from scapy.layers.inet import IP, ICMP, TCP
from scapy.layers.l2 import Ether
from scapy.layers.ppp import PPPoED, PPPoED_Tags, PPPoETag, PPPoE, PPP, \
    PPP_LCP, PPP_LCP_Configure, PPP_LCP_Echo, PPP_LCP_Auth_Protocol_Option, \
    PPP_LCP_Magic_Number_Option, PPP_PAP_Request, PPP_PAP_Response, \
    PPP_CHAP, PPP_CHAP_ChallengeResponse, PPP_IPCP, PPP_IPCP_Option_IPAddress

ETH_P_PPPOE_DISC = 0x8863
ETH_P_PPPOE_SESS = 0x8864

PADI, PADO, PADR, PADS, PADT = 0x09, 0x07, 0x19, 0x65, 0xa7
TAG_SERVICE_NAME, TAG_AC_NAME, TAG_HOST_UNIQ = 0x0101, 0x0102, 0x0103

PROTO_IPV4, PROTO_IPCP = 0x0021, 0x8021
PROTO_LCP, PROTO_PAP, PROTO_CHAP = 0xc021, 0xc023, 0xc223
RESEND = 1                      # seconds between CHAP challenges

CONF_REQ, CONF_ACK, CONF_NAK, CONF_REJ = 1, 2, 3, 4
TERM_REQ, TERM_ACK, PROTO_REJ, ECHO_REQ, ECHO_REP = 5, 6, 8, 9, 10

IPCP_ADDR, IPCP_DNS1 = 3, 129


class Server:
    """Run the access concentrator in netns until stopped"""

    def __init__(self, netns, iface="iface", user="user", password="secret",
                 auth="pap", local="10.0.0.1", peer="10.0.0.100",
                 dns="192.0.2.53", ac_name="infamy"):
        self.netns = netns
        self.process = None
        fd, self.stats = tempfile.mkstemp(prefix="pppoe-ac-")
        os.close(fd)
        self.args = [sys.executable, os.path.abspath(__file__),
                     "--iface", iface, "--user", user, "--password", password,
                     "--auth", auth, "--local", local, "--peer", peer,
                     "--dns", dns, "--ac-name", ac_name, "--stats", self.stats]

    def syn_mss(self):
        """MSS of the latest TCP SYN received over the session, or None"""
        with open(self.stats, encoding="utf-8") as f:
            data = f.read().strip()
        return int(data) if data else None

    def __enter__(self):
        self.start()
        return self

    def __exit__(self, _, __, ___):
        self.stop()

    def start(self):
        self.process = self.netns.popen(self.args)

    def stop(self):
        """Stop the server, it sends PADT to end an open session"""
        if self.process:
            self.process.terminate()
            try:
                self.process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                self.process.kill()
                self.process.wait()
            self.process = None

    def __del__(self):
        try:
            os.unlink(self.stats)
        except OSError:
            pass


class AccessConcentrator:
    """PPPoE and PPP state for one session"""

    SESSION_ID = 0x1234
    MAGIC = 0x1a2b3c4d

    def __init__(self, args):
        self.args = args
        self.sock = socket.socket(socket.AF_PACKET, socket.SOCK_RAW,
                                  socket.htons(0x0003))
        self.sock.bind((args.iface, 0))
        self.mac = self.sock.getsockname()[4]
        self.ident = 0
        self.challenge = os.urandom(16)
        self.reset()

    def reset(self):
        self.client = None
        self.lcp_up = False
        self.authed = False
        self.ipcp_acked = False

    def log(self, msg):
        """Log as a TAP comment, the output ends up in the test's output"""
        print(f"# pppoe-ac: {msg}", file=sys.stderr, flush=True)

    def next_id(self):
        self.ident = (self.ident + 1) & 0xff
        return self.ident

    # Discovery stage

    def send_disc(self, dst, code, tags, sessionid=0):
        tag_list = [PPPoETag(tag_type=t, tag_value=v) for t, v in tags]
        pkt = Ether(dst=dst, src=self.mac, type=ETH_P_PPPOE_DISC) / \
            PPPoED(code=code, sessionid=sessionid) / \
            PPPoED_Tags(tag_list=tag_list)
        self.sock.send(bytes(pkt))

    def discovery(self, pkt):
        disc = pkt[PPPoED]
        tags = []
        if disc.haslayer(PPPoED_Tags):
            tags = [(t.tag_type, bytes(t.tag_value or b""))
                    for t in disc[PPPoED_Tags].tag_list]

        # Echo the client's service name and host-uniq, add our AC-Name
        reply = [(TAG_AC_NAME, self.args.ac_name.encode())]
        reply += [t for t in tags if t[0] in (TAG_SERVICE_NAME, TAG_HOST_UNIQ)]
        if not any(t[0] == TAG_SERVICE_NAME for t in reply):
            reply.append((TAG_SERVICE_NAME, b""))

        if disc.code == PADI:
            self.log(f"PADI from {pkt.src}, sending PADO")
            self.send_disc(pkt.src, PADO, reply)
        elif disc.code == PADR:
            self.log(f"PADR from {pkt.src}, session {self.SESSION_ID:#x} up")
            self.reset()
            self.client = pkt.src
            self.send_disc(pkt.src, PADS, reply, self.SESSION_ID)
            self.send_lcp_conf_req()
        elif disc.code == PADT and pkt.src == self.client:
            self.log("PADT from client, session down")
            self.reset()

    def padt(self):
        if self.client:
            self.send_disc(self.client, PADT, [], self.SESSION_ID)
            self.reset()

    # Session stage

    def send_ppp(self, proto, payload):
        pkt = Ether(dst=self.client, src=self.mac, type=ETH_P_PPPOE_SESS) / \
            PPPoE(sessionid=self.SESSION_ID) / PPP(proto=proto) / payload
        self.sock.send(bytes(pkt))

    def send_lcp_conf_req(self):
        if self.args.auth == "chap":
            auth = PPP_LCP_Auth_Protocol_Option(auth_protocol=PROTO_CHAP,
                                                algorithm=5)
        else:
            auth = PPP_LCP_Auth_Protocol_Option(auth_protocol=PROTO_PAP)

        opts = [auth, PPP_LCP_Magic_Number_Option(magic_number=self.MAGIC)]
        self.send_ppp(PROTO_LCP, PPP_LCP_Configure(code=CONF_REQ,
                                                   id=self.next_id(),
                                                   options=opts))

    def lcp(self, raw):
        code, ident = raw[0], raw[1]
        if code == CONF_REQ:
            # Accept whatever the client asks for, echo it back as an Ack
            ack = bytes([CONF_ACK]) + raw[1:]
            self.send_ppp(PROTO_LCP, PPP_LCP(ack))
            # Our request may have gone out before the client listened,
            # send it again until the client acknowledges it
            if not self.lcp_up:
                self.send_lcp_conf_req()
        elif code == CONF_ACK:
            if self.lcp_up:
                return
            self.lcp_up = True
            self.log(f"LCP up, authenticating with {self.args.auth.upper()}")
            if self.args.auth == "chap":
                self.chap_id = self.next_id()
                self.send_chap_challenge()
        elif code in (CONF_NAK, CONF_REJ):
            self.send_lcp_conf_req()
        elif code == ECHO_REQ:
            echo = PPP_LCP_Echo(code=ECHO_REP, id=ident,
                                magic_number=self.MAGIC, data=raw[8:])
            self.send_ppp(PROTO_LCP, echo)
        elif code == TERM_REQ:
            self.log("LCP Terminate-Request from client")
            self.send_ppp(PROTO_LCP, PPP_LCP(bytes([TERM_ACK, ident, 0, 4])))
            self.lcp_up = self.authed = False

    def auth_result(self, ok, proto, ident):
        # A client that missed our reply asks again, answer it again
        # without starting over
        repeat = ok and self.authed
        msg = b"Welcome" if ok else b"Go away"
        if proto == PROTO_PAP:
            pkt = PPP_PAP_Response(code=2 if ok else 3, id=ident,
                                   message=msg)
        else:
            pkt = PPP_CHAP(code=3 if ok else 4, id=ident, data=msg)
        self.send_ppp(proto, pkt)

        if repeat:
            return
        if ok:
            self.log("authenticated, starting IPCP")
            self.authed = True
            self.send_ipcp_conf_req()
        else:
            self.log("authentication failed, terminating")
            self.send_ppp(PROTO_LCP, bytes([TERM_REQ, self.next_id(), 0, 4]))

    def pap(self, raw):
        if raw[0] != 1:
            return
        req = PPP_PAP_Request(raw)
        user = bytes(req.username).decode(errors="replace")
        password = bytes(req.password).decode(errors="replace")
        ok = user == self.args.user and password == self.args.password
        if not self.authed:
            self.log(f"PAP from {user}: {'ok' if ok else 'wrong credentials'}")
        self.auth_result(ok, PROTO_PAP, req.id)

    def send_chap_challenge(self):
        self.send_ppp(PROTO_CHAP, PPP_CHAP_ChallengeResponse(
            code=1, id=self.chap_id, value=self.challenge,
            optional_name=self.args.ac_name.encode()))

    def chap(self, raw):
        if raw[0] != 2:
            return
        resp = PPP_CHAP_ChallengeResponse(raw)
        user = bytes(resp.optional_name).decode(errors="replace")
        want = hashlib.md5(bytes([resp.id]) + self.args.password.encode() +
                           self.challenge).digest()
        ok = user == self.args.user and bytes(resp.value) == want
        if not self.authed:
            self.log(f"CHAP from {user}: {'ok' if ok else 'wrong credentials'}")
        self.auth_result(ok, PROTO_CHAP, resp.id)

    def send_ipcp_conf_req(self):
        opts = [PPP_IPCP_Option_IPAddress(data=self.args.local)]
        self.send_ppp(PROTO_IPCP, PPP_IPCP(code=CONF_REQ, id=self.next_id(),
                                           options=opts))

    def ipcp(self, raw):
        if not self.authed:
            return

        code, ident = raw[0], raw[1]
        if code != CONF_REQ:
            if code == CONF_ACK and not self.ipcp_acked:
                self.ipcp_acked = True
                self.log("IPCP: our address acknowledged")
            return

        # Like for LCP, our request may have been sent too early
        if not self.ipcp_acked:
            self.send_ipcp_conf_req()

        want = {IPCP_ADDR: socket.inet_aton(self.args.peer),
                IPCP_DNS1: socket.inet_aton(self.args.dns)}
        reject, nak = b"", b""
        opts = raw[4:int.from_bytes(raw[2:4], "big")]
        while len(opts) >= 2:
            typ, length = opts[0], opts[1]
            if length < 2:
                break
            opt, opts = opts[:length], opts[length:]
            if typ not in want:
                reject += opt
            elif opt[2:] != want[typ]:
                nak += bytes([typ, 6]) + want[typ]

        if reject:
            reply = bytes([CONF_REJ, ident]) + (4 + len(reject)).to_bytes(2, "big") + reject
        elif nak:
            reply = bytes([CONF_NAK, ident]) + (4 + len(nak)).to_bytes(2, "big") + nak
        else:
            if self.ipcp_acked:
                self.log(f"IPCP up, client {self.args.peer}, DNS {self.args.dns}")
            reply = bytes([CONF_ACK]) + raw[1:]
        self.send_ppp(PROTO_IPCP, reply)

    def tcp_syn(self, tcp):
        for opt, val in tcp.options:
            if opt == "MSS":
                self.log(f"TCP SYN to port {tcp.dport}, MSS {val}")
                with open(self.args.stats, "w", encoding="utf-8") as f:
                    f.write(f"{val}\n")

    def ipv4(self, raw):
        ip = IP(raw)
        if ip.haslayer(TCP) and ip[TCP].flags.S:
            self.tcp_syn(ip[TCP])
            return
        if ip.dst != self.args.local or not ip.haslayer(ICMP) or ip[ICMP].type != 8:
            return
        icmp = ip[ICMP]
        reply = IP(src=ip.dst, dst=ip.src) / \
            ICMP(type=0, id=icmp.id, seq=icmp.seq) / bytes(icmp.payload)
        self.send_ppp(PROTO_IPV4, reply)

    def session(self, pkt):
        if pkt.src != self.client or pkt[PPPoE].sessionid != self.SESSION_ID:
            return

        raw = bytes(pkt[PPPoE].payload)[:pkt[PPPoE].len]
        if len(raw) < 2:
            return
        proto, data = int.from_bytes(raw[:2], "big"), raw[2:]

        if proto == PROTO_LCP:
            self.lcp(data)
        elif proto == PROTO_PAP:
            self.pap(data)
        elif proto == PROTO_CHAP:
            self.chap(data)
        elif proto == PROTO_IPCP:
            self.ipcp(data)
        elif proto == PROTO_IPV4:
            self.ipv4(data)
        elif self.lcp_up:
            rej = bytes([PROTO_REJ, self.next_id()]) + \
                (6 + len(data)).to_bytes(2, "big") + raw[:2] + data
            self.send_ppp(PROTO_LCP, rej)

    def run(self):
        self.log(f"serving on {self.args.iface}, {self.args.auth.upper()} "
                 f"user {self.args.user}")
        self.sock.settimeout(RESEND)
        while True:
            try:
                frame, addr = self.sock.recvfrom(2048)
            except socket.timeout:
                # The authenticator resends the CHAP challenge, the
                # client may not have been listening for the first one
                if self.lcp_up and not self.authed and self.args.auth == "chap":
                    self.send_chap_challenge()
                continue
            etype = int.from_bytes(frame[12:14], "big")
            if addr[2] == socket.PACKET_OUTGOING or \
               etype not in (ETH_P_PPPOE_DISC, ETH_P_PPPOE_SESS):
                continue
            pkt = Ether(frame)
            if pkt.type == ETH_P_PPPOE_DISC and pkt.haslayer(PPPoED):
                self.discovery(pkt)
            elif pkt.type == ETH_P_PPPOE_SESS and pkt.haslayer(PPPoE):
                self.session(pkt)


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--iface", required=True)
    parser.add_argument("--user", required=True)
    parser.add_argument("--password", required=True)
    parser.add_argument("--auth", choices=["pap", "chap"], required=True)
    parser.add_argument("--local", required=True)
    parser.add_argument("--peer", required=True)
    parser.add_argument("--dns", required=True)
    parser.add_argument("--ac-name", required=True)
    parser.add_argument("--stats", default=os.devnull,
                        help="File to write the MSS of received TCP SYNs to")
    ac = AccessConcentrator(parser.parse_args())

    def stop(*_):
        ac.padt()
        sys.exit(0)

    signal.signal(signal.SIGTERM, stop)
    signal.signal(signal.SIGINT, stop)
    ac.run()


if __name__ == "__main__":
    main()
