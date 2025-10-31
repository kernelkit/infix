#!/usr/bin/env python3
"""
Collect operational data for infix-firewall.yang from firewalld using D-Bus,
for the full API, see:

   gdbus introspect --system --dest org.fedoraproject.FirewallD1 \
                    --object-path /org/fedoraproject/FirewallD1
"""
import dbus
import re
from . import common


def get_interface(interface="org.fedoraproject.FirewallD1"):
    try:
        bus = dbus.SystemBus()
        obj = bus.get_object("org.fedoraproject.FirewallD1",
                             "/org/fedoraproject/FirewallD1")
        return dbus.Interface(obj, dbus_interface=interface)

    except dbus.exceptions.DBusException as e:
        common.LOG.warning("Failed to connect to firewalld D-Bus: %s", e)
        return None


def get_zone_data(fw, name):
    """
    $ gdbus call --system --dest org.fedoraproject.FirewallD1               \
                 --object-path /org/fedoraproject/FirewallD1                \
                 --method org.fedoraproject.FirewallD1.zone.getForwardPorts \
                 external
    ([['443', 'tcp', '443', '192.168.2.10']],)
    """
    try:
        settings = fw.getZoneSettings2(name)
        target = settings.get('target', 'default')
        action = {
            "%%REJECT%%": "reject",
            "REJECT": "reject",
            "ACCEPT": "accept",
            "DROP": "drop",
            "default": "accept"
        }

        short = settings.get('short', '')
        immutable = False
        if short and "(immutable)" in short:
            # Remove (immutable), added by us to set ⚷ symbol in output
            short = short.replace("(immutable)", "").strip()
            immutable = True
        elif not short:
            short = ""

        zone = {
            "name": name,
            "short": short,
            "immutable": immutable,
            "description": settings.get('description', ''),
            "interface": list(settings.get('interfaces', [])),
            "network": list(settings.get('sources', [])),
            "action": action.get(target, "accept"),
            "service": list(settings.get('services', []))
        }

        # Handle port forwarding from zone
        port_forwards = []
        forwards = settings.get('forward_ports', [])
        for fwd in forwards:
            try:
                if len(fwd) >= 4:
                    port, protocol, toport, toaddr = fwd[:4]  # Fixed field order!

                    # Handle port ranges: port can be "80" or "8000-8080"
                    if '-' in str(port):
                        port_lower, port_upper = str(port).split('-', 1)
                        fwd_data = {
                            'lower': int(port_lower),
                            'upper': int(port_upper),
                            'proto': str(protocol),
                            'to': {
                                'addr': str(toaddr)
                            }
                        }
                    else:
                        fwd_data = {
                            'lower': int(port),
                            'proto': str(protocol),
                            'to': {
                                'addr': str(toaddr)
                            }
                        }

                    # Handle destination port - only store lower port, upper calculated by C code
                    if toport and str(toport).strip():
                        toport_str = str(toport).strip()
                        # Skip if toport looks like an IP address instead of port
                        if '.' not in toport_str and ':' not in toport_str:
                            fwd_data['to']['port'] = int(toport_str)
                        else:
                            # If toport looks like IP, use the same port as source lower
                            fwd_data['to']['port'] = fwd_data['lower']
                    else:
                        # No destination port specified, use same as source lower
                        fwd_data['to']['port'] = fwd_data['lower']

                    port_forwards.append(fwd_data)

            except (ValueError, IndexError, TypeError) as e:
                common.LOG.warning("Invalid port forward rule in zone %s: %s", name, e)
                continue

        if port_forwards:
            zone["port-forward"] = port_forwards

        return zone

    except Exception as e:
        common.LOG.warning("Failed querying zone %s via D-Bus: %s", name, e)
        return None


def get_zones(fw):
    """Get only active zones (loaded in kernel) instead of all zones"""
    zones = []
    try:
        fwz = get_interface("org.fedoraproject.FirewallD1.zone")
        if not fwz:
            return zones

        active_zones = fwz.getActiveZones()
        for name, zone_info in active_zones.items():
            zone_data = get_zone_data(fwz, name)
            if zone_data:
                zone_data['interface'] = list(zone_info.get('interfaces', []))
                zone_data['network'] = list(zone_info.get('sources', []))
                zones.append(zone_data)

    except Exception as e:
        common.LOG.warning("Failed querying zones: %s", e)

    return zones


