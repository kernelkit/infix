from ..host import HOST


def frame_statistics(etstats):
    STAT_MAP = {
        "eth-mac": {
            "out-frames":                               "FramesTransmittedOK",
            "out-multicast-frames":                     "MulticastFramesXmittedOK",
            "out-broadcast-frames":                     "BroadcastFramesXmittedOK",
            "in-frames":                                "FramesReceivedOK",
            "in-multicast-frames":                      "MulticastFramesReceivedOK",
            "in-broadcast-frames":                      "BroadcastFramesReceivedOK",
            "in-error-fcs-frames":                      "FrameCheckSequenceErrors",
            "in-error-mac-internal-frames":             "FramesLostDueToIntMACRcvError",
            "infix-ethernet-interface:out-good-octets": "OctetsTransmittedOK",
            "infix-ethernet-interface:in-good-octets":  "OctetsReceivedOK",

            "in-total-frames": (
                "FramesReceivedOK",
                "FrameCheckSequenceErrors",
                "FramesLostDueToIntMACRcvError",
                "AlignmentErrors",
                "etherStatsOversizePkts",
                "etherStatsJabbers",
            ),
        },

        "rmon": {
            "in-error-undersize-frames": "undersize_pkts",

            "in-error-oversize-frames": (
                "etherStatsJabbers",
                "etherStatsOversizePkts",
            ),
        },
    }

    fstats = {}

    for group, mapping in STAT_MAP.items():
        etgroup = etstats.get(group)
        if not etgroup:
            continue

        for name, source in mapping.items():
            if type(source) == str:
                counter = etgroup.get(source)
                if counter is not None:
                    fstats[name] = str(counter)
            elif type(source) == tuple:
                inputs = [etgroup.get(src) for src in source]
                if inputs := filter(lambda i: i is not None, inputs):
                    fstats[name] = str(sum(inputs))

    return fstats


def statistics(ifname):
    if etstats := HOST.run_json(["ethtool", "--json", "-S", ifname, "--all-groups"], []):
        etstats = etstats[0]
    else:
        return None

    statistics = {}

    if fstats := frame_statistics(etstats):
        statistics["frame"] = fstats

    return statistics

# ethtool reports SPEED_UNKNOWN as UINT32_MAX when no link / no medium.
_ETHTOOL_SPEED_UNKNOWN = (1 << 32) - 1

