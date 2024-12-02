import ctypes
import json
import multiprocessing
import os
import random
import subprocess
import tempfile
import time

from . import env

__libc = ctypes.CDLL(None)
CLONE_NEWUSER = 0x10000000
CLONE_NEWNET  = 0x40000000

# TODO: Replace me with os.setns once Python 3.12 is old news
def setns(fd, nstype):
    __NR_setns = 308
    __libc.syscall(__NR_setns, fd, nstype)

class IsolatedMacVlans:
    """A network namespace containing a multiple MACVLANs

    Stacks a MACVLAN on top of each specificed controller interface,
    and moves those interfaces to a separate namespace, isolating it
    from all others.

    NOTE: For the simple case when only one interface needs to be
    mapped, see IsolatedMacVlan below.

    Example:

    netns = IsolatedMacVlans({ "eth2": "a", "eth3": "b" })

             netns:
             .--------.
             | a    b | (MACVLANs)
             '-+----+-'
               |    |
    eth0 eth1 eth2 eth3

    """

    Instances = []
    def Cleanup():
        for ns in list(IsolatedMacVlans.Instances):
            ns.stop()

    def __init__(self, ifmap, lo=True):
        self.sleeper = None
        self.ifmap, self.lo = ifmap, lo
        self.ping_timeout = env.ENV.attr("ping_timeout", 5)

    def start(self):
        self.sleeper = subprocess.Popen(["unshare", "-r", "-n", "sh", "-c",
                                         "echo && exec sleep infinity"],
                                        stdout=subprocess.PIPE)
        self.sleeper.stdout.readline()

        try:
            for parent, ifname in self.ifmap.items():
                subprocess.run(["ip", "link", "add",
                                "dev", ifname,
                                "link", parent,
                                "address", self._stable_mac(parent),
                                "netns", str(self.sleeper.pid),
                                "type", "macvlan", "mode", "passthru"], check=True)
                self.runsh(f"""
                while ! ip link show dev {ifname}; do
                    sleep 0.1
                done
                """)
        except Exception as e:
            self.__exit__(None, None, None)
            raise e

        if self.lo:
            try:
                self.run(["ip", "link", "set", "dev", "lo", "up"])
            except Exception as e:
                self.__exit__(None, None, None)
                raise e

        self.Instances.append(self)
        return self

    def stop(self):
        self.sleeper.kill()
        self.sleeper.wait()

        for _ in range(100):
            promisc = False
            for parent in self.ifmap.keys():
                iplink = subprocess.run(f"ip -d -j link show dev {parent}".split(),
                                        stdout=subprocess.PIPE, check=True)
                link = json.loads(iplink.stdout)[0]
                if link["promiscuity"]:
                    # Use promisc as a substitute for an indicator
                    # of whether the kernel has actually removed
                    # the passthru MACVLAN yet or not
                    promisc = True
                    break

            if not promisc:
                break

            time.sleep(.1)
        else:
            raise TimeoutError("Lingering MACVLAN")

        if self in self.Instances:
            self.Instances.remove(self)

    def __enter__(self):
        return self.start()

    def __exit__(self, val, typ, tb):
        return self.stop()

    def _stable_mac(self, parent):
        """Generate address for MACVLAN

        By default, the kernel will assign a random address. This
        causes issues when the parent interface is an Intel X710 NIC,
        which will add those addresses to its FDB, but never remove
        them when the interface is deleted.

        Work around the issue by generating an address that is pseudo
        random, to avoid address conflicts; yet stable across
        instantiations for any given parent interface name, to avoid
        the resource exhaustion issue.

        """
        random.seed(parent)
        a = list(random.randbytes(6))
        a[0] |= 0x02
        a[0] &= ~0x01
        return \
            f"{a[0]:02x}:{a[1]:02x}:{a[2]:02x}:" + \
            f"{a[3]:02x}:{a[4]:02x}:{a[5]:02x}"

    def _ns_call(self, fn, tx):
        pid = self.sleeper.pid

        uns = os.open(f"/proc/{pid}/ns/user", os.O_RDONLY)
        setns(uns, CLONE_NEWUSER)
        os.close(uns)

        nns = os.open(f"/proc/{pid}/ns/net", os.O_RDONLY)
        setns(nns, CLONE_NEWNET)
        os.close(nns)

        tx.send(fn())
        tx.close()

    def call(self, fn):
        rx, tx = multiprocessing.Pipe(duplex=False)

        proc = multiprocessing.Process(target=self._ns_call, args=(fn, tx))
        proc.start()
        ret = rx.recv()
        rx.close()
        proc.join()
        return ret

    def _mangle_subprocess_args(self, args, kwargs):
        if args:
            args = list(args)
            if type(args[0]) == str:
                if "shell" in kwargs and kwargs["shell"]:
                    args[0] = ["/bin/sh", "-c", args[0]]
                    kwargs["shell"] = False
                else:
                    args[0] = [args[0]]

            if type(args[0]) == list:
                args[0] = ["nsenter", "-t", str(self.sleeper.pid),
                           "-n", "-U", "--preserve-credentials"] + args[0]
                return args, kwargs

        raise ValueError("Unable mangle subprocess arguments")

    def run(self, *args, **kwargs):
        args, kwargs = self._mangle_subprocess_args(args, kwargs)
        return subprocess.run(*args, **kwargs)

    def popen(self, *args, **kwargs):
        args, kwargs = self._mangle_subprocess_args(args, kwargs)
        return subprocess.Popen(*args, **kwargs)

    def runsh(self, script, *args, **kwargs):
        return self.run("/bin/sh", text=True, input=script,
                        stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                        *args, **kwargs)

    def addroute(self, subnet, nexthop, proto="ipv4", prefix_length=""):
        p = proto[3]
        if prefix_length:
            prefix_length = f"/{prefix_length}"

        self.runsh(f"""
            set -ex
            ip -{p} route add {subnet}{prefix_length} via {nexthop}
            """, check=True)

    def addip(self, ifname, addr, prefix_length=24, proto="ipv4"):
        p = proto[3]

        self.runsh(f"""
            set -ex
            ip link set dev {ifname} up
            ip -{p} addr add {addr}/{prefix_length} dev {ifname}
            """, check=True)

    def traceroute(self, addr):
        res = self.runsh(f"""
        set -ex
        traceroute -n {addr}
        """, check=True)
        result = []
        for line in res.stdout.splitlines()[2:]:
            l = line.split()
            result.append(l)
        return result

    def ping(self, daddr, id=None, timeout=None):
        timeout = timeout if timeout else self.ping_timeout
        id = f"-e {id}" if id else ""

        ping = f"ping -c1 -w1 {id} {daddr}"

        return self.run(["timeout", str(timeout), "/bin/sh"], text=True, check=True,
                        stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                        input=f"while :; do {ping} && break; done")

    def must_reach(self, *args, **kwargs):
        self.ping(*args, **kwargs)

    def must_not_reach(self, *args, **kwargs):
        try:
            res = self.ping(*args, **kwargs)
        except subprocess.CalledProcessError as e:
            return

        raise Exception(res)

    def must_receive(self, expr, ifname, timeout=None, must=True):
        timeout = timeout if timeout else self.ping_timeout

        tshark = self.run(["tshark", "-nl", f"-i{ifname}",
                           f"-aduration:{timeout}", "-c1", expr],
                          stdin=subprocess.DEVNULL,
                          stdout=subprocess.PIPE,
                          stderr=subprocess.STDOUT,
                          text=True, check=True)

        needle = "1 packet captured" if must else "0 packets captured"

        if needle not in tshark.stdout:
            raise Exception(tshark)

    def must_not_receive(self, *args, **kwargs):
        self.must_receive(*args, **kwargs, must=False)

    def pcap(self, expr, ifname):
        return Pcap(self, ifname, expr)

