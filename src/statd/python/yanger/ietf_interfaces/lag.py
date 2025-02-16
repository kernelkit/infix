"""The lag/bond oper-status always follows the carrier"""


def lower(iplink):
    """Return a dictionary of the status of a lag member"""
    port = {
        "lag": iplink['master'],
    }

    info = iplink['linkinfo']['info_slave_data']
    if info:
        # active or backup link
        port['state'] = info['state'].lower()
        port['link-failures'] = info['link_failure_count']

        # On etherlike interfaces and tap interfaces oper-status lies
        # if info['mii_status'] == "DOWN":
        #     iface['oper-status'] = "down"

        # Initialize lacp dict only if we encounter LACP-related fields
        if 'ad_aggregator_id' in info:
            port['lacp'] = {}
            port['lacp']['aggregator-id'] = info['ad_aggregator_id']
            port['lacp']['actor-state'] = info['ad_actor_oper_port_state_str']
            port['lacp']['partner-state'] = info['ad_partner_oper_port_state_str']
    else:
        port['state'] = 'backup'
        port['link-failures'] = 0

    return port


def lag(iplink):
    """Return a dictionary of the status of the lag"""
    mode = {
        "balance-xor":      "static",
        "802.3ad":          "lacp",
    }
    hash_policy = {
        "layer2": "layer2",
        "layer3+4": "layer3-4",
        "layer2+3": "layer2-3",
        "encap2+3": "encap2-3",
        "encap3+4": "encap3-4",
        "vlan+srcmac": "vlan-srcmac",
    }
    bond = {}

    info = iplink["linkinfo"]["info_data"]
    if info:
        bond["mode"] = mode.get(info['mode'], "static")
        if bond["mode"] == "lacp":
            bond["lacp"] = {
                "mode": 'active' if info['ad_lacp_active'] == "on" else 'passive',
                "rate": info['ad_lacp_rate'],
                "hash": hash_policy.get(info['xmit_hash_policy'], "layer2"),
            }

            if 'ad_info' in info:
                bond["lacp"]["aggregator-id"] = info['ad_info']['aggregator']
                bond["lacp"]["actor-key"] = info['ad_info']['actor_key']
                bond["lacp"]["partner-key"] = info['ad_info']['partner_key']
                bond["lacp"]["partner-mac"] = info['ad_info']['partner_mac']
            if 'ad_actor_sys_prio' in info:
                bond["lacp"]["system-priority"] = info['ad_actor_sys_prio']
        else:
            bond["static"] = {
                "mode": info['mode'],
                "hash": info['xmit_hash_policy']
            }

        bond["link-monitor"] = {
            "debounce": {
                "up": info['updelay'],
                "down": info['downdelay']
                }
        }

    return bond
