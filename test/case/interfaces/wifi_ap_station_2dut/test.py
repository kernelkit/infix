#!/usr/bin/env python3
r"""
WiFi Access Point and Station association across two DUTs

Two DUTs, each with its own radio: the ap runs a WPA2/WPA3-personal Access
Point, the station associates to it.  The radios are in
separate kernels, so the "air" between them is realised differently per
environment:

  * On real hardware the link is real RF -- the radios just associate.
  * On QEMU the radios are mac80211_hwsim, and the `wifimedium` relay on
    each DUT bridges hwsim frames over the Ethernet segment between the
    guests (the topology's wifi-link).  See doc/wifi.md.

The test asserts that the station associates with WPA2/WPA3-personal.  That
is a strong end-to-end check: authentication, association and the WPA
4-way handshake all require frames to cross the medium in *both*
directions, so a successful association proves real bidirectional
communication over the (relayed) radio link -- not just that an interface
came up.  It then confirms data-plane reach: the ap serves DHCP and the
station leases its address over the radio link.

Topology:
....
    host ==(mgmt)== ap  )))  ~ wifi-link ~  ((( station ==(mgmt)== host
....
"""
import infamy
import infamy.iface as iface
import infamy.wifi as wifi
from infamy.util import until, parallel

SSID = "infix-test"
PSK = "infixinfix"          # 8-63 printable ASCII, see doc/wifi.md

# The ap runs a DHCP server on wifi0; the station leases its address.  A
# completed lease is real bidirectional IP traffic over the radio link.
SUBNET = "192.168.20.0/24"
AP_IP = "192.168.20.1"
LEASE = "192.168.20.100"        # single-address pool -> deterministic lease


with infamy.Test() as test:
    with test.step("Set up topology and attach to the ap and the station"):
        env = infamy.Env()
        # Connect to both nodes concurrently to cut setup time.
        ap, station = parallel(
            lambda: env.attach("ap", "mgmt"),
            lambda: env.attach("station", "mgmt"),
        )
        wifi.skip_unless_supported(test, ap, station)

    with test.step("Configure the ap as an Access Point and the station on radio0"):
        parallel(
            lambda: ap.put_config_dicts({
                "ietf-hardware": {"hardware": {"component": [
                    wifi.radio("radio0", band="2.4GHz", channel=1)]}},
                "ietf-keystore": wifi.keystore({"wifi": PSK}),
                "ietf-interfaces": {"interfaces": {"interface": [
                    # hwsim defaults every radio0 to 02:00:00:00:00:00, so the AP
                    # and the station would otherwise share a MAC -- give each a
                    # unique address.
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
                    }, ipv4={"infix-dhcp-client:dhcp": {}}),
                ]}},
            }),
        )

    with test.step("Verify the station associates to the ap over the wifi link"):
        until(lambda: wifi.associated(station, SSID), attempts=60, interval=2)

    with test.step("Verify the station's wifi0 operational status is up"):
        until(lambda: iface.is_oper_up(station, "wifi0"), attempts=30)

    with test.step("Verify the station leases its address from the ap over wifi"):
        until(lambda: iface.address_exist(station, "wifi0", LEASE),
              attempts=60, interval=2)

    test.succeed()
