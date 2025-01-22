from functools import cache

from ..common import LOG, YangDate
from ..host import HOST

from .common import iplinks, iplinks_lower_of

from . import vlan


@cache
def bridge_vlan():
    return { v["ifname"]: v for v in HOST.run_json("bridge -j vlan show".split(), []) }

def lower(iplink):
    lower = {}

    if brpvlans := bridge_vlan().get(iplink["ifname"]):
        for brpvlan in brpvlans["vlans"]:
            if "PVID" in brpvlan.get("flags", []):
                lower["pvid"] = brpvlan["vlan"]

    if iplink.get("linkinfo", {}).get("info_kind") == "bridge":
        return lower

    info = iplink["linkinfo"]["info_slave_data"]

    return lower | {
        "bridge": iplink["master"],
        "flood": {
            "broadcast": info["bcast_flood"],
            "unicast": info["flood"],
            "multicast": info["mcast_flood"],
        },

        "multicast": {
            "fast-leave": info["fastleave"],
            "router": {
                0: "off",
                1: "auto",
                2: "permanent",
            }.get(info["multicast_router"], "UNKNOWN"),
        },

        "stp-state": info["state"],
    }


def stp_bridge_id(idstr):
    segs = idstr.split(".")
    return {
        "priority": int(segs[0], 16),
        "system-id": int(segs[1], 16),
        "address": segs[2].lower(),
    }

def stp_tree(brname, mst):
    if not (state := HOST.run_json(["mstpctl", "-f", "json", "showtree", brname, str(mst)]), {}):
        return {}

    tree = {
        "priority": int(state["bridge-id"][0], 16),
        "bridge-id": stp_bridge_id(state["bridge-id"]),
    }

    if rport := state.get("root-port"):
        tree["root-port"] = rport.split()[0]

    if state.get("topology-change-count"):
        tree["topology-change"] = {
            "count": int(state["topology-change-count"]),
            "in-progress": True if state["topology-change"] == "yes" else False,
            "port": state["topology-change-port"],
            "time": str(YangDate.from_seconds(int(state["time-since-topology-change"]))),
        }

    return tree


def stp(iplink):
    state = HOST.run_json(["mstpctl", "-f", "json", "showbridge", iplink["ifname"]],
                          default=[{}])[0]
    if not state:
        return {}

    stp = {
        "force-protocol": state["force-protocol-version"],
        "forward-delay": int(state["forward-delay"]),
        "max-age": int(state["max-age"]),
        "transmit-hold-count": int(state["tx-hold-count"]),
        "max-hops": int(state["max-hops"]),

        "cist": stp_tree(iplink["ifname"], 0),
    }

    # This information ought to be available in `showtree`, so it is
    # still an open question how we should source the per-tree root id
    # when adding MSTP support
    if state.get("designated-root"):
        stp["cist"]["root-id"] = stp_bridge_id(state["designated-root"])

    return stp


def mctlq2yang_mode(mctlq):
    if state := mctlq.get("state"):
        return "proxy" if state == "proxy" else "auto"

    return "off"


def mctl(ifname, vid):
    mctl = HOST.run_json(["mctl", "-p", "show", "igmp", "json"], default={})

    for q in mctl.get("multicast-queriers", []):
        # TODO: Also need to match against VLAN uppers (e.g. br0.1337)
        if q.get("interface") == ifname and q.get("vid") == vid:
            return q

    return {}


def multicast_filters(iplink, vid):
    filt = ["dev", iplink["ifname"]] + (["vid", str(vid)] if vid else [])
    brmdb = HOST.run_json(["bridge", "-j", "mdb", "show"] + filt)[0]["mdb"]

    mdb = {}
    for brentry in brmdb:
        if not (entry := mdb.get(brentry["grp"])):
            mdb[brentry["grp"]] = {
                "group": brentry["grp"],
                "ports": [],
            }
            entry = mdb[brentry["grp"]]

        entry["ports"].append({
            "port": brentry["port"],
            "state": {
                "temp": "temporary",
                "permanent": "permanent"
            }.get(brentry["state"], "UNKNOWN"),
        })

    return { "multicast-filter": list(mdb.values()) }


def multicast(iplink, info):
    mctlq = mctl(iplink["ifname"], info.get("vlan"))

    mcast = {
        "snooping": bool(info.get("mcast_snooping")),
        "querier": mctlq2yang_mode(mctlq),
    }

    if interval := mctlq.get("interval"):
        mcast["query-interval"] = interval

    return mcast


def vlans_add_memberships(iplink, vlans):
    brvlans = bridge_vlan()
    ports = [iplink["ifname"]] + [link["ifname"] for link in iplinks_lower_of(iplink["ifname"]).values()]

    for port in ports:
        if not (brpvlans := brvlans.get(port)):
            continue

        for brpvlan in brpvlans["vlans"]:
            if not (vlan := vlans.get(brpvlan["vlan"])):
                LOG.error(f"Unexpected vlan {brpvlans['vlan']} on {port}")
                continue

            if "Egress Untagged" in brpvlan.get("flags", []):
                vlan["untagged"].append(port)
            else:
                vlan["tagged"].append(port)


def vlans(iplink):
    if not (brgvlans := HOST.run_json(f"bridge -j vlan global show dev {iplink['ifname']}".split())):
        return []

    vlans = {
        v["vlan"]: {
            "vid": v["vlan"],
            "untagged": [],
            "tagged": [],

            "multicast": multicast(iplink, v),
            "multicast-filters": multicast_filters(iplink, v["vlan"]),
        }
        for v in brgvlans[0]["vlans"]
    }

    vlans_add_memberships(iplink, vlans)
    return list(vlans.values())


def qbridge(iplink):
    info = iplink["linkinfo"]["info_data"]

    return {
        "vlans": {
            "proto": vlan.proto2yang(info["vlan_protocol"]),
            "vlan": vlans(iplink),
        }
    }


def dbridge(iplink):
    info = iplink["linkinfo"]["info_data"]

    return {
        "multicast": multicast(iplink, info),
        "multicast-filters": multicast_filters(iplink, None),
    }


def bridge(iplink):
    info = iplink["linkinfo"]["info_data"]

    if info.get("vlan_filtering"):
        br = qbridge(iplink)
    else:
        br = dbridge(iplink)

    if info.get("stp_state"):
        br["stp"] = stp(iplink)

    return br
