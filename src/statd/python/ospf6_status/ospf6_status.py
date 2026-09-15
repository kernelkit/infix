#!/usr/bin/python3
# Transform the output of the various "show ipv6 ospf6 ..." commands into a
# single structure ordered to match the ietf-ospf YANG model (interfaces
# nested under areas, neighbors nested under interfaces), mirroring what
# ospf_status does for OSPFv2.  FRR's ospf6d JSON uses different key names
# than ospfd, so this is a dedicated reshaper.

import sys
import json
import subprocess


def run_json_cmd(cmd, default=None, check=True):
    """Run a command (array of args) with JSON output and return the JSON"""
    try:
        result = subprocess.run(cmd, check=check, stdout=subprocess.PIPE,
                                stderr=subprocess.PIPE, text=True)
        data = json.loads(result.stdout)
    except (subprocess.CalledProcessError, json.JSONDecodeError):
        if default is not None:
            return default
        raise
    return data


def area_type(area):
    """Derive the ietf-ospf area-type from ospf6d area flags."""
    if area.get("areaIsNSSA"):
        return "nssa-area"
    if area.get("areaIsStub"):
        return "stub-area"
    return "normal-area"


def iter_items(data, wrapper):
    """Yield (name, value) for an object that is either keyed directly by
    name or nested under a wrapper key (e.g. {"interfaces": {...}})."""
    if isinstance(data, dict) and wrapper in data and isinstance(data[wrapper], dict):
        data = data[wrapper]
    if isinstance(data, dict):
        for name, value in data.items():
            if isinstance(value, dict):
                yield name, value


def main():
    top = run_json_cmd(['sudo', 'vtysh', '-c', "show ipv6 ospf6 json"], default={})
    ifaces = run_json_cmd(['sudo', 'vtysh', '-c', "show ipv6 ospf6 interface json"], default={})
    neigh = run_json_cmd(['sudo', 'vtysh', '-c', "show ipv6 ospf6 neighbor json"], default={})

    if not top:
        print(json.dumps({}))
        return

    out = {"routerId": top.get("routerId"), "areas": {}}

    # Seed areas with their type.  In "show ipv6 ospf6 json" the areas are a
    # dict keyed by area-id -- the area-id is the KEY, not a field inside the
    # value -- so read the type flags (areaIsNSSA/areaIsStub) per key.
    areas = top.get("areas", {})
    if isinstance(areas, dict):
        for aid, area in areas.items():
            out["areas"][aid] = {"area-type": area_type(area), "interfaces": []}
    else:
        for area in areas:
            aid = area.get("areaId")
            if aid is not None:
                out["areas"][aid] = {"area-type": area_type(area), "interfaces": []}

    # Collect neighbors, grouped by interface name.  FRR's ospf6 neighbor
    # JSON does not carry an area field, and an interface belongs to exactly
    # one area, so the interface name alone is a unique key.
    nbrs_by_iface = {}
    nlist = neigh.get("neighbors", [])
    if isinstance(nlist, dict):
        nlist = list(nlist.values())
    for n in nlist:
        nbrs_by_iface.setdefault(n.get("interfaceName"), []).append(n)

    # Nest interfaces (with their neighbors) under areas.
    for ifname, iface in iter_items(ifaces, "interfaces"):
        aid = iface.get("areaId")
        if not aid or not iface.get("attachedToArea", True):
            continue
        if aid not in out["areas"]:
            out["areas"][aid] = {"area-type": "normal-area", "interfaces": []}

        iface["name"] = iface.get("interface", ifname)
        iface["neighbors"] = nbrs_by_iface.get(iface["name"], [])
        out["areas"][aid]["interfaces"].append(iface)

    print(json.dumps(out))


if __name__ == "__main__":
    main()
    sys.exit(0)
