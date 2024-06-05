import ctypes
import multiprocessing
import os
import random
import subprocess
import time

from . import env

__libc = ctypes.CDLL(None)
CLONE_NEWUSER = 0x10000000
CLONE_NEWNET  = 0x40000000

# TODO: Replace me with os.setns once Python 3.12 is old news
def setns(fd, nstype):
    __NR_setns = 308
    __libc.syscall(__NR_setns, fd, nstype)

class IsolatedMacVlan:
    """Create an isolated interface on top of a PC interface."""
    def __init__(self, parent, ifname="iface", lo=True):
        self.sleeper = None
        self.parent, self.ifname, self.lo = parent, ifname, lo
        self.ping_timeout = env.ENV.attr("ping_timeout", 5)

    def __enter__(self):
        self.sleeper = subprocess.Popen(["unshare", "-r", "-n", "sh", "-c",
                                         "echo && exec sleep infinity"],
                                        stdout=subprocess.PIPE)
        self.sleeper.stdout.readline()

        try:
            subprocess.run(["ip", "link", "add",
                            "dev", self.ifname,
                            "link", self.parent,
                            "address", self._stable_mac(),
                            "netns", str(self.sleeper.pid),
                            "type", "macvlan"], check=True)
            self.runsh(f"""
            while ! ip link show dev {self.ifname}; do
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

        return self

    def __exit__(self, val, typ, tb):
        self.sleeper.kill()
        self.sleeper.wait()
        time.sleep(0.5)

    def _stable_mac(self):
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
        random.seed(self.parent)
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
                        stdout=subprocess.PIPE, stderr=subprocess.STDOUT, *args, **kwargs)

    def addroute(self, subnet, nexthop, proto="ipv4", prefix_length=""):
        p=proto[3]
        if prefix_length:
            prefix_length=f"/{prefix_length}"

        self.runsh(f"""
            set -ex
            ip -{p} route add {subnet}{prefix_length} via {nexthop}
            """, check=True)

    def addip(self, addr, prefix_length=24, proto="ipv4"):
        p=proto[3]

        self.runsh(f"""
            set -ex
            ip link set iface up
            ip -{p} addr add {addr}/{prefix_length} dev iface
            """, check=True)


    def traceroute(self, addr):
        res=self.runsh(f"""
        set -ex
        traceroute -n {addr}
        """, check=True)
        result=[]
        for line in res.stdout.splitlines()[2:]:
            l=line.split()
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

    def must_receive(self, expr, timeout=None, ifname=None, must=True):
        ifname = ifname if ifname else self.ifname
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
