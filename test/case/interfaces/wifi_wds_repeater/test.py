#!/usr/bin/env python3
r"""
WiFi repeater with two SSIDs in VLANs over a 4-address (WDS) backhaul

Four DUTs.  The root runs the backhaul access point 'infix-backhaul' and
a VLAN filtering bridge: the WDS port and the wired uplink carry VLAN 10
and VLAN 20 tagged, and the root serves DHCP in each VLAN.  The repeater
has a 4-address station on the backhaul, carrying both VLANs tagged, and
two access points on the same radio as access ports: 'infix-home'
untagged in VLAN 10 and 'infix-guest' untagged in VLAN 20.  Two plain
stations join them, one per SSID.

Each station must lease an address from the DHCP server of its own VLAN
on the root.  That proves both that the 4-address backhaul carries the
stations' own MAC addresses and that the VLAN tags survive the trip.
The host behind the root reaches both stations on their VLANs.

Taking the backhaul down and up again shows it is a transparent bridge
port: the stations lose and regain reach without re-associating.

Topology:
....
    host ==(lan, VLAN 10+20)== root (AP 'infix-backhaul')
                                wds0 )))  ~ cell ~  ((( wifi0 repeater wifi1 (AP 'infix-home', VLAN 10)  ))) home
                                                                      wifi2 (AP 'infix-guest', VLAN 20) ))) guest
....
"""
import infamy
import infamy.iface as iface
import infamy.wifi as wifi
from infamy.util import until, parallel

BACKHAUL_SSID = "infix-backhaul"
HOME_SSID = "infix-home"
GUEST_SSID = "infix-guest"
PSK = "infixinfix"

ROOT_AP_MAC = "02:00:00:00:00:01"
REPEATER_STA_MAC = "02:00:00:00:00:02"
HOME_AP_MAC = "02:00:00:00:0a:02"
GUEST_AP_MAC = "02:00:00:00:0b:02"
HOME_MAC = "02:00:00:00:00:09"
GUEST_MAC = "02:00:00:00:00:0a"

HOME_HOST_IP = "10.10.0.1"
HOME_ROOT_IP = "10.10.0.2"
HOME_IP = "10.10.0.9"
GUEST_HOST_IP = "10.20.0.1"
GUEST_ROOT_IP = "10.20.0.2"
GUEST_IP = "10.20.0.9"

SECRETS = {"backhaul": PSK, "home": PSK, "guest": PSK}


def root_config(uplink):
    return {
        "ietf-hardware": {"hardware": {"component": [
            wifi.radio("radio0", band="2.4GHz", channel=1)]}},
        "ietf-keystore": wifi.keystore(SECRETS),
        "ietf-interfaces": {"interfaces": {"interface": [
            {"name": "br0", "type": "infix-if-type:bridge", "enabled": True,
             "bridge": {"vlans": {"vlan": [
                 {"vid": 10, "tagged": [uplink, "wds0", "br0"]},
                 {"vid": 20, "tagged": [uplink, "wds0", "br0"]},
             ]}}},
            {"name": "vlan10", "type": "infix-if-type:vlan", "enabled": True,
             "vlan": {"id": 10, "lower-layer-if": "br0"},
             "ipv4": {"address": [{"ip": HOME_ROOT_IP, "prefix-length": 24}]}},
            {"name": "vlan20", "type": "infix-if-type:vlan", "enabled": True,
             "vlan": {"id": 20, "lower-layer-if": "br0"},
             "ipv4": {"address": [{"ip": GUEST_ROOT_IP, "prefix-length": 24}]}},
            {"name": uplink, "enabled": True,
             "infix-interfaces:bridge-port": {"bridge": "br0"}},
            wifi.iface("wifi0", ROOT_AP_MAC, {
                "radio": "radio0",
                "access-point": {
                    "ssid": BACKHAUL_SSID,
                    "security": {"mode": "wpa2-wpa3-personal", "secret": "backhaul"},
                },
            }),
            wifi.wds_link("wds0", "wifi0", REPEATER_STA_MAC, bridge="br0"),
        ]}},
        "infix-dhcp-server": {"dhcp-server": {"subnet": [
            {"subnet": "10.10.0.0/24",
             "pool": {"start-address": "10.10.0.100", "end-address": "10.10.0.100"},
             "host": [{"address": HOME_IP, "match": {"mac-address": HOME_MAC}}]},
            {"subnet": "10.20.0.0/24",
             "pool": {"start-address": "10.20.0.100", "end-address": "10.20.0.100"},
             "host": [{"address": GUEST_IP, "match": {"mac-address": GUEST_MAC}}]},
        ]}},
    }


