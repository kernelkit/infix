#!/usr/bin/env python3
import logging
import logging.handlers
import subprocess
import json
import sys  # (built-in module)
import os
import argparse
from re import match
from datetime import datetime, timedelta, timezone

from . import common
from . import host

TESTPATH = ""
logger = None

def datetime_now():
    if TESTPATH:
        return datetime(2023, 1, 1, 12, 0, 0, tzinfo=timezone.utc)
    return datetime.now(timezone.utc)

def get_proc_value(procfile):
    """Return contents of /proc file, or None"""
    try:
        with open(procfile, 'r') as file:
            data = file.read().strip()
            return data
    except FileNotFoundError:
        # This is considered OK
        return None
    except IOError:
        logger.error(f"failed reading from {procfile}")


def lookup(obj, *keys):
    """This function returns a value from a nested json object"""
    curr = obj
    for key in keys:
        if isinstance(curr, dict) and key in curr:
            curr = curr[key]
        else:
            return None
    return curr


def insert(obj, *path_and_value):
    """"This function inserts a value into a nested json object"""
    if len(path_and_value) < 2:
        raise ValueError("Error: insert() takes at least two args")

    *path, value = path_and_value

    curr = obj
    for key in path[:-1]:
        if key not in curr or not isinstance(curr[key], dict):
            curr[key] = {}
        curr = curr[key]

    curr[path[-1]] = value


def run_cmd(cmd, testfile, default=None, check=True):
    """Run a command (array of args) and return an array of lines"""

    if TESTPATH and testfile:
        cmd = ['cat', os.path.join(TESTPATH, testfile)]

    try:
        result = subprocess.run(cmd, check=check, stderr=subprocess.DEVNULL,
                                stdout=subprocess.PIPE, text=True)
        output = result.stdout
        return output.splitlines()
    except subprocess.CalledProcessError as err:
        logger.error(f"{err}")
        if default is not None:
            return default
        raise


def run_json_cmd(cmd, testfile, default=None, check=True):
    """Run a command (array of args) with JSON output and return the JSON"""

    if TESTPATH and testfile:
        cmd = ['cat', os.path.join(TESTPATH, testfile)]

    try:
        result = subprocess.run(cmd, check=check, stdout=subprocess.PIPE,
                                stderr=subprocess.PIPE, text=True)
        output = result.stdout
        data = json.loads(output)
    except subprocess.CalledProcessError as err:
        logger.error(f"{err}")
        if default is not None:
            return default
        raise
    except json.JSONDecodeError as err:
        if check is True:
            logger.error("failed parsing JSON output of command: "
                         f"{' '.join(cmd)}, error: {err}")
        if default is not None:
            return default
        raise
    return data

def json_get_yang_type(iface_in):
    if iface_in['link_type'] == "loopback":
        return "infix-if-type:loopback"

    if iface_in['link_type'] in ("gre", "gre6"):
        return "infix-if-type:gre"

    if iface_in['link_type'] != "ether":
        return "infix-if-type:other"

    if 'parentbus' in iface_in and iface_in['parentbus'] == "virtio":
        return "infix-if-type:etherlike"

    if 'linkinfo' not in iface_in:
        return "infix-if-type:ethernet"

    if 'info_kind' not in iface_in['linkinfo']:
        return "infix-if-type:ethernet"

    if iface_in['linkinfo']['info_kind'] == "veth":
        return "infix-if-type:veth"

    if iface_in['linkinfo']['info_kind'] in ("gretap", "ip6gretap"):
        return "infix-if-type:gretap"

    if iface_in['linkinfo']['info_kind'] == "vlan":
        return "infix-if-type:vlan"

    if iface_in['linkinfo']['info_kind'] == "bridge":
        return "infix-if-type:bridge"

    if iface_in['linkinfo']['info_kind'] == "dsa":
        return "infix-if-type:ethernet"

    if iface_in['linkinfo']['info_kind'] == "dummy":
        return "infix-if-type:dummy"

    # Fallback
    return "infix-if-type:ethernet"


def json_get_yang_origin(addr):
    """Translate kernel IP address origin to YANG"""
    xlate = {
        "kernel_ll":        "link-layer",
        "kernel_ra":        "link-layer",
        "static":           "static",
        "dhcp":             "dhcp",
        "random":           "random",
    }
    proto = addr['protocol']

    if proto in ("kernel_ll", "kernel_ra"):
        if "stable-privacy" in addr:
            return "random"

    return xlate.get(proto, "other")




def iface_is_dsa(iface_in):
    """Check if interface is a DSA/intra-switch port"""
    if "linkinfo" not in iface_in:
        return False
    if "info_kind" not in iface_in['linkinfo']:
        return False
    if iface_in['linkinfo']['info_kind'] != "dsa":
        return False
    return True


