#!/usr/bin/env python3

import subprocess
import json
import sys # (built-in module)
import os
import argparse
from datetime import datetime

TESTPATH = ""

def json_get_yang_type(iface_in):
    if iface_in['link_type'] == "loopback":
        return "infix-if-type:loopback"

    if iface_in['link_type'] != "ether":
        return "infix-if-type:other"

    if 'parentbus' in iface_in and iface_in['parentbus'] == "virtio":
        return "infix-if-type:etherlike"

    if not 'linkinfo' in iface_in:
        return "infix-if-type:ethernet"

    if not 'info_kind' in iface_in['linkinfo']:
        return "infix-if-type:ethernet"

    if iface_in['linkinfo']['info_kind'] == "veth":
        return "infix-if-type:veth"

    if iface_in['linkinfo']['info_kind'] == "vlan":
        return "infix-if-type:vlan"

    if iface_in['linkinfo']['info_kind'] == "bridge":
        return "infix-if-type:bridge"

    if iface_in['linkinfo']['info_kind'] == "dsa":
        return "infix-if-type:ethernet"

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

def getitem(data, key):
    """Get sub-object from an object"""
    while key:
        data = data[key[0]]
        key = key[1:]

    return data

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
        print(f"Error: reading from {procfile}", file=sys.stderr)

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

def run_cmd(cmd, testfile):
    """Run a command (array of args) and return an array of lines"""

    if TESTPATH and testfile:
        cmd = ['cat', os.path.join(TESTPATH, testfile)]

    try:
        output = subprocess.check_output(cmd, stderr=subprocess.DEVNULL, text=True)
        return output.splitlines()
    except subprocess.CalledProcessError:
        print("Error: command returned error", file=sys.stderr)
        sys.exit(1)

def run_json_cmd(cmd, testfile):
    """Run a command (array of args) that returns JSON text output and return JSON"""

    if TESTPATH and testfile:
        cmd = ['cat', os.path.join(TESTPATH, testfile)]

    try:
        result = subprocess.run(cmd, check=True, stdout=subprocess.PIPE,
                                stderr=subprocess.PIPE, text=True)
        output = result.stdout
        data = json.loads(output)
    except subprocess.CalledProcessError as err:
        data = {}
    except json.JSONDecodeError as err:
        print(f"Error: parsing JSON output: {err.msg}", file=sys.stderr)
        sys.exit(1)
    return data

def iface_is_dsa(iface_in):
    """Check if interface is a DSA/intra-switch port"""
    if not "linkinfo" in iface_in:
        return False
    if not "info_kind" in iface_in['linkinfo']:
        return False
    if iface_in['linkinfo']['info_kind'] != "dsa":
        return False
    return True

def get_vpd_vendor_extensions(data):
    vendor_extensions=[]
    for ext in data:
        vendor_extension = {}
        vendor_extension["iana-enterprise-number"] = ext[0]
        vendor_extension["extension-data"] = ext[1]
        vendor_extensions.append(vendor_extension)
    return vendor_extensions

def get_vpd_data(vpd):
    component={}
    component["name"]=vpd.get("board")
    component["infix-hardware:vpd-data"] = {}

    if vpd.get("data"):
        component["class"] = "infix-hardware:vpd"
        if vpd["data"].get("manufacture-date"):
            component["mfg-date"]=datetime.strptime(vpd["data"]["manufacture-date"],"%m/%d/%Y %H:%M:%S").strftime("%Y-%m-%dT%H:%M:%SZ")
        if vpd["data"].get("manufacter"):
            component["mfg-name"]=vpd["data"]["manufacturer"]
        if vpd["data"].get("product-name"):
            component["model-name"]=vpd["data"]["product-name"]
        if vpd["data"].get("serial-number"):
            component["serial-num"]=vpd["data"]["serial-number"]

        # Set VPD-data entrys
        for k,v in vpd["data"].items():
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
                        data=int(f.readline().strip())
                        enabled="unlocked" if data == 1 else "locked"
                        port["state"] = {}
                        port["state"]["admin-state"] = enabled
                        port["name"] = usb_port["name"]
                        port["class"] = "infix-hardware:usb"
                        port["state"]["oper-state"] = "enabled"
                        ports.append(port)

    return ports


