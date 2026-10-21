import signal
import subprocess
from .netns import NetnsService

class _MCastService(NetnsService):
    """msend, mreceive and the scapy sender all exit on SIGINT"""
    stop_signal = signal.SIGINT
    # Nothing reads the output.  A pipe would fill up and block the
    # process, so discard it.
    popen_kwargs = dict(stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)

    def __init__(self, netns, group):
        super().__init__(netns)
        self.group = group

class _IPMCastService(_MCastService):
    """msend and mreceive need -6 for an IPv6 group"""
    def __init__(self, netns, group):
        super().__init__(netns, group)
        self.family = "-6" if ":" in group else "-4"

class MCastSender(_IPMCastService):
    def argv(self):
        return f"msend {self.family} -I iface -g {self.group}".split(" ")

class MCastReceiver(_IPMCastService):
    def argv(self):
        return f"mreceive {self.family} -I iface -g {self.group}".split(" ")

class MacMCastSender(_MCastService):
    def argv(self):
        send_cmd = (
            "from scapy.all import sendp, Ether; "
            f"pkt=Ether(src='aa:bb:cc:dd:ee:ff', dst='{self.group}', type=0xdead); "
            "sendp(pkt, iface='iface', loop=1, inter=1./10)"
        )
        return ["python3", "-c", send_cmd]