def get_vpd_vendor_extensions(data):
    vendor_extensions = []
    for ext in data:
        vendor_extension = {}
        vendor_extension["iana-enterprise-number"] = ext[0]
        vendor_extension["extension-data"] = ext[1]
        vendor_extensions.append(vendor_extension)
    return vendor_extensions


def get_vpd_data(vpd):
    component = {}
    component["name"] = vpd.get("board")
    component["infix-hardware:vpd-data"] = {}

    if vpd.get("data"):
        component["class"] = "infix-hardware:vpd"
        if vpd["data"].get("manufacture-date"):
            component["mfg-date"] = datetime.strptime(vpd["data"]["manufacture-date"],"%m/%d/%Y %H:%M:%S").strftime("%Y-%m-%dT%H:%M:%SZ")
        if vpd["data"].get("manufacter"):
            component["mfg-name"] = vpd["data"]["manufacturer"]
        if vpd["data"].get("product-name"):
            component["model-name"] = vpd["data"]["product-name"]
        if vpd["data"].get("serial-number"):
            component["serial-num"] = vpd["data"]["serial-number"]

        # Set VPD-data entrys
        for k, v in vpd["data"].items():
            if vpd["data"].get(k):
                if k != "vendor-extension":
                    component["infix-hardware:vpd-data"][k] = v
                else:
                    vendor_extensions=get_vpd_vendor_extensions(v)
                    component["infix-hardware:vpd-data"]["infix-hardware:vendor-extension"] = vendor_extensions
    return component

def get_usb_ports(usb_ports):
    ports=[]
    names=[]
    for usb_port in usb_ports:
        port={}
        if usb_port.get("path"):
            if usb_port["name"] in names:
                continue

            path = usb_port["path"]
            if os.path.basename(path) == "authorized_default":
                if os.path.exists(path):
                    with open(path, "r") as f:
                        names.append(usb_port["name"])
                        data = int(f.readline().strip())
                        enabled = "unlocked" if data == 1 else "locked"
                        port["state"] = {}
                        port["state"]["admin-state"] = enabled
                        port["name"] = usb_port["name"]
                        port["class"] = "infix-hardware:usb"
                        port["state"]["oper-state"] = "enabled"
                        ports.append(port)

    return ports


def add_hardware(hw_out):
    data = run_json_cmd(['cat', "/run/system.json"], "system.json")
    components = []
    for _, vpd in data.get("vpd").items():
        component = get_vpd_data(vpd)
        components.append(component)
    if data.get("usb-ports", None):
        components.extend(get_usb_ports(data["usb-ports"]))
    insert(hw_out, "component", components)


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
    current_time = datetime_now()
    last_updated = current_time - uptime_delta
    date_timestd = last_updated.strftime('%Y-%m-%dT%H:%M:%S%z')

    return date_timestd[:-2] + ':' + date_timestd[-2:]


