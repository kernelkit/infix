from functools import cache

from ..common import LOG, YangDate
from ..host import HOST

from .common import iplinks, iplinks_lower_of

from . import vlan


@cache
def bridge_vlan():
    return { v["ifname"]: v for v in HOST.run_json("bridge -j vlan show".split(), []) }


def stp_bridge_id(idstr):
    segs = idstr.split(".")
    return {
        "priority": int(segs[0], 16),
        "system-id": int(segs[1], 16),
        "address": segs[2].lower(),
    }


def stp_port_id(idstr):
    segs = idstr.split(".")
    return {
        "priority": int(segs[0], 16),
        "port-id": int(segs[1], 16),
    }


def stp_port_statistics(port):
    STATMAP = {
        "in-bpdus": "num-rx-bpdu",
        "in-bpdus-filtered": "num-rx-bpdu-filtered",
        "in-tcns": "num-rx-tcn",

        "out-bpdus": "num-tx-bpdu",
        "out-tcns": "num-tx-tcn",

        "to-blocking": "num-transition-blk",
        "to-forwarding": "num-transition-fwd",
    }

    stats = {}
    for yname , pname in STATMAP.items():
        if pstat := port.get(pname):
            # Do _not_ convert pstat to int, since the JSON encoding
            # of >32bit YANG types are strings.
            stats[yname] = pstat

    return stats


def lower_stp_tree(iplink, msti):
    if not (tport := HOST.run_json(["mstpctl", "-f", "json",
                                   "showtreeport",
                                    iplink["master"], iplink["ifname"], str(msti)],
                                   default={})):
        return {}

    port = {
        "port-id": stp_port_id(tport["port-id"]),
        # "state": None # Sourced from bridge
        "role": tport["role"].lower(),
        "disputed": tport["disputed"] == "yes",
    }

    designated = {}
    if dbr := tport["designated-bridge"]:
        designated["bridge-id"] = stp_bridge_id(dbr)

    if dp := tport["designated-port"]:
        designated["port-id"] = stp_port_id(dp)

    if designated:
        port["designated"] = designated

    return port


def lower_stp(iplink):
    info = iplink["linkinfo"]["info_slave_data"]
    ciststate = info.get("state", "disabled")

    if not (port := HOST.run_json(["mstpctl", "-f", "json",
                                   "showportdetail",
                                   iplink["master"], iplink["ifname"]],
                                  default=[{}])[0]):
        return {
            "cist": {
                "state": ciststate,
            }
        }

    stp = {
        "edge": port.get("oper-edge-port") == "yes",
        "cist": lower_stp_tree(iplink, 0),
    }

    stp["cist"]["state"] = ciststate

    if ecost := port.get("external-path-cost"):
        stp["cist"]["external-path-cost"] = ecost

    if stats := stp_port_statistics(port):
        stp["statistics"] = stats

    return stp


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

        "stp": lower_stp(iplink),
    }