# Map (ethtool port string, speed in Mb/s, duplex) -> (phy-type, pmd-type)
# identity suffixes per IEEE Std 802.3.2-2025 (ieee802-ethernet-phy-type).
#
# phy-type names the PHY family / line coding (e.g. 1000BASE-X is the 8B/10B
# family covering LX/SX/ZX/CX); pmd-type names the specific physical medium
# (1000BASE-LX vs 1000BASE-SX).  For media where ethtool's (port, speed,
# duplex) tuple uniquely identifies the variant — copper, DAC, copper-T —
# both leaves are populated with the right identities (often the same name).
# For generic fiber where the same tuple can be SR/LR/ER/etc., we only
# populate phy-type (family) and leave pmd-type as None — emitting a guess
# would be misleading.  Refining the PMD on fiber needs the SFP EEPROM
# (ethtool -m), deferred.
_LINK_MODES = {
    # (port, speed Mb/s, duplex):                 (phy-type,    pmd-type or None)
    ("Twisted Pair",         10,     "full"):    ("10BASE-T",   "10BASE-T"),
    ("Twisted Pair",         10,     "half"):    ("10BASE-T",   "10BASE-T"),
    ("Twisted Pair",         100,    "full"):    ("100BASE-X",  "100BASE-TX"),
    ("Twisted Pair",         100,    "half"):    ("100BASE-X",  "100BASE-TX"),
    ("Twisted Pair",         1000,   "full"):    ("1000BASE-T", "1000BASE-T"),
    ("Twisted Pair",         1000,   "half"):    ("1000BASE-T", "1000BASE-T"),
    ("Twisted Pair",         2500,   "full"):    ("2.5GBASE-T", "2.5GBASE-T"),
    ("Twisted Pair",         5000,   "full"):    ("5GBASE-T",   "5GBASE-T"),
    ("Twisted Pair",         10000,  "full"):    ("10GBASE-T",  "10GBASE-T"),
    ("Twisted Pair",         25000,  "full"):    ("25GBASE-T",  "25GBASE-T"),
    ("Twisted Pair",         40000,  "full"):    ("40GBASE-T",  "40GBASE-T"),
    ("MII",                  10,     "full"):    ("10BASE-T",   "10BASE-T"),
    ("MII",                  10,     "half"):    ("10BASE-T",   "10BASE-T"),
    ("MII",                  100,    "full"):    ("100BASE-X",  "100BASE-TX"),
    ("MII",                  100,    "half"):    ("100BASE-X",  "100BASE-TX"),
    ("FIBRE",                100,    "full"):    ("100BASE-X",  None),
    ("FIBRE",                1000,   "full"):    ("1000BASE-X", None),
    ("FIBRE",                10000,  "full"):    ("10GBASE-R",  None),
    ("FIBRE",                25000,  "full"):    ("25GBASE-R",  None),
    ("FIBRE",                40000,  "full"):    ("40GBASE-R",  None),
    ("FIBRE",                100000, "full"):    ("100GBASE-R", None),
    # SFP+ DAC has no IEEE-standardised pmd-type (10GBASE-CR is industry
    # shorthand, not a YANG identity); the phy-type-10GBASE-R family is as
    # specific as we honestly get.
    ("Direct Attach Copper", 10000,  "full"):    ("10GBASE-R",  None),
    ("Direct Attach Copper", 25000,  "full"):    ("25GBASE-R",  "25GBASE-CR"),
    ("Direct Attach Copper", 40000,  "full"):    ("40GBASE-R",  "40GBASE-CR4"),
    ("Direct Attach Copper", 100000, "full"):    ("100GBASE-R", "100GBASE-CR4"),
}


# Map ethtool link-mode string (e.g. '10000baseLR') to IEEE pmd-type identity
# suffix.  The kernel reports a separate entry per (mode, duplex) — we strip
# the trailing '/Full' or '/Half' before lookup, then dedupe in the caller.
# Where the kernel collapses several IEEE variants into one family bit
# (e.g. 1000baseX covers LX/SX/ZX/CX) we pick the most common long-reach
# variant as our representative — same convention as _LINK_MODES.
_ETHTOOL_TO_PMD = {
    "10baseT":         "10BASE-T",
    "10baseT1L":       "10BASE-T1L",
    "100baseT":        "100BASE-TX",     # kernel uses 100baseT for 100BASE-TX
    "100baseT1":       "100BASE-T1",
    "100baseFX":       "100BASE-FX",
    "1000baseT":       "1000BASE-T",
    "1000baseT1":      "1000BASE-T1",
    "1000baseX":       "1000BASE-LX",    # 8B/10B family — could be SX/LX/ZX
    "1000baseKX":      "1000BASE-KX",
    "2500baseT":       "2.5GBASE-T",
    "2500baseX":       "2.5GBASE-X",
    "5000baseT":       "5GBASE-T",
    "10000baseT":      "10GBASE-T",
    "10000baseSR":     "10GBASE-SR",
    "10000baseLR":     "10GBASE-LR",
    "10000baseLRM":    "10GBASE-LRM",
    "10000baseER":     "10GBASE-ER",
    "10000baseKR":     "10GBASE-KR",
    "10000baseKX4":    "10GBASE-KX4",
    "25000baseCR":     "25GBASE-CR",
    "25000baseSR":     "25GBASE-SR",
    "25000baseKR":     "25GBASE-KR",
    "40000baseCR4":    "40GBASE-CR4",
    "40000baseSR4":    "40GBASE-SR4",
    "40000baseLR4":    "40GBASE-LR4",
    "40000baseKR4":    "40GBASE-KR4",
    "100000baseCR4":   "100GBASE-CR4",
    "100000baseSR4":   "100GBASE-SR4",
    "100000baseLR4_ER4": "100GBASE-LR4",
    "100000baseKR4":   "100GBASE-KR4",
}


