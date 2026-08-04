from .common import insert
from .host import HOST


def frr_to_ietf_neighbor_state(state):
    """Fetch OSPF neighbor state from Frr"""
    state = state.split("/")[0]
    # ospfd spells it "TwoWay", ospf6d "Twoway".
    if state.lower() == "twoway":
        return "2-way"
    return state.lower()


def frr_to_ietf_neighbor_role(role):
    """Translate FRR neighbor role to YANG enumeration values"""
    if role == "Backup":
        return "BDR"
    # DR and DROther are already correct
    return role


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

        # Add area information if available
        # Note: augmented by infix-routing.yang since standard ietf-ospf doesn't include it
        # Must use the augmenting module's prefix
        if info.get("area") is not None:
            route["infix-routing:area-id"] = info["area"]

        # Add metric (cost) if available
        if info.get("cost") is not None:
            route["metric"] = info["cost"]
        elif info.get("metric") is not None:
            route["metric"] = info["metric"]

        # Add route-tag for external routes
        if info.get("tag") is not None:
            route["route-tag"] = info["tag"]

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
            elif iface["networkType"] == "BROADCAST":
                interface["interface-type"] = "broadcast"
            elif iface["networkType"] == "POINTOMULTIPOINT":
                if iface.get("p2mpNonBroadcast", False):
                    interface["interface-type"] = "point-to-multipoint"
                else:
                    interface["interface-type"] = "hybrid"
            elif iface["networkType"] == "NBMA":
                interface["interface-type"] = "non-broadcast"

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

            # Interface priority (for DR/BDR election)
            if iface.get("priority") is not None:
                interface["priority"] = iface["priority"]

            # Interface cost
            if iface.get("cost") is not None:
                interface["cost"] = iface["cost"]

            # Configuration timers (in seconds)
            if iface.get("timerDeadSecs") is not None:
                interface["dead-interval"] = iface["timerDeadSecs"]

            if iface.get("timerRetransmitSecs") is not None:
                interface["retransmit-interval"] = iface["timerRetransmitSecs"]

            if iface.get("transmitDelaySecs") is not None:
                interface["transmit-delay"] = iface["transmitDelaySecs"]

            # Hello interval - convert from milliseconds to seconds
            if iface.get("timerMsecs") is not None:
                hello_sec = iface["timerMsecs"] // 1000
                # timer-value-seconds16 requires range 1..65535, use max(1, value)
                if hello_sec >= 1:
                    interface["hello-interval"] = hello_sec

            # Operational state timers (config false)
            # Hello timer - time remaining until next Hello (convert ms to seconds)
            if iface.get("timerHelloInMsecs") is not None:
                hello_timer_sec = iface["timerHelloInMsecs"] // 1000
                # timer-value-seconds16 requires range 1..65535, use max(1, value)
                if hello_timer_sec >= 1:
                    interface["hello-timer"] = hello_timer_sec

            # Wait timer - time until interface exits Waiting state
            if iface.get("timerWaitSecs") is not None:
                wait_sec = iface["timerWaitSecs"]
                # timer-value-seconds16 requires range 1..65535
                if wait_sec >= 1:
                    interface["wait-timer"] = wait_sec

            neighbors = []
            for neigh in iface["neighbors"]:
                neighbor = {}
                neighbor["neighbor-router-id"] = neigh["neighborIp"]
                neighbor["address"] = neigh["ifaceAddress"]

                # Priority - use existing YANG leaf for operational data
                if neigh.get("nbrPriority") is not None:
                    neighbor["priority"] = neigh["nbrPriority"]

                # Uptime - convert from milliseconds to seconds
                # Note: augmented by infix-routing.yang
                # Use lastPrgrsvChangeMsec from detail output (time since last progressive state change)
                if neigh.get("lastPrgrsvChangeMsec") is not None:
                    uptime_sec = neigh["lastPrgrsvChangeMsec"] // 1000
                    neighbor["infix-routing:uptime"] = uptime_sec

                # Dead timer - convert from milliseconds to seconds
                # timer-value-seconds16 requires range 1..65535
                if neigh.get("routerDeadIntervalTimerDueMsec") is not None:
                    dead_timer_sec = neigh["routerDeadIntervalTimerDueMsec"] // 1000
                    if dead_timer_sec >= 1:
                        neighbor["dead-timer"] = dead_timer_sec

                neighbor["state"] = frr_to_ietf_neighbor_state(neigh["nbrState"])

                # Store role (DR/BDR/DROther) for display
                # Note: augmented by infix-routing.yang
                if neigh.get("role"):
                    neighbor["infix-routing:role"] = frr_to_ietf_neighbor_role(neigh["role"])

                # Store interface name with local address (e.g., "e5:10.0.23.1")
                # Note: augmented by infix-routing.yang
                # Compose from ifaceName and localIfaceAddress
                if neigh.get("ifaceName") and neigh.get("localIfaceAddress"):
                    neighbor["infix-routing:interface-name"] = f"{neigh['ifaceName']}:{neigh['localIfaceAddress']}"
                elif neigh.get("ifaceName"):
                    neighbor["infix-routing:interface-name"] = neigh["ifaceName"]

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


def ospf6_interface_type(op):
    """Map ospf6d operatingAsType to an ietf-ospf interface-type.

    ospf6d has no non-broadcast/NBMA network type, and its point-to-multipoint
    is always multicast, which corresponds to Infix's 'hybrid' type (matching
    how OSPFv2 maps non-NBMA point-to-multipoint)."""
    xlate = {
        "BROADCAST":        "broadcast",
        "POINTOPOINT":      "point-to-point",
        "POINTOMULTIPOINT": "hybrid",
    }
    return xlate.get(op)


