#!/usr/bin/env python3
"""Verify physical topology

Walk the physical topology file and check that every wired link is where
it says.  DUT-to-DUT links are checked by what LLDP reports at both ends,
host-to-DUT links by sending an LLDP frame from the host port and finding
it on the DUT port.  Wireless cells are not checked.
"""
import infamy
from infamy.util import parallel, until

from scapy.all import Ether, sendp
from scapy.contrib.lldp import (
    LLDPDU, LLDPDUChassisID, LLDPDUPortID, LLDPDUTimeToLive, LLDPDUEndOfLLDPDU
)

LOCAL = 7


def send_lldp(iface, chassis, port):
    frame = Ether(src="02:00:00:00:00:01", dst="01:80:c2:00:00:0e", type=0x88cc)
    frame /= LLDPDU()
    frame /= LLDPDUChassisID(subtype=LOCAL, id=chassis)
    frame /= LLDPDUPortID(subtype=LOCAL, id=port)
    frame /= LLDPDUTimeToLive(ttl=120) / LLDPDUEndOfLLDPDU()
    sendp(frame, iface=iface, verbose=False)


def macs(dut):
    """All MAC addresses of dut, lldpd picks its chassis id among them."""
    data = dut.get_data("/ietf-interfaces:interfaces") or {}
    return {i["phys-address"].lower() for i in data.get("interfaces", {}).get("interface", [])
            if i.get("phys-address")}


def neighbors(dut, owner):
    """Map each port of dut to the (system, port) pairs LLDP heard there."""
    data = dut.get_data("/ieee802-dot1ab-lldp:lldp") or {}
    out = {}
    for port in data.get("lldp", {}).get("port", []):
        out[port["name"]] = [(owner.get(n.get("chassis-id", "").lower(), n.get("chassis-id")),
                              n.get("port-desc") or n.get("port-id"))
                             for n in port.get("remote-systems-data", [])]
    return out


def missing(links, tables):
    return [(a, pa, b, pb) for a, pa, b, pb in links
            if (b, pb) not in tables.get(a, {}).get(pa, [])]


def report(links, tables):
    for a, pa, b, pb in links:
        seen = tables.get(a, {}).get(pa, [])
        status = "ok  " if (b, pb) in seen else "FAIL"
        print(f"{status} {a}:{pa} -- {b}:{pb}  sees {seen}")


with infamy.Test() as test:
    with test.step("Discover the physical topology and attach to every DUT"):
        env = infamy.Env(False)
        ctrl = env.ptop.get_ctrl()
        infixen = env.ptop.get_infixen()

        wired = []      # DUT-to-DUT, both directions
        uplinks = []    # DUT-to-host
        wireless = set()
        for a, b, attrs in env.ptop.g.edges(data=True):
            pa, pb = attrs[a], attrs[b]
            if a in infixen and b in infixen:
                wired += [(a, pa, b, pb), (b, pb, a, pa)]
            elif ctrl in (a, b) and (a in infixen or b in infixen):
                if a == ctrl:
                    a, pa, b, pb = b, pb, a, pa
                uplinks.append((a, pa, b, pb))
            elif a in infixen or b in infixen:
                wireless.add((a, pa) if a in infixen else (b, pb))

        print(f"{len(wired) // 2} DUT-to-DUT links, {len(uplinks)} host links")

        def attach(ix):
            _, ixport = env.ptop.get_mgmt_link(ctrl, ix)
            return env.attach(ix, ixport)

        duts = dict(zip(infixen, parallel(*(lambda ix=ix: attach(ix)
                                            for ix in infixen))))

        owner = {}
        for name, dut in duts.items():
            owner.update({mac: name for mac in macs(dut)})

    with test.step("Enable LLDP and bring up every port in the topology"):
        def configure(name):
            ports = {pa for a, pa, _, _ in wired if a == name}
            ports |= {pb for _, _, b, pb in uplinks if b == name}
            duts[name].put_config_dicts({
                "ieee802-dot1ab-lldp": {"lldp": {"infix-lldp:enabled": True}},
                "ietf-interfaces": {"interfaces": {"interface": [
                    {"name": port, "enabled": True} for port in sorted(ports)
                ]}},
            })

        parallel(*(lambda name=name: configure(name) for name in infixen))

    with test.step("Verify every DUT-to-DUT link is reported by LLDP at both ends"):
        tables = {}

        def settled():
            tables.update({name: neighbors(dut, owner) for name, dut in duts.items()})
            return not missing(wired, tables)

        try:
            until(settled, attempts=90)
        except Exception:
            pass
        report(wired, tables)
        assert not missing(wired, tables), "DUT-to-DUT links differ from the topology"

    with test.step("Verify every host-to-DUT link by sending LLDP from the host port"):
        def heard():
            for _, _, _, cport in uplinks:
                send_lldp(cport, ctrl, cport)
            tables.update({name: neighbors(dut, owner) for name, dut in duts.items()})
            return not missing(uplinks, tables)

        try:
            until(heard, attempts=60)
        except Exception:
            pass
        report(uplinks, tables)
        assert not missing(uplinks, tables), "host links differ from the topology"

    with test.step("Verify no wired port sees a DUT or host port the topology does not list"):
        expected = {}
        for a, pa, b, pb in wired + uplinks:
            expected.setdefault((a, pa), set()).add((b, pb))

        miswired = []
        for name, table in tables.items():
            for port, seen in table.items():
                if (name, port) in wireless:
                    continue
                for peer in seen:
                    if peer in expected.get((name, port), set()):
                        continue
                    print(f"{name}:{port} also sees {peer[0]}:{peer[1]}")
                    if peer[0] in infixen or peer[0] == ctrl:
                        miswired.append((name, port, peer))
        assert not miswired, f"Ports cabled outside the topology: {miswired}"

    test.succeed()
