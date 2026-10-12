"""Sniff for network packets using tcpdump/tshark"""
import signal
from .netns import Pcap

class Sniffer(Pcap):
    """Capture on the namespace's "iface", read back with tcpdump"""
    stop_signal = signal.SIGINT
    stop_delay = 0

    def __init__(self, netns, expr):
        super().__init__(netns, "iface", expr)

    def output(self):
        """Return PCAP output"""
        return self.netns.runsh(f"tcpdump -n -r {self.pcap.name}")

    def packets(self):
        """Filtered text output, skipping tcpdump "reading from file" initial line"""
        lines = self.output().stdout.split('\n')
        return '\n'.join(lines[1:])
