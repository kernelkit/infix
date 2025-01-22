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


def link(ethtool):
    """Parse speed/duplex/autoneg from ethtool output"""
    eth = {}

    for line in ethtool:
        kv = [s.strip() for s in line.split(":")]
        if len(kv) != 2:
            continue

        key, val = kv
        match key:
            case "Auto-negotiation":
                eth["auto-negotiation"] = { "enable": val == "on" }
            case "Duplex":
                match val:
                    case "Half":
                        eth["duplex"] = "half"
                    case "Full":
                        eth["duplex"] = "full"
                    case _:
                        eth["duplex"] = "unknown"
            case "Speed":
                mbps = "".join(filter(str.isdigit, val))
                if mbps:
                    gbps = round((int(mbps) / 1000), 3)
                    eth["speed"] = str(gbps)

    return eth


def ethernet(iplink):
    ethtool = HOST.run_multiline(["ethtool", iplink["ifname"]])

    eth = link(ethtool)

    if stats := statistics(iplink["ifname"]):
        eth["statistics"] = stats

    return eth
