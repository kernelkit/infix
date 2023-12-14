def _get_routes(target, protocol):
    xpath=f"/ietf-routing:routing"
    rib = target.get_data(xpath)["routing"]["ribs"]["rib"]
    for r in rib:
        if r["name"] != protocol:
            continue
        return r.get("routes", {}).get("route",{})
    return {}

def _exist_route(target, prefix, nexthop, protocol):
    routes = _get_routes(target, protocol)
    for r in routes:
        if r["destination-prefix"] != prefix:
            continue
        nh = r["next-hop"]
        if nexthop and nh["next-hop-address"] != nexthop:
            continue
        return True
    return False

def ipv4_route_exist(target, prefix, nexthop=None):
    return _exist_route(target, prefix, nexthop, "ipv4")

def ipv6_route_exist(target, prefix, nexthop=None):
    return _exist_route(target, prefix, nexthop, "ipv6")
