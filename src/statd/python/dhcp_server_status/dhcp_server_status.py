#!/usr/bin/python3
"""
This script is used to query a single dnsmasq daemon via dbus
and fills/clears the infix-dhcp-server packet statistics.
"""

import argparse
import json
import dbus

DNSMASQ_NAME = "uk.org.thekelleys.dnsmasq"
DNSMASQ_IFACE = "uk.org.thekelleys.dnsmasq"
DNSMASQ_OBJECT = "/uk/org/thekelleys/dnsmasq"


def main():
    parser = argparse.ArgumentParser(prog='dhcp-server-status')
    parser.add_argument("-c", "--clear", action="store_true",
                        help="Clear DHCP server metrics")
    args = parser.parse_args()

    try:
        bus = dbus.SystemBus()
        dnsmasq = dbus.Interface(bus.get_object(DNSMASQ_NAME, DNSMASQ_OBJECT),
                                 DNSMASQ_IFACE)

        if args.clear:
            print("Clearing metrics for DHCP server")
            dnsmasq.ClearMetrics()
        else:
            print(json.dumps({"metrics": dnsmasq.GetMetrics()}))

    except dbus.DBusException as e:
        print(f"Error: Unable to connect to dnsmasq via D-Bus: {e}")


if __name__ == "__main__":
    main()
