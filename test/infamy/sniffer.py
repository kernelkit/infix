"""Sniff for network packets using tcpdump/tshark"""
import os
import signal
import subprocess
import tempfile
import time

class Sniffer:
    """Helper class for tcpdump"""
    def __init__(self, netns, expr):
        self.pcap = tempfile.NamedTemporaryFile(suffix=".pcap", delete=False)
        self.expr = expr
        self.netns = netns
        self.proc = None

    def __del__(self):
        self.pcap.close()
        os.unlink(self.pcap.name)

    def __enter__(self):
        cmd = f"tshark -lni iface -w {self.pcap.name} {self.expr}"
        arg = cmd.split(" ")
        self.proc = self.netns.popen(arg, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)

    def __exit__(self, _, __, ___):
        if not self.proc:
            return False

        self.proc.send_signal(signal.SIGINT)
        time.sleep(1)
        if not self.proc.poll():
            try:
                self.proc.kill()
            except OSError:
                pass
        self.proc.wait()

    def output(self):
        """Return PCAP output"""
        return self.netns.runsh(f"tcpdump -n -r {self.pcap.name}")

    def packets(self):
        """Filtered text output, skipping tcpdump "reading from file" initial line"""
        lines = self.output().stdout.split('\n')
        return '\n'.join(lines[1:])
