#!/usr/bin/python3
#
# This script is used to query dnsmasq daemons via dbus and
# fills/cleans the infix-dhcp-server packet statistics.
#

import argparse
import json
import dbus
import re


DBUS_NAME = "org.freedesktop.DBus"
DBUS_OBJECT = "/org/freedesktop/DBus"
DBUS_IFACE = "org.freedesktop.DBus"

DNSMASQ_NAME = "uk.org.thekelleys.dnsmasq"
DNSMASQ_IFACE = "uk.org.thekelleys.dnsmasq"
DNSMASQ_OBJECT = "/uk/org/thekelleys/dnsmasq"


def get_servers(bus):
    try:
        remote_object = bus.get_object(DBUS_NAME, DBUS_OBJECT)
        iface = dbus.Interface(remote_object, DBUS_IFACE)
    except dbus.DBusException:
        return []

    servers = []
    for name in iface.ListNames():
        r = re.search(r"%s.(\w+)$" % DNSMASQ_NAME, name)
        if r:
            server = {
                "ifc": r.group(1),
                "name": str(name)
            }
            servers.append(server)

    return servers


def get_iface(bus, name):
    try:
        remote_object = bus.get_object(name, DNSMASQ_OBJECT)
        iface = dbus.Interface(remote_object, DNSMASQ_IFACE)
    except dbus.DBusException:
        return None
    finally:
        return iface


def main():
    bus = dbus.SystemBus()
    servers = get_servers(bus)

    parser = argparse.ArgumentParser(prog='dhcp-server-status')
    parser.add_argument("-c", "--clean", help="DHCP server interface")
    args = parser.parse_args()

    if args.clean:
        for server in servers:
            if server["ifc"] != args.clean:
                continue
            iface = get_iface(bus, server["name"])
            if not iface:
                continue
            print("Cleaning metrics for DHCP server on %s" % server["ifc"])
            iface.ClearMetrics()
    else:
        data = []
        for server in servers:
            iface = get_iface(bus, server["name"])
            if not iface:
                continue
            data.append({
                "if-name": server["ifc"],
                "metrics": iface.GetMetrics()
            })
        print(json.dumps(data))


if __name__ == "__main__":
    main()
