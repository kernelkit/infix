"""Start NTP server in the background"""
import subprocess
import time


class Server:
    """BusyBox ntpd serving the local clock (-l), never touching it (-w)."""

    def __init__(self, netns, iface="iface"):
        self.iface = iface
        self.process = None
        self.netns = netns

    def __enter__(self):
        self.start()
        return self

    def __exit__(self, _, __, ___):
        self.stop()

    def start(self):
        cmd = ["ntpd", "-w", "-n", "-l", "-I", self.iface]
        self.process = self.netns.popen(cmd, stderr=subprocess.DEVNULL)

        # ntpd exits immediately on bad options or a missing interface;
        # fail loudly instead of serving nothing
        time.sleep(1)
        if self.process.poll() is not None:
            code = self.process.returncode
            self.stop()
            raise RuntimeError(f"ntpd failed to start (exit {code})")

    def stop(self):
        if self.process:
            self.process.terminate()
            self.process.wait()
            self.process = None
