import subprocess

class IsolatedMacVlan:
    def __init__(self, parent, ifname="iface", lo=True):
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

    def runsh(self, script):
        return self.run("/bin/sh", text=True, input=script,
                        stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
