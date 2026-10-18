import argparse
import os
import pydot
import shlex
import subprocess
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


# Bridge port states, link events, per-port packet counts and learned
# unicast addresses of every attached DUT, logged when a test fails.
_DUT_STATE_SH = r"""
echo "uptime $(cut -d' ' -f1 /proc/uptime)"
bridge link show 2>/dev/null | awk '{ s = m = "-"
    for (i = 1; i < NF; i++) { if ($i == "state") s = $(i + 1); if ($i == "master") m = $(i + 1) }
    sub(":", "", $2); printf "%s:%s:%s ", $2, m, s } END { if (NR) print "" }'
for d in /sys/class/net/*; do
    [ -e $d/device ] && [ "$(cat $d/operstate)" = up ] || continue
    printf "%s rx=%s tx=%s txdrop=%s  " ${d##*/} $(cat $d/statistics/rx_packets \
        $d/statistics/tx_packets $d/statistics/tx_dropped)
done; echo
dmesg | grep -E "Link is (Up|Down)" | tail -10 | sed 's/sparx5-switch [^ ]* //'
bridge fdb show 2>/dev/null | awk '$0 ~ / master / && $0 !~ /permanent|^33:33|^01:00:5e/ {
    for (i = 1; i < NF; i++) if ($i == "dev") d[$1] = $(i + 1)
    if ($0 ~ /offload/) hw[$1] = 1 }
    END { for (m in d) printf "%s@%s%s ", m, d[m], (m in hw ? "" : "(sw)"); if (length(d)) print "" }'
"""
_attached = {}


def _dump_duts():
    if not _attached:
        return

    def state(dev):
        try:
            if not isinstance(dev, ssh.Device):
                dev = ssh.device_for(dev)
            res = dev.runsh(_DUT_STATE_SH, timeout=30)
        except subprocess.TimeoutExpired:
            return "cannot read state: timed out"
        except Exception as e:
            return f"cannot read state: {type(e).__name__}"
        if res.returncode == ssh.TRANSPORT_ERROR:
            return "cannot read state: unreachable"
        return res.stdout

    duts = list(_attached.items())
    out = util.parallel(*[lambda d=d: state(d) for _, d in duts])
    print("State of the DUTs at failure, bridge ports as port:bridge:state:")
    for (name, _), text in zip(duts, out):
        for line in text.strip().splitlines():
            print(f"{name}: {line}")


tap.on_failure(_dump_duts)


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
            ltop = topology.Topology(ldot)
            if not ltop.map_to(self.ptop,
                               nodes_compatible=nodes_compatible,
                               edge_mappings=edge_mappings):
                raise tap.TestSkip()

            print(repr(ltop))
            self.reset_cell_neighbours(ltop)
            self.ltop = ltop

        global ENV
        ENV = self

    def __getitem__(self, node):
        """Port mapping of a logical node, e.g. env["host"]["data"]."""
        return self.ltop.mapping[node]

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

    def reset_cell_neighbours(self, ltop):
        """Reset the DUTs this test leaves out but shares a wireless cell with.

        attach() resets the nodes a test uses.  Another DUT on one of
        its cells is on the same air, and whatever the previous test
        left running there, an access point say, is in this test's way.
        Called before the mapping is stored, so attach() takes the
        physical names.  A DUT without test mode is left as it is.
        """
        used = {m[None] for m in ltop.mapping.values()}
        cells = [n for n in used
                 if topology.compatible(self.ptop.g.nodes[n], {"requires": {"wifi-radio"}})]
        strays = {n for cell in cells for n in self.ptop.g.neighbors(cell)}
        strays &= set(self.ptop.get_infixen())
        strays -= used

        def reset_one(node):
            dev = self.attach(node, test_reset=False)
            if not dev.has_feature("infix-test", "test-mode-enable"):
                print(f"{node}: not in test mode, left as is")
                return
            dev.test_reset()
            cport, _ = self.ptop.get_mgmt_link(self.ptop.get_ctrl(), node)
            util.until(lambda: self.is_reachable(node, cport), 30)

        util.parallel(*[lambda n=n: reset_one(n) for n in sorted(strays)])

    def attach(self, node, port="mgmt", protocol=None, test_reset=True, username=None, password=None):
        """Attach to node on port using protocol."""
        dev = self._attach(node, port, protocol, test_reset, username, password)
        if username in (None, "admin"):
            phys = self.ltop.xlate(node) if self.ltop else node
            _attached[f"{node} ({phys})"] = dev
        return dev

    def _attach(self, node, port, protocol, test_reset, username, password):
        name = node
        mapping = None
        if self.ltop and node in self.ltop.mapping:
            mapping = self.ltop.mapping[node]
            node, port = self.ltop.xlate(node, port)

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
        util.parallel(lambda: util.until(lambda: self.is_reachable(node, cport), 300))

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
                util.until(lambda: self.is_reachable(node, cport), 30)
            return dev

        if protocol == "ssh":
            dev = ssh.Device(name, ssh.Location(mgmtip, username, password))
            return dev

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
                util.until(lambda: self.is_reachable(node, cport), 30)
            return dev

        raise Exception(f"Unsupported management procotol \"{protocol}\"")
