#!/usr/bin/env python3
"""
Get operational

For every Infix device, verify the operational datastore against the
test-config -- the running config is the source of truth, so it works
regardless of how interfaces differ between devices:

  1. The operational interfaces are exactly the configured interfaces --
     no missing interface, and nothing operational that was not
     configured.
  2. Features the test-config does not enable (NTP, containers, routing
     protocols) emit no operational data.

Both checks use specific-path GETs, which behave consistently across
NETCONF and RESTCONF.  A full-datastore GET is not portable: RESTCONF does
not serve the operational datastore root, and NETCONF's operational
provider errors when asked for an empty subtree.

This test has no logical topology -- it reads state from whatever Infix
DUTs the physical topology provides.
"""
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
]


def configured_interfaces(dut):
    cfg = dut.get_config_dict("/ietf-interfaces:interfaces")
    return {i["name"] for i in cfg["interfaces"]["interface"]}


def operational_interfaces(dut):
    oper = dut.get_data("/ietf-interfaces:interfaces")["interfaces"]["interface"]
    return {i["name"] for i in oper}


def interfaces_match(name, dut, want):
    """True once operational interfaces equal the configured set.

    A preceding test (e.g. a container use_case) may leave veth endpoints
    that are still being torn down when this test starts, so operational
    momentarily carries interfaces that are not in the configuration.
    Rather than asserting on that transient state, poll until the kernel --
    and thus operational -- has converged on the configured set.
    """
    have = operational_interfaces(dut)
    if have != want:
        print(f"{name}: operational {sorted(have)} != configured {sorted(want)}, waiting...")
        return False
    return True


def absent(dut, xpath):
    # RESTCONF returns nothing for an absent subtree; NETCONF's operational
    # provider errors when asked for one -- both mean "not present".
    try:
        return not dut.get_data(xpath)
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
    with test.step("Attach to all Infix DUTs in the topology"):
        env = infamy.Env(ltop=False)
        infixen = env.ptop.get_infixen()
        assert infixen, "no Infix devices found in topology"
        duts = dict(zip(infixen, parallel(*(lambda n=name: env.attach(n, "mgmt")
                                            for name in infixen))))

    with test.step("Verify operational interfaces match the test-config"):
        def check(name, dut):
            want = configured_interfaces(dut)
            until(lambda: interfaces_match(name, dut, want), attempts=60)

        parallel(*(lambda n=name, d=dut: check(n, d) for name, dut in duts.items()))

    with test.step("Verify unconfigured feature subtrees are absent"):
        def check_absent(name, dut):
            # NTP is pruned by a periodic collector with a 60s poll
            # interval, so a stale subtree can take a full cycle to clear;
            # wait comfortably past one interval rather than racing it.
            until(lambda: features_absent(name, dut), attempts=120)

        parallel(*(lambda n=name, d=dut: check_absent(n, d)
                   for name, dut in duts.items()))

    test.succeed()
