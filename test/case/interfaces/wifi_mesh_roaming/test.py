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

  1. the three nodes form a mesh (each lists the other two as peers, and
     reports the mesh id and forwarding it runs with);
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
import infamy
import infamy.wifi as wifi
from infamy.util import until, parallel

SSID = "campus"
MESH_ID = "backhaul"
MESH_PSK = "meshmeshmesh"
WIFI_PSK = "infixinfix"

CLIENT_IP = "10.0.0.9"
HOST_IP = "10.0.0.1"
GWS = ["gw1", "gw2", "gw3"]

SECRETS = {"mesh-secret": MESH_PSK, "wifi-secret": WIFI_PSK}


def gw_config(uplink=None):
    interfaces = [
        {"name": "br0", "type": "infix-if-type:bridge", "enabled": True},
        wifi.iface("wifi0", {
            "radio": "radio0",
            "mesh-point": {
                "mesh-id": MESH_ID,
                "security": {"secret": "mesh-secret"},
            },
        }, bridge="br0"),
        wifi.iface("wifi1", {
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
        }, bridge="br0"),
    ]
    if uplink:
        interfaces.append({
            "name": uplink, "enabled": True,
            "infix-interfaces:bridge-port": {"bridge": "br0"},
        })
    return {
        "ietf-hardware": wifi.hardware(
            wifi.radio("radio0", band="5GHz", channel=36),
            wifi.radio("radio1", band="2.4GHz", channel=1)),
        "ietf-keystore": wifi.keystore(SECRETS),
        "ietf-interfaces": {"interfaces": {"interface": interfaces}},
    }


