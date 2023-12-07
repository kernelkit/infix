def _get_routes(target, protocol):
    xpath="/ietf-routing:routing/ribs"
    rib = target.get_data(xpath)["routing"]["ribs"]["rib"]
    for r in rib:
        if r["name"] != protocol:
            continue
        return r.get("routes", {}).get("route",{})
    return {}

def _exist_route(target, prefix, nexthop, version,source_protocol):
    routes = _get_routes(target, version)
    for r in routes:
        if r["destination-prefix"] != prefix:
            continue
        if source_protocol and r.get("source-protocol") != source_protocol:
            continue

        nh = r["next-hop"]
        if nexthop and nh["next-hop-address"] != nexthop:
            continue
        return True
    return False

def ipv4_route_exist(target, prefix, nexthop=None,source_protocol=None):
    return _exist_route(target, prefix, nexthop, "ipv4",source_protocol)

def ipv6_route_exist(target, prefix, nexthop=None,source_protocol=None):
    return _exist_route(target, prefix, nexthop, "ipv6",source_protocol)