def add_hardware(hw_out):
    data = run_json_cmd(['cat', "/run/system.json"], "system.json")
    components=[]
    for _,vpd in data.get("vpd").items():
        component=get_vpd_data(vpd)
        components.append(component)
    if data.get("usb-ports", None):
        components.extend(get_usb_ports(data["usb-ports"]))
    insert(hw_out, "component", components)

def get_routes(routes, proto, data):
    """Populate routes"""
    out={}
    out["route"] = []

    if proto == "ipv4":
        default = "0.0.0.0/0"
        host_prefix_length="32"
    else:
        default = "::/0"
        host_prefix_length="128"

    for entry in data:
        new = {}
        if entry['dst'] == "default":
            entry['dst'] = default
        if entry['dst'].find('/') == -1:
            entry['dst'] = entry['dst'] + "/" + host_prefix_length
        new[f'ietf-{proto}-unicast-routing:destination-prefix'] = entry['dst']
        new['source-protocol'] = "infix-routing:" + entry['protocol']
        if entry.get("metric"):
            new['route-preference'] = entry['metric']
        else:
            new['route-preference'] = 0

        if entry.get('nexthops'):
            next_hops = []
            for hop in entry.get('nexthops'):
                next_hop = {}
                if hop.get("dev"):
                    next_hop['outgoing-interface'] = hop['dev']
                if hop.get("gateway"):
                    next_hop[f'ietf-{proto}-unicast-routing:address'] = hop['gateway']
                next_hops.append(next_hop)

            insert(new, 'next-hop', 'next-hop-list', 'next-hop', next_hops)
        else:
            next_hop = {}
            if entry['type'] == "blackhole":
                next_hop['special-next-hop'] = "blackhole"
            if entry['type'] == "unreachable":
                next_hop['special-next-hop'] = "unreachable"
            if entry['type'] == "unicast":
                if entry.get("dev"):
                    next_hop['outgoing-interface'] = entry['dev']
                if entry.get("gateway"):
                    next_hop[f'ietf-{proto}-unicast-routing:next-hop-address'] = entry['gateway']

            new['next-hop'] = next_hop

        out['route'].append(new)

    insert(routes, 'routes', out)

def add_ipv4_route(routes):
    """Fetch IPv4 routes from kernel and populate tree"""
    data = run_json_cmd(['ip', '-4', '-s', '-d', '-j', 'route'], "ip-4-route.json")
    get_routes(routes, "ipv4", data)

