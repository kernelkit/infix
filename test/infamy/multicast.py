import time
import subprocess
import signal
import sys
from scapy.all import Ether, sendp

class MCastSender:
    def __init__(self, netns,group):
        self.group = group
        self.netns = netns

    def __enter__(self):
        cmd = f"msend -I iface -g {self.group}"
        arg = cmd.split(" ")

        self.proc = self.netns.popen(arg, stdout=subprocess.PIPE, stderr=subprocess.PIPE)

    def __exit__(self, _, __, ___):
        if not self.proc:
            return False

        sys.stdout.flush()
        self.proc.send_signal(signal.SIGINT)
        time.sleep(1)
        if not self.proc.poll():
            try:
                self.proc.kill()
            except OSError:
                pass
        self.proc.wait()

class MCastReceiver:
    def __init__(self, netns, group):
        self.group = group
        self.netns = netns

    def __enter__(self):
        cmd = f"mreceive -I iface -g {self.group}"
        arg = cmd.split(" ")

        self.proc = self.netns.popen(arg, stdout=subprocess.PIPE, stderr=subprocess.PIPE)

    def __exit__(self, _, __, ___):
        if not self.proc:
            return False

        sys.stdout.flush()
        self.proc.send_signal(signal.SIGINT)
        time.sleep(3)
        if not self.proc.poll():
            print("PROC")
            try:
                self.proc.kill()
            except OSError:
                print("ERR")
                pass
        self.proc.wait()
        
class MacMCastSender:
    def __init__(self, netns, group):
        self.group = group
        self.netns = netns

    def __enter__(self):
        send_cmd = (
            "from scapy.all import sendp, Ether; "
            f"pkt=Ether(src='aa:bb:cc:dd:ee:ff', dst='{self.group}', type=0xdead); "
            # loop until SIGINT (set on __exit__), like the IPv4 MCastSender's
            # msend; a fixed count would drain before later verification steps
            # run and they would capture nothing.
            "sendp(pkt, iface='iface', inter=1./10, loop=1)"
        )
        self.proc = self.netns.popen(["python3", "-c", send_cmd], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        return self  

    def __exit__(self, _, __, ___):
        if self.proc:
            sys.stdout.flush()
            self.proc.send_signal(signal.SIGINT)
            time.sleep(1)
            if not self.proc.poll():
                try:
                    self.proc.kill()
                except OSError:
                    pass
            self.proc.wait()
