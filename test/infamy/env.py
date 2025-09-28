import argparse
import os
import pydot
import shlex
import sys
import random
import inspect

from . import neigh, netconf, restconf, ssh, tap, topology, util


class NullEnv:
    def attr(self, _, default=None):
        return default


ENV = NullEnv()


class ArgumentParser():
    def DefaultTransport():
        """Pick pseudo-random transport

        If the user does not specify a particular transport, make sure
        that any (test, $PYTHONHASHSEED) tuple will always map to the
        same transport.

        """
        name = "/".join(os.path.realpath(sys.argv[0]).split(os.sep)[-2:])
        seed = os.environ.get('PYTHONHASHSEED', 0)
        random.seed(f"{name}-{seed}")

        return random.choice(["netconf", "restconf"])

    def __init__(self, top = None):
        self.args = argparse.ArgumentParser(top)

        self.args.add_argument("-d", "--debug", default=False, action="store_true")
        self.args.add_argument("-p", "--package", default=None)
        self.args.add_argument("-y", "--yangdir", default=None)
        self.args.add_argument("-t", "--transport", default=ArgumentParser.DefaultTransport())
        self.args.add_argument("ptop", nargs=1, metavar="topology")
        self.args.add_argument("-l", "--logical-topology", dest="ltop", default=top)

    def add_argument(self, *args, **kwargs):
        kwargs["required"] = True
        self.args.add_argument(*args, **kwargs)

    def parse_args(self, argv):
        return self.args.parse_args(argv)


def test_argument(option, **kwargs):
    """See lag_failure/test.py for an example @infamy.test_argumet()"""
    def decorator(cls):
        super_init = cls.__init__

        def new_init(self, *args, **kw):
            super_init(self, *args, **kw)
            self.add_argument(option, **kwargs)

        cls.__init__ = new_init
        return cls
    return decorator


class Env(object):
    def __init__(self, ltop=None, args=None, argv=sys.argv[1::], environ=os.environ,
                 nodes_compatible=topology.compatible,
                 edge_mappings=topology.edge_mappings):
        if "INFAMY_ARGS" in environ:
            argv = shlex.split(environ["INFAMY_ARGS"]) + argv

        if args:
            self.argp = args
        else:
            self.argp = ArgumentParser(ltop)
        self.args = self.argp.parse_args(argv)
        pdot = pydot.graph_from_dot_file(self.args.ptop[0])[0]
        self.ptop = topology.Topology(pdot)

        self.ltop = None
        if self.args.ltop != False:
            if self.args.ltop is None:
                stack = inspect.stack()
                caller_frame = stack[1]
                top_path = caller_frame.filename
                top_path = os.path.join(os.path.dirname(top_path), "topology.dot")
            else:
                top_path = self.args.ltop

            ldot = pydot.graph_from_dot_file(top_path)[0]
            self.ltop = topology.Topology(ldot)
            if not self.ltop.map_to(self.ptop,
                                    nodes_compatible=nodes_compatible,
                                    edge_mappings=edge_mappings):
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

    def get_password(self, node):
        return self.ptop.get_password(node)

    def is_reachable(self, node, port):
        ip = neigh.ll6ping(port)
        if not ip:
            return False

        return util.is_reachable(ip, self, self.get_password(node))

    def attach(self, node, port="mgmt", protocol=None, test_reset=True, username=None, password=None):
        """Attach to node on port using protocol."""

        name = node
        if self.ltop:
            mapping = self.ltop.mapping[node]
            node, port = self.ltop.xlate(node, port)
        else:
            mapping = None

        # Precedence:
        # 1. Caller specifies `protocol`
        # 2. User specifies `-t` when executing test
        # 3. One is pseudo-randomly picked based on $PYTHONHASHSEED
        if protocol is None:
            protocol = self.args.transport

        if password is None:
            password = self.get_password(node)
        if username is None:
            username = "admin"

        ctrl = self.ptop.get_ctrl()
        cport, _ = self.ptop.get_mgmt_link(ctrl, node)

        print("Waiting for DUTs to become reachable...")
        util.parallel(util.until(lambda: self.is_reachable(node, cport), 300))

        print(f"Probing {node} on port {cport} for IPv6LL mgmt address ...")
        mgmtip = neigh.ll6ping(cport)
        if not mgmtip:
            raise Exception(f"Failed, cannot find mgmt IP for {node}")

        if protocol == "netconf":
            dev = netconf.Device(name,
                                 location=netconf.Location(cport,
                                                           mgmtip,
                                                           username,
                                                           password),
                                 mapping=mapping,
                                 yangdir=self.args.yangdir)
            if test_reset:
                dev.test_reset()
            return dev

        if protocol == "ssh":
            return ssh.Device(name, ssh.Location(mgmtip, username, password))

        if protocol == "restconf":
            dev = restconf.Device(name,
                                  location=restconf.Location(cport,
                                                             mgmtip,
                                                             username,
                                                             password),
                                  mapping=mapping,
                                  yangdir=self.args.yangdir)
            if test_reset:
                dev.test_reset()
            return dev

        raise Exception(f"Unsupported management procotol \"{protocol}\"")