def add_ipv6_route(routes):
    """Fetch IPv6 routes from kernel and populate tree"""
    data = run_json_cmd(['ip', '-6', '-s', '-d', '-j', 'route'], "ip-6-route.json")
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
    data = run_json_cmd(cmd, "")
    routes=[]

    for prefix,info in data.items():
        if prefix.find("/") == -1: # Ignore router IDs
            continue

        route={}
        route["prefix"] = prefix

        nexthops=[]
        routetype=info["routeType"].split(" ")

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
            nexthop={}
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
    data = run_json_cmd(cmd, "")

    if data == {}:
        return # No OSPF data available

    control_protocol={}
    control_protocol["type"] = "ietf-ospf:ospfv2"
    control_protocol["name"] = "default"
    control_protocol["ietf-ospf:ospf"] = {}
    control_protocol["ietf-ospf:ospf"]["ietf-ospf:areas"] = {}


    control_protocol["ietf-ospf:ospf"]["ietf-ospf:router-id"] = data.get("routerId")
    control_protocol["ietf-ospf:ospf"]["ietf-ospf:address-family"] = "ipv4"
    areas=[]

    for area_id,values in data.get("areas", {}).items():
        area={}
        area["ietf-ospf:area-id"] = area_id
        area["ietf-ospf:interfaces"] = {}
        if values.get("area-type"):
            area["ietf-ospf:area-type"] = values["area-type"]
        interfaces=[]
        for iface in values.get("interfaces", {}):
            interface={}
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
                neighbor={}
                neighbor["neighbor-router-id"] = neigh["neighborIp"]
                neighbor["address"] = neigh["ifaceAddress"]
                neighbor["dr-router-id"] = neigh["routerDesignatedId"]
                neighbor["bdr-router-id"] = neigh["routerDesignatedBackupId"]
                neighbor["dead-timer"] = neigh["routerDeadIntervalTimerDueMsec"]
                neighbor["state"]=frr_to_ietf_neighbor_state(neigh["nbrState"])
                neighbors.append(neighbor)

            interface["ietf-ospf:neighbors"] = {}
            interface["ietf-ospf:neighbors"]["ietf-ospf:neighbor"] = neighbors
            interfaces.append(interface)

        area["ietf-ospf:interfaces"]["ietf-ospf:interface"] = interfaces
        areas.append(area)

    add_ospf_routes(control_protocol["ietf-ospf:ospf"]);
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

    states = [ 'disabled', 'listening', 'learning', 'forwarding', 'blocking' ]
    if 'state' in iface and iface['state'] in states:
        return iface['state']

    return None

def container_inspect(name, key):
    """Call podman inspect {name}, return object at {path} or None"""
    cmd = ['podman', 'inspect', name]
    raw = run_json_cmd(cmd, "")

    return getitem(raw[0], key)

def add_container(containers):
    """In container-state we list *all* containers, not just ones manged in configuration."""
    cmd = ['podman', 'ps', '-a', '--format=json']

    raw = run_json_cmd(cmd, "")
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
        networks = container_inspect(container["name"], ("NetworkSettings", "Networks"))
        if "host" in networks:
            container["network"] = { "host": True }
        else:
            container["network"] = {
                "interface": [],
                "publish": []
            }

            if entry["Networks"]:
                for net in entry["Networks"]:
                    container["network"]["interface"].append({ "name": net })

            if running and entry["Ports"]:
                for port in entry["Ports"]:
                    addr = ""
                    if port["host_ip"]:
                        addr = f"{port['host_ip']}:"

                    pub = f"{addr}{port['host_port']}->{port['container_port']}/{port['protocol']}"
                    container["network"]["publish"].append(pub)

        containers.append(container)

def get_brport_multicast(ifname):
    data=run_json_cmd(['mctl', 'show', 'igmp', 'json'], "bridge-mdb.json")
    multicast={}
    if ifname in data.get('fast-leave-ports', []):
        multicast["fast-leave"] = True
    else:
        multicast["fast-leave"] = False
    if ifname in data.get('multicast-router-ports', []):
        multicast["router"] = "permanent";
    else:
        multicast["router"] = "auto";
    return multicast

def add_ip_link(ifname, iface_out):
    """Fetch interface link information from kernel"""
    data = run_json_cmd(['ip', '-s', '-d', '-j', 'link', 'show', 'dev', ifname],
            f"ip-link-show-dev-{ifname}.json")
    if len(data) != 1:
        print("Error: expected ip link output to be array with length 1", file=sys.stderr)
        sys.exit(1)

    iface_in = data[0]

    if 'ifname' in iface_in:
        iface_out['name'] = iface_in['ifname']

    if 'ifindex' in iface_in:
        iface_out['if-index'] = iface_in['ifindex']

    if 'address' in iface_in:
        iface_out['phys-address'] = iface_in['address']

    if 'master' in iface_in:
        insert(iface_out, "infix-interfaces:bridge-port", "bridge", iface_in['master'])

        pvid = get_bridge_port_pvid(ifname)
        if pvid is not None:
            insert(iface_out, "infix-interfaces:bridge-port", "pvid", pvid)

        stp_state = get_bridge_port_stp_state(ifname)
        if stp_state is not None:
            insert(iface_out, "infix-interfaces:bridge-port", "stp-state", stp_state)

        multicast = get_brport_multicast(ifname)
        insert(iface_out, "infix-interfaces:bridge-port", "multicast", multicast)
    if 'link' in iface_in and not iface_is_dsa(iface_in):
        insert(iface_out, "infix-interfaces:vlan", "lower-layer-if", iface_in['link'])

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

    val = lookup(iface_in, "stats64", "rx", "bytes")
    if val is not None:
        insert(iface_out, "statistics", "out-octets", str(val))

    val = lookup(iface_in, "stats64", "tx", "bytes")
    if val is not None:
        insert(iface_out, "statistics", "in-octets", str(val))

