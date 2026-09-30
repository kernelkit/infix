from . import iface, topology

def edge_mappings(les, pes):
    """Specialized topology edge mapper for LAG tests

    In addition to the standard provides/requires validation, ensure
    that for all logical ports marked with a "lag" attribute, the
    corresponding physical ports are all of the same link type
    (e.g. "link-10gbase-r").

    """
    def links_compatible(candidate):
        seen = None
        for (le, pe) in candidate:
            if le.get("lag"):
                link = set(filter(lambda f: f.startswith("link-"), pe["provides"]))
                if seen is None:
                    seen = link
                elif link != seen:
                    return False

        return True

    for candidate in topology.edge_mappings(les, pes):
        if links_compatible(candidate):
            yield candidate


def lacp_synced(target, *ports):
    """True when all ports are collecting and distributing, as seen by both ends"""
    for port in ports:
        data = target.get_data(iface.get_xpath(port)) or {}
        for entry in data.get("interfaces", {}).get("interface", []):
            # netconf presents lag-port, restconf prefixes it with the model
            lagport = entry.get("lag-port") or entry.get("infix-interfaces:lag-port") or {}
            lacp = lagport.get("lacp", {})
            for state in (lacp.get("actor-state", []), lacp.get("partner-state", [])):
                if not {"collecting", "distributing"} <= set(state):
                    return False

    return True
