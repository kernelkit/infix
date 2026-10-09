"""Start NTP server in the background"""
import subprocess
from .netns import NetnsService


class Server(NetnsService):
    """BusyBox ntpd serving the local clock (-l), never touching it (-w)."""
    popen_kwargs = dict(stderr=subprocess.DEVNULL)

    def __init__(self, netns, iface="iface"):
        super().__init__(netns)
        self.iface = iface

    def argv(self):
        return ["ntpd", "-w", "-n", "-l", "-I", self.iface]

    def ready(self):
        # ntpd exits immediately on bad options or a missing interface,
        # fail loudly instead of serving nothing
        try:
            code = self.proc.wait(timeout=1)
        except subprocess.TimeoutExpired:
            return
        raise RuntimeError(f"ntpd failed to start (exit {code})")
