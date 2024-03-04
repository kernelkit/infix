import time
import subprocess
import signal
import sys

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
