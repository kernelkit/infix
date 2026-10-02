#!/usr/bin/env python3
r"""
WiFi repeater over a 4-address (WDS) backhaul

Three DUTs.  The root runs the backhaul access point 'infix-backhaul' with
a WDS port bridged to its wired uplink.  The repeater has a 4-address
station on that backhaul, bridged with its own access point 'infix-client'
on the same radio.  The client is a plain station on 'infix-client'.

Only the repeater advertises 'infix-client', so the client can land
nowhere else; the test still checks the BSSID it reports.  Reaching the
client from the host behind the root is what only 4-address frames can
do: the client's own MAC address has to cross the backhaul in both
directions inside the station's frames.

Taking the backhaul down and up again shows it is a transparent bridge
port: the client loses and regains reach without re-associating.

Topology:
....
    host ==(lan)== root (AP 'infix-backhaul')
                    wds0 )))  ~ cell ~  ((( wifi0 repeater wifi1 (AP 'infix-client')
                                                             )))  ~ cell ~  ((( client
....
"""
import infamy
import infamy.iface as iface
import infamy.wifi as wifi
from infamy.util import until, parallel

BACKHAUL_SSID = "infix-backhaul"
CLIENT_SSID = "infix-client"
PSK = "infixinfix"

ROOT_AP_MAC = "02:00:00:00:00:01"
REPEATER_STA_MAC = "02:00:00:00:00:02"
REPEATER_AP_MAC = "02:00:00:00:0a:02"
CLIENT_MAC = "02:00:00:00:00:09"

HOST_IP = "10.0.0.1"
CLIENT_IP = "10.0.0.9"

SECRETS = {"backhaul": PSK, "client": PSK}


def root_config(uplink):
    return {
        "ietf-hardware": {"hardware": {"component": [
            wifi.radio("radio0", band="2.4GHz", channel=1)]}},
        "ietf-keystore": wifi.keystore(SECRETS),
        "ietf-interfaces": {"interfaces": {"interface": [
            {"name": "br0", "type": "infix-if-type:bridge", "enabled": True},
            {"name": uplink, "enabled": True,
             "infix-interfaces:bridge-port": {"bridge": "br0"}},
            wifi.iface("wifi0", ROOT_AP_MAC, {
                "radio": "radio0",
                "access-point": {
                    "ssid": BACKHAUL_SSID,
                    "security": {"mode": "wpa2-wpa3-personal", "secret": "backhaul"},
                },
            }),
            wifi.wds_link("wds0", "radio0", "wifi0", REPEATER_STA_MAC, bridge="br0"),
        ]}},
    }


def repeater_config():
    return {
        "ietf-hardware": {"hardware": {"component": [
            wifi.radio("radio0", band="2.4GHz", channel=1)]}},
        "ietf-keystore": wifi.keystore(SECRETS),
        "ietf-interfaces": {"interfaces": {"interface": [
            {"name": "br0", "type": "infix-if-type:bridge", "enabled": True},
            wifi.iface("wifi0", REPEATER_STA_MAC, {
                "radio": "radio0",
                "station": {
                    "ssid": BACKHAUL_SSID,
                    "wds": True,
                    "peer-bssid": ROOT_AP_MAC,
                    "security": {"mode": "auto", "secret": "backhaul"},
                },
            }, bridge="br0"),
            wifi.iface("wifi1", REPEATER_AP_MAC, {
                "radio": "radio0",
                "access-point": {
                    "ssid": CLIENT_SSID,
                    "security": {"mode": "wpa2-wpa3-personal", "secret": "client"},
                },
            }, bridge="br0"),
        ]}},
    }


def client_config():
    return {
        "ietf-hardware": {"hardware": {"component": [wifi.radio("radio0")]}},
        "ietf-keystore": wifi.keystore(SECRETS),
        "ietf-interfaces": {"interfaces": {"interface": [
            wifi.iface("wifi0", CLIENT_MAC, {
                "radio": "radio0",
                "station": {
                    "ssid": CLIENT_SSID,
                    "security": {"mode": "auto", "secret": "client"},
                },
            }, ipv4={"address": [{"ip": CLIENT_IP, "prefix-length": 24}]}),
        ]}},
    }


def backhaul_enabled(enabled):
    return {"ietf-interfaces": {"interfaces": {"interface": [
        {"name": "wifi0", "enabled": enabled}]}}}


with infamy.Test() as test:
    with test.step("Set up topology and attach to the root, the repeater and the client"):
        env = infamy.Env()
        root, repeater, client = parallel(
            lambda: env.attach("root", "mgmt"),
            lambda: env.attach("repeater", "mgmt"),
            lambda: env.attach("client", "mgmt"),
        )
        wifi.skip_unless_supported(test, root, repeater, client)

    with test.step("Configure the root with access point 'infix-backhaul' and WDS port wds0 bridged with the uplink"):
        _, uplink = env.ltop.xlate("root", "uplink")
        root.put_config_dicts(root_config(uplink))

    with test.step("Configure the repeater with a 4-address station for 'infix-backhaul' bridged with access point 'infix-client'"):
        repeater.put_config_dicts(repeater_config())

    with test.step("Configure the client as a station for 'infix-client' with address 10.0.0.9"):
        client.put_config_dicts(client_config())

    with test.step("Verify the repeater's wifi0 associates to 'infix-backhaul'"):
        until(lambda: wifi.associated(repeater, BACKHAUL_SSID), attempts=60, interval=2)

    with test.step("Verify wds0 on the root is up"):
        until(lambda: iface.is_oper_up(root, "wds0"), attempts=30, interval=2)

    with test.step("Verify the client associates to 'infix-client'"):
        until(lambda: wifi.associated(client, CLIENT_SSID), attempts=60, interval=2)

    with test.step("Verify the client is on the repeater's access point, BSSID 02:00:00:00:0a:02"):
        until(lambda: wifi.station_bssid(client) == REPEATER_AP_MAC, attempts=30, interval=2)

    _, hlan = env.ltop.xlate("host", "lan")
    with infamy.IsolatedMacVlan(hlan) as ns:
        ns.addip(HOST_IP)

        with test.step("Verify the host behind the root reaches the client at 10.0.0.9 through the repeater"):
            ns.must_reach(CLIENT_IP)

        with test.step("Disable the repeater's backhaul station wifi0"):
            repeater.put_config_dicts(backhaul_enabled(False))

        with test.step("Verify the client at 10.0.0.9 is no longer reachable from the host"):
            until(lambda: not iface.is_oper_up(root, "wds0"), attempts=30, interval=2)
            ns.must_not_reach(CLIENT_IP)

        with test.step("Enable the repeater's backhaul station wifi0 again"):
            repeater.put_config_dicts(backhaul_enabled(True))

        with test.step("Verify the host reaches the client at 10.0.0.9 again"):
            until(lambda: iface.is_oper_up(root, "wds0"), attempts=60, interval=2)
            ns.must_reach(CLIENT_IP)

    with test.step("Verify the client is still on BSSID 02:00:00:00:0a:02"):
        if wifi.station_bssid(client) != REPEATER_AP_MAC:
            test.fail()

    test.succeed()
