"""
Port scanning utilities

Lightweight wrapper around nmap for automated firewall testing.
Supports:

- Scanning individual TCP/UDP ports with detailed state detection
- Parallel scanning of multiple ports using threads
- Predefined set of common service ports for convenience
"""
import threading
from typing import List, Dict, Union, Tuple


class PortScanner:
    """Simple port scanner using netcat for firewall testing"""

    # Well-known ports for testing common services
    WELL_KNOWN_PORTS = [
        (22,   "tcp", "ssh"),
        (53,   "udp", "dns"),
        (67,   "udp", "dhcp"),
        (69,   "udp", "tftp"),
        (80,   "tcp", "http"),
        (443,  "tcp", "https"),
        (5353, "udp", "mdns"),
        (7681, "tcp", "ttyd"),
        (8080, "tcp", "http-alt"),
        (8443, "tcp", "https-alt"),
        (7,    "tcp", "echo"),
        (1234, "tcp", "test-mid"),
        (9999, "tcp", "test-high"),
    ]

    def __init__(self, netns=None):
        """
        Initialize port scanner
        Args:
            netns: Network namespace object (IsolatedMacVlan) or netns name
        """
        self.netns = netns

    def scan_port(self, host: str, port: int, protocol: str = "tcp",
                  timeout: int = 3) -> Dict[str, Union[str, bool]]:
        """
        Scan a single port using nmap for accurate firewall state detection
        Args:
            host:     Target hostname or IP address
            port:     Port number to scan
            protocol: 'tcp' or 'udp'
            timeout:  Connection timeout in seconds
        Returns:
            Dict with keys: 'open' (bool), 'status' (str), 'response' (str)
        """
        # Build optimized nmap command based on protocol
        if protocol.lower() == "tcp":
            proto = "-sT"
        elif protocol.lower() == "udp":
            proto = "-sU"
        else:
            raise ValueError(f"Unsupported protocol: {protocol}")

        cmd = f"nmap -n {proto} -Pn -p {port} --host-timeout={timeout} " \
              f"--min-rate=1000 --max-retries=1 --disable-arp-ping {host}"
        result = self.netns.runsh(cmd)

        # Parse nmap output to determine port state
        output = result.stdout

        # Look for the specific port line in nmap output
        # Format: "PORT   STATE SERVICE" or "22/tcp open  ssh"
        port_line = None
        for line in output.split('\n'):
            if f"{port}/" in line and protocol in line:
                port_line = line.strip()
                break

        if port_line:
            if "open|filtered" in port_line:
                # No ICMP port unreachable, likely firewall dropping silently
                status = "open|filtered"
                is_open = False
            elif "closed|filtered" in port_line:
                # No service running, or firewall dropping
                status = "closed|filtered"
                is_open = False
            elif "open" in port_line:
                status = "open"
                is_open = True
            elif "filtered" in port_line:
                # Blocked/Rejected by firewall
                status = "filtered"
                is_open = False
            elif "closed" in port_line:
                # Port reachable but service not running
                status = "closed"
                is_open = False
            else:
                # Unknown state
                status = "unknown"
                is_open = False
        else:
            # No port line found - likely filtered or error
            status = "filtered"
            is_open = False

        return {
            "open": is_open,
            "status": status,
            "response": output.strip()
        }

    def scan_ports(self, host: str,
                   port_specs: List[Tuple[int, str, str]],
                   timeout: int = 3) -> List[Tuple[int, str, Dict]]:
        """
        Scan multiple ports in parallel using threads
        Args:
            host: Target hostname or IP address
            port_specs: List (port, protocol, name) e.g.
                        [(80, "tcp", "http"), (53, "udp", "dns")]
            timeout: Connection timeout per port
        Returns:
            List of (port, name, result) scan results.
        """
        results = []
        threads = []
        lock = threading.Lock()

        def scan_worker(port: int, protocol: str, name: str):
            try:
                result = self.scan_port(host, port, protocol, timeout)
                with lock:
                    results.append((port, name, result))
            except Exception as e:
                with lock:
                    results.append((port, name, {
                        "open": False,
                        "status": "error",
                        "response": str(e)
                    }))

        for port, protocol, name in port_specs:
            thread = threading.Thread(target=scan_worker, args=(port, protocol, name))
            threads.append(thread)
            thread.start()

        for thread in threads:
            thread.join()

        # Sort results by port number for consistent output
        results.sort(key=lambda x: x[0])
        return results
