"""
IETF Routing helper methods
"""


def _get_routes(target, protocol):
    xpath = "/ietf-routing:routing/ribs"
    rib = target.get_data(xpath)["routing"]["ribs"]["rib"]
    for r in rib:
        if r["name"] != protocol:
            continue
        return r.get("routes", {}).get("route", {})
    return {}

def _installed(route):
    """True if at least one next-hop of the route is installed in the FIB"""
    nh = route.get("next-hop", {})
    hops = nh.get("next-hop-list", {}).get("next-hop", [nh])
    return any("installed" in h or "infix-routing:installed" in h for h in hops)


def _exist_route(target, dest, nexthop=None, ip=None, proto=None, pref=None, active_check=False):
    routes = _get_routes(target, ip)
    for r in routes:
        # netconf presents destination-prefix, restconf prefix with model
        dst = r.get("destination-prefix") or r.get(f"ietf-{ip}-unicast-routing:destination-prefix")
        if dst != dest:
            continue

        if proto is not None and r.get("source-protocol") != proto:
            continue

        if pref is not None and r.get("route-preference") != pref:
            continue

        if nexthop is not None:
            nh = r.get("next-hop")
            if not nh:
                continue

            next_hop_list = nh.get("next-hop-list")
            if next_hop_list:
                if not any(nhl.get("address") == nexthop or nhl.get(f"ietf-{ip}-unicast-routing:address") == nexthop
                           for nhl in next_hop_list["next-hop"]):
                    continue
            else:
                nh_addr = nh.get("next-hop-address") or nh.get(f"ietf-{ip}-unicast-routing:next-hop-address")
                if nh_addr != nexthop:
                    continue

        if active_check and ("active" not in r or not _installed(r)):
            continue

        return True

    return False


def ipv4_route_exist(target, dest, nexthop=None, proto=None, pref=None, active_check=False):
    return _exist_route(target, dest, nexthop=nexthop, ip="ipv4", proto=proto, pref=pref, active_check=active_check)


def ipv6_route_exist(target, dest, nexthop=None, proto=None, pref=None, active_check=False):
    return _exist_route(target, dest, nexthop=nexthop, ip="ipv6", proto=proto, pref=pref, active_check=active_check)


def route_exist(target, dest, af="ipv4", nexthop=None, proto=None, pref=None, active_check=False):
    """Address family agnostic route lookup, for tests running both IPv4 and IPv6"""
    return _exist_route(target, dest, nexthop=nexthop, ip=af, proto=proto, pref=pref, active_check=active_check)


def _get_ospf_status(target, proto="infix-routing:ospfv2"):
    xpath = "/ietf-routing:routing/control-plane-protocols"
    protos = target.get_data(xpath)["routing"]["control-plane-protocols"]
    rib = protos.get("control-plane-protocol", {})
    for p in rib:
        if p["type"] == proto:
            return p.get("ospf") or p.get("ietf-ospf:ospf", {})

    return {}


def _get_ospf_status_area(target, area_id, proto="infix-routing:ospfv2"):
    ospf = _get_ospf_status(target, proto)
    for area in ospf.get("areas", {}).get("area", {}):
        if area["area-id"] == area_id:
            return area

    return {}


def _get_ospf_status_area_interface(target, area_id, ifname, proto="infix-routing:ospfv2"):
    area = _get_ospf_status_area(target, area_id, proto)
    for interface in area.get("interfaces", {}).get("interface", {}):
        if interface.get("name") == ifname:
            return interface

    return {}


def ospf_get_neighbor(target, area_id, ifname, neighbour_id, full=True, proto="infix-routing:ospfv2"):
    ospf_interface = _get_ospf_status_area_interface(target, area_id, ifname, proto)
    for neighbor in ospf_interface.get("neighbors", {}).get("neighbor", {}):
        if neighbor.get("neighbor-router-id") == neighbour_id:
            if full is False:
                return True
            if neighbor.get("state") == "full":
                return True

    return False


def ospf_get_interface_type(target, area_id, ifname, proto="infix-routing:ospfv2"):
    ospf_interface = _get_ospf_status_area_interface(target, area_id, ifname, proto)
    return ospf_interface.get("interface-type", None)


def ospf_get_interface_passive(target, area_id, ifname, proto="infix-routing:ospfv2"):
    ospf_interface = _get_ospf_status_area_interface(target, area_id, ifname, proto)
    return ospf_interface.get("passive", False)


def ospf_is_area_nssa(target, area_id, proto="infix-routing:ospfv2"):
    area = _get_ospf_status_area(target, area_id, proto)
    if area.get("area-type", "") == "ietf-ospf:nssa-area":
        return True

    return False


def ospf_has_neighbors(target, proto="infix-routing:ospfv2"):
    ospf = _get_ospf_status(target, proto)
    for area in ospf.get("areas", {}).get("area", []):
        for interface in area.get("interfaces", {}).get("interface", []):
            if interface.get("neighbors"):
                return True

    return False


def _get_bfd_sessions(target):
    xpath = "/ietf-routing:routing/control-plane-protocols"
    protos = target.get_data(xpath)["routing"]["control-plane-protocols"]
    for p in protos.get("control-plane-protocol", {}):
        if p["type"] == "infix-routing:bfdv1":
            bfd = p.get("bfd") or p.get("ietf-bfd:bfd", {})
            ip_sh = bfd.get("ip-sh") or bfd.get("ietf-bfd-ip-sh:ip-sh", {})
            return ip_sh.get("sessions", {}).get("session", [])

    return []


def bfd_session_up(target, peer):
    for session in _get_bfd_sessions(target):
        if session.get("dest-addr") != peer:
            continue
        running = session.get("session-running", {})
        return running.get("local-state") == "up"

    return False


def bfd_sessions_up(target):
    """Number of running BFD sessions, for peers not known in advance"""
    up = 0
    for session in _get_bfd_sessions(target):
        if session.get("session-running", {}).get("local-state") == "up":
            up += 1

    return up
