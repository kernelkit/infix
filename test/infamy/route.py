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

        if active_check and "active" not in r:
            continue

        return True

    return False


def ipv4_route_exist(target, dest, nexthop=None, proto=None, pref=None, active_check=False):
    return _exist_route(target, dest, nexthop=nexthop, ip="ipv4", proto=proto, pref=pref, active_check=active_check)


def ipv6_route_exist(target, dest, nexthop=None, proto=None, pref=None, active_check=False):
    return _exist_route(target, dest, nexthop=nexthop, ip="ipv6", proto=proto, pref=pref, active_check=active_check)


def _get_ospf_status(target):
    xpath = "/ietf-routing:routing/control-plane-protocols"
    protos = target.get_data(xpath)["routing"]["control-plane-protocols"]
    rib = protos.get("control-plane-protocol", {})
    for p in rib:
        if p["type"] == "infix-routing:ospfv2":
            return p.get("ospf") or p.get("ietf-ospf:ospf", {})

    return {}


def _get_ospf_status_area(target, area_id):
    ospf = _get_ospf_status(target)
    for area in ospf.get("areas", {}).get("area", {}):
        if area["area-id"] == area_id:
            return area

    return {}


def _get_ospf_status_area_interface(target, area_id, ifname):
    area = _get_ospf_status_area(target, area_id)
    for interface in area.get("interfaces", {}).get("interface", {}):
        if interface.get("name") == ifname:
            return interface

    return {}


def ospf_get_neighbor(target, area_id, ifname, neighbour_id, full=True):
    ospf_interface = _get_ospf_status_area_interface(target, area_id, ifname)
    for neighbor in ospf_interface.get("neighbors", {}).get("neighbor", {}):
        if neighbor.get("neighbor-router-id") == neighbour_id:
            if full is False:
                return True
            if neighbor.get("state") == "full":
                return True

    return False


def ospf_get_interface_type(target, area_id, ifname):
    ospf_interface = _get_ospf_status_area_interface(target, area_id, ifname)
    return ospf_interface.get("interface-type", None)


def ospf_get_interface_passive(target, area_id, ifname):
    ospf_interface = _get_ospf_status_area_interface(target, area_id, ifname)
    return ospf_interface.get("passive", False)


def ospf_is_area_nssa(target, area_id):
    area = _get_ospf_status_area(target, area_id)
    if area.get("area-type", "") == "ietf-ospf:nssa-area":
        return True

    return False
