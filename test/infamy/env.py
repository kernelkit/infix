import argparse
import networkx
import os
import pydot
import shlex
import sys

from . import neigh, netconf, tap, topology

class ArgumentParser(argparse.ArgumentParser):
    def __init__(self, ltop):
        super().__init__()

        self.add_argument("-d", "--debug", default=False, action="store_true")
        self.add_argument("-l", "--logical-topology", dest="ltop", default=ltop)
        self.add_argument("-y", "--yangdir", default=None)
        self.add_argument("ptop", nargs=1, metavar="topology")


class Env(object):
    def __init__(self, ltop=None, argv=sys.argv[1::], environ=os.environ):
        if "INFAMY_ARGS" in environ:
            argv = shlex.split(environ["INFAMY_ARGS"]) + argv

        self.args = ArgumentParser(ltop).parse_args(argv)

        pdot = pydot.graph_from_dot_file(self.args.ptop[0])[0]
        self.ptop = networkx.nx_pydot.read_dot(self.args.ptop[0])

        if self.args.ltop:
            self.ltop = networkx.nx_pydot.read_dot(self.args.ltop)
            ldot = pydot.graph_from_dot_file(self.args.ltop)[0]
            mapping = topology.find_mapping(pdot, ldot)
            if not mapping:
                raise tap.TestSkip()

            self.mapping = {}
            for (log, phy) in mapping.items():
                if ":" in log:
                    lnode, lport = log.split(":")
                    pnode, pport = phy.split(":")

                    self._map_node(lnode, pnode)
                    self._map_port((lnode, lport), (pnode, pport))
                else:
                    self._map_node(log, phy)

    def _map_node(self, lnode, pnode):
        if lnode in self.mapping:
            assert(self.mapping[lnode][None] == pnode)
        else:
            self.mapping[lnode] = { None: pnode }

    def _map_port(self, log, phy):
        (lnode, lport) = log
        (pnode, pport) = phy

        if lport in self.mapping[lnode]:
            assert(self.mapping[lnode][lport] == pport)
        else:
            self.mapping[lnode][lport] = pport

    def xlate(self, lnode, lport=None):
        if lnode not in self.mapping:
            return None

        nodemap = self.mapping[lnode]

        if lport not in nodemap:
            return None

        if not lport:
            return nodemap[None]

        return (nodemap[None], nodemap[lport])


    def attach(self, node, port):
        if self.mapping:
            mapping = self.mapping[node]
            node, port = mapping[None], mapping[port]
        else:
            mapping = None

        hostport = list(self.ptop.neighbors(f"{node}:{port}"))[0]
        hnode, hport = hostport.split(":")

        print(f"Probing {node} on port {hport} for IPv6LL mgmt address ...")
        mgmtip = neigh.ll6ping(hport)
        if not mgmtip:
            raise Exception(f"Failed, cannot find mgmt IP for {node}")

        print(f"Mgmt IP {mgmtip}")
        return netconf.Device(
            location=netconf.Location(mgmtip),
            mapping=mapping,
            yangdir=self.args.yangdir
        )