class IsolatedMacVlan(IsolatedMacVlans):
    """A network namespace containing a single MACVLAN

    Stacks a MACVLAN on top of an interface on the controller, and
    moves that interface to a separate namespace, isolating it from
    all other interfaces.

    Example:

    netns = IsolatedMacVlan("eth3")

                netns:
                .-------.
                | iface | (MACVLAN)
                '---+---'
                    |
    eth0 eth1 eth2 eth3

    """
    def __init__(self, parent, ifname="iface", lo=True):
        self._ifname = ifname
        return super().__init__(ifmap={ parent: ifname }, lo=lo)

    def addip(self, addr, prefix_length=24, proto="ipv4"):
        return super().addip(ifname=self._ifname, addr=addr, prefix_length=prefix_length, proto=proto)

    def must_receive(self, expr, timeout=None, ifname=None, must=True):
        ifname = ifname if ifname else self._ifname
        return super().must_receive(expr=expr, ifname=ifname, timeout=timeout, must=must)

    def pcap(self, expr, ifname=None):
        ifname = ifname if ifname else self._ifname
        return super().pcap(expr=expr, ifname=ifname)

class Pcap:
    def __init__(self, netns, ifname, expr):
        self.netns, self.ifname, self.expr = netns, ifname, expr
        self.pcap = tempfile.NamedTemporaryFile(suffix=".pcap", delete=False)
        self.proc = None

    def __del__(self):
        self.pcap.close()
        os.unlink(self.pcap.name)

    def __enter__(self):
        self.start()
        return self

    def __exit__(self, _, __, ___):
        self.stop()

    def start(self):
        assert self.proc == None, "Can't start an already running Pcap"

        argv = f"tshark -ln -i {self.ifname} -w {self.pcap.name} {self.expr}".split()
        self.proc = self.netns.popen(argv,
                                     stdin=subprocess.DEVNULL,
                                     stdout=subprocess.DEVNULL,
                                     stderr=subprocess.PIPE,
                                     text=True)

        while " -- Capture started." not in self.proc.stderr.readline():
            pass

        print("Capture running")

    def stop(self, sleep=3):
        assert self.proc, "Can't stop an already stopped Pcap"

        if sleep:
            # In the common case, stop() will be called right after
            # the final packet of whatever we're testing has just been
            # sent. Therefore, allow for some time to pass before
            # terminating the capture.
            time.sleep(sleep)

        self.proc.terminate()
        try:
            _, stderr = self.proc.communicate(5)
            print(stderr)
            return
        except subprocess.TimeoutExpired:
            try:
                self.proc.kill()
            except OSError:
                pass

        self.proc.wait()

    def tcpdump(self, args=""):
        tcpdump = subprocess.run((f"tcpdump -r {self.pcap.name} -n " + args).split(),
                                 stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                                 text=True, check=True)
        return tcpdump.stdout

