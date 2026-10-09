#!/usr/bin/env python3
r"""
WiFi station set up from a scan-only interface

A WiFi interface with only a radio, no station or access point, is in
scan-only mode.  That is how the factory configuration of boards with a
built-in radio ships, so the usual way to get online is to add the station
settings to that existing interface.  The connection has to come up from
that change alone, without a reboot or a service restart.

Topology:
....
    host ==(mgmt)== ap  )))  ~ cell ~  ((( station ==(mgmt)== host
....
"""
import infamy
import infamy.iface as iface
import infamy.wifi as wifi
from infamy.util import until, parallel

SSID = "infix-scan"
PSK = "infixinfix"

SUBNET = "192.168.21.0/24"
AP_IP = "192.168.21.1"
LEASE = "192.168.21.100"


with infamy.Test() as test:
    with test.step("Set up topology and attach to the ap and the station"):
        env = infamy.Env()
        ap, station = parallel(
            lambda: env.attach("ap", "mgmt"),
            lambda: env.attach("station", "mgmt"),
        )
        wifi.skip_unless_supported(test, ap, station)

    with test.step("Configure the ap with access point 'infix-scan' and a DHCP server on 192.168.21.1"):
        ap.put_config_dicts({
            "ietf-hardware": wifi.hardware(wifi.radio("radio0", band="2.4GHz", channel=1)),
            "ietf-keystore": wifi.keystore({"wifi": PSK}),
            "ietf-interfaces": {"interfaces": {"interface": [
                wifi.iface("wifi0", "02:00:00:00:00:01", {
                    "radio": "radio0",
                    "access-point": {
                        "ssid": SSID,
                        "security": {"mode": "wpa2-wpa3-personal", "secret": "wifi"},
                    },
                }, ipv4={"address": [{"ip": AP_IP, "prefix-length": 24}]}),
            ]}},
            "infix-dhcp-server": {"dhcp-server": {"subnet": [{
                "subnet": SUBNET,
                "pool": {"start-address": LEASE, "end-address": LEASE},
            }]}},
        })

    with test.step("Configure wifi0 on the station with only radio0, scan-only mode"):
        station.put_config_dicts({
            "ietf-hardware": wifi.hardware(wifi.radio("radio0")),
            "ietf-interfaces": {"interfaces": {"interface": [
                wifi.iface("wifi0", "02:00:00:00:00:02", {"radio": "radio0"},
                           ipv4={"infix-dhcp-client:dhcp": {}}),
            ]}},
        })

    with test.step("Verify the station sees 'infix-scan' in its scan results"):
        until(lambda: SSID in {n.get("ssid") for n in
                               wifi.station(station).get("scan-results") or []},
              attempts=60, interval=2)

    with test.step("Add station settings for 'infix-scan' to wifi0 on the station"):
        station.put_config_dicts({
            "ietf-keystore": wifi.keystore({"wifi": PSK}),
            "ietf-interfaces": {"interfaces": {"interface": [
                {"name": "wifi0", "infix-interfaces:wifi": {
                    "radio": "radio0",
                    "station": {
                        "ssid": SSID,
                        "security": {"mode": "auto", "secret": "wifi"},
                    },
                }},
            ]}},
        })

    with test.step("Verify the station associates to 'infix-scan' without a restart"):
        until(lambda: wifi.associated(station, SSID), attempts=60, interval=2)

    with test.step("Verify the station leases 192.168.21.100 over wifi"):
        until(lambda: iface.address_exist(station, "wifi0", LEASE),
              attempts=60, interval=2)

    test.succeed()
