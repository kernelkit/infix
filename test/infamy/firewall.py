"""
Firewall testing utilities

Provides a helper class and supporting tools to validate firewall
behavior in automated tests. Supports:

- SNAT verification by inspecting captured ICMP traffic
- Zone policy checks using targeted port scans
- Positive and negative policy validation (allowed vs. blocked ports)
"""
import time
from typing import Tuple, List
from .sniffer import Sniffer
from .portscanner import PortScanner
from .util import until


class Firewall:
    """Specialized utilities for testing firewall functionality"""

    def __init__(self, source=None, dest=None):
        """
        Initialize firewall tester
        Args:
            source: Source network namespace (for traffic generation)
            dest:   Destination network namespace (for traffic capture)
        """
        self.srcns = source
        self.dstns = dest

    @staticmethod
    def wait_for_operational(target, expected_zones, timeout=30):
        """Wait for firewall config to be activated/available in operational"""
        def check_operational():
            try:
                oper = target.get_data("/infix-firewall:firewall")
                if not oper or "firewall" not in oper:
                    return False

                if "zone" not in oper["firewall"]:
                    return False

                zones = {z["name"]: z for z in oper["firewall"]["zone"]}

                for zone_name, expected in expected_zones.items():
                    if zone_name not in zones:
                        return False
                    for key, value in expected.items():
                        if zones[zone_name].get(key) != value:
                            return False
                return True
            except:
                return False

        until(check_operational, attempts=timeout)

    def verify_snat(self, dest_ip: str, snat_ip: str,
                    timeout: int = 3) -> Tuple[bool, str]:
        """
        Verify SNAT (masquerading) by analyzing source IP of ICMP traffic

        Args:
            dest_ip: Destination IP address to ping
            snat_ip: Expected source IP after SNAT (router's WAN IP)
            timeout: Test timeout in seconds

        Returns:
            Tuple of (snat_working: bool, details: str)
        """

        try:
            sniffer = Sniffer(self.dstns, "icmp")
            with sniffer:
                time.sleep(0.5)
                self.srcns.runsh(f"ping -c3 -W{timeout} {dest_ip}")
                time.sleep(0.5)

            rc = sniffer.output()
            packets = rc.stdout
            if rc.returncode or not packets.strip():
                return False, "No packets captured — routing may be broken"

            lines = packets.strip().split('\n')
            snat_ip_found = False
            lan_ip_found = False

            for line in lines:
                if not line.strip():
                    continue

                # Check if we see the expected SNAT IP as source
                if f"{snat_ip} > {dest_ip}" in line:
                    snat_ip_found = True

                # Check if we see any other source IP (SNAT not working)
                if f"> {dest_ip}" in line and snat_ip not in line:
                    parts = line.split()
                    for part in parts:
                        if f"> {dest_ip}" in part:
                            src_ip = part.split('>')[0].strip()
                            if '.' in src_ip and src_ip != snat_ip:
                                lan_ip_found = True
                                break

            if snat_ip_found and not lan_ip_found:
                return True, f"SNAT working: only traffic from {snat_ip}"
            if lan_ip_found and not snat_ip_found:
                return False, f"SNAT broken: LAN IPs visible, no {snat_ip}"
            if snat_ip_found and lan_ip_found:
                return False, f"SNAT broken: both {snat_ip} and LAN IPs on WAN"

            return False, f"Unclear SNAT status, see capture:\n{packets}"

        except Exception as e:
            return False, f"SNAT verification failed with error: {e}"

    def verify_blocked(self, dest_ip: str, ports: List[Tuple[int, str, str]] = None,
                       exempt: List[int] = None, timeout: int = 3) -> Tuple[bool, List[str], List[str]]:
        """
        Verify specified ports are blocked, with optional exceptions
        Args:
            dest_ip:  Target hostname or IP address
            ports:    List of port tuples, defaults to
                      PortScanner.WELL_KNOWN_PORTS
            exempt:   List of ports that should be excempt
            timeout:  Connection timeout per port
        Returns:
            When exempt=None:  Tuple of (all_blocked: bool, open_ports: List[str], [])
            When exempt=[...]: Tuple of (policy_correct: bool, unexpected_open: List[str],
                                         unexpected_filtered_allowed: List[str])
        """
        if ports is None:
            ports = PortScanner.WELL_KNOWN_PORTS

        scanner = PortScanner(self.srcns)
        results = scanner.scan_ports(dest_ip, ports, timeout)

        if exempt is None:
            # Simple "all blocked" behavior - only "open" is bad
            open_ports = []
            for port, name, result in results:
                if result["status"] == "open":
                    open_ports.append(f"{name}({port})")
            return len(open_ports) == 0, open_ports, []

        unexpected_open = []
        unexpected_filtered_allowed = []

        for port, name, result in results:
            if port in exempt:
                # This port should be allowed (not filtered by firewall)
                status = result["status"]
                if status in ["filtered", "open|filtered", "closed|filtered"]:
                    unexpected_filtered_allowed.append(f"{name}({port})")
            else:
                # This port should be blocked - only "open" is bad
                if result["status"] == "open":
                    unexpected_open.append(f"{name}({port})")

        policy_correct = (len(unexpected_open) == 0 and
                          len(unexpected_filtered_allowed) == 0)
        return policy_correct, unexpected_open, unexpected_filtered_allowed

    def verify_allowed(self, dest_ip: str, ports: List[Tuple[int, str, str]] = None,
                       timeout: int = 3) -> Tuple[bool, List[str]]:
        """
        Verify specified ports are allowed (open or closed, not filtered)
        Args:
            dest_ip: Target hostname or IP address
            ports:   List of port tuples, defaults to
                     PortScanner.WELL_KNOWN_PORTS
            timeout: Connection timeout per port
        Returns:
            Tuple of (all_allowed: bool, filtered_ports: List[str])
        """
        if ports is None:
            ports = PortScanner.WELL_KNOWN_PORTS

        scanner = PortScanner(self.srcns)
        results = scanner.scan_ports(dest_ip, ports, timeout)
        filtered_ports = []

        for port, name, result in results:
            status = result["status"]
            # Consider any form of filtering as "not allowed"
            if status in ["filtered", "open|filtered", "closed|filtered"]:
                filtered_ports.append(f"{name}({port})")

        return len(filtered_ports) == 0, filtered_ports