with infamy.Test() as test:
    with test.step("Set up topology and attach to gw1, gw2, gw3 and the client"):
        env = infamy.Env()
        # Connect to all four nodes concurrently -- each attach probes the
        # node and downloads its YANG models, so doing them in parallel cuts
        # the setup time roughly four-fold.
        gw1, gw2, gw3, client, gw1sh, gw2sh, gw3sh, clientsh = parallel(
            lambda: env.attach("gw1", "mgmt"),
            lambda: env.attach("gw2", "mgmt"),
            lambda: env.attach("gw3", "mgmt"),
            lambda: env.attach("client", "mgmt"),
            lambda: env.attach("gw1", "mgmt", "ssh"),
            lambda: env.attach("gw2", "mgmt", "ssh"),
            lambda: env.attach("gw3", "mgmt", "ssh"),
            lambda: env.attach("client", "mgmt", "ssh"),
        )
        gw_duts = [gw1, gw2, gw3]
        shells = {"gw1": gw1sh, "gw2": gw2sh, "gw3": gw3sh}
        gws = list(zip(GWS, gw_duts))

        wifi.skip_unless_supported(test, client, *gw_duts)

    with test.step("Configure gw1, gw2, gw3 as mesh nodes with a roaming AP"):
        gw1_uplink = gw1["uplink"]
        parallel(*[lambda name=name, dut=dut:
                   dut.put_config_dicts(gw_config(uplink=gw1_uplink if name == "gw1" else None))
                   for name, dut in gws])

    with test.step("Configure the client as a station for the 'campus' SSID"):
        # The client joins the gw APs, which run on radio1 (2.4GHz).  In the
        # virtual topology a wireless cell is shared per radio index, so the
        # client's station must use radio1 too -- a station and the AP it
        # associates to live in the same cell only when they share an index.
        # See doc/wifi.md and test/virt/quad.
        client.put_config_dicts({
            "ietf-hardware": wifi.hardware(wifi.radio("radio1", band="2.4GHz", channel=1)),
            "ietf-keystore": wifi.keystore(SECRETS),
            "ietf-interfaces": {"interfaces": {"interface": [
                wifi.iface("wifi0", {
                    "radio": "radio1",
                    "station": {
                        "ssid": SSID,
                        "security": {"mode": "auto", "secret": "wifi-secret"},
                    },
                }, ipv4={"address": [{"ip": CLIENT_IP, "prefix-length": 24}]}),
            ]}},
        })

    with test.step("Verify each node lists the other two nodes as mesh peers"):
        mesh_macs = {name: wifi.phys_address(dut, "wifi0") for name, dut in gws}
        for name, dut in gws:
            until(lambda dut=dut, mesh_mac=mesh_macs[name]:
                  wifi.mesh_peer_macs(dut) == set(mesh_macs.values()) - {mesh_mac},
                  attempts=60, interval=2)

    with test.step("Verify each node reports mesh-id 'backhaul' with forwarding enabled"):
        for name, dut in gws:
            mp = wifi.mesh_point(dut)
            if mp.get("mesh-id") != MESH_ID or mp.get("forwarding") is not True:
                print(f"{name}: {mp}")
                test.fail()

    with test.step("Verify the client associates to the 'campus' SSID"):
        until(lambda: wifi.associated(client, SSID), attempts=60, interval=2)

    # The client reports the BSSID it is on; with all three APs sharing the
    # SSID, that BSSID is what tells them apart.
    aps = {wifi.phys_address(dut, "wifi1"): (name, dut) for name, dut in gws}

    def on_campus_ap(*not_on):
        bssid = wifi.station_bssid(client)
        return bssid if bssid in aps and bssid not in not_on else None

    with test.step("Verify the client is connected to one of the campus APs"):
        first_bssid = until(on_campus_ap, attempts=60, interval=2)
        first_ap, first_dut = aps[first_bssid]
        print(f"client is on {first_ap} ({first_bssid})")

    # The host sits on the wired LAN behind gw1; reaching the client proves
    # the frames cross the mesh backhaul (the client may be on any gw's AP).
    hlan = env["host"]["lan"]
    with infamy.IsolatedMacVlan(hlan) as ns:
        ns.addip(HOST_IP)

        # Give a ping a few tries, the radios have just come up.
        def reaches(addr):
            try:
                ns.ping(addr)
                return True
            except Exception:
                return False

        with test.step("Verify the client is reachable across the mesh"):
            until(lambda: reaches(CLIENT_IP), attempts=10, interval=2)

        with test.step("Take down the client's current AP to force a roam"):
            first_dut.put_config_dicts({"ietf-interfaces": {"interfaces": {
                "interface": [{"name": "wifi1", "enabled": False}]}}})

        with test.step("Verify the client roams to another node's AP"):
            new_bssid = until(lambda: on_campus_ap(first_bssid), attempts=90, interval=2)
            new_ap, _ = aps[new_bssid]
            print(f"client roamed from {first_ap} to {new_ap}")

        with test.step("Verify connectivity is restored after roaming"):
            until(lambda: reaches(CLIENT_IP), attempts=15, interval=2)

        # A node going down for a reboot or an upgrade asks its clients
        # to move first, so a client with a strong signal roams instead
        # of waiting for the beacons to stop.
        second_bssid = wifi.station_bssid(client)
        second_ap, _ = aps[second_bssid]
        mark = int(clientsh.runsh("wc -l < /var/log/syslog").stdout)

        with test.step("Stop the WiFi service on the client's current node"):
            shells[second_ap].runsh("initctl stop hostapd")

        with test.step("Verify the client roams to the remaining node's AP"):
            third_bssid = until(lambda: on_campus_ap(first_bssid, second_bssid),
                                attempts=30, interval=1)
            third_ap, _ = aps[third_bssid]
            print(f"client roamed from {second_ap} to {third_ap}")

        with test.step("Verify the client was asked to move, given candidates, and roamed without disconnecting"):
            def log():
                return clientsh.runsh(f"tail -n +{mark + 1} /var/log/syslog").stdout
            until(lambda: "Disassociation Imminent" in log(), attempts=10, interval=1)
            assert "Preferred List Available" in log(), log()
            assert "CTRL-EVENT-DISCONNECTED" not in log(), log()

        with test.step("Verify connectivity is restored after the handover"):
            until(lambda: reaches(CLIENT_IP), attempts=15, interval=2)

        with test.step("Start the WiFi service again on the stopped node"):
            shells[second_ap].runsh("initctl start hostapd")

    test.succeed()
