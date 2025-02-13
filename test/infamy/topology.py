import networkx as nx
from networkx.algorithms import isomorphism

from itertools import permutations
import json

def _qstrip(text):
    if text is None:
        return None

    if text.startswith("\"") and text.endswith("\""):
        return text[1:-1]

    return text

def compatible(physical, logical):
    return logical["requires"].issubset(physical["provides"])

def edge_mappings(les, pes):
    les = les.values()
    pes = pes.values()

    for perm in permutations(pes, len(les)):
        candidate = tuple(zip(les, perm))
        if all(map(lambda pair: compatible(pair[1], pair[0]), candidate)):
            yield candidate


class Topology:
    def __init__(self, dotg):
        self.dotg = dotg
        self.g = nx.MultiGraph()

        for n in self.dotg.get_nodes():
            name = n.get_name()
            if name in ("node", "edge"):
                continue

            repr(n.get_attributes())
            attrs = { _qstrip(k): _qstrip(v) for k, v in n.get_attributes().items() if k != "label" }
            for attr in ("requires", "provides"):
                attrs[attr] = set(attrs.get(attr, "").split())

            self.g.add_node(name, **attrs)

        for e in self.dotg.get_edges():
            sn, sp = e.get_source().split(":")
            dn, dp = e.get_destination().split(":")

            attrs = {_qstrip(k): _qstrip(v) for k, v in e.get_attributes().items()}
            attrs[sn] = sp
            attrs[dn] = dp

            for attr in ("requires", "provides"):
                attrs[attr] = set(attrs.get(attr, "").split())

            self.g.add_edge(sn, dn, **attrs)

    def __repr__(self):
        if not self.mapping:
            return ""

        out = ""

        for n in self.mapping:
            out += f"{n + ':':<8} {self.mapping[n][None]}\n"
            for e in self.mapping[n]:
                if not e:
                    continue

                out += f"    {e + ':':<8} {self.mapping[n][e]}\n"

        return out

    def map_to(self, phy,
               nodes_compatible=compatible, edge_mappings=edge_mappings):
        mapper = isomorphism.MultiGraphMatcher(phy.g, self.g,
                                               edge_match=lambda pes, les: any(edge_mappings(les, pes)),
                                               node_match=nodes_compatible)
        if not mapper.subgraph_is_monomorphic():
            return False

#        breakpoint()
        self.phy = phy
        self.mapping = {}

        for pn, ln in mapper.mapping.items():
            self.mapping.setdefault(ln, { None: pn })

        for lsrc, ldst in set(self.g.edges()):
            psrc = self.mapping[lsrc][None]
            pdst = self.mapping[ldst][None]

            les = self.g.get_edge_data(lsrc, ldst)
            pes = self.phy.g.get_edge_data(psrc, pdst)

            for le, pe in next(edge_mappings(les, pes)):
                self.mapping[lsrc][le[lsrc]] = pe[psrc]
                self.mapping[ldst][le[ldst]] = pe[pdst]

        return True

    def xlate(self, lnode, lport=None):
        assert self.mapping

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
        password = b.get("password")

        return qstrip(password) if password is not None else "admin"

    def get_link(self, src, dst, flt=lambda _: True):
        es = self.g.get_edge_data(src, dst)
        for e in es.values():
            if flt(e):
                return e[src], e[dst]

        return None

    def get_mgmt_link(self, src, dst):
        return self.get_link(src, dst, lambda e: compatible(e, {"requires": {"mgmt"}}))

    def get_ctrl(self):
        ns = self.get_nodes(lambda _, attrs: compatible(attrs, {"requires": {"controller"}}))
        assert len(ns) == 1
        return ns[0]

    def get_infixen(self):
        return self.get_nodes(lambda _, attrs: compatible(attrs, {"requires": {"infix"}}))


    def get_attr(self, name, default=None):
        return _qstrip(self.dotg.get_attributes().get(name, default))


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
