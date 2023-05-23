import networkx as nx
from networkx.algorithms import isomorphism

def qstrip(text):
    if text.startswith("\"") and text.endswith("\""):
        return text[1:-1]
    return text

def match_kind(n1, n2):
    return qstrip(n1["kind"]) == qstrip(n2["kind"])

def find_mapping(phy, log, node_match=match_kind):
    def annotate(nxg, dotg):
        for e in list(nxg.edges):
            s, d = e
            nxg.nodes[s]["kind"] = "port"
            nxg.nodes[d]["kind"] = "port"

            sn, sp = s.split(":")
            dn, dp = d.split(":")

            try:
                sk = dotg.get_node(sn)[0].get_attributes()["kind"]
            except:
                raise ValueError("\"{}\"'s kind is not known".format(sn))

            try:
                dk = dotg.get_node(dn)[0].get_attributes()["kind"]
            except:
                raise ValueError("\"{}\"'s kind is not known".format(dn))

            nxg.add_node(sn, kind=sk)
            nxg.add_edge(sn, s)
            nxg.add_node(dn, kind=dk)
            nxg.add_edge(dn, d)

    phyedges = [(e.get_source(), e.get_destination()) for e in phy.get_edges()]
    logedges = [(e.get_source(), e.get_destination()) for e in log.get_edges()]

    phyedges.sort()
    logedges.sort()
    nxphy = nx.Graph(phyedges)
    nxlog = nx.Graph(logedges)
    annotate(nxphy, phy)
    annotate(nxlog, log)

    nxmap = isomorphism.GraphMatcher(nxphy, nxlog, node_match=node_match)
    if nxmap.subgraph_is_isomorphic():
        return { v: k for (k, v) in nxmap.mapping.items() }

    return None

# This let's us call this script like so...
#
#     python3 topology.py <physical> <logical>
#
# to inspect the graph matcher's results in isolation from the rest of
# the system.
if __name__ == "__main__":
    import json
    import pydot
    import sys

    pdot = pydot.graph_from_dot_file(sys.argv[1])[0]
    ldot = pydot.graph_from_dot_file(sys.argv[2])[0]
    mapping = find_mapping(pdot, ldot)

    print(json.dumps(mapping))
