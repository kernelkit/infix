#!/usr/bin/env python3
r"""
WiFi scan and channel survey on a connected radio

A scan lists the networks in range, a channel survey tells how busy each
channel is.  Both mean leaving the operating channel, so neither is done
in the background but on request, with the scan and channel-survey
actions on the radio.  The actions have to work on a radio that is in
use: here on the ap, which is serving a station, and on the station,
which is associated to the ap.

Topology:
....
    host ==(mgmt)== ap  )))  ~ cell ~  ((( station ==(mgmt)== host
....
"""
import infamy
import infamy.wifi as wifi
from infamy.util import until, parallel

SSID = "infix-survey"
PSK = "infixinfix"
FREQ = 2412                     # channel 1


with infamy.Test() as test:
    with test.step("Set up topology and attach to the ap and the station"):
        env = infamy.Env()
        ap, station = parallel(
            lambda: env.attach("ap", "mgmt"),
            lambda: env.attach("station", "mgmt"),
        )
        wifi.skip_unless_supported(test, ap, station)

    with test.step("Configure the ap as an Access Point on channel 1 and the station on radio0"):
        parallel(
            lambda: ap.put_config_dicts({
                "ietf-hardware": wifi.hardware(wifi.radio("radio0", band="2.4GHz", channel=1)),
                "ietf-keystore": wifi.keystore({"wifi": PSK}),
                "ietf-interfaces": {"interfaces": {"interface": [
                    wifi.iface("wifi0", {
                        "radio": "radio0",
                        "access-point": {
                            "ssid": SSID,
                            "security": {"mode": "wpa2-wpa3-personal", "secret": "wifi"},
                        },
                    }),
                ]}},
            }),
            lambda: station.put_config_dicts({
                "ietf-hardware": wifi.hardware(wifi.radio("radio0")),
                "ietf-keystore": wifi.keystore({"wifi": PSK}),
                "ietf-interfaces": {"interfaces": {"interface": [
                    wifi.iface("wifi0", {
                        "radio": "radio0",
                        "station": {
                            "ssid": SSID,
                            "security": {"mode": "auto", "secret": "wifi"},
                        },
                    }),
                ]}},
            }),
        )

    with test.step("Verify the station associates to the ap over the wifi link"):
        until(lambda: wifi.associated(station, SSID), attempts=60, interval=2)

    with test.step("Scan for networks on radio0 of the station"):
        networks = wifi.scan(station, "radio0")

    with test.step("Verify the station's scan lists the ap's SSID on 2412 MHz"):
        ap_mac = wifi.phys_address(ap, "wifi0")
        found = [n for n in networks if n.get("bssid", "").lower() == ap_mac]
        assert found, networks
        assert found[0].get("ssid") == SSID, found
        assert int(found[0].get("frequency", 0)) == FREQ, found

    with test.step("Run a channel survey on radio0 of the station"):
        channels = wifi.channel_survey(station, "radio0")

    with test.step("Verify the station's survey reports 2412 MHz as the channel in use"):
        assert wifi.in_use_frequency(channels) == FREQ, channels

    with test.step("Verify the station's survey covers more channels than the one in use"):
        assert len(channels) > 1, channels

    with test.step("Run a channel survey on radio0 of the ap"):
        channels = wifi.channel_survey(ap, "radio0")

    with test.step("Verify the ap's survey reports 2412 MHz as the channel in use"):
        assert wifi.in_use_frequency(channels) == FREQ, channels

    with test.step("Verify the station is still associated to the ap after the scans"):
        until(lambda: wifi.associated(station, SSID), attempts=30, interval=2)

    test.succeed()
