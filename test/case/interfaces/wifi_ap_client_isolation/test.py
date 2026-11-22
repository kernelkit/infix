#!/usr/bin/env python3
r"""
WiFi Access Point with client isolation

One Access Point and two Stations, each on its own DUT.  Without client
isolation the stations reach each other through the Access Point.  With
client isolation enabled they still reach the Access Point, but no
longer each other.

Topology:
....
                       ((( station1 ==(mgmt)== host
    host ==(mgmt)== ap
                       ((( station2 ==(mgmt)== host
....
"""
import infamy
import infamy.wifi as wifi
from infamy.util import until, parallel

SSID = "infix-test"
PSK = "infixinfix"

AP_IP = "192.168.20.1"
STATION1_IP = "192.168.20.11"
STATION2_IP = "192.168.20.12"


def ap_config(isolation):
    return {
        "ietf-hardware": wifi.hardware(wifi.radio("radio0", band="2.4GHz", channel=1)),
        "ietf-keystore": wifi.keystore({"wifi": PSK}),
        "ietf-interfaces": {"interfaces": {"interface": [
            wifi.iface("wifi0", {
                "radio": "radio0",
                "access-point": {
                    "ssid": SSID,
                    "client-isolation": isolation,
                    "security": {"mode": "wpa2-wpa3-personal", "secret": "wifi"},
                },
            }, ipv4={"address": [{"ip": AP_IP, "prefix-length": 24}]}),
        ]}},
    }


def station_config(addr):
    return {
        "ietf-hardware": wifi.hardware(wifi.radio("radio0")),
        "ietf-keystore": wifi.keystore({"wifi": PSK}),
        "ietf-interfaces": {"interfaces": {"interface": [
            wifi.iface("wifi0", {
                "radio": "radio0",
                "station": {
                    "ssid": SSID,
                    "security": {"mode": "auto", "secret": "wifi"},
                },
            }, ipv4={"address": [{"ip": addr, "prefix-length": 24}]}),
        ]}},
    }


def reaches(sh, addr):
    return sh.runsh(f"ping -c 1 -W 2 {addr}").returncode == 0


with infamy.Test() as test:
    with test.step("Set up topology and attach to the ap and two stations"):
        env = infamy.Env()
        ap, station1, station2, station1sh, station2sh = parallel(
            lambda: env.attach("ap", "mgmt"),
            lambda: env.attach("station1", "mgmt"),
            lambda: env.attach("station2", "mgmt"),
            lambda: env.attach("station1", "mgmt", "ssh"),
            lambda: env.attach("station2", "mgmt", "ssh"),
        )
        wifi.skip_unless_supported(test, ap, station1, station2)

    with test.step("Configure the ap as an Access Point without client isolation"):
        ap.put_config_dicts(ap_config(False))

    with test.step("Configure the stations with 192.168.20.11/24 and 192.168.20.12/24"):
        parallel(
            lambda: station1.put_config_dicts(station_config(STATION1_IP)),
            lambda: station2.put_config_dicts(station_config(STATION2_IP)),
        )

    with test.step("Verify both stations associate to the ap"):
        for name, dut in (("station1", station1), ("station2", station2)):
            wifi.until_diagnosed(env, lambda dut=dut: wifi.associated(dut, SSID),
                                 interval=2, ap="ap", clients=(name,))

    with test.step("Verify station1 reaches the ap on 192.168.20.1"):
        until(lambda: reaches(station1sh, AP_IP), attempts=30)

    with test.step("Verify station1 reaches station2 on 192.168.20.12"):
        until(lambda: reaches(station1sh, STATION2_IP), attempts=30)

    with test.step("Enable client isolation on the ap"):
        ap.put_config_dicts(ap_config(True))

    with test.step("Verify both stations are associated to the ap"):
        for name, dut in (("station1", station1), ("station2", station2)):
            wifi.until_diagnosed(env, lambda dut=dut: wifi.associated(dut, SSID),
                                 interval=2, ap="ap", clients=(name,))

    with test.step("Verify station1 still reaches the ap on 192.168.20.1"):
        until(lambda: reaches(station1sh, AP_IP), attempts=30)

    with test.step("Verify station2 still reaches the ap on 192.168.20.1"):
        until(lambda: reaches(station2sh, AP_IP), attempts=30)

    with test.step("Verify station1 no longer reaches station2 on 192.168.20.12"):
        assert not reaches(station1sh, STATION2_IP), \
            "station1 reached station2 through an isolating access point"

    test.succeed()