def repeater_config():
    return {
        "ietf-hardware": {"hardware": {"component": [
            wifi.radio("radio0", band="2.4GHz", channel=1)]}},
        "ietf-keystore": wifi.keystore(SECRETS),
        "ietf-interfaces": {"interfaces": {"interface": [
            {"name": "br0", "type": "infix-if-type:bridge", "enabled": True,
             "bridge": {"vlans": {"vlan": [
                 {"vid": 10, "untagged": ["wifi1"], "tagged": ["wifi0"]},
                 {"vid": 20, "untagged": ["wifi2"], "tagged": ["wifi0"]},
             ]}}},
            wifi.iface("wifi0", REPEATER_STA_MAC, {
                "radio": "radio0",
                "station": {
                    "ssid": BACKHAUL_SSID,
                    "wds": True,
                    "peer-bssid": ROOT_AP_MAC,
                    "security": {"mode": "auto", "secret": "backhaul"},
                },
            }, bridge="br0"),
            wifi.iface("wifi1", HOME_AP_MAC, {
                "radio": "radio0",
                "access-point": {
                    "ssid": HOME_SSID,
                    "security": {"mode": "wpa2-wpa3-personal", "secret": "home"},
                },
            }, bridge="br0", pvid=10),
            wifi.iface("wifi2", GUEST_AP_MAC, {
                "radio": "radio0",
                "access-point": {
                    "ssid": GUEST_SSID,
                    "security": {"mode": "wpa2-wpa3-personal", "secret": "guest"},
                },
            }, bridge="br0", pvid=20),
        ]}},
    }


def station_config(mac, ssid, secret):
    return {
        "ietf-hardware": {"hardware": {"component": [wifi.radio("radio0")]}},
        "ietf-keystore": wifi.keystore(SECRETS),
        "ietf-interfaces": {"interfaces": {"interface": [
            wifi.iface("wifi0", mac, {
                "radio": "radio0",
                "station": {
                    "ssid": ssid,
                    "security": {"mode": "auto", "secret": secret},
                },
            }, ipv4={"infix-dhcp-client:dhcp": {}}),
        ]}},
    }


def backhaul_enabled(enabled):
    return {"ietf-interfaces": {"interfaces": {"interface": [
        {"name": "wifi0", "enabled": enabled}]}}}


def reaches(ns, addr):
    try:
        ns.ping(addr)
        return True
    except Exception:
        return False


