#!/usr/bin/env python3
"""
Interface Speed Duplex (Copper)

Verify that the interface operates at the expected speed/duplex by
restricting the set of PMD types auto-negotiation may advertise:

1. Single-mode advertise — host and target both advertise exactly
   one PMD/duplex combination; the link comes up at that mode.
2. Multi-mode advertise — host advertises a set, target advertises
   another set; the link comes up at the highest common.
3. Auto-negotiation off — both peers forced to a fixed speed/duplex,
   the escape hatch for link partners that don't speak auto-negotiation.
   Disabling autoneg also disables Auto-MDIX, so the ends use opposite
   MDI/MDI-X pinouts (host mdix on, DUT mdi).  The link bounces and can
   take a few seconds to settle, so the forced steps poll for it.

The legacy "auto-negotiation: off + fixed speed + fixed duplex"
configuration was retired together with the obsoletion of eth:speed
in IEEE Std 802.3.2-2025; the standards-correct way to pin a link to
a specific mode is to advertise only that mode.  See the augment in
infix-ethernet-interface.yang.
"""

import infamy
import infamy.iface as iface
import subprocess
from infamy.util import until

ADVERTISE_MODES = {
    # Values from ethtool's ETHTOOL_LINK_MODE bit positions
    # See: https://elixir.bootlin.com/linux/latest/source/include/uapi/linux/ethtool.h
    "10half":   0x0001,
    "10full":   0x0002,
    "100half":  0x0004,
    "100full":  0x0008,
    "1000full": 0x0020,
}

# Mapping from (ethtool advertise key) → (IEEE PMD identity, duplex enum).
# Mirrors the migration table in src/confd/share/migrate/1.9/.
TARGET_MODES = {
    "10half":   ("ieee802-ethernet-phy-type:pmd-type-10BASE-T",    "half", 10),
    "10full":   ("ieee802-ethernet-phy-type:pmd-type-10BASE-T",    "full", 10),
    "100half":  ("ieee802-ethernet-phy-type:pmd-type-100BASE-TX",  "half", 100),
    "100full":  ("ieee802-ethernet-phy-type:pmd-type-100BASE-TX",  "full", 100),
    "1000full": ("ieee802-ethernet-phy-type:pmd-type-1000BASE-T",  "full", 1000),
}


def advertise_host_modes(interface, modes):
    mask = 0
    for mode in modes:
        mask |= ADVERTISE_MODES[mode]
    try:
        subprocess.run(["ethtool", "-s", interface, "advertise", hex(mask)],
                       check=True)
    except subprocess.CalledProcessError as e:
        raise RuntimeError(f"Failed to advertise modes via ethtool: {e}")


def enable_host_autoneg_full(interface):
    """Restore the host NIC to autoneg-on with Auto-MDIX.

    'autoneg on' re-enables negotiation and advertises all supported modes;
    each test step sets the host advertise explicitly, so no extra mask
    handling is needed here.  Separately clear any forced MDI/MDI-X pinout
    left by force_host_fixed — best-effort, since not every NIC exposes mdix.
    """
    try:
        subprocess.run(["ethtool", "-s", interface, "autoneg", "on"], check=True)
    except subprocess.CalledProcessError as e:
        raise RuntimeError(f"Failed to restore host autoneg: {e}")
    subprocess.run(["ethtool", "-s", interface, "mdix", "auto"], check=False)


def force_host_fixed(interface, speed_mbps, duplex, mdix="on"):
    """Pin host to a fixed speed/duplex with auto-negotiation OFF.

    Auto-MDIX is disabled in forced mode, so host and DUT take opposite
    pinouts; default mdix=on (MDI-X) to pair with the DUT's mdi.
    """
    try:
        subprocess.run(["ethtool", "-s", interface, "autoneg", "off",
                        "speed", str(speed_mbps), "duplex", duplex,
                        "mdix", mdix],
                       check=True)
    except subprocess.CalledProcessError as e:
        raise RuntimeError(f"Failed to force host fixed mode: {e}")


def get_target_speed_duplex(target, interface):
    data = target.get_data(iface.get_xpath(interface)) \
                  ["interfaces"]["interface"][interface]
    eth = data.get("ethernet", {})
    # Operational speed under ietf-interfaces:speed (bits/s, RFC 8343).
    speed_bps = data.get("speed")
    return speed_bps, eth.get("duplex")


