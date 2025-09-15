#!/usr/bin/env python3
"""mDNS allow/deny interfaces

Verify the mDNS responder interface allow/deny configuration.  Both
settings can be used independently and in concert.  We verify operation
with three scenarios:

 1. Allow p2, no mDNS traffic should be received on p1 and p3
 2. Deny p2, mDNS traffic should only be received on p1 and p3
 3. Allow p1 and p3, deny p2 and p3, traffic only on p1

"""
import re
import infamy


def mdns_scan():
    """Start packet captures, trigger mDNS scan, return capture results"""
    pcap1 = ns1.pcap("host 10.0.1.1 and port 5353")
    pcap2 = ns2.pcap("host 10.0.2.1 and port 5353")
    pcap3 = ns3.pcap("host 10.0.3.1 and port 5353")

    with pcap1, pcap2, pcap3:
        ssh.runsh("logger -t scan 'calling avahi-browse ...'")
        ssh.runsh("avahi-browse -lat")

    def has_packets(output):
        if not output:
            return False
        lines = output.strip().split('\n')
        m = re.search(r'^(\d+) packets.*', lines[1])
        if m and int(m.group(1)) > 0:
            return True
        return False

    out1 = pcap1.tcpdump("--count")
    out2 = pcap2.tcpdump("--count")
    out3 = pcap3.tcpdump("--count")

    return (has_packets(out1), has_packets(out2), has_packets(out3))


def check(expected, allow=None, deny=None):
    """Execute complete mDNS test scenario"""
    try:
        dut.delete_xpath("/infix-services:mdns/interfaces")
    except ValueError:
        # Ignore if xpath doesn't exist (first run)
        pass

    mdns_config = {"mdns": {"interfaces": {}}}
    if allow:
        mdns_config["mdns"]["interfaces"]["allow"] = allow
    if deny:
        mdns_config["mdns"]["interfaces"]["deny"] = deny

    dut.put_config_dict("infix-services", mdns_config)

    actual = mdns_scan()
    if actual != tuple(expected):
        raise AssertionError(f"Expected {expected}, got {actual}")


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        dut = env.attach("dut", "mgmt")
        ssh = env.attach("dut", "mgmt", "ssh")
        _, p1 = env.ltop.xlate("dut", "p1")
        _, p2 = env.ltop.xlate("dut", "p2")
        _, p3 = env.ltop.xlate("dut", "p3")
        _, eth1 = env.ltop.xlate("host", "eth1")
        _, eth2 = env.ltop.xlate("host", "eth2")
        _, eth3 = env.ltop.xlate("host", "eth3")

    with test.step("Configure device"):
        dut.put_config_dicts(
            {
                "ietf-interfaces": {
                    "interfaces": {
                        "interface": [{
                            "name": p1,
                            "enabled": True,
                            "ipv4": {
                                "address": [{
                                    "ip": "10.0.1.1",
                                    "prefix-length": 24
                                }]
                            }
                        }, {
                            "name": p2,
                            "enabled": True,
                            "ipv4": {
                                "address": [{
                                    "ip": "10.0.2.1",
                                    "prefix-length": 24
                                }]
                            }
                        }, {
                            "name": p3,
                            "enabled": True,
                            "ipv4": {
                                "address": [{
                                    "ip": "10.0.3.1",
                                    "prefix-length": 24
                                }]
                            }
                        }]
                    }
                },
                "ietf-system": {
                    "system": {"hostname": "dut"}
                },
                "infix-services": {
                    "mdns": {"enabled": True}
                }
            }
        )

    with infamy.IsolatedMacVlan(eth1) as ns1, \
         infamy.IsolatedMacVlan(eth2) as ns2, \
         infamy.IsolatedMacVlan(eth3) as ns3:
        ns1.addip("10.0.1.2")
        ns2.addip("10.0.2.2")
        ns3.addip("10.0.3.2")

        with test.step("Allow mDNS on a single interface: p2"):
            # p1:no, p2:yes, p3:no
            check([False, True, False], allow=[p2])

        with test.step("Deny mDNS on a single interface: p2"):
            # p1:yes, p2:no, p3:yes
            check([True, False, True], deny=[p2])

        with test.step("Allow mDNS on p1, p3 deny on p2, p3"):
            # p1:yes, p2:no, p3:no (deny overrides allow)
            check([True, False, False], allow=[p1, p3], deny=[p2, p3])

    test.succeed()
