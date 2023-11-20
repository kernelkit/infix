def _get_routes(target,protocol):
    xpath=f"/ietf-routing:routing"
    rib = target.get_data(xpath)["routing"]["ribs"]["rib"]
    for r in rib:
        if r["name"] != protocol:
            continue
        return r.get("routes", {}).get("route",{})
    return {}

def _exist_route(target,destination_prefix, protocol):
    routes=_get_routes(target,protocol)
    for r in routes:
        if(r["destination-prefix"] == destination_prefix):
            return True

    return False

def ipv4_route_exist(target, destination_prefix):
    return _exist_route(target,destination_prefix, "ipv4")

def ipv6_route_exist(target, destination_prefix):
    return _exist_route(target,destination_prefix, "ipv6")
