#!/usr/bin/env python3
"""PPPoE client

Verify that a PPPoE client session comes up and is used, against a
minimal PPPoE server on the host.

The client authenticates with PAP and gets its IPv4 address and a DNS
server from the server.  Its default route through the session must be
a static route with the configured route preference, the same as a
DHCP route.

A host on the LAN behind the client must reach the server over the
session, which needs the IPv4 forwarding configured on the PPP
interface.  The MSS of its TCP connections to the server must arrive
clamped to the session MTU.

The PPP interface must exist, with its description set, before the
session is up.  When the server drops the session the interface must
stay, without the session's address and route, and the client must come
back on its own when the server returns, this time with CHAP.
Disabling the PPP interface must end the session but keep the interface,
and enabling it again must bring the session back.  Taking the port
under the session down must end the session, and the client must come
back on its own when the port is up again.  With a wrong
password the session must not come up.  Deleting the PPP interface must
remove it.

"""
import infamy
import infamy.iface as iface
import infamy.pppoe
import infamy.route as route
from infamy.util import until
from infamy.wifi import keystore

PEER = "10.0.0.100"
SERVER = "10.0.0.1"
DNS = "192.0.2.53"
LAN_CLIENT = "192.168.1.1"
LAN_HOST = "192.168.1.2"
CLAMPED_MSS = 1492 - 40         # PPPoE MTU minus IPv4 and TCP headers


def set_enabled(target, ifname, enabled):
    target.put_config_dicts({"ietf-interfaces": {"interfaces": {"interface": [{
        "name": ifname,
        "enabled": enabled,
    }]}}})


def session_up(target):
    """wan has the address from the server, pppd sets no origin"""
    return iface.exist(target, "wan") and \
        iface.address_exist(target, "wan", PEER, prefix_length=32, proto="other")


def forwarding(target):
    """wan is a routing interface, i.e., has forwarding enabled"""
    data = target.get_data("/ietf-routing:routing/interfaces")
    return "wan" in data.get("routing", {}).get("interfaces", {}).get("interface", [])


def peer_dns(target):
    """The server's DNS server is in use, learned on wan"""
    data = target.get_data("/ietf-system:system-state/infix-system:dns-resolver")
    resolver = data.get("system-state", {}).get("dns-resolver", {})
    return any(srv.get("address") == DNS and srv.get("interface") == "wan"
               for srv in resolver.get("server", []))


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        client = env.attach("client", "mgmt")
        if not client.has_feature("infix-interfaces", "ppp"):
            print("DUT does not advertise the 'ppp' feature -- skipping")
            test.skip()

        _, host = env.ltop.xlate("host", "data")
        _, port = env.ltop.xlate("client", "data")
        _, hostlan = env.ltop.xlate("host", "lan")
        _, lan = env.ltop.xlate("client", "lan")

    with test.step("Configure PPPoE client on wan, on top of the data port"):
        client.put_config_dicts({
            "ietf-keystore": keystore({"pppoe": "secret"}),
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": port,
                        "enabled": True
                    }, {
                        "name": lan,
                        "enabled": True,
                        "ipv4": {
                            "forwarding": True,
                            "address": [{"ip": LAN_CLIENT, "prefix-length": 24}]
                        }
                    }, {
                        "name": "wan",
                        "type": "infix-if-type:pppoe",
                        "description": "Uplink",
                        "ipv4": {
                            "forwarding": True
                        },
                        "infix-interfaces:ppp": {
                            "username": "user",
                            "secret": "pppoe"
                        },
                        "infix-interfaces:pppoe": {
                            "lower-layer-if": port
                        }
                    }]
                }
            }
        })

    with test.step("Verify wan exists and is configured before the session is up"):
        until(lambda: iface.exist(client, "wan"), attempts=20)
        until(lambda: iface.get_param(client, "wan", "description") == "Uplink", attempts=10)
        if session_up(client):
            test.fail()

    with infamy.IsolatedMacVlan(host) as ns:
        with infamy.pppoe.Server(ns, auth="pap", local=SERVER, peer=PEER, dns=DNS) as server:
            with test.step(f"Verify session comes up with PAP, wan gets {PEER}"):
                until(lambda: session_up(client), attempts=60)

            with test.step("Verify default route via wan is static with preference 5"):
                until(lambda: route.ipv4_route_exist(client, "0.0.0.0/0", proto="ietf-routing:static",
                                                     pref=5, active_check=True), attempts=20)

            with test.step(f"Verify DNS server {DNS} from the server is used"):
                until(lambda: peer_dns(client), attempts=20)

            with test.step("Verify IPv4 forwarding is enabled on wan"):
                until(lambda: forwarding(client), attempts=10)

            with infamy.IsolatedMacVlan(hostlan) as lanhost:
                lanhost.addip(LAN_HOST)
                lanhost.addroute(f"{SERVER}/32", LAN_CLIENT)

                with test.step(f"Verify LAN host reaches server {SERVER} over the session"):
                    lanhost.must_reach(SERVER, timeout=10)

                with test.step(f"Verify TCP MSS from the LAN is clamped to {CLAMPED_MSS}"):
                    # The server never answers, the SYN is all we need
                    syn = f"import socket; socket.create_connection(('{SERVER}', 80), timeout=2)"
                    until(lambda: lanhost.run(["python3", "-c", syn], capture_output=True) and
                          server.syn_mss() == CLAMPED_MSS, attempts=10)

            with test.step("Stop server, verify wan stays without session and default route"):
                server.stop()
                until(lambda: not session_up(client), attempts=30)
                if not iface.exist(client, "wan"):
                    test.fail()
                until(lambda: not route.ipv4_route_exist(client, "0.0.0.0/0",
                                                         proto="ietf-routing:static"), attempts=20)

        with infamy.pppoe.Server(ns, auth="chap", local=SERVER, peer=PEER, dns=DNS):
            with test.step("Restart server with CHAP, verify the client reconnects"):
                until(lambda: session_up(client), attempts=60)

            with test.step("Disable wan, verify the session ends and wan stays"):
                set_enabled(client, "wan", False)
                until(lambda: not session_up(client), attempts=20)
                if not iface.exist(client, "wan"):
                    test.fail()

            with test.step("Enable wan, verify the session comes back"):
                set_enabled(client, "wan", True)
                until(lambda: session_up(client), attempts=30)

            with test.step("Bounce the data port, verify the session ends and comes back"):
                set_enabled(client, port, False)
                until(lambda: not session_up(client), attempts=20)
                set_enabled(client, port, True)
                until(lambda: session_up(client), attempts=60)

            with test.step("Change password to a wrong one, verify the session stays down"):
                client.put_config_dicts({"ietf-keystore": keystore({"pppoe": "wrong"})})
                until(lambda: not session_up(client), attempts=30)
                for _ in range(10):
                    if session_up(client):
                        test.fail()

            with test.step("Delete wan, verify the interface is removed"):
                client.delete_xpath("/ietf-interfaces:interfaces/interface[name='wan']")
                until(lambda: not iface.exist(client, "wan"), attempts=20)

    test.succeed()
