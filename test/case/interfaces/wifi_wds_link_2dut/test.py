#!/usr/bin/env python3
r"""
WiFi 4-address (WDS) link between two bridges

Two DUTs: the root runs an access point with a WDS port, the satellite a
4-address station.  Both ends are bridge ports, so the radio link joins
the two bridges at layer 2.

On the root, wds0 is created by configuration before any station shows up
and gets its VLAN membership like any other bridge port.  The access point
binds the station with the configured MAC address to it: the port comes up
when the station associates and goes down, but stays, when it leaves.  On
the satellite, the station is a bridge port, which needs 4-address mode.

The DHCP lease over the link is the data-plane check: the request and the
reply cross both bridges and the radio in opposite directions.

Topology:
....
    host ==(mgmt)== root  )))  ~ cell ~  ((( satellite ==(mgmt)== host
    br0/vlan10 -- wds0 ~~~~~~~~~~~~~~~~~~~~~~ wifi0 -- br0
....
"""
import infamy
import infamy.iface as iface
import infamy.wifi as wifi
from infamy.util import until, parallel

SSID = "infix-wds"
PSK = "infixinfix"

ROOT_AP_MAC = "02:00:00:00:00:01"
SAT_MAC = "02:00:00:00:00:02"

SUBNET = "192.168.20.0/24"
ROOT_IP = "192.168.20.1"
LEASE = "192.168.20.100"


def root_config():
    return {
        "ietf-hardware": {"hardware": {"component": [
            wifi.radio("radio0", band="2.4GHz", channel=1)]}},
        "ietf-keystore": wifi.keystore({"wifi": PSK}),
        "ietf-interfaces": {"interfaces": {"interface": [
            {
                "name": "br0",
                "type": "infix-if-type:bridge",
                "enabled": True,
                "bridge": {"vlans": {"vlan": [
                    {"vid": 10, "untagged": ["wds0"], "tagged": ["br0"]},
                ]}},
            },
            {
                "name": "vlan10",
                "type": "infix-if-type:vlan",
                "enabled": True,
                "vlan": {"id": 10, "lower-layer-if": "br0"},
                "ipv4": {"address": [{"ip": ROOT_IP, "prefix-length": 24}]},
            },
            wifi.iface("wifi0", ROOT_AP_MAC, {
                "radio": "radio0",
                "access-point": {
                    "ssid": SSID,
                    "security": {"mode": "wpa2-wpa3-personal", "secret": "wifi"},
                },
            }),
            wifi.wds_link("wds0", "radio0", "wifi0", SAT_MAC, bridge="br0", pvid=10),
        ]}},
        "infix-dhcp-server": {"dhcp-server": {"subnet": [{
            "subnet": SUBNET,
            "pool": {"start-address": LEASE, "end-address": LEASE},
        }]}},
    }


def satellite_config():
    return {
        "ietf-hardware": {"hardware": {"component": [wifi.radio("radio0")]}},
        "ietf-keystore": wifi.keystore({"wifi": PSK}),
        "ietf-interfaces": {"interfaces": {"interface": [
            {
                "name": "br0",
                "type": "infix-if-type:bridge",
                "enabled": True,
                "ietf-ip:ipv4": {"infix-dhcp-client:dhcp": {}},
            },
            wifi.iface("wifi0", SAT_MAC, {
                "radio": "radio0",
                "station": {
                    "ssid": SSID,
                    "wds": True,
                    "peer-bssid": ROOT_AP_MAC,
                    "security": {"mode": "auto", "secret": "wifi"},
                },
            }, bridge="br0"),
        ]}},
    }


def station_enabled(enabled):
    return {"ietf-interfaces": {"interfaces": {"interface": [
        {"name": "wifi0", "enabled": enabled}]}}}


with infamy.Test() as test:
    with test.step("Set up topology and attach to the root and the satellite"):
        env = infamy.Env()
        root, satellite = parallel(
            lambda: env.attach("root", "mgmt"),
            lambda: env.attach("satellite", "mgmt"),
        )
        wifi.skip_unless_supported(test, root, satellite)

    with test.step("Configure the root with access point 'infix-wds' and WDS port wds0 untagged in VLAN 10 on br0"):
        root.put_config_dicts(root_config())

    with test.step("Verify wds0 on the root exists, is down and is an untagged member of VLAN 10"):
        until(lambda: iface.exist(root, "wds0"), attempts=30)
        until(lambda: "wds0" in wifi.bridge_vlan_members(root, "br0", 10), attempts=30)
        if iface.is_oper_up(root, "wds0"):
            test.fail()

    with test.step("Configure the satellite with a 4-address station for 'infix-wds' in br0, br0 as DHCP client"):
        satellite.put_config_dicts(satellite_config())

    with test.step("Verify the satellite's wifi0 associates to 'infix-wds'"):
        until(lambda: wifi.associated(satellite, SSID), attempts=60, interval=2)

    with test.step("Verify wds0 on the root comes up and reports the station connected"):
        until(lambda: iface.is_oper_up(root, "wds0"), attempts=30, interval=2)
        until(lambda: wifi.wds_connected(root, "wds0"), attempts=30, interval=2)

    with test.step("Verify the satellite leases 192.168.20.100 on br0 over the WDS link"):
        until(lambda: iface.address_exist(satellite, "br0", LEASE),
              attempts=60, interval=2)

    with test.step("Disable wifi0 on the satellite"):
        satellite.put_config_dicts(station_enabled(False))

    with test.step("Verify wds0 on the root goes down but remains a member of VLAN 10"):
        until(lambda: not iface.is_oper_up(root, "wds0"), attempts=90, interval=2)
        until(lambda: not wifi.wds_connected(root, "wds0"), attempts=30, interval=2)
        if "wds0" not in wifi.bridge_vlan_members(root, "br0", 10):
            test.fail()

    with test.step("Enable wifi0 on the satellite again"):
        satellite.put_config_dicts(station_enabled(True))

    with test.step("Verify wds0 on the root comes up again"):
        until(lambda: iface.is_oper_up(root, "wds0"), attempts=60, interval=2)
        until(lambda: wifi.wds_connected(root, "wds0"), attempts=30, interval=2)

    test.succeed()