def stp_tree(brname, mst):
    if not (state := HOST.run_json(["mstpctl", "-f", "json",
                                    "showtree",
                                    brname, str(mst)],
                                   default={})):
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
    state = HOST.run_json(["mstpctl", "-f", "json",
                           "showbridge",
                           iplink["ifname"]], default=[{}])[0]
    if not state:
        return {}

    stp = {
        "force-protocol": state["force-protocol-version"],
        "hello-time": int(state["hello-time"]),
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


def mctl_queriers():
    """Fetch all IGMP multicast querier data in one call"""
    return HOST.run_json(["mctl", "-p", "show", "igmp", "json"], default={})


def mctl(ifname, vid, mctldata, key="multicast-queriers", upper=None):
    """Querier of a bridge, or bridge VLAN, also when run on its VLAN upper"""
    for q in mctldata.get(key, []):
        if q.get("interface") == ifname and q.get("vid") == vid:
            return q
        if upper and q.get("interface") == upper and q.get("vid") is None:
            return q

    return {}


def vlan_uppers(brname):
    """Map VID to the VLAN interface on top of the bridge"""
    uppers = {}
    for link in iplinks().values():
        info = link.get("linkinfo", {})
        if link.get("link") == brname and info.get("info_kind") == "vlan":
            uppers[info["info_data"]["id"]] = link["ifname"]

    return uppers


def active_queriers(ifname, vid, mctldata, upper=None):
    queriers = []
    for key in ("multicast-queriers", "mld-queriers"):
        q = mctl(ifname, vid, mctldata, key, upper)
        if q.get("state") not in ("up", "proxy"):
            continue

        querier = {
            "family": q["family"],
            "address": q["querier"],
            "local": "timeout" not in q,
            "version": q["version"],
            "query-interval": q["interval"],
        }
        if not querier["local"]:
            querier["timeout"] = q["timeout"]
        queriers.append(querier)

    return queriers


def mctl_router_ports(brname, mctldata):
    for entry in mctldata.get("multicast-router-ports", []):
        if entry.get("bridge") == brname:
            return entry.get("ports", [])
    return []


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


def fdb_state(entry):
    return {
        "permanent": "local",
        "static": "static",
    }.get(entry.get("state"), "dynamic")


def forwarding_database(iplink):
    """Bridge FDB containers, keyed by VID (None on VLAN unaware bridges)"""
    brname = iplink["ifname"]
    swports = {name for name, link in iplinks_lower_of(brname).items()
               if "phys_switch_id" in link}
    brfdb = HOST.run_json(["bridge", "-s", "-j", "fdb", "show", "br", brname], [])

    fdb = {}
    for brentry in brfdb:
        # Group addresses belong in the MDB, the model only allows unicast
        if int(brentry["mac"][:2], 16) & 1:
            continue

        bridged = brentry.get("master") == brname
        # A switch port's self entries are its hardware FDB
        if not bridged and (brentry["ifname"] not in swports
                            or brentry.get("state") == "permanent"):
            continue

        entry = {
            "address": brentry["mac"],
            "port": brentry["ifname"],
            "state": fdb_state(brentry),
            "offloaded": not bridged or "offload" in brentry.get("flags", []),
        }
        if entry["state"] == "dynamic" and "updated" in brentry:
            entry["age"] = brentry["updated"]

        # The bridge's own entry wins over the switch's copy of it
        entries = fdb.setdefault(brentry.get("vlan"), {})
        if bridged or entry["address"] not in entries:
            entries[entry["address"]] = entry

    return {vid: {"entry": list(entries.values())} for vid, entries in fdb.items()}


def multicast(iplink, info, mctldata, upper=None):
    mctlq = mctl(iplink["ifname"], info.get("vlan"), mctldata, upper=upper)

    mcast = {
        "snooping": bool(info.get("mcast_snooping")),
        "querier": mctlq2yang_mode(mctlq),
    }

    if interval := mctlq.get("interval"):
        mcast["query-interval"] = interval

    if ports := mctl_router_ports(iplink["ifname"], mctldata):
        mcast["router-ports"] = ports

    if queriers := active_queriers(iplink["ifname"], info.get("vlan"), mctldata, upper):
        mcast["active-querier"] = queriers

    return mcast


def vlans_add_memberships(iplink, vlans):
    brvlans = bridge_vlan()
    ports = [iplink["ifname"]] + [link["ifname"] for link in iplinks_lower_of(iplink["ifname"]).values()]

    for port in ports:
        if not (brpvlans := brvlans.get(port)):
            continue

        for brpvlan in brpvlans["vlans"]:
            vid = brpvlan["vlan"]
            if not (vlan := vlans.get(vid)):
                LOG.error(f"Unexpected vlan {vid} on {port}")
                continue

            if "Egress Untagged" in brpvlan.get("flags", []):
                vlan["untagged"].append(port)
            else:
                vlan["tagged"].append(port)


def vlans(iplink):
    if not (brgvlans := HOST.run_json(f"bridge -j vlan global show dev {iplink['ifname']}".split())):
        return []

    mctldata = mctl_queriers()
    fdb = forwarding_database(iplink)
    uppers = vlan_uppers(iplink["ifname"])

    vlans = {}
    for v in brgvlans[0]["vlans"]:
        start = v["vlan"]
        end = v.get("vlanEnd", start)
        for vid in range(start, end + 1):
            vlans[vid] = {
                "vid": vid,
                "untagged": [],
                "tagged": [],
                "multicast": multicast(iplink, v, mctldata, uppers.get(vid)),
                "multicast-filters": multicast_filters(iplink, vid),
            }
            if vfdb := fdb.get(vid):
                vlans[vid]["forwarding-database"] = vfdb

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

    br = {
        "multicast": multicast(iplink, info, mctl_queriers()),
        "multicast-filters": multicast_filters(iplink, None),
    }
    if fdb := forwarding_database(iplink).get(None):
        br["forwarding-database"] = fdb

    return br


def bridge(iplink):
    info = iplink["linkinfo"]["info_data"]

    if info.get("vlan_filtering"):
        br = qbridge(iplink)
    else:
        br = dbridge(iplink)

    br["aging-time"] = info["ageing_time"] // 100

    if info.get("stp_state"):
        br["stp"] = stp(iplink)

    return br
