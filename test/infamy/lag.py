from . import topology

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
