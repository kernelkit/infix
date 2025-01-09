from .common import insert
from .host import HOST


def frr_to_ietf_neighbor_state(state):
    """Fetch OSPF neighbor state from Frr"""
    state = state.split("/")[0]
    if state == "TwoWay":
        return "2-way"
    return state.lower()


def add_routes(ospf):
    """Fetch OSPF routes from Frr"""
    cmd = ['vtysh', '-c', "show ip ospf route json"]
    data = HOST.run_json(cmd, default=[])
    if data == []:
        return  # No OSPF routes available

    routes = []
    for prefix, info in data.items():
        if prefix.find("/") == -1:  # Ignore router IDs
            continue

        route = {}
        route["prefix"] = prefix

        nexthops = []
        routetype = info["routeType"].split(" ")

        if len(routetype) > 1:
            if routetype[1] == "E1":
                route["route-type"] = "external-1"
            elif routetype[1] == "E2":
                route["route-type"] = "external-2"
            elif routetype[1] == "IA":
                route["route-type"] = "inter-area"
        elif routetype[0] == "N":
            route["route-type"] = "intra-area"

        for hop in info["nexthops"]:
            nexthop = {}
            if hop["ip"] != " ":
                nexthop["next-hop"] = hop["ip"]
            else:
                nexthop["outgoing-interface"] = hop["directlyAttachedTo"]
            nexthops.append(nexthop)

        route["next-hops"] = {}
        route["next-hops"]["next-hop"] = nexthops
        routes.append(route)

    insert(ospf, "ietf-ospf:local-rib", "ietf-ospf:route", routes)


def add_areas(control_protocols):
    """Populate OSPF status"""
    cmd = ['/usr/libexec/statd/ospf-status']
    data = HOST.run_json(cmd, default={})
    if data == {}:
        return  # No OSPF data available

    control_protocol = {}
    control_protocol["type"] = "infix-routing:ospfv2"
    control_protocol["name"] = "default"
    control_protocol["ietf-ospf:ospf"] = {}
    control_protocol["ietf-ospf:ospf"]["ietf-ospf:areas"] = {}


    control_protocol["ietf-ospf:ospf"]["ietf-ospf:router-id"] = data.get("routerId")
    control_protocol["ietf-ospf:ospf"]["ietf-ospf:address-family"] = "ipv4"
    areas = []

    for area_id, values in data.get("areas", {}).items():
        area = {}
        area["ietf-ospf:area-id"] = area_id
        area["ietf-ospf:interfaces"] = {}
        if values.get("area-type"):
            area["ietf-ospf:area-type"] = values["area-type"]
        interfaces = []
        for iface in values.get("interfaces", {}):
            interface = {}
            interface["ietf-ospf:neighbors"] = {}
            interface["name"] = iface["name"]

            if iface.get("drId"):
                interface["dr-router-id"] = iface["drId"]
            if iface.get("drAddress"):
                interface["dr-ip-addr"] = iface["drAddress"]
            if iface.get("bdrId"):
                interface["bdr-router-id"] = iface["bdrId"]
            if iface.get("bdrAddress"):
                interface["bdr-ip-addr"] = iface["bdrAddress"]

            if iface.get("timerPassiveIface"):
                interface["passive"] = True
            else:
                interface["passive"] = False

            interface["enabled"] = iface["ospfEnabled"]
            if iface["networkType"] == "POINTOPOINT":
                interface["interface-type"] = "point-to-point"
            if iface["networkType"] == "BROADCAST":
                interface["interface-type"] = "broadcast"

            if iface.get("state"):
                # Wev've never seen "DependUpon", and has no entry in
                # the YANG model, but is listed before down in Frr
                xlate = {
                    "DependUpon":     "down",
                    "Down":           "down",
                    "Waiting":        "waiting",
                    "Loopback":       "loopback",
                    "Point-To-Point": "point-to-point",
                    "DROther":        "dr-other",
                    "Backup":         "bdr",
                    "DR":             "dr"
                }
                val = xlate.get(iface["state"], "unknown")
                interface["state"] = val

            neighbors = []
            for neigh in iface["neighbors"]:
                neighbor = {}
                neighbor["neighbor-router-id"] = neigh["neighborIp"]
                neighbor["address"] = neigh["ifaceAddress"]
                neighbor["dead-timer"] = neigh["routerDeadIntervalTimerDueMsec"]
                neighbor["state"] = frr_to_ietf_neighbor_state(neigh["nbrState"])
                if neigh.get("routerDesignatedId"):
                    neighbor["dr-router-id"] = neigh["routerDesignatedId"]
                if neigh.get("routerDesignatedBackupId"):
                    neighbor["bdr-router-id"] = neigh["routerDesignatedBackupId"]
                neighbors.append(neighbor)

            interface["ietf-ospf:neighbors"] = {}
            interface["ietf-ospf:neighbors"]["ietf-ospf:neighbor"] = neighbors
            interfaces.append(interface)

        area["ietf-ospf:interfaces"]["ietf-ospf:interface"] = interfaces
        areas.append(area)

    add_routes(control_protocol["ietf-ospf:ospf"])
    control_protocol["ietf-ospf:ospf"]["ietf-ospf:areas"]["ietf-ospf:area"] = areas
    insert(control_protocols, "control-plane-protocol", [control_protocol])


def operational():
    out = {
        "ietf-routing:routing": {
            "control-plane-protocols": {
            }
        }
    }

    add_areas(out['ietf-routing:routing']['control-plane-protocols'])
    return out
