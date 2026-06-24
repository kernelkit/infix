#!/usr/bin/env python3
r"""
WiFi Mesh backhaul with roaming Access Points

The worked example from doc/whitepaper-wifi-mesh-roaming.md, as a test.

Three gateway nodes (gw1, gw2, gw3) each do two jobs on two radios:

  * radio0 -- an 802.11s mesh point.  The three join one mesh ("backhaul")
    on 5GHz; this carries traffic between the nodes so only gw1 needs a wire
    to the rest of the LAN.
  * radio1 -- a WPA2/WPA3-personal Access Point on 2.4GHz.  All three share
    the same SSID ("campus") and 802.11r mobility domain, so a client roams
    between them as one network.

Each node bridges its mesh and AP (and, on gw1, the wired uplink) into br0,
so the mesh is a transparent layer-2 backhaul.  A fourth node is the client.

The test checks the claims the whitepaper makes:

  1. the three nodes form a mesh (each sees its two peers);
  2. the client associates to the "campus" SSID;
  3. traffic reaches the client across the mesh backhaul (host behind gw1
     pings the client, which is attached to some gw's AP);
  4. roaming: the client reports the BSSID it is connected to, which is one
     of the three gw APs; when that AP is taken down the client moves to
     another node -- same SSID, same mobility domain -- so the reported
     BSSID changes to a different gw, and traffic recovers.

In simulation every radio hears every other at one fixed strength, so there
is no signal gradient to drift the client between APs.  Step 4 forces the
move instead, which is a stronger check: it proves a second AP accepts the
client and the backhaul re-converges.

Topology:
....
    host ==(mgmt)== gw1 (mesh+AP) ))  ~ mesh ~  (( gw2 (mesh+AP)
      \\__(lan, wired)__/  \                    /
                            )  ~ mesh ~  (( gw3 (mesh+AP)
                            client (((roams between the gw APs)))
....
"""
import base64

import infamy
from infamy.util import until, parallel

SSID = "campus"
MESH_ID = "backhaul"
MESH_PSK = base64.b64encode(b"meshmeshmesh").decode()
WIFI_PSK = base64.b64encode(b"infixinfix").decode()
COUNTRY = "SE"

CLIENT_IP = "10.0.0.9"
HOST_IP = "10.0.0.1"
CLIENT_MAC = "02:00:00:00:00:09"

# Unique MACs: hwsim defaults every radioN to the same address across guests,
# so the mesh peers and the AP BSSIDs would collide without these.  The AP MAC
# is the BSSID the client reports when associated to that node.
GWS = [
    # name, mesh radio0 MAC, AP radio1 MAC (BSSID)
    ("gw1", "02:00:00:00:00:01", "02:00:00:00:0a:01"),
    ("gw2", "02:00:00:00:00:02", "02:00:00:00:0a:02"),
    ("gw3", "02:00:00:00:00:03", "02:00:00:00:0a:03"),
]


def radio(name, band, channel):
    return {
        "name": name,
        "class": "infix-hardware:wifi",
        "infix-hardware:wifi-radio": {
            "country-code": COUNTRY, "band": band, "channel": channel,
        },
    }


def keystore():
    return {"keystore": {"symmetric-keys": {"symmetric-key": [
        {"name": "mesh-secret",
         "key-format": "infix-crypto-types:passphrase-key-format",
         "cleartext-symmetric-key": MESH_PSK},
        {"name": "wifi-secret",
         "key-format": "infix-crypto-types:passphrase-key-format",
         "cleartext-symmetric-key": WIFI_PSK},
    ]}}}


def gw_config(mesh_mac, ap_mac, uplink=None):
    interfaces = [
        {"name": "br0", "type": "infix-if-type:bridge", "enabled": True},
        {
            "name": "wifi0", "type": "infix-if-type:wifi", "enabled": True,
            "infix-interfaces:custom-phys-address": {"static": mesh_mac},
            "infix-interfaces:wifi": {
                "radio": "radio0",
                "mesh-point": {
                    "mesh-id": MESH_ID,
                    "security": {"secret": "mesh-secret"},
                },
            },
            "infix-interfaces:bridge-port": {"bridge": "br0"},
        },
        {
            "name": "wifi1", "type": "infix-if-type:wifi", "enabled": True,
            "infix-interfaces:custom-phys-address": {"static": ap_mac},
            "infix-interfaces:wifi": {
                "radio": "radio1",
                "access-point": {
                    "ssid": SSID,
                    "security": {"mode": "wpa2-wpa3-personal", "secret": "wifi-secret"},
                    "roaming": {
                        "dot11r": {"mobility-domain": "hash"},
                        "dot11k": {},
                        "dot11v": {},
                    },
                },
            },
            "infix-interfaces:bridge-port": {"bridge": "br0"},
        },
    ]
    if uplink:
        interfaces.append({
            "name": uplink, "enabled": True,
            "infix-interfaces:bridge-port": {"bridge": "br0"},
        })
    return {
        "ietf-hardware": {"hardware": {"component": [
            radio("radio0", "5GHz", 36),
            radio("radio1", "2.4GHz", 1),
        ]}},
        "ietf-keystore": keystore(),
        "ietf-interfaces": {"interfaces": {"interface": interfaces}},
    }


