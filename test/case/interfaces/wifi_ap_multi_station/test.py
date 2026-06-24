#!/usr/bin/env python3
r"""
WiFi Access Point serving multiple Stations

One AP and three Stations, each on its own DUT: the ap runs a
WPA2/WPA3-personal Access Point on radio0, and station1, station2 and
station3 each associate to it.  The radios are in separate kernels, so the
"air" between them is realised differently per environment:

  * On real hardware the links are real RF -- the stations just associate.
  * On QEMU the radios are mac80211_hwsim, and the `wifimedium` relay on each
    DUT bridges hwsim frames over the Ethernet segments between the guests
    (the topology's wifi-links).  See doc/wifi.md.

This is the multi-client case: a single AP cell must serve more than one
station at once.  Each station associating with WPA2/WPA3-personal is a
strong end-to-end check -- authentication, association and the WPA 4-way
handshake all require frames to cross the medium in *both* directions -- so
three successful associations prove the AP keeps several stations
associated simultaneously over the (relayed) radio links.  It then confirms
data-plane reach: the ap serves DHCP and each station leases an address
from the pool over the radio link.

Topology:
....
                       ((( station1 ==(mgmt)== host
    host ==(mgmt)== ap ((( station2 ==(mgmt)== host
                       ((( station3 ==(mgmt)== host
....
"""
import base64

import infamy
import infamy.iface as iface
from infamy.util import until, parallel

SSID = "infix-test"
PSK = "infixinfix"          # 8-63 printable ASCII, see doc/wifi.md
PSK_B64 = base64.b64encode(PSK.encode()).decode()
COUNTRY = "SE"

# hwsim defaults every radio0 to 02:00:00:00:00:00, so the AP and all the
# stations would otherwise share one MAC -- give each DUT a unique address.
STATIONS = [
    ("station1", "02:00:00:00:00:02"),
    ("station2", "02:00:00:00:00:03"),
    ("station3", "02:00:00:00:00:04"),
]
AP_MAC = "02:00:00:00:00:01"

# The ap runs a DHCP server on wifi0; every station leases an address from
# the pool.  A completed lease is real bidirectional IP traffic over the
# radio link, so a lease per station proves all three reach the ap at L3.
SUBNET = "192.168.20.0/24"
AP_IP = "192.168.20.1"
POOL_START = "192.168.20.100"
POOL_END = "192.168.20.102"     # three addresses, one per station


def radio(name, band=None, channel=None):
    """ietf-hardware component for a radio."""
    wifi_radio = {"country-code": COUNTRY}
    if band:
        wifi_radio["band"] = band
    if channel is not None:
        wifi_radio["channel"] = channel
    return {
        "name": name,
        "class": "infix-hardware:wifi",
        "infix-hardware:wifi-radio": wifi_radio,
    }


def keystore():
    return {
        "keystore": {
            "symmetric-keys": {
                "symmetric-key": [{
                    "name": "wifi",
                    "key-format": "infix-crypto-types:passphrase-key-format",
                    "cleartext-symmetric-key": PSK_B64,
                }]
            }
        }
    }


def wifi_iface(mac, wifi, ipv4=None):
    ifc = {
        "name": "wifi0",
        "type": "infix-if-type:wifi",
        "infix-interfaces:custom-phys-address": {"static": mac},
        "infix-interfaces:wifi": wifi,
    }
    if ipv4:
        ifc["ietf-ip:ipv4"] = ipv4
    return {"interfaces": {"interface": [ifc]}}


def leased(dut):
    """True once wifi0 holds an address handed out by the ap's DHCP server."""
    for addr in iface.get_ipv4_address(dut, "wifi0") or []:
        if addr.get("origin") == "dhcp" and \
           POOL_START <= addr.get("ip", "") <= POOL_END:
            return True
    return False


with infamy.Test() as test:
    with test.step("Set up topology and attach to the ap and three stations"):
        env = infamy.Env()
        # Connect to the AP and all stations concurrently to cut setup time.
        ap, *station_duts = parallel(
            lambda: env.attach("ap", "mgmt"),
            *((lambda n=name: env.attach(n, "mgmt")) for name, _ in STATIONS),
        )
        stations = [(name, mac, dut)
                    for (name, mac), dut in zip(STATIONS, station_duts)]

        for dut in [ap] + [s for _, _, s in stations]:
            if not dut.has_feature("infix-interfaces", "wifi"):
                print("DUT does not advertise the 'wifi' feature -- skipping")
                test.skip()

    with test.step("Configure the ap as an Access Point on radio0"):
        ap.put_config_dicts({
            "ietf-hardware": {"hardware": {"component": [
                radio("radio0", band="2.4GHz", channel=1)]}},
            "ietf-keystore": keystore(),
            "ietf-interfaces": wifi_iface(AP_MAC, {
                "radio": "radio0",
                "access-point": {
                    "ssid": SSID,
                    "security": {"mode": "wpa2-wpa3-personal", "secret": "wifi"},
                },
            }, ipv4={"address": [{"ip": AP_IP, "prefix-length": 24}]}),
            "infix-dhcp-server": {"dhcp-server": {"subnet": [{
                "subnet": SUBNET,
                "pool": {"start-address": POOL_START, "end-address": POOL_END},
            }]}},
        })

    for name, mac, dut in stations:
        with test.step("Configure the station on radio0"):
            print(f"Configuring {name}")
            dut.put_config_dicts({
                "ietf-hardware": {"hardware": {"component": [radio("radio0")]}},
                "ietf-keystore": keystore(),
                "ietf-interfaces": wifi_iface(mac, {
                    "radio": "radio0",
                    "station": {
                        "ssid": SSID,
                        "security": {"mode": "auto", "secret": "wifi"},
                    },
                }, ipv4={"infix-dhcp-client:dhcp": {}}),
            })

    for name, _mac, dut in stations:
        with test.step("Verify the station associates to the ap over the wifi link"):
            print(f"Verifying {name}")
            def associated(dut=dut):
                ifc = dut.get_iface("wifi0")
                if not ifc:
                    return False
                wifi = ifc.get("infix-interfaces:wifi") or ifc.get("wifi") or {}
                station = wifi.get("station", {})
                return station.get("ssid") == SSID and \
                    station.get("signal-strength") is not None
            until(associated, attempts=60, interval=2)

    for name, _mac, dut in stations:
        with test.step("Verify the station's wifi0 operational status is up"):
            print(f"Verifying {name}")
            until(lambda dut=dut: iface.is_oper_up(dut, "wifi0"), attempts=30)

    for name, _mac, dut in stations:
        with test.step("Verify the station leases an address from the ap over wifi"):
            print(f"Verifying {name}")
            until(lambda dut=dut: leased(dut), attempts=60, interval=2)

    test.succeed()