def get_routes(routes, proto, data):
    """Populate routes from vtysh JSON output"""

    # Mapping of FRR protocol names to IETF routing-protocol
    pmap = {
        'kernel': 'infix-routing:kernel',
        'connected': 'direct',
        'static': 'static',
        'ospf': 'ietf-ospf:ospfv2',
        'ospf6': 'ietf-ospf:ospfv3',
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

            # Metric only available in the model for OSPF routes
            if 'ospf' in frr:
                new['ietf-ospf:metric'] = route.get('metric', 0)

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


def add_ipv4_route(routes):
    """Fetch IPv4 routes from kernel and populate tree"""
    data = run_json_cmd(['sudo', 'vtysh', '-c', "show ip route json"],
                        "vtysh-ip4-route.json", check=False, default={})
    get_routes(routes, "ipv4", data)


def add_ipv6_route(routes):
    """Fetch IPv6 routes from kernel and populate tree"""
    data = run_json_cmd(['sudo', 'vtysh', '-c', "show ipv6 route json"],
                        "vtysh-ip6-route.json", check=False, default={})
    get_routes(routes, "ipv6", data)


def frr_to_ietf_neighbor_state(state):
    """Fetch OSPF neighbor state from Frr"""
    state = state.split("/")[0]
    if state == "TwoWay":
        return "2-way"
    return state.lower()


def add_ospf_routes(ospf):
    """Fetch OSPF routes from Frr"""
    cmd = ['vtysh', '-c', "show ip ospf rout json"]
    data = run_json_cmd(cmd, "", check=False, default=[])
    if data == []:
        return  # No OSPF routes available

    routes = []
    for prefix, info in data.items():
        if prefix.find("/") == -1:  # Ignore router IDs
            continue

        route = {}
        route["prefix"] = prefix

        nexthops = []
        routetype = info["routeType"].split(" ")

        if len(routetype) > 1:
            if routetype[1] == "E1":
                route["route-type"] = "external-1"
            elif routetype[1] == "E2":
                route["route-type"] = "external-2"
            elif routetype[1] == "IA":
                route["route-type"] = "inter-area"
        elif routetype[0] == "N":
            route["route-type"] = "intra-area"

        for hop in info["nexthops"]:
            nexthop = {}
            if hop["ip"] != " ":
                nexthop["next-hop"] = hop["ip"]
            else:
                nexthop["outgoing-interface"] = hop["directlyAttachedTo"]
            nexthops.append(nexthop)

        route["next-hops"] = {}
        route["next-hops"]["next-hop"] = nexthops
        routes.append(route)

    insert(ospf, "ietf-ospf:local-rib", "ietf-ospf:route", routes)


def add_ospf(control_protocols):
    """Populate OSPF status"""
    cmd = ['/usr/libexec/statd/ospf-status']
    data = run_json_cmd(cmd, "", check=False, default={})
    if data == {}:
        return  # No OSPF data available

    control_protocol = {}
    control_protocol["type"] = "infix-routing:ospfv2"
    control_protocol["name"] = "default"
    control_protocol["ietf-ospf:ospf"] = {}
    control_protocol["ietf-ospf:ospf"]["ietf-ospf:areas"] = {}


    control_protocol["ietf-ospf:ospf"]["ietf-ospf:router-id"] = data.get("routerId")
    control_protocol["ietf-ospf:ospf"]["ietf-ospf:address-family"] = "ipv4"
    areas = []

    for area_id, values in data.get("areas", {}).items():
        area = {}
        area["ietf-ospf:area-id"] = area_id
        area["ietf-ospf:interfaces"] = {}
        if values.get("area-type"):
            area["ietf-ospf:area-type"] = values["area-type"]
        interfaces = []
        for iface in values.get("interfaces", {}):
            interface = {}
            interface["ietf-ospf:neighbors"] = {}
            interface["name"] = iface["name"]

            if iface.get("drId"):
                interface["dr-router-id"] = iface["drId"]
            if iface.get("drAddress"):
                interface["dr-ip-addr"] = iface["drAddress"]
            if iface.get("bdrId"):
                interface["bdr-router-id"] = iface["bdrId"]
            if iface.get("bdrAddress"):
                interface["bdr-ip-addr"] = iface["bdrAddress"]

            if iface.get("timerPassiveIface"):
                interface["passive"] = True
            else:
                interface["passive"] = False

            interface["enabled"] = iface["ospfEnabled"]
            if iface["networkType"] == "POINTOPOINT":
                interface["interface-type"] = "point-to-point"
            if iface["networkType"] == "BROADCAST":
                interface["interface-type"] = "broadcast"

            if iface.get("state"):
                # Wev've never seen "DependUpon", and has no entry in
                # the YANG model, but is listed before down in Frr
                xlate = {
                    "DependUpon":     "down",
                    "Down":           "down",
                    "Waiting":        "waiting",
                    "Loopback":       "loopback",
                    "Point-To-Point": "point-to-point",
                    "DROther":        "dr-other",
                    "Backup":         "bdr",
                    "DR":             "dr"
                }
                val = xlate.get(iface["state"], "unknown")
                interface["state"] = val

            neighbors = []
            for neigh in iface["neighbors"]:
                neighbor = {}
                neighbor["neighbor-router-id"] = neigh["neighborIp"]
                neighbor["address"] = neigh["ifaceAddress"]
                neighbor["dead-timer"] = neigh["routerDeadIntervalTimerDueMsec"]
                neighbor["state"] = frr_to_ietf_neighbor_state(neigh["nbrState"])
                if neigh.get("routerDesignatedId"):
                    neighbor["dr-router-id"] = neigh["routerDesignatedId"]
                if neigh.get("routerDesignatedBackupId"):
                    neighbor["bdr-router-id"] = neigh["routerDesignatedBackupId"]
                neighbors.append(neighbor)

            interface["ietf-ospf:neighbors"] = {}
            interface["ietf-ospf:neighbors"]["ietf-ospf:neighbor"] = neighbors
            interfaces.append(interface)

        area["ietf-ospf:interfaces"]["ietf-ospf:interface"] = interfaces
        areas.append(area)

    add_ospf_routes(control_protocol["ietf-ospf:ospf"])
    control_protocol["ietf-ospf:ospf"]["ietf-ospf:areas"]["ietf-ospf:area"] = areas
    insert(control_protocols, "control-plane-protocol", [control_protocol])


def get_bridge_port_pvid(ifname):
    data = run_json_cmd(['bridge', '-j', 'vlan', 'show', 'dev', ifname],
                        f"bridge-vlan-show-dev-{ifname}.json")
    if len(data) != 1:
        return None

    iface = data[0]

    for vlan in iface['vlans']:
        if 'flags' in vlan and 'PVID' in vlan['flags']:
            return vlan['vlan']

    return None


def get_bridge_port_stp_state(ifname):
    data = run_json_cmd(['bridge', '-j', 'link', 'show', 'dev', ifname],
                        f"bridge-link-show-dev-{ifname}.json")
    if len(data) != 1:
        return None

    iface = data[0]

    states = ['disabled', 'listening', 'learning', 'forwarding', 'blocking']
    if 'state' in iface and iface['state'] in states:
        return iface['state']

    return None


def container_inspect(name):
    """Call podman inspect {name}, return object at {path} or None."""
    cmd = ['podman', 'inspect', name]
    try:
        return run_json_cmd(cmd, "", default=[])
    except Exception as e:
        logging.error(f"Error running podman inspect: {e}")
        return []


def add_container(containers):
    """We list *all* containers, not just those in the configuraion."""
    cmd = ['podman', 'ps', '-a', '--format=json']

    raw = run_json_cmd(cmd, "", default=[])
    for entry in raw:
        running = entry["State"] == "running"

        container = {
            "name":     entry["Names"][0],
            "id":       entry["Id"],
            "image":    entry["Image"],
            "image-id": entry["ImageID"],
            "running":  running,
            "status":   entry["Status"]
        }

        # Bonus information, may not be available
        if entry["Command"]:
            container["command"] = " ".join(entry["Command"])

        # The 'podman ps' command lists ports even in host mode, but
        # that's not applicable, so skip networks and port forwardings
        cont = container_inspect(container["name"])
        if cont and isinstance(cont, list) and len(cont) > 0:
            cont = cont[0]
        else:
            cont = {}

        networks = cont.get("NetworkSettings", {}).get("Networks")
        if networks and "host" in networks:
            container["network"] = {"host": True}
        else:
            container["network"] = {
                "interface": [],
                "publish": []
            }

            if entry["Networks"]:
                for net in entry["Networks"]:
                    container["network"]["interface"].append({"name": net})

            if running and entry["Ports"]:
                for port in entry["Ports"]:
                    addr = ""
                    if port["host_ip"]:
                        addr = f"{port['host_ip']}:"

                    pub = f"{addr}{port['host_port']}->{port['container_port']}/{port['protocol']}"
                    container["network"]["publish"].append(pub)

        containers.append(container)


def get_brport_multicast(ifname):
    """Check if multicast snooping is enabled on bridge, default: nope"""
    data = run_json_cmd(['mctl', 'show', 'igmp', 'json'], "igmp-status.json",
                        default={}, check=False)
    multicast = {}

    if ifname in data.get('fast-leave-ports', []):
        multicast["fast-leave"] = True
    else:
        multicast["fast-leave"] = False

    if ifname in data.get('multicast-router-ports', []):
        multicast["router"] = "permanent"
    else:
        multicast["router"] = "auto"

    return multicast


# We always get all interfaces for two reasons.
# 1) To increase speed on large iron with many ports.
# 2) To simplify testing (single dummy file ip-link-show.json).
def get_ip_link():
    """Fetch interface link information from kernel"""
    return run_json_cmd(['ip', '-s', '-d', '-j', 'link', 'show'],
                        "ip-link-show.json")

def netns_get_ip_link(netns):
    """Fetch interface link information from within a network namespace"""
    return run_json_cmd(['ip', 'netns', 'exec', netns, 'ip', '-s', '-d', '-j', 'link', 'show'],
                        f"netns-{netns}-ip-link-show.json")

def get_ip_addr():
    """Fetch interface address information from kernel"""
    return run_json_cmd(['ip', '-j', 'addr', 'show'],
                        "ip-addr-show.json")

def netns_get_ip_addr(netns):
    """Fetch interface address information from within a network namespace"""
    return run_json_cmd(['ip', 'netns', 'exec', netns, 'ip', '-j', 'addr', 'show'],
                        f"netns-{netns}-ip-addr-show.json")

def get_netns_list():
    """Fetch a list of network namespaces"""
    return run_json_cmd(['ip', '-j', 'netns', 'list'],
                        "netns-list.json")

def netns_find_ifname(ifname):
    """Find which network namespace owns ifname (if any)"""
    for netns in get_netns_list():
        for iface in netns_get_ip_link(netns['name']):
            if 'ifalias' in iface and iface['ifalias'] == ifname:
                    return netns['name']
    return None

def netns_ifindex_to_ifname(ifindex):
    """Look through all network namespaces for an interface index and return its name"""
    for netns in get_netns_list():
        for iface in netns_get_ip_link(netns['name']):
            if iface['ifindex'] == ifindex:
                if 'ifalias' in iface:
                    return iface['ifalias']
                if 'ifname' in iface:
                    return iface['ifname']
                return None

    return None

def add_bridge_port_common(ifname, iface_in, iface_out):
    li = iface_in.get("linkinfo", {})
    if not (li.get("info_slave_kind") == "bridge" or \
            li.get("info_kind") == "bridge"):
        return

    pvid = get_bridge_port_pvid(ifname)
    if pvid is not None:
        insert(iface_out, "infix-interfaces:bridge-port", "pvid", pvid)

def add_bridge_port_lower(ifname, iface_in, iface_out):
    li = iface_in.get("linkinfo", {})
    if not li.get("info_slave_kind") == "bridge":
        return

    insert(iface_out, "infix-interfaces:bridge-port", "bridge", iface_in['master'])

    stp_state = get_bridge_port_stp_state(ifname)
    if stp_state is not None:
        insert(iface_out, "infix-interfaces:bridge-port", "stp-state", stp_state)

    multicast = get_brport_multicast(ifname)
    insert(iface_out, "infix-interfaces:bridge-port", "multicast", multicast)


def add_gre(iface_in, iface_out):
     if 'link_type' in iface_in:
        val = json_get_yang_type(iface_in)
        if val != "infix-if-type:gre" and val != "infix-if-type:gretap":
            return;
        gre={}
        info_data=iface_in.get("linkinfo", {}).get("info_data", {})
        gre["local"] = info_data.get("local")
        gre["remote"] = info_data.get("remote")
        insert(iface_out, "infix-interfaces:gre", gre)


def add_ip_link(ifname, iface_in, iface_out):
    if 'ifname' in iface_in:
        iface_out['name'] = ifname

    if 'ifindex' in iface_in:
        iface_out['if-index'] = iface_in['ifindex']

    if 'ifalias' in iface_in:
        iface_out['description'] = iface_in['ifalias']

    if 'address' in iface_in and not "POINTOPOINT" in iface_in["flags"]:
        iface_out['phys-address'] = iface_in['address']

    add_bridge_port_common(ifname, iface_in, iface_out)
    add_bridge_port_lower(ifname, iface_in, iface_out)

    if not iface_is_dsa(iface_in):
        if iface_in.get('link'):
            insert(iface_out, "infix-interfaces:vlan", "lower-layer-if", iface_in['link'])
        elif 'link_index' in iface_in:
            # 'link_index' is the only reference we have if the link iface is in a namespace
            lower = netns_ifindex_to_ifname(iface_in['link_index'])
            if lower:
                insert(iface_out, "infix-interfaces:vlan", "lower-layer-if", lower)

    if 'flags' in iface_in:
        iface_out['admin-status'] = "up" if "UP" in iface_in['flags'] else "down"

    if 'operstate' in iface_in:
        xlate = {
                "DOWN":                "down",
                "UP":                  "up",
                "DORMANT":             "dormant",
                "TESTING":             "testing",
                "LOWERLAYERDOWN":      "lower-layer-down",
                "NOTPRESENT":          "not-present"
                }
        val = xlate.get(iface_in['operstate'], "unknown")
        iface_out['oper-status'] =  val

    if 'link_type' in iface_in:
        val = json_get_yang_type(iface_in)
        iface_out['type'] = val
    add_gre(iface_in, iface_out)

    val = lookup(iface_in, "stats64", "rx", "bytes")
    if val is not None:
        insert(iface_out, "statistics", "out-octets", str(val))

    val = lookup(iface_in, "stats64", "tx", "bytes")
    if val is not None:
        insert(iface_out, "statistics", "in-octets", str(val))


def add_ip_addr(ifname, iface_in, iface_out):
    if 'mtu' in iface_in and ifname != "lo":
        insert(iface_out, "ietf-ip:ipv4", "mtu", iface_in['mtu'])

    # We avoid importing os to check if the file exists (for performance)
    val = get_proc_value(f"/proc/sys/net/ipv6/conf/{ifname}/mtu")
    if val is not None:
        insert(iface_out, "ietf-ip:ipv6", "mtu", int(val))

    if 'addr_info' in iface_in:
        inet = []
        inet6 = []

        for addr in iface_in['addr_info']:
            new = {}

            if 'family' not in addr:
                logger.error("'family' missing from 'addr_info'")
                continue

            if 'local' in addr:
                new['ip'] = addr['local']
            if 'prefixlen' in addr:
                new['prefix-length'] = addr['prefixlen']
            if 'protocol' in addr:
                new['origin'] = json_get_yang_origin(addr)

            if addr['family'] == "inet":
                inet.append(new)
            elif addr['family'] == "inet6":
                inet6.append(new)
            else:
                logger.error("invalid 'family' in 'addr_info'")
                sys.exit(1)

        insert(iface_out, "ietf-ip:ipv4", "address", inet)
        insert(iface_out, "ietf-ip:ipv6", "address", inet6)


def add_ethtool_groups(ifname, iface_out):
    """Fetch interface counters from kernel (need new JSON format!)"""
    cmd = ['ethtool', '--json', '-S', ifname, '--all-groups']
    try:
        data = run_json_cmd(cmd, f"ethtool-groups-{ifname}.json")
        if len(data) != 1:
            logger.warning("%s: no counters available, skipping.", ifname)
            return
    except subprocess.CalledProcessError:
        # Allow comand to fail, not all NICs support --json yet
        return

    iface_in = data[0]

    # TODO: room for improvement, the "frame" creation could be more dynamic.
    if "eth-mac" in iface_in or "rmon" in iface_in:
        insert(iface_out, "ieee802-ethernet-interface:ethernet", "statistics", "frame", {})
        frame = iface_out['ieee802-ethernet-interface:ethernet']['statistics']['frame']

    if "eth-mac" in iface_in:
        mac_in = iface_in['eth-mac']

        if "FramesTransmittedOK" in mac_in:
            frame['out-frames'] = str(mac_in['FramesTransmittedOK'])
        if "MulticastFramesXmittedOK" in mac_in:
            frame['out-multicast-frames'] = str(mac_in['MulticastFramesXmittedOK'])
        if "BroadcastFramesXmittedOK" in mac_in:
            frame['out-broadcast-frames'] = str(mac_in['BroadcastFramesXmittedOK'])
        if "FramesReceivedOK" in mac_in:
            frame['in-frames'] = str(mac_in['FramesReceivedOK'])
        if "MulticastFramesReceivedOK" in mac_in:
            frame['in-multicast-frames'] = str(mac_in['MulticastFramesReceivedOK'])
        if "BroadcastFramesReceivedOK" in mac_in:
            frame['in-broadcast-frames'] = str(mac_in['BroadcastFramesReceivedOK'])
        if "FrameCheckSequenceErrors" in mac_in:
            frame['in-error-fcs-frames'] = str(mac_in['FrameCheckSequenceErrors'])
        if "FramesLostDueToIntMACRcvError" in mac_in:
            frame['in-error-mac-internal-frames'] = str(mac_in['FramesLostDueToIntMACRcvError'])

        if "OctetsTransmittedOK" in mac_in:
            frame['infix-ethernet-interface:out-good-octets'] = str(mac_in['OctetsTransmittedOK'])
        if "OctetsReceivedOK" in mac_in:
            frame['infix-ethernet-interface:in-good-octets'] = str(mac_in['OctetsReceivedOK'])

        tot = 0
        found = False
        if "FramesReceivedOK" in mac_in:
            tot += mac_in['FramesReceivedOK']
            found = True
        if "FrameCheckSequenceErrors" in mac_in:
            tot += mac_in['FrameCheckSequenceErrors']
            found = True
        if "FramesLostDueToIntMACRcvError" in mac_in:
            tot += mac_in['FramesLostDueToIntMACRcvError']
            found = True
        if "AlignmentErrors" in mac_in:
            tot += mac_in['AlignmentErrors']
            found = True
        if "etherStatsOversizePkts" in mac_in:
            tot += mac_in['etherStatsOversizePkts']
            found = True
        if "etherStatsJabbers" in mac_in:
            tot += mac_in['etherStatsJabbers']
            found = True
        if found:
            frame['in-total-frames'] = str(tot)

    if "rmon" in iface_in:
        rmon_in = iface_in['rmon']

        if "undersize_pkts" in rmon_in:
            frame['in-error-undersize-frames'] = str(rmon_in['undersize_pkts'])

        tot = 0
        found = False
        if "etherStatsJabbers" in rmon_in:
            tot += rmon_in['etherStatsJabbers']
            found = True
        if "etherStatsOversizePkts" in rmon_in:
            tot += rmon_in['etherStatsOversizePkts']
            found = True
        if found:
            frame['in-error-oversize-frames'] = str(tot)

def add_ethtool_std(ifname, iface_out):
    """Fetch interface speed/duplex/autoneg from kernel"""
    keys = ['Speed', 'Duplex', 'Auto-negotiation']
    result = {}

    lines = run_cmd(['ethtool', ifname], f"ethtool-{ifname}.txt")
    for line in lines:
        line = line.strip()
        key = line.split(':', 1)[0].strip()
        if key in keys:
            key, value = line.split(':', 1)
            result[key.strip()] = value.strip()

    if "Auto-negotiation" in result:
        if result['Auto-negotiation'] == "on":
            insert(iface_out, "ieee802-ethernet-interface:ethernet", "auto-negotiation", "enable", True)
        else:
            insert(iface_out, "ieee802-ethernet-interface:ethernet", "auto-negotiation", "enable", False)

    if "Duplex" in result:
        if result['Duplex'] == "Half":
            insert(iface_out, "ieee802-ethernet-interface:ethernet", "duplex", "half")
        elif result['Duplex'] == "Full":
            insert(iface_out, "ieee802-ethernet-interface:ethernet", "duplex", "full")
        else:
            insert(iface_out, "ieee802-ethernet-interface:ethernet", "duplex", "unknown")

    if "Speed" in result and result['Speed'] != "Unknown!":
        # Avoid importing re (performance)
        num = ''.join(filter(str.isdigit, result['Speed']))
        if num:
            num = round((int(num) / 1000), 3)
            insert(iface_out, "ieee802-ethernet-interface:ethernet", "speed", str(num))

def get_querier_data(querier_data):
    multicast={}

    if not querier_data:
        multicast["snooping"] = False
        return multicast
    multicast["snooping"] = True
    if(querier_data.get("query-interval")):
       multicast["query-interval"] = querier_data["query-interval"]

    return multicast

def get_multicast_filters(filters):
    multicast_filters=[]
    for f in filters:
        multicast_filter={}
        multicast_filter["group"] = f["group"]
        multicast_filter["ports"] = []
        for p in f["ports"]:
            port={}
            port["port"] = p
            multicast_filter["ports"].append(port)
        multicast_filters.append(multicast_filter)
    return multicast_filters

def add_mdb_to_bridge(brname, iface_out, mc_status):
    filters = [entry for entry in mc_status.get("multicast-groups", [])  if entry.get('vid') == None and entry.get('bridge') == brname]
    querier = next((querier for querier in mc_status.get('multicast-queriers', []) if (querier.get("interface", "") == brname) and (querier.get("vid") is None)), None)
    multicast = get_querier_data(querier)
    multicast_filters = get_multicast_filters(filters)
    insert(iface_out, "infix-interfaces:bridge", "multicast", multicast)
    insert(iface_out, "infix-interfaces:bridge", "multicast-filters", "multicast-filter", multicast_filters)

def add_container_ifaces(yang_ifaces):
    """Add all podman interfaces with limited data"""
    interfaces={}
    try:
         containers = run_json_cmd(['podman', 'ps', '--format', 'json'], "podman-ps.json", default=[])
    except Exception as e:
        logging.error(f"Error, unable to run podman: {e}")
        return

    for container in containers:
        name = container.get('Names', ['Unknown'])[0]
        networks = container.get('Networks', [])

        for network in networks:
            if not network in interfaces:
                interfaces[network] = []
            if name not in interfaces[network]:
                interfaces[network].append(name)

    for ifname, containers in interfaces.items():
        iface_out = {}
        iface_out['name'] = ifname
        iface_out['type'] = "infix-if-type:other" # Fallback
        insert(iface_out, "infix-interfaces:container-network", "containers", containers)

        netns = netns_find_ifname(ifname)
        if netns is not None:
            ip_link_data = netns_get_ip_link(netns)
            ip_link_data = next((d for d in ip_link_data if d.get('ifalias') == ifname), None)
            add_ip_link(ifname, ip_link_data, iface_out)

        yang_ifaces.append(iface_out)

# Helper function to add tagged/untagged interfaces to a vlan dict in a list
def _add_vlan_iface(vlans, multicast_filter, multicast, vid, key, val):
    for d in vlans:
        if d['vid'] == vid:
            if key in d:
                d[key].append(val)
            else:
                d[key] = [val]
            return

    vlans.append({'vid': vid, "multicast-filters": {"multicast-filter": multicast_filter}, "multicast": multicast, key: [val]})

def add_vlans_to_bridge(brname, iface_out, mc_status):
    slaves = [] # Contains all interfaces that has this bridge as 'master'
    for iface in run_json_cmd(['bridge', '-j', 'link'], "bridge-link.json"):
        if "master" in iface and iface['master'] == brname:
            slaves.append(iface['ifname'])

    vlans = [] # Contains all vlans and slaves belonging to this bridge
    for iface in run_json_cmd(['bridge', '-j', 'vlan'], "bridge-vlan.json"):
        if iface['ifname'] not in slaves and iface['ifname'] != brname:
            continue
        for vlan in iface['vlans']:
            querier = next((querier for querier in mc_status.get('multicast-queriers', []) if (querier.get("vid") == vlan['vlan'])), None)
            filters = [entry for entry in mc_status.get("multicast-groups", []) if (entry.get("vid") == vlan["vlan"])]
            if 'flags' in vlan and 'Egress Untagged' in vlan['flags']:
                _add_vlan_iface(vlans, get_multicast_filters(filters), get_querier_data(querier), vlan['vlan'], 'untagged', iface['ifname'])
            else:
                _add_vlan_iface(vlans, get_multicast_filters(filters), get_querier_data(querier), vlan['vlan'], 'tagged', iface['ifname'])

    insert(iface_out, "infix-interfaces:bridge", "vlans", "vlan", vlans)

def get_iface_data(ifname, ip_link_data, ip_addr_data):
    iface_out = {}

    add_ip_link(ifname, ip_link_data, iface_out)
    add_ip_addr(ifname, ip_addr_data, iface_out)

    if 'type' in iface_out and iface_out['type'] == "infix-if-type:ethernet":
        add_ethtool_groups(ifname, iface_out)
        add_ethtool_std(ifname, iface_out)

    if 'type' in iface_out and iface_out['type'] == "infix-if-type:bridge":
        # Fail silent, multicast snooping may not be enabled on bridge
        mc_status = run_json_cmd(['mctl', '-p', 'show', 'igmp', 'json'],
                                 "igmp-status.json", default={},
                                 check=False)

        add_vlans_to_bridge(ifname, iface_out, mc_status)
        add_mdb_to_bridge(ifname, iface_out, mc_status)

    return iface_out

def _add_interface(ifname, ip_link_data, ip_addr_data, yang_ifaces):
    # We expect both ip addr and link data to exist.
    if not ip_link_data or not ip_addr_data:
        return

    # Skip internal interfaces.
    if 'group' in ip_link_data and ip_link_data['group'] == "internal":
        return

    yang_ifaces.append(get_iface_data(ifname, ip_link_data, ip_addr_data))

def add_interface(ifname, yang_ifaces):
    ip_link_data = get_ip_link()
    ip_addr_data = get_ip_addr()

    if ifname:
        ip_link_data = next((d for d in ip_link_data if d.get('ifname') == ifname), None)
        ip_addr_data = next((d for d in ip_addr_data if d.get('ifname') == ifname), None)
        _add_interface(ifname, ip_link_data, ip_addr_data, yang_ifaces)
    else:
        for link in ip_link_data:
            addr = next((d for d in ip_addr_data if d.get('ifname') == link["ifname"]), None)
            _add_interface(link["ifname"], link, addr, yang_ifaces)

        add_container_ifaces(yang_ifaces)

def main():
    global TESTPATH
    global logger

    parser = argparse.ArgumentParser(description="YANG data creator")
    parser.add_argument("model", help="YANG Model")
    parser.add_argument("-p", "--param", default=None, help="Model dependent parameter")
    parser.add_argument("-t", "--test", default=None, help="Test data base path")
    args = parser.parse_args()


    # Set up syslog output for critical errors to aid debugging
    logger = logging.getLogger('yanger')
    if os.path.exists('/dev/log'):
        log = logging.handlers.SysLogHandler(address='/dev/log')
    else:
        # Use /dev/null as a fallback for unit tests
        log = logging.FileHandler('/dev/null')

    fmt = logging.Formatter('%(name)s[%(process)d]: %(message)s')
    log.setFormatter(fmt)
    logger.setLevel(logging.INFO)
    logger.addHandler(log)
    common.LOG = logger

    if args.test:
        TESTPATH = args.test
        host.HOST = host.Testhost(args.test)
    else:
        TESTPATH = ""
        host.HOST = host.Localhost()

    if args.model == 'ietf-interfaces':
        yang_data = {
            "ietf-interfaces:interfaces": {
                "interface": []
            }
        }
        add_interface(args.param, yang_data['ietf-interfaces:interfaces']['interface'])

    elif args.model == 'ietf-routing':
        yang_data = {
            "ietf-routing:routing": {
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

        ipv4routes = yang_data['ietf-routing:routing']['ribs']['rib'][0]
        ipv6routes = yang_data['ietf-routing:routing']['ribs']['rib'][1]
        add_ipv4_route(ipv4routes)
        add_ipv6_route(ipv6routes)

    elif args.model == 'ietf-ospf':
        yang_data = {
            "ietf-routing:routing": {
                "control-plane-protocols": {
                }
            }
        }
        add_ospf(yang_data['ietf-routing:routing']['control-plane-protocols'])

    elif args.model == 'ietf-hardware':
        yang_data = {
            "ietf-hardware:hardware": {
            }
        }
        add_hardware(yang_data["ietf-hardware:hardware"])

    elif args.model == 'infix-containers':
        yang_data = {
            "infix-containers:containers": {
                "container": []
            }
        }
        add_container(yang_data['infix-containers:containers']['container'])

    elif args.model == 'ietf-system':
        from . import ietf_system
        yang_data = ietf_system.operational()
    else:
        logger.warning(f"Unsupported model {args.model}", file=sys.stderr)
        sys.exit(1)

    print(json.dumps(yang_data, indent=2))


if __name__ == "__main__":
    main()
