from datetime import timedelta
from re import match

from .common import insert, YangDate
from .host import HOST

def uptime2datetime(uptime):
    """
    Convert uptime to YANG format (YYYY-MM-DDTHH:MM:SS+00:00)

    Handles the following input formats (frrtime):
    HH:MM:SS
    XdXXhXXm
    XXwXdXXh
    """
    h = m = s = 0

    # Format HH:MM:SS
    if match(r'^\d{2}:\d{2}:\d{2}$', uptime):
        h, m, s = map(int, uptime.split(':'))

    # Format XdXXhXXm (days, hours, minutes)
    elif match(r'^\d+d\d{2}h\d{2}m$', uptime):
        days = int(uptime.split('d')[0])
        h = int(uptime.split('d')[1].split('h')[0])
        m = int(uptime.split('h')[1].split('m')[0])
        h += days * 24

    # Format XwXdXXh (weeks, days, hours)
    elif match(r'^\d{2}w\d{1}d\d{2}h$', uptime):
        weeks = int(uptime.split('w')[0])
        days = int(uptime.split('w')[1].split('d')[0])
        h = int(uptime.split('d')[1].split('h')[0])
        h += weeks * 7 * 24
        h += days * 24

    uptime_delta = timedelta(hours=h, minutes=m, seconds=s)
    return str(YangDate.from_delta(uptime_delta))


def add_protocol(routes, proto):
    """Populate routes from vtysh JSON output"""

    frrproto = "ip" if proto == "ipv4" else proto
    data = HOST.run_json(['vtysh', '-c', f"show {frrproto} route json"], {})

    # Mapping of FRR protocol names to IETF routing-protocol
    pmap = {
        'kernel': 'infix-routing:kernel',
        'connected': 'direct',
        'static': 'static',
        'ospf': 'ietf-ospf:ospfv2',
        'ospf6': 'ietf-ospf:ospfv3',
        'rip': 'ietf-rip:rip',
    }

    out = {}
    out["route"] = []

    if proto == "ipv4":
        default = "0.0.0.0/0"
        host_prefix_length = "32"
    else:
        default = "::/0"
        host_prefix_length = "128"

    for prefix, entries in data.items():
        for route in entries:
            new = {}
            dst = route.get('prefix', default)
            if '/' not in dst:
                dst = f"{dst}/{route.get('prefixLen', host_prefix_length)}"

            new[f'ietf-{proto}-unicast-routing:destination-prefix'] = dst
            frr = route.get('protocol', 'infix-routing:kernel')
            new['source-protocol'] = pmap.get(frr, 'infix-routing:kernel')
            new['route-preference'] = route.get('distance', 0)

            # Metric only available in the model for OSPF and RIP routes
            if 'ospf' in frr:
                new['ietf-ospf:metric'] = route.get('metric', 0)
            elif 'rip' in frr:
                new['ietf-rip:metric'] = route.get('metric', 0)

            # See https://datatracker.ietf.org/doc/html/rfc7951#section-6.9
            # for details on how presence leaves are encoded in JSON: [null]
            if route.get('selected', False):
                new['active'] = [None]

            new['last-updated'] = uptime2datetime(route.get('uptime', 0))
            installed = route.get('installed', False)

            next_hops = []
            for hop in route.get('nexthops', []):
                next_hop = {}
                if hop.get('ip'):
                    next_hop[f'ietf-{proto}-unicast-routing:address'] = hop['ip']
                elif hop.get('interfaceName'):
                    next_hop['outgoing-interface'] = hop['interfaceName']
                # See zebra/zebra_vty.c:re_status_outpupt_char()
                if installed and hop.get('fib', False):
                    next_hop['infix-routing:installed'] = [None]
                next_hops.append(next_hop)

            if next_hops:
                new['next-hop'] = {'next-hop-list': {'next-hop': next_hops}}
            else:
                next_hop = {}
                protocol = route.get('protocol', 'unicast')
                if protocol == "blackhole":
                    next_hop['special-next-hop'] = "blackhole"
                elif protocol == "unreachable":
                    next_hop['special-next-hop'] = "unreachable"
                else:
                    if route.get('interfaceName'):
                        next_hop['outgoing-interface'] = route['interfaceName']
                    if route.get('nexthop'):
                        next_hop[f'ietf-{proto}-unicast-routing:next-hop-address'] = route['nexthop']

                new['next-hop'] = next_hop

            out['route'].append(new)

    insert(routes, 'routes', out)


def get_routing_interfaces():
    """Get list of interfaces with IPv4 or IPv6 forwarding enabled"""
    import json

    # Get all interfaces
    links_json = HOST.run(tuple(['ip', '-j', 'link', 'show']), default="[]")
    links = json.loads(links_json)

    # Fetch all forwarding sysctls in two calls instead of 2 per interface
    ipv4_sysctls = HOST.run(tuple(['sysctl', 'net.ipv4.conf']), default="")
    ipv6_sysctls = HOST.run(tuple(['sysctl', 'net.ipv6.conf']), default="")

    # Parse "net.ipv4.conf.<iface>.forwarding = 1" lines into a set
    ipv4_fwd = set()
    ipv6_fwd = set()
    for line in ipv4_sysctls.splitlines():
        if '.forwarding = 1' in line:
            # net.ipv4.conf.IFNAME.forwarding = 1
            parts = line.split('.')
            if len(parts) >= 5:
                ipv4_fwd.add(parts[3])

    for line in ipv6_sysctls.splitlines():
        if '.force_forwarding = 1' in line:
            # net.ipv6.conf.IFNAME.force_forwarding = 1
            parts = line.split('.')
            if len(parts) >= 5:
                ipv6_fwd.add(parts[3])

    routing_ifaces = []
    for link in links:
        ifname = link.get('ifname')
        if not ifname:
            continue

        if ifname in ipv4_fwd or ifname in ipv6_fwd:
            routing_ifaces.append(ifname)

    return routing_ifaces


def operational():
    out = {
        "ietf-routing:routing": {
            "interfaces": {
                "interface": get_routing_interfaces()
            },
            "ribs":  {
                "rib": [{
                    "name": "ipv4",
                    "address-family": "ipv4"
                }, {
                    "name": "ipv6",
                    "address-family": "ipv6"
                }]
            }
        }
    }

    ipv4routes = out['ietf-routing:routing']['ribs']['rib'][0]
    ipv6routes = out['ietf-routing:routing']['ribs']['rib'][1]
    add_protocol(ipv4routes, "ipv4")
    add_protocol(ipv6routes, "ipv6")

    return out
