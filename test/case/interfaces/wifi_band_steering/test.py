#!/usr/bin/env python3
r"""
WiFi Band Steering: a dual-band AP nudges a client onto 5GHz

One DUT is a dual-band Access Point whose two radios sit on the *same*
wireless cell: one radio serves the SSID "campus" on 2.4GHz and the other
serves the same SSID on 5GHz.  Both BSSes are bridged into br0 with a single DHCP
server, so the two bands are one network.  Infix runs all AP radios in a
single hostapd process, which is what lets the cross-radio band-steering
directive resolve (see src/confd/src/hardware.c).

The other DUT is a client with a *single* dual-band radio.  Because both
BSSes share one cell, that one radio hears "campus" on both bands -- the
device band steering acts on.

Enabling 802.11v (`roaming dot11v`) on both BSSes turns on MBO band steering
by default: hostapd tracks which band it has seen a client on and, on the
2.4GHz BSS, suppresses probe responses to a client it has seen on the 5GHz
BSS (`no_probe_resp_if_seen_on`).  A dual-band client is therefore answered
only on 5GHz and associates there; a 2.4-only client, never seen on 5GHz, is
still answered and works normally.  See doc/wifi.md.

The test asserts the client associates to "campus", that band steering lands
it on the 5GHz BSS (not the 2.4GHz one), and that it leases an address -- a
completed association plus DHCP lease is real bidirectional traffic over the
(relayed) radio link.

Topology:
....
                       (( 2.4GHz ))
    host ==(mgmt)== ap            ~ one cell ~  client ==(mgmt)== host
                       ((  5GHz  ))         (single dual-band radio)
....
"""
import infamy
import infamy.iface as iface
import infamy.wifi as wifi
from infamy.util import until, parallel

SSID = "campus"
PSK = "infixinfix"          # 8-63 printable ASCII, see doc/wifi.md

# Both BSSes are bridged into br0 behind one DHCP server: the two bands are
# a single subnet, so a lease proves the client reached the network.  A
# two-address pool keeps the lease deterministic.
SUBNET = "192.168.30.0/24"
AP_IP = "192.168.30.1"
POOL_START = "192.168.30.100"
POOL_END = "192.168.30.101"

# hwsim defaults every radioN to the same address across guests, so the AP
# BSSIDs and the client's radio would collide without explicit addresses.
AP_BSSID_24 = "02:00:00:00:0a:01"
AP_BSSID_5 = "02:00:00:00:0b:01"
CLIENT_MAC = "02:00:00:00:00:0c"


def ap_bss(name, radio_name, bssid):
    """A band-steering AP BSS on the given radio, bridged into br0."""
    return wifi.iface(name, bssid, {
        "radio": radio_name,
        "access-point": {
            "ssid": SSID,
            "security": {"mode": "wpa2-wpa3-personal", "secret": "wifi"},
            # dot11v enables MBO band steering by default; the two BSSes
            # share the SSID so band steering has another band to point at.
            "roaming": {"dot11v": {}},
        },
    }, bridge="br0")


with infamy.Test() as test:
    with test.step("Set up topology and attach to the ap and the client"):
        env = infamy.Env()
        ap, client = parallel(
            lambda: env.attach("ap", "mgmt"),
            lambda: env.attach("client", "mgmt"),
        )
        wifi.skip_unless_supported(test, ap, client)

    with test.step("Configure the dual-band AP and the client with a single dual-band station radio"):
        # radio2 and radio3 are dut1's two extra radios, both wired to the
        # dedicated band-steering cell (cell2) in test/virt/quad.
        parallel(
            lambda: ap.put_config_dicts({
                "ietf-hardware": {"hardware": {"component": [
                    wifi.radio("radio2", band="2.4GHz", channel=1),
                    wifi.radio("radio3", band="5GHz", channel=36),
                ]}},
                "ietf-keystore": wifi.keystore({"wifi": PSK}),
                "ietf-interfaces": {"interfaces": {"interface": [
                    {"name": "br0", "type": "infix-if-type:bridge", "enabled": True,
                     "ietf-ip:ipv4": {"address": [
                         {"ip": AP_IP, "prefix-length": 24}]}},
                    ap_bss("wifi0", "radio2", AP_BSSID_24),
                    ap_bss("wifi1", "radio3", AP_BSSID_5),
                ]}},
                "infix-dhcp-server": {"dhcp-server": {"subnet": [{
                    "subnet": SUBNET,
                    "pool": {"start-address": POOL_START, "end-address": POOL_END},
                }]}},
            }),
            # radio2 is the client DUT's extra radio on the band-steering cell
            # (cell2).  No band/channel pinned: the one radio scans both bands and
            # lets band steering decide where it lands.
            lambda: client.put_config_dicts({
                "ietf-hardware": {"hardware": {"component": [wifi.radio("radio2")]}},
                "ietf-keystore": wifi.keystore({"wifi": PSK}),
                "ietf-interfaces": {"interfaces": {"interface": [
                    wifi.iface("wifi0", CLIENT_MAC, {
                        "radio": "radio2",
                        "station": {
                            "ssid": SSID,
                            "security": {"mode": "auto", "secret": "wifi"},
                        },
                    }, ipv4={"infix-dhcp-client:dhcp": {}}),
                ]}},
            }),
        )

    with test.step("Verify the client associates to the 'campus' SSID"):
        until(lambda: wifi.associated(client, SSID), attempts=60, interval=2)

    with test.step("Verify band steering put the client on the 5GHz BSS"):
        # Steered: the client shows up on the 5GHz BSS (wifi1) and not on the
        # 2.4GHz BSS (wifi0) -- the 2.4GHz BSS suppressed its probe responses
        # once it had been seen on 5GHz.
        until(lambda: CLIENT_MAC in wifi.ap_stations(ap, "wifi1"),
              attempts=90, interval=2)
        assert CLIENT_MAC not in wifi.ap_stations(ap, "wifi0"), \
            "client associated on 2.4GHz; band steering did not steer it to 5GHz"

    with test.step("Verify the client leases an address over 5GHz"):
        until(lambda: iface.is_oper_up(client, "wifi0"), attempts=30)
        until(lambda: iface.address_exist(client, "wifi0", POOL_START) or
              iface.address_exist(client, "wifi0", POOL_END),
              attempts=60, interval=2)

    test.succeed()
