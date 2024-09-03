import networkx as nx
from networkx.algorithms import isomorphism


def _qstrip(text):
    if text is None:
        return None

    if text.startswith("\"") and text.endswith("\""):
        return text[1:-1]

    return text


def map_edges(les, pes):
    acc = []
    les = sorted(list(les.values()), key=lambda x: x.get("kind", ""), reverse=True)
    pes = sorted(list(pes.values()), key=lambda x: x.get("kind", ""), reverse=True)

    for i in range(len(les)):
        if pes[i].get("kind") != les[i].get("kind"):
            return None

        acc.append((les[i], pes[i]))

    return acc


def match_node(pn, ln):
    return pn.get("kind") == ln.get("kind")


def match_edge(pes, les):
    return map_edges(les, pes) is not None


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
            self.g.add_node(name, **attrs)

        for e in self.dotg.get_edges():
            sn, sp = e.get_source().split(":")
            dn, dp = e.get_destination().split(":")

            attrs = {_qstrip(k): _qstrip(v) for k, v in e.get_attributes().items()}
            attrs[sn] = sp
            attrs[dn] = dp
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

    def map_to(self, phy):
        mapper = isomorphism.MultiGraphMatcher(phy.g, self.g,
                                               edge_match=match_edge,
                                               node_match=match_node)
        if not mapper.subgraph_is_monomorphic():
            return False

        self.phy = phy
        self.mapping = {}

        for pn, ln in mapper.mapping.items():
            self.mapping.setdefault(ln, { None: pn })

        for lsrc, ldst in set(self.g.edges()):
            psrc = self.mapping[lsrc][None]
            pdst = self.mapping[ldst][None]

            les = self.g.get_edge_data(lsrc, ldst)
            pes = self.phy.g.get_edge_data(psrc, pdst)

            for le, pe in map_edges(les, pes):
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
        return self.get_link(src, dst, lambda e: e.get("kind") == "mgmt")

    def get_ctrl(self):
        ns = self.get_nodes(lambda _, attrs: attrs.get("kind") == "controller")
        assert len(ns) == 1
        return ns[0]

    def get_infixen(self):
        return self.get_nodes(lambda _, attrs: attrs.get("kind") == "infix")

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
