"""Container networking with ip(8) and nftables, no CNI.

Each container with interfaces gets a named network namespace,
/run/netns/<name>, created before crun starts so interfaces can be moved
in and addressed first.  confd describes every container-network
interface in /run/containers/net/<ifname>.json:

  type host    the interface itself is moved into the container, with the
               addresses and routes from its ietf-ip configuration
  type bridge  a veth pair is created, the host end enslaved to the bridge,
               the container end addressed from the bridge's subnets

The host's name of a moved interface is kept in ifalias, so yangerd can
report it under its configured name and so it can always be given back.  Masquerading and published ports
are nftables rules in table inet ixcont, regenerated from all container
state on every change.
"""
import contextlib
import fcntl
import hashlib
import ipaddress
import json
import os
import shutil
import subprocess

from . import paths
from .state import ContainerError, all_states, log, sh

DEFAULT_SUBNET = "172.17.0.0/16"
LOCK = os.path.join(paths.RUN, ".netlock")


@contextlib.contextmanager
def lock():
    """Finit starts containers in parallel: one at a time in the lease
    file and the ruleset."""
    os.makedirs(paths.RUN, exist_ok=True)
    fd = os.open(LOCK, os.O_RDWR | os.O_CREAT, 0o600)
    try:
        fcntl.flock(fd, fcntl.LOCK_EX)
        yield
    finally:
        os.close(fd)


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
        return {"type": "host", "addresses": [], "routes": []}


def _options(iface):
    opts = {}
    for item in iface.get("option", []):
        key, _, val = item.partition("=")
        opts[key] = val
    return opts


def _hostend(name, idx):
    h = hashlib.sha256(name.encode()).hexdigest()[:8]
    return f"vt{h}{idx}"


# --- IPAM, one lease file per bridge --------------------------------------

def _leases_path(bridge):
    return os.path.join(paths.IPAM, bridge + ".json")


def _load_leases(bridge):
    try:
        with open(_leases_path(bridge)) as f:
            return json.load(f)
    except (FileNotFoundError, ValueError):
        return {}


def _save_leases(bridge, leases):
    os.makedirs(paths.IPAM, exist_ok=True)
    with open(_leases_path(bridge), "w") as f:
        json.dump(leases, f)


def _subnets(netconf):
    subnets = netconf.get("subnets") or [{"subnet": DEFAULT_SUBNET}]
    out = []
    for s in subnets:
        net = ipaddress.ip_network(s["subnet"], strict=False)
        gw = ipaddress.ip_address(s["gateway"]) if s.get("gateway") else next(net.hosts())
        out.append((net, gw))
    return out


def _allocate(bridge, name, net, gw, wanted):
    leases = _load_leases(bridge)
    taken = {ipaddress.ip_address(a) for a, owner in leases.items() if owner != name}
    taken.add(gw)
    if wanted:
        addr = ipaddress.ip_address(wanted)
        if addr not in net:
            raise ContainerError(f"{wanted} is not in {net}")
        if addr in taken:
            raise ContainerError(f"{wanted} is already in use on {bridge}")
    else:
        addr = None
        for cand in net.hosts():
            if cand not in taken:
                addr = cand
                break
        if addr is None:
            raise ContainerError(f"no free address in {net} on {bridge}")
    leases = {a: o for a, o in leases.items() if o != name or ipaddress.ip_address(a).version != addr.version}
    leases[str(addr)] = name
    _save_leases(bridge, leases)
    return addr


def _release(name):
    if not os.path.isdir(paths.IPAM):
        return
    for fn in os.listdir(paths.IPAM):
        bridge = fn[:-5]
        leases = {a: o for a, o in _load_leases(bridge).items() if o != name}
        _save_leases(bridge, leases)


# --- setup / teardown -----------------------------------------------------

def _addr_add(ns, dev, addr, plen):
    _ipn(ns, "addr", "add", f"{addr}/{plen}", "dev", dev)


def _routes(ns, dev, routes, default_gw=None):
    for r in routes:
        cmd = ["route", "add", r["subnet"]]
        if r.get("gateway"):
            cmd += ["via", r["gateway"]]
        cmd += ["dev", dev]
        _ipn(ns, *cmd)
    if default_gw is not None and not routes:
        family = "-6" if default_gw.version == 6 else "-4"
        sh("ip", family, "-n", ns, "route", "add", "default", "via", str(default_gw), "dev", dev)


def _setup_host(ns, name, ifname, inner, netconf):
    _ip("link", "set", ifname, "down")
    _ip("link", "set", ifname, "netns", ns, "name", inner)
    _ipn(ns, "link", "set", inner, "alias", ifname)
    addrs = []
    for a in netconf.get("addresses", []):
        _addr_add(ns, inner, a["ip"], a["prefix-length"])
        addrs.append(a["ip"])
    _ipn(ns, "link", "set", inner, "up")
    _routes(ns, inner, netconf.get("routes", []))
    return {"name": ifname, "ifname": inner, "type": "host", "addresses": addrs}


