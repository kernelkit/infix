import subprocess

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

    def addroute(self, subnet, nexthop, prefix_length=""):
        if prefix_length:
            prefix_length=f"/{prefix_length}"

        self.runsh(f"""
            set -ex
            ip route add {subnet}{prefix_length} via {nexthop}
            """, check=True)

    def addip(self, addr, prefix_length=24):
        self.runsh(f"""
            set -ex 
            ip link set iface up
            ip addr add {addr}/{prefix_length} dev iface
            """, check=True)

    def ping(self, daddr, count=1, timeout=2, check=False):
        return self.runsh(f"""set -ex; ping -c {count} -w {timeout} {daddr}""", check=check)

    def must_reach(self, daddr):
        res = self.ping(daddr)
        if res.returncode != 0:
            raise Exception(res.stdout)

    def must_not_reach(self, daddr):
        res = self.ping(daddr)
        if res.returncode == 0:
            raise Exception(res.stdout)
