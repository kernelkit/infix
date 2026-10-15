#!/usr/bin/env python3
"""
Get operational

Verify each device's operational datastore against its test-config.
The running config is the source of truth, so the checks hold no matter
how interfaces differ between devices:

  1. The operational interfaces are exactly the configured ones: none
     missing, and none present that were never configured.
  2. Each interface carries exactly the configured addresses, apart
     from link-local and autoconfigured IPv6 ones.
  3. The routing tables hold only connected routes for the configured
     addresses and kernel routes, nothing static and nothing learned.
  4. Features the test-config leaves disabled (NTP, containers, routing
     protocols, firewall, PTP) emit no operational data, and the DHCP
     server holds no leases.

Both checks use specific-path GETs, which behave the same on NETCONF and
RESTCONF.  A full-datastore GET is not portable: RESTCONF does not serve
the operational datastore root, and NETCONF's operational provider errors
when asked for an empty subtree.

The test has no logical topology; it reads state from whatever DUTs the
physical topology provides.
"""
import ipaddress
import infamy
from infamy.util import parallel, until

# Feature subtrees that must be absent because the test-config does not
# enable them.  /ietf-routing:routing itself is always present (its RIB
# reflects the kernel's connected/local routes), so we target the
# config-gated control-plane-protocols child rather than all of routing.
ABSENT = [
    "/ietf-ntp:ntp",
    "/infix-containers:containers",
    "/ietf-routing:routing/control-plane-protocols",
    "/infix-dhcp-server:dhcp-server/leases/lease",
    "/infix-firewall:firewall",
    "/ieee1588-ptp:ptp",
]

# IPv6 addresses the kernel adds on its own, never part of the config.
AUTO_ORIGINS = ("link-layer", "random")


def configured_interfaces(dut):
    cfg = dut.get_config_dict("/ietf-interfaces:interfaces")
    return {i["name"] for i in cfg["interfaces"]["interface"]}


def addresses(iface, family):
    """(ip, prefix-length) pairs of an interface's address family, with
    or without the ietf-ip module prefix, in canonical form."""
    ip = iface.get(family) or iface.get(f"ietf-ip:{family}") or {}
    return {(ipaddress.ip_address(a["ip"]).compressed, int(a["prefix-length"]), a.get("origin"))
            for a in ip.get("address", [])}


def configured_addresses(dut):
    """{ifname: {family: {(ip, plen)}}} from the running config."""
    cfg = dut.get_config_dict("/ietf-interfaces:interfaces")
    return {i["name"]: {fam: {(ip, plen) for ip, plen, _ in addresses(i, fam)}
                        for fam in ("ipv4", "ipv6")}
            for i in cfg["interfaces"]["interface"]}


def operational_addresses(dut):
    """Same shape as configured_addresses(), without kernel-added IPv6."""
    oper = dut.get_data("/ietf-interfaces:interfaces")["interfaces"]["interface"]
    return {i["name"]: {fam: {(ip, plen) for ip, plen, origin in addresses(i, fam)
                              if origin not in AUTO_ORIGINS}
                        for fam in ("ipv4", "ipv6")}
            for i in oper}


def addresses_match(name, dut, want):
    """True once every interface carries exactly its configured addresses."""
    have = operational_addresses(dut)
    diff = [(ifname, fam, sorted(have.get(ifname, {}).get(fam, set())), sorted(want[ifname][fam]))
            for ifname in want for fam in ("ipv4", "ipv6")
            if have.get(ifname, {}).get(fam, set()) != want[ifname][fam]]
    if diff:
        for ifname, fam, got, exp in diff:
            print(f"{name}: {ifname} {fam} operational {got} != configured {exp}, waiting...")
        return False
    return True


def configured_networks(want):
    """Networks a connected route may exist for, from the configured addresses."""
    nets = {ipaddress.ip_network("fe80::/64")}
    for fams in want.values():
        for fam in fams.values():
            for ip, plen in fam:
                nets.add(ipaddress.ip_network(f"{ip}/{plen}", strict=False))
    return nets


