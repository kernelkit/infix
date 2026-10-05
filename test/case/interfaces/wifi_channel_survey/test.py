#!/usr/bin/env python3
r"""
WiFi channel survey on a connected radio

A channel survey tells how busy each channel is.  Collecting it means
leaving the operating channel, so it is not done in the background but
on request, with the channel-survey action on the radio.  The action has
to work on a radio that is in use: here on the ap, which is serving a
station, and on the station, which is associated to the ap.

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
                "ietf-hardware": {"hardware": {"component": [
                    wifi.radio("radio0", band="2.4GHz", channel=1)]}},
                "ietf-keystore": wifi.keystore({"wifi": PSK}),
                "ietf-interfaces": {"interfaces": {"interface": [
                    wifi.iface("wifi0", "02:00:00:00:00:01", {
                        "radio": "radio0",
                        "access-point": {
                            "ssid": SSID,
                            "security": {"mode": "wpa2-wpa3-personal", "secret": "wifi"},
                        },
                    }),
                ]}},
            }),
            lambda: station.put_config_dicts({
                "ietf-hardware": {"hardware": {"component": [wifi.radio("radio0")]}},
                "ietf-keystore": wifi.keystore({"wifi": PSK}),
                "ietf-interfaces": {"interfaces": {"interface": [
                    wifi.iface("wifi0", "02:00:00:00:00:02", {
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

    with test.step("Verify the station is still associated to the ap after the surveys"):
        until(lambda: wifi.associated(station, SSID), attempts=30, interval=2)

    test.succeed()
