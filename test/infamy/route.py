def _get_routes(target, protocol):
    xpath="/ietf-routing:routing/ribs"
    rib = target.get_data(xpath)["routing"]["ribs"]["rib"]
    for r in rib:
        if r["name"] != protocol:
            continue
        return r.get("routes", {}).get("route",{})
    return {}

def _exist_route(target, prefix, nexthop, version, source_protocol):
    routes = _get_routes(target, version)
    for r in routes:
        if r["destination-prefix"] != prefix:
            continue

        if not nexthop and not source_protocol: # Only want the route to exist
            return True

        if source_protocol and r.get("source-protocol") == source_protocol:
            return True

        nh = r["next-hop"]
        next_hop_list = nh.get("next-hop-list")
        if nexthop:
            if next_hop_list:
                for nhl in next_hop_list["next-hop"]:
                    address = nhl.get("address")
                    if address == nexthop:
                        return True
            else:
                if nh["next-hop-address"] == nexthop:
                    return True
    return False

def ipv4_route_exist(target, prefix, nexthop=None,source_protocol=None):
    return _exist_route(target, prefix, nexthop, "ipv4",source_protocol)

def ipv6_route_exist(target, prefix, nexthop=None,source_protocol=None):
    return _exist_route(target, prefix, nexthop, "ipv6",source_protocol)


def _get_ospf_status(target):
    xpath="/ietf-routing:routing/control-plane-protocols/control-plane-protocol/ietf-ospf:ospf"
    rib = target.get_data(xpath)["routing"]["control-plane-protocols"]["control-plane-protocol"]
    for p in rib:
        if p["type"] == "ietf-ospf:ospfv2":
            return p["ospf"]

def _get_ospf_status_area(target, area_id):
    ospf=_get_ospf_status(target)
    for area in ospf.get("areas").get("area"):
        if area["area-id"] == area_id:
            return area
    return {}

def _get_ospf_status_area_interface(target, area_id, ifname):
    area=_get_ospf_status_area(target,area_id)
    if area is None:
        return {}

    for interface in area.get("interfaces").get("interface"):
        if interface["name"] == ifname:
            return interface
    return {}

def ospf_get_neighbor(target, area_id, ifname, neighbour_id, full=True):
    ospf_interface=_get_ospf_status_area_interface(target,area_id, ifname)
    for neighbor in ospf_interface.get("neighbors").get("neighbor"):
        if(neighbor["neighbor-router-id"] == neighbour_id):
            if full == False:
                return True
            if(neighbor["state"] == "full"):
                return True

def ospf_get_interface_type(target, area_id, ifname):
    ospf_interface=_get_ospf_status_area_interface(target,area_id,ifname)
    return ospf_interface.get("interface-type", None)

def ospf_get_interface_passive(target, area_id, ifname):
    ospf_interface=_get_ospf_status_area_interface(target,area_id,ifname)
    return ospf_interface.get("passive", False)