def stray_routes(dut, nets):
    """Routes that are neither kernel nor connected to a configured address."""
    ribs = dut.get_data("/ietf-routing:routing/ribs")["routing"]["ribs"]["rib"]
    stray, total = [], 0
    for rib in ribs:
        for route in rib.get("routes", {}).get("route", []):
            total += 1
            dst = next(v for k, v in route.items() if k.endswith("destination-prefix"))
            proto = route.get("source-protocol", "").split(":")[-1]
            if proto == "kernel":
                continue
            if proto == "direct" and ipaddress.ip_network(dst) in nets:
                continue
            stray.append((rib["name"], dst, proto))
    return stray, total


def routes_clean(name, dut, nets):
    """True once the ribs hold nothing but kernel and connected routes."""
    stray, total = stray_routes(dut, nets)
    if stray:
        print(f"{name}: stray routes {stray}, waiting...")
        return False
    print(f"{name}: {total} routes, none stray")
    return True


def operational_interfaces(dut):
    oper = dut.get_data("/ietf-interfaces:interfaces")["interfaces"]["interface"]
    return {i["name"] for i in oper}


def interfaces_match(name, dut, want):
    """True once operational interfaces equal the configured set.

    A preceding test (e.g. a container use_case) may leave veth endpoints
    that are still being torn down when this test starts, so operational
    momentarily carries interfaces that are not in the configuration.
    Rather than asserting on that transient state, poll until the kernel,
    and thus operational, has converged on the configured set.
    """
    have = operational_interfaces(dut)
    if have != want:
        print(f"{name}: operational {sorted(have)} != configured {sorted(want)}, waiting...")
        return False
    return True


def empty(data):
    """True when data holds no leaf, only (nested) empty containers or lists."""
    if isinstance(data, dict):
        return all(empty(v) for v in data.values())
    if isinstance(data, list):
        return all(empty(v) for v in data)
    return data is None


def absent(dut, xpath):
    # RESTCONF returns nothing for an absent subtree; NETCONF's operational
    # provider errors when asked for one, or hands back the empty containers
    # leading up to it.  All of them mean "not present".
    try:
        return empty(dut.get_data(xpath))
    except Exception:
        return True


def features_absent(name, dut):
    """True once every unconfigured feature subtree is gone.

    Like the interface set, these subtrees can momentarily linger: a
    preceding test (containers, NTP, a routing protocol) leaves state that
    yangerd only prunes once the underlying daemon/config is torn down.
    Poll until operational has converged rather than asserting on the
    transient overlap.
    """
    pending = [xpath for xpath in ABSENT if not absent(dut, xpath)]
    if pending:
        print(f"{name}: feature data still present {pending}, waiting...")
        return False
    return True


with infamy.Test() as test:
    with test.step("Attach to all DUTs in the topology"):
        env = infamy.Env(ltop=False)
        infixen = env.ptop.get_infixen()
        assert infixen, "no devices found in topology"
        duts = dict(zip(infixen, parallel(*(lambda n=name: env.attach(n, "mgmt")
                                            for name in infixen))))

    with test.step("Verify operational interfaces match the test-config"):
        def check(name, dut):
            want = configured_interfaces(dut)
            until(lambda: interfaces_match(name, dut, want), attempts=60)

        parallel(*(lambda n=name, d=dut: check(n, d) for name, dut in duts.items()))

    with test.step("Verify operational addresses on each interface match the test-config"):
        def check_addresses(name, dut):
            want = configured_addresses(dut)
            until(lambda: addresses_match(name, dut, want), attempts=60)

        parallel(*(lambda n=name, d=dut: check_addresses(n, d) for name, dut in duts.items()))

    with test.step("Verify the routing tables hold only kernel and connected routes for configured addresses"):
        def check_routes(name, dut):
            nets = configured_networks(configured_addresses(dut))
            until(lambda: routes_clean(name, dut, nets), attempts=60)

        parallel(*(lambda n=name, d=dut: check_routes(n, d) for name, dut in duts.items()))

    with test.step("Verify unconfigured feature subtrees are absent"):
        def check_absent(name, dut):
            # NTP is pruned by a periodic collector with a 60s poll
            # interval, so a stale subtree can take a full cycle to clear;
            # wait comfortably past one interval rather than racing it.
            until(lambda: features_absent(name, dut), attempts=120)

        parallel(*(lambda n=name, d=dut: check_absent(n, d)
                   for name, dut in duts.items()))

    test.succeed()