def _ethtool_modes_to_pmd_identities(modes):
    """Translate ethtool 'supported/advertised-link-modes' to PMD identities.

    'modes' is the list of strings ethtool emits, e.g. ['1000baseT/Full',
    '10000baseLR/Full'].  Returns a deduped, order-preserving list of
    fully-qualified ieee802-ethernet-phy-type:pmd-type-* identities, skipping
    entries we don't have a mapping for (kernel-only "filler" bits like
    Autoneg, TP, FIBRE, Pause, Asym_Pause that share the link-modes bitset
    namespace but aren't link modes).
    """
    seen = set()
    out = []
    for entry in modes or []:
        # Drop '/Full' or '/Half' suffix
        base = entry.split("/", 1)[0]
        pmd = _ETHTOOL_TO_PMD.get(base)
        if pmd is None or pmd in seen:
            continue
        seen.add(pmd)
        out.append(f"ieee802-ethernet-phy-type:pmd-type-{pmd}")
    return out


def link(ifname):
    """Read link properties from ethtool.

    Returns (eth_container_dict, interface_speed_bps_or_None); the
    interface speed is lifted onto ietf-interfaces:speed by the caller.
    """
    if data := HOST.run_json(["ethtool", "--json", ifname], {}):
        data = data[0]
    else:
        return {}, None

    eth = {"auto-negotiation": {"enable": data.get("auto-negotiation", False)}}

    duplex = (data.get("duplex") or "").lower()
    if duplex in ("full", "half"):
        eth["duplex"] = duplex

    # Expose what the kernel currently considers supported as a config-false
    # leaf-list — varies on SFP cages with the inserted module, so it's a
    # useful diagnosis facet on its own.
    supported = _ethtool_modes_to_pmd_identities(data.get("supported-link-modes"))
    if supported:
        eth["infix-ethernet-interface:supported-pmd-types"] = supported

    # Suppress when advertised == supported — that's the default
    # "advertise everything" state with no diagnostic value.
    advertised = _ethtool_modes_to_pmd_identities(data.get("advertised-link-modes"))
    if advertised and set(advertised) != set(supported):
        eth["auto-negotiation"]["infix-ethernet-interface:advertised-pmd-types"] = advertised

    speed_bps = None
    speed_mbps = data.get("speed")
    if isinstance(speed_mbps, int) and 0 < speed_mbps < _ETHTOOL_SPEED_UNKNOWN:
        speed_bps = speed_mbps * 1_000_000
        port = data.get("port") or ""
        if mapping := _LINK_MODES.get((port, speed_mbps, duplex)):
            phy, pmd = mapping
            eth["phy-type"] = f"ieee802-ethernet-phy-type:phy-type-{phy}"
            if pmd is not None:
                eth["pmd-type"] = f"ieee802-ethernet-phy-type:pmd-type-{pmd}"
        # Refine pmd-type from supported list when the kernel reports
        # exactly one mode (typically an SFP/SFP+ with a specific optic
        # plugged in) — strictly more accurate than the (port, speed,
        # duplex) lookup since the SFP nailed it down for us.
        if len(supported) == 1:
            eth["pmd-type"] = supported[0]

    return eth, speed_bps


def ethernet(iplink):
    """Return (ethernet container, interface speed in bits/s or None)."""
    eth, speed_bps = link(iplink["ifname"])

    if stats := statistics(iplink["ifname"]):
        eth["statistics"] = stats

    return eth, speed_bps
