"""SSDP client implementation"""

import threading

class SsdpClient:
    """SSDP device scanner sends an UPnP M-Search message in the background"""
    def __init__(self, netns, retries=5):
        self.netns = netns
        self.retries = retries
        self.irq = False
        self.thread = threading.Thread(target=self.scan)

    def scan(self):
        """Send SSDP M-SEARCH * message to scan for devices"""
        for _ in range(self.retries):
            if self.irq:
                return
            #print("Sending M-SEARCH * ...")
            rc = self.netns.runsh(f"""
                set -ex
                /bin/echo -ne "M-SEARCH * HTTP/1.1\r\n"        \
                              "Host: 239.255.255.250:1900\r\n" \
                              "Man: \"ssdp:discover\"\r\n"     \
                              "MX: 1\r\nST: ssdp:all\r\n\r\n"  \
                         | nc -w1 -p 1234 -u 239.255.255.250 1900
                """)
            if rc.returncode:
                print(f"SSDP fail {rc.returncode}: {rc.stdout}")
                return

    def start(self):
        """Start SSDP scanner"""
        self.irq = False
        self.thread.start()

    def stop(self):
        """Stop SSDP scanner"""
        self.irq = True
        self.thread.join()
