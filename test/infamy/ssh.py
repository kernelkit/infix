import subprocess

from dataclasses import dataclass

@dataclass
class Location:
    host: str
    password: str
    username: str = "admin"
    port: int = 22

class Device(object):
    def __init__(self, location: Location):
        self.location = location

    def _mangle_subprocess_args(self, args, kwargs):
        if not args:
            return

        args = list(args)
        if type(args[0]) == str:
            if kwargs.get("shell"):
                args[0] = ["/bin/sh", "-c", args[0]]
                kwargs["shell"] = False
            else:
                args[0] = [args[0]]

        args[0] = [ "ssh",
                    "-oStrictHostKeyChecking no",
                    "-oUserKnownHostsFile /dev/null",
	            "-oLogLevel QUIET",
                    f"-l{self.location.username}",
                    self.location.host ] + args[0]

        if self.location.password:
            args[0] = [ "sshpass" , f"-p{self.location.password}" ] + args[0]

        return args, kwargs

    def run(self, *args, **kwargs):
        args, kwargs = self._mangle_subprocess_args(args, kwargs)
        return subprocess.run(*args, **kwargs)

    def runsh(self, script, *args, **kwargs):
        return self.run("/bin/sh", text=True, input=script,
                        stdout=subprocess.PIPE, stderr=subprocess.STDOUT, *args, **kwargs)