def get_policy_data(fw, name):
    try:
        settings = fw.getPolicySettings(name)
        policy = {
            "name": name,
            "action": "reject",
            "priority": 32767,
            "ingress": [],
            "egress": []
        }

        target = settings.get('target', 'CONTINUE')
        action = {
            "CONTINUE": "continue",
            "ACCEPT": "accept",
            "REJECT": "reject",
            "DROP": "drop"
        }
        policy["action"] = action.get(target, "reject")

        priority = settings.get('priority', 32767)
        if isinstance(priority, int):
            policy["priority"] = priority

        description = settings.get('description', '')
        if description:
            policy["description"] = description

        short = settings.get('short', '')
        policy["immutable"] = bool(short and "(immutable)" in short)

        ingress = settings.get('ingress_zones', [])
        if ingress:
            policy["ingress"] = list(ingress)

        egress = settings.get('egress_zones', [])
        if egress:
            policy["egress"] = list(egress)

        services = settings.get('services', [])
        if services:
            policy["service"] = list(services)

        policy["masquerade"] = bool(settings.get('masquerade', 0))

        # Handle custom filters from rich_rules
        custom_filters = []
        rich_rules = settings.get('rich_rules', [])

        for rule in rich_rules:
            # Extract family (default to both if not specified)
            family = "both"
            if 'family="ipv4"' in rule:
                family = "ipv4"
            elif 'family="ipv6"' in rule:
                family = "ipv6"

            icmp_type = None
            action = None
            prio = -1

            if 'priority' in rule:
                prio_match = re.search(r'.*priority=([^ ]+)', rule)
                if prio_match:
                    val = prio_match.group(1)
                    if isinstance(val, int):
                        prio = val

            if 'icmp-type' in rule and 'name=' in rule:
                name_match = re.search(r'.*name="([^"]+)"', rule)
                if name_match:
                    icmp_type = name_match.group(1)

                    action = "accept"
                    if ' drop' in rule:
                        action = "drop"
                    elif ' reject' in rule:
                        action = "reject"
            elif 'icmp-block' in rule and 'name=' in rule:
                name_match = re.search(r'.*name="([^"]+)"', rule)
                if name_match:
                    icmp_type = name_match.group(1)
                    action = "reject"

            if icmp_type and action:
                filter_entry = {
                    "name": f"icmp-{icmp_type}",
                    "priority": prio,
                    "family": family,
                    "action": action,
                    "icmp": {
                        "type": icmp_type
                    }
                }
                custom_filters.append(filter_entry)

        if custom_filters:
            policy["custom"] = {
                "filter": custom_filters
            }


        return policy

    except Exception as e:
        common.LOG.warning("Failed querying policy %s via D-Bus: %s", name, e)
        return None


def get_policies(fw):
    policies = []
    try:
        fwp = get_interface("org.fedoraproject.FirewallD1.policy")
        if not fwp:
            return policies

        for name in fwp.getPolicies():
            data = get_policy_data(fwp, name)
            if data:
                policies.append(data)

    except Exception as e:
        common.LOG.warning("Failed querying policies: %s", e)

    # Add implicit drop/reject policy as the last rule
    implicit_policy = {
        "name": "default-drop",
        "description": "Default deny rule - drops all unmatched traffic",
        "action": "drop",
        "priority": 32767,  # Highest priority number (lowest precedence)
        "ingress": ["ANY"],
        "egress": ["ANY"],
        "immutable": True
    }
    policies.append(implicit_policy)

    return policies


def get_service_data(fw, name):
    try:
        settings = fw.getServiceSettings2(name)

        service = {
            "name": name,
            "port": []
        }

        description = settings.get('description', '')
        if description:
            service["description"] = description

        ports = settings.get('ports', [])
        for port_info in ports:
            if len(port_info) >= 2:
                port, protocol = port_info[:2]
                port_data = {'proto': protocol}

                if '-' in str(port):
                    lower, upper = str(port).split('-', 1)
                    port_data['lower'] = int(lower)
                    port_data['upper'] = int(upper)
                else:
                    port_data['lower'] = int(port)

                service["port"].append(port_data)

        return service

    except Exception as e:
        common.LOG.warning("Failed querying service %s via D-Bus: %s", name, e)
        return None


def get_services(fw):
    services = []
    try:
        for name in fw.listServices():
            data = get_service_data(fw, name)
            if data:
                services.append(data)

    except Exception as e:
        common.LOG.warning("Failed querying services: %s", e)

    return services


def operational():
    try:
        fw = get_interface()
        if not fw:
            return {}

    except Exception as e:
        common.LOG.warning("Failed checking firewalld state: %s", e)
        return {}

    data = {
        "infix-firewall:firewall": {
            "default": fw.getDefaultZone(),
            "logging": fw.getLogDenied(),
            "lockdown": bool(fw.queryPanicMode())
        }
    }

    zones = get_zones(fw)
    if zones:
        data["infix-firewall:firewall"]["zone"] = zones

    policies = get_policies(fw)
    if policies:
        data["infix-firewall:firewall"]["policy"] = policies

    services = get_services(fw)
    if services:
        data["infix-firewall:firewall"]["service"] = services

    return data