def _setup_bridge(ns, name, bridge, inner, idx, netconf, opts):
    hostend = _hostend(name, idx)
    sh("ip", "link", "del", hostend, check=False, quiet=True)
    _ip("link", "add", hostend, "type", "veth", "peer", "name", inner, "netns", ns)
    _ip("link", "set", hostend, "master", bridge, "up")
    if "mac" in opts:
        _ipn(ns, "link", "set", inner, "address", opts["mac"])
    _ipn(ns, "link", "set", inner, "alias", bridge)

    addrs = []
    gateways = {}
    for net, gw in _subnets(netconf):
        wanted = opts.get("ip6" if net.version == 6 else "ip")
        addr = _allocate(bridge, name, net, gw, wanted)
        _addr_add(ns, inner, addr, net.prefixlen)
        addrs.append(str(addr))
        gateways[net.version] = gw
    _ipn(ns, "link", "set", inner, "up")

    routes = [dict(r) for r in netconf.get("routes", [])]
    for r in routes:
        if not r.get("gateway"):
            version = ipaddress.ip_network(r["subnet"], strict=False).version
            r["gateway"] = str(gateways[version])
    for gw in gateways.values():
        _routes(ns, inner, [r for r in routes if ipaddress.ip_network(r["subnet"], strict=False).version == gw.version], gw)

    return {"name": bridge, "ifname": inner, "type": "bridge", "hostend": hostend,
            "addresses": addrs}


def setup(cfg):
    """Create the container's netns and populate it, return the state networks."""
    with lock():
        try:
            return _setup(cfg)
        except Exception:
            _teardown(cfg["name"], None)
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
        opts = _options(iface)
        inner = opts.get("interface_name", f"eth{idx}")
        netconf = load_netconf(ifname)
        if netconf.get("type") == "bridge":
            networks.append(_setup_bridge(ns, name, ifname, inner, idx, netconf, opts))
        else:
            networks.append(_setup_host(ns, name, ifname, inner, netconf))
    return networks


def teardown(name, networks):
    """Return moved interfaces to the host and delete the netns."""
    with lock():
        _teardown(name, networks)


def _giveback(ns, inner, ifname):
    sh("ip", "-n", ns, "link", "set", inner, "down", check=False, quiet=True)
    sh("ip", "-n", ns, "link", "set", inner, "netns", "1", "name", ifname, check=False, quiet=True)


def _moved(ns):
    """Interfaces in ns that belong to the host: (name in ns, name on host).

    The veth end of a bridge network carries the bridge's name, which is
    still on the host, and goes away with the namespace.
    """
    res = sh("ip", "-n", ns, "-j", "link", check=False, quiet=True)
    try:
        links = json.loads(res.stdout or "[]")
    except ValueError:
        links = []
    for link in links:
        alias = link.get("ifalias")
        if alias and not os.path.exists(os.path.join("/sys/class/net", alias)):
            yield link["ifname"], alias


def _teardown(name, networks):
    ns = name
    if os.path.exists(netns_path(ns)):
        for inner, ifname in _moved(ns):
            _giveback(ns, inner, ifname)
        sh("ip", "netns", "del", ns, check=False, quiet=True)
    for n in networks or []:
        if n.get("hostend"):
            sh("ip", "link", "del", n["hostend"], check=False, quiet=True)
    _release(name)


def detach(st, ifname):
    """Give a host interface back to the host now, before confd deletes it."""
    ns = st["name"]
    keep = []
    for n in st.get("networks") or []:
        if n.get("name") != ifname or n.get("type") != "host":
            keep.append(n)
            continue
        _giveback(ns, n["ifname"], ifname)
    st["networks"] = keep
    return st


# --- nftables -------------------------------------------------------------

def _publish(entry, addrs):
    """Parse [[ip:][hostPort]:]containerPort[/proto] into DNAT rule parts."""
    spec, _, proto = entry.partition("/")
    proto = proto or "tcp"
    parts = spec.rsplit(":", 2)
    hostip = None
    if len(parts) == 3:
        hostip, hostport, cport = parts
    elif len(parts) == 2:
        hostport, cport = parts
    else:
        return None
    if not hostport:
        return None
    for addr in addrs:
        if ipaddress.ip_address(addr).version == 4:
            return proto, hostip, hostport, addr, cport
    return None


def ruleset():
    lines = ["table inet ixcont", "delete table inet ixcont", "table inet ixcont {"]
    dnat, masq = [], []
    for st in all_states():
        for n in st.get("networks") or []:
            if n.get("type") != "bridge":
                continue
            for addr in n.get("addresses", []):
                ip = ipaddress.ip_address(addr)
                fam = "ip6" if ip.version == 6 else "ip"
                masq.append(f'\t\t{fam} saddr {addr} oifname != "{n["name"]}" masquerade')
            for entry in st.get("publish") or []:
                rule = _publish(entry, n.get("addresses", []))
                if not rule:
                    continue
                proto, hostip, hostport, caddr, cport = rule
                match = f"ip daddr {hostip} " if hostip else ""
                dnat.append(f"\t\t{match}{proto} dport {hostport} dnat ip to {caddr}:{cport}")
    lines.append("\tchain prerouting {")
    lines.append("\t\ttype nat hook prerouting priority dstnat; policy accept;")
    lines += dnat
    lines.append("\t}")
    lines.append("\tchain output {")
    lines.append("\t\ttype nat hook output priority dstnat; policy accept;")
    lines += dnat
    lines.append("\t}")
    lines.append("\tchain postrouting {")
    lines.append("\t\ttype nat hook postrouting priority srcnat; policy accept;")
    lines += masq
    lines.append("\t}")
    lines.append("}")
    return "\n".join(lines) + "\n"


def apply_nft():
    if not shutil.which("nft"):
        return
    with lock():
        res = subprocess.run(["nft", "-f", "-"], input=ruleset(), text=True, capture_output=True)
    if res.returncode:
        log(f"nft: {res.stderr.strip()}")


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
    from .state import conf_names, load_conf
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
