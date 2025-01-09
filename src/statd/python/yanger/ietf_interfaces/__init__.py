from ..common import insert, lookup, LOG
from ..host import HOST


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


def get_bridge_port_pvid(ifname):
    data = HOST.run_json(['bridge', '-j', 'vlan', 'show', 'dev', ifname])
    if len(data) != 1:
        return None

    iface = data[0]

    for vlan in iface['vlans']:
        if 'flags' in vlan and 'PVID' in vlan['flags']:
            return vlan['vlan']

    return None


def get_bridge_port_stp_state(ifname):
    data = HOST.run_json(['bridge', '-j', 'link', 'show', 'dev', ifname])
    if len(data) != 1:
        return None

    iface = data[0]

    states = ['disabled', 'listening', 'learning', 'forwarding', 'blocking']
    if 'state' in iface and iface['state'] in states:
        return iface['state']

    return None


def get_brport_multicast(ifname):
    """Check if multicast snooping is enabled on bridge, default: nope"""
    data = HOST.run_json(['mctl', '-p', 'show', 'igmp', 'json'], default={})
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
    return HOST.run_json(['ip', '-s', '-d', '-j', 'link', 'show'])

def netns_get_ip_link(netns):
    """Fetch interface link information from within a network namespace"""
    return HOST.run_json(['ip', 'netns', 'exec', netns, 'ip', '-s', '-d', '-j', 'link', 'show'])

def get_ip_addr():
    """Fetch interface address information from kernel"""
    return HOST.run_json(['ip', '-j', 'addr', 'show'])

def netns_get_ip_addr(netns):
    """Fetch interface address information from within a network namespace"""
    return HOST.run_json(['ip', 'netns', 'exec', netns, 'ip', '-j', 'addr', 'show'])

def get_netns_list():
    """Fetch a list of network namespaces"""
    return HOST.run_json(['ip', '-j', 'netns', 'list'], [])

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

    val = HOST.read(f"/proc/sys/net/ipv6/conf/{ifname}/mtu")
    if val is not None:
        insert(iface_out, "ietf-ip:ipv6", "mtu", int(val.strip()))

    if 'addr_info' in iface_in:
        inet = []
        inet6 = []

        for addr in iface_in['addr_info']:
            new = {}

            if 'family' not in addr:
                LOG.error("'family' missing from 'addr_info'")
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
                LOG.error("invalid 'family' in 'addr_info'")
                sys.exit(1)

        insert(iface_out, "ietf-ip:ipv4", "address", inet)
        insert(iface_out, "ietf-ip:ipv6", "address", inet6)


def add_ethtool_groups(ifname, iface_out):
    """Fetch interface counters from kernel (need new JSON format!)"""
    cmd = ['ethtool', '--json', '-S', ifname, '--all-groups']
    try:
        data = HOST.run_json(cmd)
        if len(data) != 1:
            LOG.warning("%s: no counters available, skipping.", ifname)
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

    lines = HOST.run_multiline(['ethtool', ifname])
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
         containers = HOST.run_json(['podman', 'ps', '-a', '--format=json'], default=[])
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
    for iface in HOST.run_json(['bridge', '-j', 'link']):
        if "master" in iface and iface['master'] == brname:
            slaves.append(iface['ifname'])

    vlans = [] # Contains all vlans and slaves belonging to this bridge
    for iface in HOST.run_json(['bridge', '-j', 'vlan']):
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
        mc_status = HOST.run_json(['mctl', '-p', 'show', 'igmp', 'json'], default={})

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

def operational(ifname=None):
    out = {
        "ietf-interfaces:interfaces": {
            "interface": []
        }
    }
    out_ifaces = out["ietf-interfaces:interfaces"]["interface"]

    ip_link_data = get_ip_link()
    ip_addr_data = get_ip_addr()

    if ifname:
        ip_link_data = next((d for d in ip_link_data if d.get('ifname') == ifname), None)
        ip_addr_data = next((d for d in ip_addr_data if d.get('ifname') == ifname), None)
        _add_interface(ifname, ip_link_data, ip_addr_data, out_ifaces)
    else:
        for link in ip_link_data:
            addr = next((d for d in ip_addr_data if d.get('ifname') == link["ifname"]), None)
            _add_interface(link["ifname"], link, addr, out_ifaces)

        add_container_ifaces(out_ifaces)

    return out
