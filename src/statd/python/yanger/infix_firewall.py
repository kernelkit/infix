#!/usr/bin/env python3
"""
Collect operational data for infix-firewall.yang from firewalld using D-Bus,
for the full API, see:

   gdbus introspect --system --dest org.fedoraproject.FirewallD1 \
                    --object-path /org/fedoraproject/FirewallD1
"""
import dbus
from . import common


def get_interface(interface="org.fedoraproject.FirewallD1"):
    try:
        bus = dbus.SystemBus()
        obj = bus.get_object("org.fedoraproject.FirewallD1",
                             "/org/fedoraproject/FirewallD1")
        return dbus.Interface(obj, interface)

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
        zone = {
            "name": name,
            "policy": "accept",
            "interface": list(settings.get('interfaces', [])),
            "source": list(settings.get('sources', [])),
            "service": list(settings.get('services', [])),
            "port-forward": [],
            "forwarding": False
        }

        target = settings.get('target', 'default')
        policy = {
            "%%REJECT%%": "reject",
            "REJECT": "reject",
            "ACCEPT": "accept",
            "DROP": "drop",
            "default": "accept"
        }
        zone["policy"] = policy.get(target, "accept")
        zone["forwarding"] = bool(settings.get('forward', 0))
        zone["description"] = settings.get('description', 0)

        forwards = settings.get('forward_ports', [])
        for fwd in forwards:
            try:
                if len(fwd) >= 4:
                    port, protocol, toport, toaddr = fwd[:4]  # Fixed field order!

                    # TODO: extend YANG model with lower/upper
                    fwd_data = {
                        'port': int(port),  # TODO: Single port number now.
                        'proto': str(protocol),
                        'to': {
                            'addr': str(toaddr)
                        }
                    }

                    # Handle destination port - might be empty or IP
                    if toport and str(toport).strip():
                        toport_str = str(toport).strip()
                        # Skip if toport looks like an IP address instead of port
                        if '.' not in toport_str and ':' not in toport_str:
                            fwd_data['to']['port'] = int(toport_str)
                        else:
                            # If toport looks like IP, use the same port as source
                            fwd_data['to']['port'] = int(port)
                    else:
                        # No destination port specified, use same as source
                        fwd_data['to']['port'] = int(port)

                    zone["port-forward"].append(fwd_data)

            except (ValueError, TypeError) as e:
                common.LOG.warning("Skipping invalid port forward rule in zone %s: %s (data: %s)",
                                   name, e, fwd)
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
                # Override interfaces and sources with active zone data to ensure accuracy
                zone_data['interface'] = list(zone_info.get('interfaces', []))
                zone_data['source'] = list(zone_info.get('sources', []))
                zones.append(zone_data)

    except Exception as e:
        common.LOG.warning("Failed querying zones: %s", e)

    return zones


def get_policy_data(fw, name):
    try:
        settings = fw.getPolicySettings(name)

        policy = {
            "name": name,
            "policy": "reject",
            # "priority": 32767,
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
        policy["policy"] = action.get(target, "reject")

        # priority = settings.get('priority', 32767)
        # if isinstance(priority, int):
        #     policy["priority"] = priority

        description = settings.get('description', '')
        if description:
            policy["description"] = description

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
            "logging": fw.getLogDenied()
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