def set_target_advertise(target, interface, mode_keys):
    """Configure the target interface to advertise the given set of PMDs.

    mode_keys is a list of host-side mode names (e.g. ['10full', '100full']).
    Translates to the corresponding (pmd-type, duplex) for the target.
    When all entries share the same duplex, that duplex is also set on the
    interface as an additional restriction.
    """
    pmds = []
    duplexes = set()
    for key in mode_keys:
        pmd, duplex, _ = TARGET_MODES[key]
        if pmd not in pmds:
            pmds.append(pmd)
        duplexes.add(duplex)

    eth = {
        "auto-negotiation": {
            "infix-ethernet-interface:advertised-pmd-types": pmds,
        },
    }
    if len(duplexes) == 1:
        eth["duplex"] = duplexes.pop()

    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [{"name": interface, "ethernet": eth}]
            }
        }
    })


def force_target_fixed(target, interface, mode_key, mdi_x=False):
    """Configure the target with auto-negotiation disabled, pinned to one PMD.

    Exercises confd's enable=false escape hatch: the apply path emits
    'ethtool --change <if> autoneg off speed N duplex D mdix ...' from the
    single advertised-pmd-types entry, the duplex leaf, and the mdi-x leaf.
    Default mdi_x=False (MDI) pairs with the host's forced MDI-X.
    """
    pmd, duplex, _ = TARGET_MODES[mode_key]
    # enable=false requires exactly one advertised-pmd-types entry, but
    # put_config_dicts (PATCH) merges leaf-lists, so successive forced
    # steps would accumulate entries.  Reset the ethernet sub-config first
    # (tolerate absence on the first call).
    try:
        clear_target_advertise(target, interface)
    except Exception:
        pass
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": interface,
                    "ethernet": {
                        "auto-negotiation": {
                            "enable": False,
                            "infix-ethernet-interface:advertised-pmd-types": [pmd],
                        },
                        "duplex": duplex,
                        "infix-ethernet-interface:mdi-x": mdi_x,
                    },
                }]
            }
        }
    })


def clear_target_advertise(target, interface):
    """Drop the entire ethernet sub-config on the target.

    Targets the parent container rather than the individual leaves because
    RESTCONF DELETE on a leaf-list URI without a '=value' key is a silent
    no-op (RFC 8040 §3.5.3.2: leaf-list instance URIs must carry the value
    as the key — rousette/libyang materialise an empty leaf-list node that
    matches no entry).  Deleting the container removes all descendants in
    one go, which is exactly what this test needs.
    """
    target.delete_xpath(iface.get_xpath(interface, "ieee802-ethernet-interface:ethernet"))


def verify_speed_duplex(target, ns, interface, exp_mbps, exp_duplex):
    until(lambda: _speed_duplex_present(target, interface))
    speed_bps, duplex = get_target_speed_duplex(target, interface)
    if speed_bps is None or duplex is None:
        print(f"Could not fetch speed/duplex from target for interface {interface}")
        test.fail()

    act_mbps = int(speed_bps) // 1_000_000
    if act_mbps != exp_mbps:
        print(f"act_mbps: {act_mbps}, exp_mbps: {exp_mbps}")
        test.fail()

    if duplex.lower() != exp_duplex.lower():
        print(f"act_duplex: {duplex}, exp_duplex: {exp_duplex}")
        test.fail()

    ns.must_reach("10.0.0.2")
    print(f"Verified: {interface} is operating at {act_mbps} Mbps, {duplex} duplex")


def _speed_duplex_present(target, interface):
    speed_bps, duplex = get_target_speed_duplex(target, interface)
    return speed_bps is not None and duplex is not None


def verify_forced_speed_duplex(target, ns, interface, exp_mbps, exp_duplex):
    """Verify a forced (autoneg off) link reaches exp speed/duplex.

    Turning auto-negotiation off bounces the link; it can take a few
    seconds to settle, so poll the operational speed/duplex AND a ping
    together until both hold instead of checking once.
    """
    def settled():
        speed_bps, duplex = get_target_speed_duplex(target, interface)
        if not speed_bps or not duplex:
            return False
        if int(speed_bps) // 1_000_000 != exp_mbps:
            return False
        if duplex.lower() != exp_duplex.lower():
            return False
        try:
            ns.ping("10.0.0.2", timeout=3)
        except Exception:
            return False
        return True

    until(settled, attempts=30)
    print(f"Verified: {interface} forced to {exp_mbps} Mbps, {exp_duplex} duplex")