def add_routes6(ospf):
    """Fetch OSPFv3 routes from ospf6d for the OSPF local-rib view."""
    data = HOST.run_json(['vtysh', '-c', "show ipv6 ospf6 route json"], default={})
    # ospf6d abbreviates the path type: IA=intra-area, IE=inter-area,
    # E1/E2=external type 1/2.
    path_type = {
        "IA": "intra-area",
        "IE": "inter-area",
        "E1": "external-1",
        "E2": "external-2",
    }

    routes = []
    for prefix, paths in data.get("routes", {}).items():
        if "/" not in prefix or not paths:
            continue

        # ospf6d lists one entry per path; keep the installed (best) one so
        # the local-rib list stays keyed uniquely by prefix.
        entry = next((p for p in paths if p.get("isBestRoute")), paths[0])

        route = {"prefix": prefix}
        rtype = path_type.get(entry.get("pathType"))
        if rtype:
            route["route-type"] = rtype

        nexthops = []
        for hop in entry.get("nextHops", []):
            nexthop = {}
            # "::" marks a directly-connected prefix (no gateway).
            if hop.get("nextHop") and hop["nextHop"] != "::":
                nexthop["next-hop"] = hop["nextHop"]
            elif hop.get("interfaceName"):
                nexthop["outgoing-interface"] = hop["interfaceName"]
            if nexthop:
                nexthops.append(nexthop)
        if nexthops:
            route["next-hops"] = {"next-hop": nexthops}

        routes.append(route)

    if routes:
        insert(ospf, "ietf-ospf:local-rib", "ietf-ospf:route", routes)


def add_areas6(control_protocols):
    """Populate OSPFv3 (ospf6d) operational status as a second
    control-plane-protocol of type infix-routing:ospfv3."""
    data = HOST.run_json(['/usr/libexec/statd/ospf6-status'], default={})
    if data == {}:
        return  # No OSPFv3 data available (ospf6d not running)

    control_protocol = {}
    control_protocol["type"] = "infix-routing:ospfv3"
    control_protocol["name"] = "default"
    control_protocol["ietf-ospf:ospf"] = {}
    control_protocol["ietf-ospf:ospf"]["ietf-ospf:areas"] = {}
    control_protocol["ietf-ospf:ospf"]["ietf-ospf:router-id"] = data.get("routerId")
    control_protocol["ietf-ospf:ospf"]["ietf-ospf:address-family"] = "ipv6"

    state_xlate = {
        "Down":           "down",
        "Waiting":        "waiting",
        "Loopback":       "loopback",
        "PointToPoint":   "point-to-point",
        "DROther":        "dr-other",
        "BDR":            "bdr",
        "DR":             "dr",
    }

    areas = []
    for area_id, values in data.get("areas", {}).items():
        area = {}
        area["ietf-ospf:area-id"] = area_id
        area["ietf-ospf:interfaces"] = {}
        if values.get("area-type"):
            area["ietf-ospf:area-type"] = values["area-type"]

        interfaces = []
        for iface in values.get("interfaces", []):
            interface = {}
            interface["name"] = iface["name"]
            interface["enabled"] = True

            # FRR ospf6 reports the *operating* OSPF network type in
            # "operatingAsType" (BROADCAST/POINTOPOINT/POINTOMULTIPOINT);
            # "type" is the L2 type (always BROADCAST for ethernet).
            itype = ospf6_interface_type(iface.get("operatingAsType") or iface.get("type"))
            if itype:
                interface["interface-type"] = itype

            interface["passive"] = bool(iface.get("timerPassiveIface"))

            if iface.get("cost") is not None:
                interface["cost"] = iface["cost"]
            if iface.get("priority") is not None:
                interface["priority"] = iface["priority"]

            # Only set state when it maps to a valid ietf-ospf enum; ospf6
            # has states (e.g. "PtMultipoint") with no ietf-ospf equivalent,
            # and emitting an invalid value makes sysrepo reject the whole
            # operational tree (HTTP 500 on the RESTCONF GET).
            st = state_xlate.get(iface.get("ospf6InterfaceState"))
            if st:
                interface["state"] = st

            if iface.get("timerIntervalsConfigDead") is not None:
                interface["dead-interval"] = iface["timerIntervalsConfigDead"]
            if iface.get("timerIntervalsConfigRetransmit") is not None:
                interface["retransmit-interval"] = iface["timerIntervalsConfigRetransmit"]
            if iface.get("transmitDelaySec") is not None:
                interface["transmit-delay"] = iface["transmitDelaySec"]
            if iface.get("timerIntervalsConfigHello") is not None:
                interface["hello-interval"] = iface["timerIntervalsConfigHello"]

            neighbors = []
            for neigh in iface.get("neighbors", []):
                neighbor = {}
                neighbor["neighbor-router-id"] = neigh.get("neighborId")
                if neigh.get("linkLocalAddress"):
                    neighbor["address"] = neigh["linkLocalAddress"]
                if neigh.get("priority") is not None:
                    neighbor["priority"] = neigh["priority"]
                # FRR ospf6 neighbor JSON reports the adjacency state in "state".
                if neigh.get("state"):
                    neighbor["state"] = frr_to_ietf_neighbor_state(neigh["state"])
                neighbors.append(neighbor)

            interface["ietf-ospf:neighbors"] = {}
            interface["ietf-ospf:neighbors"]["ietf-ospf:neighbor"] = neighbors
            interfaces.append(interface)

        area["ietf-ospf:interfaces"]["ietf-ospf:interface"] = interfaces
        areas.append(area)

    add_routes6(control_protocol["ietf-ospf:ospf"])
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
    add_areas6(out['ietf-routing:routing']['control-plane-protocols'])
    return out
