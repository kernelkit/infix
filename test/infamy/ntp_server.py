"""Start NTP server in the background"""
import subprocess

class Server:
    def __init__(self, netns, iface="iface"):
        self.iface = iface
        self.process = None
        self.netns = netns

    def __enter__(self):
        self.start()

    def __exit__(self, _, __, ___):
        self.stop()

    def start(self):
        cmd=f"ntpd -w -n -l -I {self.iface}"
        self.process = self.netns.popen(cmd.split(" "),stderr=subprocess.DEVNULL)

    def stop(self):
        if self.process:
            self.process.terminate()
            self.process.wait()
            self.process = None