with infamy.Test() as test:
    with test.step("Set up topology and attach to the root, the repeater, home and guest"):
        env = infamy.Env()
        root, repeater, home, guest = parallel(
            lambda: env.attach("root", "mgmt"),
            lambda: env.attach("repeater", "mgmt"),
            lambda: env.attach("home", "mgmt"),
            lambda: env.attach("guest", "mgmt"),
        )
        wifi.skip_unless_supported(test, root, repeater, home, guest)

    with test.step("Configure the root with access point 'infix-backhaul', WDS port wds0 and the uplink tagged in VLAN 10 and 20, DHCP in each VLAN"):
        _, uplink = env.ltop.xlate("root", "uplink")
        root.put_config_dicts(root_config(uplink))

    with test.step("Configure the repeater with a 4-address station tagged in VLAN 10 and 20, access point 'infix-home' untagged in VLAN 10 and 'infix-guest' untagged in VLAN 20"):
        repeater.put_config_dicts(repeater_config())

    with test.step("Configure home as a DHCP station for 'infix-home' and guest as a DHCP station for 'infix-guest'"):
        parallel(
            lambda: home.put_config_dicts(station_config(HOME_MAC, HOME_SSID, "home")),
            lambda: guest.put_config_dicts(station_config(GUEST_MAC, GUEST_SSID, "guest")),
        )

    with test.step("Verify the repeater's wifi0 associates to 'infix-backhaul'"):
        until(lambda: wifi.associated(repeater, BACKHAUL_SSID), attempts=60, interval=2)

    with test.step("Verify wds0 on the root is up"):
        until(lambda: iface.is_oper_up(root, "wds0"), attempts=30, interval=2)

    with test.step("Verify home is on the repeater's 'infix-home' access point, BSSID 02:00:00:00:0a:02"):
        until(lambda: wifi.station_bssid(home) == HOME_AP_MAC, attempts=60, interval=2)

    with test.step("Verify guest is on the repeater's 'infix-guest' access point, BSSID 02:00:00:00:0b:02"):
        until(lambda: wifi.station_bssid(guest) == GUEST_AP_MAC, attempts=60, interval=2)

    with test.step("Verify home leases 10.10.0.9 from the root's VLAN 10 DHCP server through the backhaul"):
        until(lambda: iface.address_exist(home, "wifi0", HOME_IP), attempts=60, interval=2)

    with test.step("Verify guest leases 10.20.0.9 from the root's VLAN 20 DHCP server through the backhaul"):
        until(lambda: iface.address_exist(guest, "wifi0", GUEST_IP), attempts=60, interval=2)

    _, hlan = env.ltop.xlate("host", "lan")
    with infamy.IsolatedMacVlan(hlan) as ns:
        ns.runsh(f"""
            set -ex
            ip link add dev vlan10 link iface up type vlan id 10
            ip link add dev vlan20 link iface up type vlan id 20
            ip addr add {HOME_HOST_IP}/24 dev vlan10
            ip addr add {GUEST_HOST_IP}/24 dev vlan20
            """)

        with test.step("Verify the host reaches home at 10.10.0.9 on VLAN 10 through the repeater"):
            until(lambda: reaches(ns, HOME_IP), attempts=30, interval=2)

        with test.step("Verify the host reaches guest at 10.20.0.9 on VLAN 20 through the repeater"):
            until(lambda: reaches(ns, GUEST_IP), attempts=30, interval=2)

        with test.step("Disable the repeater's backhaul station wifi0"):
            repeater.put_config_dicts(backhaul_enabled(False))

        with test.step("Verify home at 10.10.0.9 and guest at 10.20.0.9 are no longer reachable from the host"):
            until(lambda: not iface.is_oper_up(root, "wds0"), attempts=30, interval=2)
            ns.must_not_reach(HOME_IP)
            ns.must_not_reach(GUEST_IP)

        with test.step("Enable the repeater's backhaul station wifi0 again"):
            repeater.put_config_dicts(backhaul_enabled(True))

        with test.step("Verify the host reaches home at 10.10.0.9 and guest at 10.20.0.9 again"):
            until(lambda: iface.is_oper_up(root, "wds0"), attempts=60, interval=2)
            until(lambda: reaches(ns, HOME_IP), attempts=30, interval=2)
            until(lambda: reaches(ns, GUEST_IP), attempts=30, interval=2)

    with test.step("Verify home and guest are still on their access points"):
        if wifi.station_bssid(home) != HOME_AP_MAC or wifi.station_bssid(guest) != GUEST_AP_MAC:
            test.fail()

    test.succeed()
