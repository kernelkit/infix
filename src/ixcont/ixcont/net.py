"""The network inside a container, set up with ip(8).

Each container with interfaces gets a named network namespace,
/run/netns/<name>, created before crun starts so interfaces can be moved
in and addressed first.  An interface is created and connected by confd
like any other, here it is only moved into the namespace, with the
addresses of its ietf-ip configuration and the routes of the container,
which confd describes in /run/containers/net/<ifname>.json.

The host's name of a moved interface is kept in ifalias, so yangerd can
report it under its configured name and so it can always be given back.
"""
import json
import os

from . import paths
from .state import ContainerError, all_states, conf_names, load_conf, sh


def _ip(*args):
    return sh("ip", *args)


def _ipn(ns, *args):
    return sh("ip", "-n", ns, *args)


def netns_path(name):
    return os.path.join(paths.NETNS, name)


def load_netconf(ifname):
    try:
        with open(os.path.join(paths.NETCONF, ifname + ".json")) as f:
            return json.load(f)
    except FileNotFoundError:
        return {"addresses": [], "routes": []}


def _options(iface):
    opts = {}
    for item in iface.get("option", []):
        key, _, val = item.partition("=")
        opts[key] = val
    return opts


# --- setup / teardown -----------------------------------------------------

def _move(ns, ifname, inner, netconf):
    _ip("link", "set", ifname, "down")
    _ip("link", "set", ifname, "netns", ns, "name", inner)
    _ipn(ns, "link", "set", inner, "alias", ifname)

    addrs = []
    for a in netconf.get("addresses", []):
        _ipn(ns, "addr", "add", f"{a['ip']}/{a['prefix-length']}", "dev", inner)
        addrs.append(a["ip"])
    _ipn(ns, "link", "set", inner, "up")

    for r in netconf.get("routes", []):
        via = ["via", r["gateway"]] if r.get("gateway") else []
        _ipn(ns, "route", "add", r["subnet"], *via, "dev", inner)

    return {"name": ifname, "ifname": inner, "addresses": addrs}


def setup(cfg):
    """Create the container's netns and populate it, return the state networks."""
    try:
        return _setup(cfg)
    except Exception:
        teardown(cfg["name"])
        raise


def _setup(cfg):
    name = cfg["name"]
    network = cfg.get("network", {})
    if network.get("host"):
        return None

    ns = name
    sh("ip", "netns", "del", ns, check=False, quiet=True)
    _ip("netns", "add", ns)
    _ipn(ns, "link", "set", "lo", "up")

    networks = []
    for idx, iface in enumerate(network.get("interface", [])):
        ifname = iface["name"]
        inner = _options(iface).get("interface_name", f"eth{idx}")
        networks.append(_move(ns, ifname, inner, load_netconf(ifname)))
    return networks


def _giveback(ns, inner, ifname):
    sh("ip", "-n", ns, "link", "set", inner, "down", check=False, quiet=True)
    sh("ip", "-n", ns, "link", "set", inner, "netns", "1", "name", ifname, check=False, quiet=True)


def _moved(ns):
    """Interfaces in ns that belong to the host: (name in ns, name on host)."""
    res = sh("ip", "-n", ns, "-j", "link", check=False, quiet=True)
    try:
        links = json.loads(res.stdout or "[]")
    except ValueError:
        links = []
    for link in links:
        alias = link.get("ifalias")
        if alias and not os.path.exists(os.path.join("/sys/class/net", alias)):
            yield link["ifname"], alias


def teardown(name):
    """Return moved interfaces to the host and delete the netns."""
    ns = name
    if os.path.exists(netns_path(ns)):
        for inner, ifname in _moved(ns):
            _giveback(ns, inner, ifname)
        sh("ip", "netns", "del", ns, check=False, quiet=True)


def detach(st, ifname):
    """Give an interface back to the host now, before confd deletes it."""
    ns = st["name"]
    keep = []
    for n in st.get("networks") or []:
        if n.get("name") != ifname:
            keep.append(n)
            continue
        _giveback(ns, n["ifname"], ifname)
    st["networks"] = keep
    return st


# --- lookups for confd and yangerd ----------------------------------------

def owner(ifname):
    """The container state whose networks include ifname, or None."""
    for st in all_states():
        for n in st.get("networks") or []:
            if n.get("name") == ifname:
                return st
    return None


def inner_ifname(pid, ifname):
    res = sh("nsenter", "-t", str(pid), "-n", "ip", "-d", "-j", "link", check=False, quiet=True)
    if res.returncode:
        return None
    for link in json.loads(res.stdout or "[]"):
        if link.get("ifalias") == ifname:
            return link.get("ifname")
    return None


def users(ifname):
    """Names of containers configured to use interface ifname."""
    names = []
    for name in conf_names():
        try:
            cfg = load_conf(name)
        except ContainerError:
            continue
        for iface in cfg.get("network", {}).get("interface", []):
            if iface["name"] == ifname:
                names.append(name)
    return names