def add_ip_addr(ifname, iface_out):
    """Fetch interface address information from kernel"""

    data = run_json_cmd(['ip', '-j', 'addr', 'show', 'dev', ifname],
            f"ip-addr-show-dev-{ifname}.json")
    if len(data) != 1:
        print("Error: expected ip addr output to be array with length 1", file=sys.stderr)
        sys.exit(1)

    iface_in = data[0]

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

            if not 'family' in addr:
                print("Error: 'family' missing from 'addr_info'", file=sys.stderr)
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
                print("Error: invalid 'family' in 'addr_info'", file=sys.stderr)
                sys.exit(1)

        insert(iface_out, "ietf-ip:ipv4", "address", inet)
        insert(iface_out, "ietf-ip:ipv6", "address", inet6)

def add_ethtool_groups(ifname, iface_out):
    """Fetch interface counters from kernel"""

    data = run_json_cmd(['ethtool', '--json', '-S', ifname, '--all-groups'],
                        f"ethtool-groups-{ifname}.json")
    if len(data) != 1:
        print(f"Error: no counters available for {ifname}, skipping.")
        return

    iface_in = data[0]

    # TODO: room for improvement here, the "frame" creation could be more dynamic.
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

def main():
    global TESTPATH

    parser = argparse.ArgumentParser(description="YANG data creator")
    parser.add_argument("model", help="YANG Model")
    parser.add_argument("-p", "--param", default=None, help="Model dependant parameter")
    parser.add_argument("-t", "--test", default=None, help="Test data base path")
    args = parser.parse_args()

    if args.test:
        TESTPATH = args.test
    else:
        TESTPATH = ""

    if args.model == 'ietf-interfaces':
        # For now, we handle each interface separately, as this is how it's
        # currently implemented in sysrepo. I.e sysrepo will subscribe to
        # each individual interface and query it for YANG data.

        if not args.param:
            print("usage: yanger ietf-interfaces -p INTERFACE", file=sys.stderr)
            sys.exit(1)

        yang_data = {
            "ietf-interfaces:interfaces": {
                "interface": [{}]
            }
        }

        ifname = args.param
        iface_out = yang_data['ietf-interfaces:interfaces']['interface'][0]

        add_ip_link(ifname, iface_out, )
        add_ip_addr(ifname, iface_out)

        if 'type' in iface_out and iface_out['type'] == "infix-if-type:ethernet":
            add_ethtool_groups(ifname, iface_out)
            add_ethtool_std(ifname, iface_out)

        if 'type' in iface_out and iface_out['type'] == "infix-if-type:bridge":
            mc_status = run_json_cmd(['mctl', '-p', 'show', 'igmp', 'json'], "igmp-status.json")

            add_vlans_to_bridge(ifname, iface_out, mc_status)
            add_mdb_to_bridge(ifname, iface_out, mc_status)

    elif args.model == 'ietf-routing':
        yang_data = {
            "ietf-routing:routing": {
                "ribs":  {
                    "rib": [{
                        "name": "ipv4",
                        "address-family": "ipv4"
                    },
                    {
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

    else:
        print(f"Unsupported model {args.model}", file=sys.stderr)
        sys.exit(1)

    print(json.dumps(yang_data, indent=2))

if __name__ == "__main__":
    main()
