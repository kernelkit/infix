import ctypes
import multiprocessing
import subprocess
import os

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

    def __enter__(self):
        self.sleeper = subprocess.Popen(["unshare", "-r", "-n", "sh", "-c",
                                         "echo && exec sleep infinity"],
                                        stdout=subprocess.PIPE)
        self.sleeper.stdout.readline()

        try:
            subprocess.run(["ip", "link", "add",
                            "dev", self.ifname,
                            "link", self.parent,
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

    def __ns_call(self, fn, tx):
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

        proc = multiprocessing.Process(target=self.__ns_call, args=(fn, tx))
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

    def ping(self, daddr, count=1, timeout=5, interval=2, check=False):
        return self.runsh(f"""set -ex; ping -c {count} -w {timeout} -i {interval} {daddr}""", check=check)

    def must_reach(self, *args, **kwargs):
        res = self.ping(*args, **kwargs)
        if res.returncode != 0:
            raise Exception(res.stdout)

    def must_not_reach(self, *args, **kwargs):
        res = self.ping(*args, **kwargs)
        if res.returncode == 0:
            raise Exception(res.stdout)
