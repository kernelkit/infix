import argparse
import networkx
import os
import pydot
import shlex
import sys

from . import neigh, netconf, restconf, ssh, tap, topology

class NullEnv:
    def attr(self, _, default=None):
        return default

ENV = NullEnv()

class ArgumentParser(argparse.ArgumentParser):
    def __init__(self, ltop):
        super().__init__()

        self.add_argument("-d", "--debug", default=False, action="store_true")
        self.add_argument("-l", "--logical-topology", dest="ltop", default=ltop)
        self.add_argument("-p", "--package", default=None)
        self.add_argument("-y", "--yangdir", default=None)
        self.add_argument("ptop", nargs=1, metavar="topology")


class Env(object):
    def __init__(self, ltop=None, argv=sys.argv[1::], environ=os.environ):
        if "INFAMY_ARGS" in environ:
            argv = shlex.split(environ["INFAMY_ARGS"]) + argv

        self.args = ArgumentParser(ltop).parse_args(argv)

        pdot = pydot.graph_from_dot_file(self.args.ptop[0])[0]
        self.ptop = topology.Topology(pdot)

        self.ltop = None
        if self.args.ltop:
            ldot = pydot.graph_from_dot_file(self.args.ltop)[0]
            self.ltop = topology.Topology(ldot)
            if not self.ltop.map_to(self.ptop):
                raise tap.TestSkip()

            print(repr(self.ltop))

        global ENV
        ENV = self

    def attr(self, name, default=None):
        val = self.ptop.get_attr("ix_" + name)
        if val is None:
            return default

        try:
            return int(val, 0)
        except ValueError:
            pass

        return val

    def attach(self, node, port, protocol="netconf", factory_default=True):
        if self.ltop:
            mapping = self.ltop.mapping[node]
            node, port = self.ltop.xlate(node, port)
        else:
            mapping = None

        password=self.ptop.get_password(node)

        if not password:
            password = "admin"
        ctrl = self.ptop.get_ctrl()
        cport, _ = self.ptop.get_mgmt_link(ctrl, node)

        print(f"Probing {node} on port {cport} for IPv6LL mgmt address ...")
        mgmtip = neigh.ll6ping(cport)
        if not mgmtip:
            raise Exception(f"Failed, cannot find mgmt IP for {node}")

        if protocol == "netconf":
            return netconf.Device(
                location=netconf.Location(cport, mgmtip, password),
                mapping=mapping,
                yangdir=self.args.yangdir,
                factory_default=factory_default
            )
        if protocol == "ssh":
            return ssh.Device(ssh.Location(mgmtip, password))
        if protocol == "restconf":
            return restconf.Device(location=restconf.Location(cport,
                                                              mgmtip,
                                                              password),
                                   mapping=mapping,
                                   factory_default=factory_default)

        raise Exception(f"Unsupported management procotol \"{protocol}\"")