def enable_target_interface(target, interface):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": interface,
                    "enabled": True,
                    "ipv4": {"address": [{"ip": "10.0.0.2", "prefix-length": 24}]}
                }]
            }
        }
    })


def cleanup(target, hdata, tdata):
    """Restore both host and target to 'advertise everything supported'."""
    print("Restoring interfaces to default (advertise all)")
    try:
        enable_host_autoneg_full(hdata)
    except Exception as e:
        print(f"Host restore failed: {e}")
    try:
        enable_target_interface(target, tdata)
        clear_target_advertise(target, tdata)
    except Exception as e:
        print(f"Target restore failed: {e}")


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, hdata = env.ltop.xlate("host", "data")
        _, tdata = env.ltop.xlate("target", "data")
        test.push_test_cleanup(lambda: cleanup(target, hdata, tdata))

    with test.step("Enable target interface"):
        enable_target_interface(target, tdata)

    with infamy.IsolatedMacVlan(hdata) as ns:
        ns.addip("10.0.0.1")

        # Pinned-mode tests: both peers advertise exactly one mode.
        with test.step("Advertise 10/full only on both peers"):
            advertise_host_modes(hdata, ["10full"])
            set_target_advertise(target, tdata, ["10full"])
            verify_speed_duplex(target, ns, tdata, 10, "full")

        with test.step("Advertise 10/half only on both peers"):
            advertise_host_modes(hdata, ["10half"])
            set_target_advertise(target, tdata, ["10half"])
            verify_speed_duplex(target, ns, tdata, 10, "half")

        with test.step("Advertise 100/full only on both peers"):
            advertise_host_modes(hdata, ["100full"])
            set_target_advertise(target, tdata, ["100full"])
            verify_speed_duplex(target, ns, tdata, 100, "full")

        with test.step("Advertise 100/half only on both peers"):
            advertise_host_modes(hdata, ["100half"])
            set_target_advertise(target, tdata, ["100half"])
            verify_speed_duplex(target, ns, tdata, 100, "half")

        # Multi-mode advertise tests: host restricted, target advertises all.
        with test.step("Switch target back to advertising all supported modes"):
            advertise_host_modes(hdata, ["100half"])
            clear_target_advertise(target, tdata)
            verify_speed_duplex(target, ns, tdata, 100, "half")

        with test.step("Host advertises 10/full only"):
            advertise_host_modes(hdata, ["10full"])
            verify_speed_duplex(target, ns, tdata, 10, "full")

        with test.step("Host advertises 10/half only"):
            advertise_host_modes(hdata, ["10half"])
            verify_speed_duplex(target, ns, tdata, 10, "half")

        with test.step("Host advertises 100/full only"):
            advertise_host_modes(hdata, ["100full"])
            verify_speed_duplex(target, ns, tdata, 100, "full")

        with test.step("Host advertises 100/half only"):
            advertise_host_modes(hdata, ["100half"])
            verify_speed_duplex(target, ns, tdata, 100, "half")

        with test.step("Host advertises 10/half + 10/full + 100/half"):
            advertise_host_modes(hdata, ["10half", "10full", "100half"])
            verify_speed_duplex(target, ns, tdata, 100, "half")

        with test.step("Host advertises every mode up to 1G"):
            advertise_host_modes(hdata, ["10half", "10full", "100half",
                                         "100full", "1000full"])
            verify_speed_duplex(target, ns, tdata, 1000, "full")

        # Auto-negotiation off — escape hatch for non-autoneg peers.  Both
        # ends forced to a fixed mode with autoneg disabled (host mdix on,
        # DUT mdi).  The link bounces, so verify_forced_speed_duplex polls
        # until it settles.
        with test.step("Both sides forced to 100/full, autoneg off"):
            force_host_fixed(hdata, 100, "full")
            force_target_fixed(target, tdata, "100full")
            verify_forced_speed_duplex(target, ns, tdata, 100, "full")

        with test.step("Both sides forced to 10/full, autoneg off"):
            force_host_fixed(hdata, 10, "full")
            force_target_fixed(target, tdata, "10full")
            verify_forced_speed_duplex(target, ns, tdata, 10, "full")

    test.succeed()