def wifi_of(ifc):
    return (ifc or {}).get("infix-interfaces:wifi") or (ifc or {}).get("wifi") or {}


def mesh_peers(dut):
    mp = wifi_of(dut.get_iface("wifi0")).get("mesh-point") or {}
    return (mp.get("peers") or {}).get("peer") or []


def station(dut, ifname="wifi0"):
    return wifi_of(dut.get_iface(ifname)).get("station", {})


def station_bssid(dut):
    """The BSSID the client's station is currently associated to."""
    return (station(dut).get("bssid") or "").lower()


with infamy.Test() as test:
    with test.step("Set up topology and attach to gw1, gw2, gw3 and the client"):
        env = infamy.Env()
        # Connect to all four nodes concurrently -- each attach probes the
        # node and downloads its YANG models, so doing them in parallel cuts
        # the setup time roughly four-fold.
        gw1, gw2, gw3, client = parallel(
            lambda: env.attach("gw1", "mgmt"),
            lambda: env.attach("gw2", "mgmt"),
            lambda: env.attach("gw3", "mgmt"),
            lambda: env.attach("client", "mgmt"),
        )
        gw_duts = [gw1, gw2, gw3]
        gws = [(name, dut, mesh, ap) for (name, mesh, ap), dut in zip(GWS, gw_duts)]

        for dut in gw_duts + [client]:
            if not dut.has_feature("infix-interfaces", "wifi"):
                print("DUT does not advertise the 'wifi' feature -- skipping")
                test.skip()

    with test.step("Configure gw1, gw2, gw3 as mesh nodes with a roaming AP"):
        _, gw1_uplink = env.ltop.xlate("gw1", "uplink")
        for name, dut, mesh_mac, ap_mac in gws:
            dut.put_config_dicts(gw_config(mesh_mac, ap_mac,
                                           uplink=gw1_uplink if name == "gw1" else None))

    with test.step("Configure the client as a station for the 'campus' SSID"):
        # The client joins the gw APs, which run on radio1 (2.4GHz).  In the
        # virtual topology a wireless cell is shared per radio index, so the
        # client's station must use radio1 too -- a station and the AP it
        # associates to live in the same cell only when they share an index.
        # See doc/wifi.md and test/virt/quad.
        client.put_config_dicts({
            "ietf-hardware": {"hardware": {"component": [radio("radio1", "2.4GHz", 1)]}},
            "ietf-keystore": keystore(),
            "ietf-interfaces": {"interfaces": {"interface": [{
                "name": "wifi0", "type": "infix-if-type:wifi", "enabled": True,
                "infix-interfaces:custom-phys-address": {"static": CLIENT_MAC},
                "infix-interfaces:wifi": {
                    "radio": "radio1",
                    "station": {
                        "ssid": SSID,
                        "security": {"mode": "auto", "secret": "wifi-secret"},
                    },
                },
                "ietf-ip:ipv4": {"address": [{"ip": CLIENT_IP, "prefix-length": 24}]},
            }]}},
        })

    with test.step("Verify the three nodes form the mesh backhaul"):
        for name, dut, _, _ in gws:
            until(lambda dut=dut: len(mesh_peers(dut)) >= 2, attempts=60, interval=2)

    with test.step("Verify the client associates to the 'campus' SSID"):
        def associated():
            sta = station(client)
            return sta.get("ssid") == SSID and sta.get("signal-strength") is not None
        until(associated, attempts=60, interval=2)

    # The client reports the BSSID it is on; with all three APs sharing the
    # SSID, that BSSID is what tells them apart.
    aps = {ap_mac.lower(): (name, dut) for name, dut, _, ap_mac in gws}

    with test.step("Verify the client is connected to one of the campus APs"):
        until(lambda: station_bssid(client) in aps, attempts=60, interval=2)
        first_bssid = station_bssid(client)
        first_ap, first_dut = aps[first_bssid]
        print(f"client is on {first_ap} ({first_bssid})")

    # The host sits on the wired LAN behind gw1; reaching the client proves
    # the frames cross the mesh backhaul (the client may be on any gw's AP).
    _, hlan = env.ltop.xlate("host", "lan")
    with infamy.IsolatedMacVlan(hlan) as ns:
        ns.addip(HOST_IP)

        with test.step("Verify the client is reachable across the mesh"):
            ns.must_reach(CLIENT_IP)

        with test.step("Take down the client's current AP to force a roam"):
            first_dut.put_config_dicts({"ietf-interfaces": {"interfaces": {
                "interface": [{"name": "wifi1", "enabled": False}]}}})

        with test.step("Verify the client roams to another node's AP"):
            until(lambda: station_bssid(client) in aps and station_bssid(client) != first_bssid,
                  attempts=90, interval=2)
            new_ap, _ = aps[station_bssid(client)]
            print(f"client roamed from {first_ap} to {new_ap}")

        with test.step("Verify connectivity is restored after roaming"):
            ns.must_reach(CLIENT_IP)

    test.succeed()
