import networkx as nx
from networkx.algorithms import isomorphism

def qstrip(text):
    if text.startswith("\"") and text.endswith("\""):
        return text[1:-1]
    return text

def match_node(n1attrs, n2attrs):
    return n1attrs.get("kind") == n2attrs.get("kind")

def match_edge(e1attrs, e2attrs):
    if "kind" in e1attrs or "kind" in e2attrs:
        return e1attrs.get("kind") == e2attrs.get("kind")

    return True


class Topology:
    def __init__(self, dotg):
        self.dotg = dotg
        edges = {}
        for e in self.dotg.get_edges():
            attrs = e.get_attributes()
            if "weight" not in attrs:
                attrs["weight"] = 1
            
            edges[tuple(e.get_source().split(":"))] = { tuple(e.get_destination().split(":")): attrs }


        self.g = nx.Graph(edges, weight=1)
        for e in list(self.g.edges):
            s, d = e
            self.g.nodes[s]["kind"] = "port"
            self.g.nodes[d]["kind"] = "port"

            sn, sp = s
            dn, dp = d

            try:
                sk = qstrip(self.dotg.get_node(sn)[0].get_attributes()["kind"])
            except:
                raise ValueError("\"{}\"'s kind is not known".format(sn))

            try:
                dk = qstrip(self.dotg.get_node(dn)[0].get_attributes()["kind"])
            except:
                raise ValueError("\"{}\"'s kind is not known".format(dn))

            self.g.add_node(sn, kind=sk)
            self.g.add_edge(sn, s, weight=0)
            self.g.add_node(dn, kind=dk)
            self.g.add_edge(dn, d, weight=0)

    def map_to(self, phy):
        def _map_node(lnode, pnode):
            if lnode in self.mapping:
                assert(self.mapping[lnode][None] == pnode)
            else:
                self.mapping[lnode] = { None: pnode }

        def _map_port(log, phy):
            (lnode, lport) = log
            (pnode, pport) = phy

            if lport in self.mapping[lnode]:
                assert(self.mapping[lnode][lport] == pport)
            else:
                self.mapping[lnode][lport] = pport

        nxmap = isomorphism.GraphMatcher(phy.g, self.g, edge_match=match_edge, node_match=match_node)
        if not nxmap.subgraph_is_isomorphic():
            return False

        self.phy = phy
        self.mapping = {}
        for (phy, log) in nxmap.mapping.items():
            if isinstance(log, tuple):
                lnode, lport = log
                pnode, pport = phy

                _map_node(lnode, pnode)
                _map_port((lnode, lport), (pnode, pport))
            else:
                _map_node(log, phy)

        return True

    def xlate(self, lnode, lport=None):
        assert(self.mapping)

        if lnode not in self.mapping:
            return None

        nodemap = self.mapping[lnode]

        if lport not in nodemap:
            return None

        if not lport:
            return nodemap[None]

        return (nodemap[None], nodemap[lport])

    def get_nodes(self, flt):
        out = []
        for name in self.g.nodes:
            if flt(name, self.g.nodes[name]):
                out.append(name)

        return out

    def get_password(self, node):
        n = self.dotg.get_node(node)
        b = n[0] if n else {}
        password=b.get("password")
        return qstrip(password) if password is not None else None

    def get_ports(self, node):
        ports = self.get_nodes(lambda name, _: name.startswith(f"{node}:"))
        return { p.removeprefix(f"{node}:") for p in ports }

    def get_path(self, src, dst):
        path = nx.shortest_path(self.g, src, dst)
        return path[1:-1] if path else None

    def get_paths(self, src, dst):
        paths = nx.all_shortest_paths(self.g, src, dst)
        if not paths:
            return None

        return map(lambda path: path[1:-1], paths)

    def get_ctrl(self):
        ns = self.get_nodes(lambda _, attrs: attrs.get("kind") == "controller")
        assert(len(ns) == 1)
        return ns[0]

    def get_infixen(self):
        return self.get_nodes(lambda _, attrs: attrs.get("kind") == "infix")


# Support calling this script like so...
#
#     python3 topology.py <physical> <logical>
#
# to inspect the graph matcher's results in isolation from the rest of
# the system.
if __name__ == "__main__":
    import json
    import pydot
    import sys

    phy = Topology(pydot.graph_from_dot_file(sys.argv[1])[0])
    log = Topology(pydot.graph_from_dot_file(sys.argv[2])[0])
    if log.map_to(phy):
        print(json.dumps(log.mapping))
        sys.exit(0)

    print("{}")
    sys.exit(1)