class TPMR(IsolatedMacVlans):
    """Two-Port MAC Relay

    Creates a network namespace containing two controller interfaces
    (`a` and `b`). By default, tc rules are setup to copy all frames
    ingressing on `a` to egress on `b`, and vice versa.

    These rules can be removed and reinserted dynamically using the
    `block()` and `forward()` methods, respectively.

    This is useful to verify the correctness of fail-over behavior in
    various protocols. See ospf_bfd for a usage example.
    """

    def __init__(self, a, b):
        super().__init__(ifmap={ a: "a", b: "b" }, lo=False)

    def start(self, forward=True):
        ret = super().start()

        for dev in ("a", "b"):
            self.run(f"ip link set dev {dev} promisc on up".split())
            self.run(f"tc qdisc add dev {dev} clsact".split())

        if forward:
            self.forward()

        return ret

    def _clear_ingress(self, iface):
        return self.run(f"tc filter del dev {iface} ingress".split())

    def _add_redir(self, frm, to):
        cmd = \
            "tc filter add dev".split() \
            + [frm] \
            + "ingress matchall action mirred egress redirect dev".split() \
            + [to]
        return self.run(cmd)

    def forward(self):
        for iface in ("a", "b"):
            self._clear_ingress(iface)
            self._add_redir(iface, "a" if iface == "b" else "b")

    def block(self):
        for iface in ("a", "b"):
            self._clear_ingress(iface)
