#!/usr/bin/env python3
"""
Combined test of DHCP + NTP + DNS

Verify statically configured DNS and NTP servers are not lost when receiving
servers from a DHCP server.
"""
import infamy
import infamy.iface as iface
from infamy.util import until

def any_dhcp_address(target, iface_name):
    try:
        addrs = iface.get_ipv4_address(target, iface_name)
        if addrs:
            for addr in addrs:
                # Origin can be 'dhcp' or 'ietf-ip:dhcp' depending on how it's reported
                if addr.get('origin') in ['dhcp', 'ietf-ip:dhcp']:
                    return True
    except Exception:
        return False
    return False

def has_dns_server(target, dns_servers):
    """Verify system has all the given DNS servers in operational state"""
    data = target.get_data("/ietf-system:system-state/infix-system:dns-resolver")
    if not data:
        return False
    
    # In Infix, augmented nodes show up under their name in system-state
    resolver = data.get('system-state', {}).get('dns-resolver', {})
    servers = resolver.get('server', [])
    current_addresses = {s.get('address') for s in servers}
    return all(addr in current_addresses for addr in dns_servers)

def has_ntp_server(target, ntp_servers):
    """Verify system has all the given NTP servers in operational state"""
    data = target.get_data("/ietf-system:system-state/infix-system:ntp")
    if not data:
        return False
        
    ntp_state = data.get('system-state', {}).get('ntp', {})
    sources = ntp_state.get('sources', {}).get('source', [])
    current_addresses = {s.get('address') for s in sources}
    return all(addr in current_addresses for addr in ntp_servers)

def has_system_servers(target, dns, ntp_list):
    """Verify DUT has all DNS and NTP server(s)"""
    return has_dns_server(target, dns) and has_ntp_server(target, ntp_list)


with infamy.Test() as test:
    SERVER_IP = '192.168.3.1'
    CLIENT_STATIC_IP = '192.168.3.10'
    
    STATIC_DNS = '1.1.1.1'
    STATIC_NTP = '2.2.2.2'
    
    DHCP_DNS = SERVER_IP
    DHCP_NTP = SERVER_IP

    with test.step("Set up topology and configure static IP on client"):
        env = infamy.Env()
        server = env.attach("server", "mgmt")
        client = env.attach("client", "mgmt")

        client_data = client["server"]
        server_data = server["client"]

        # Configure server with static IP and NTP server
        server.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": server_data,
                        "type": "infix-if-type:ethernet",
                        "enabled": True,
                        "ipv4": {
                            "address": [{"ip": SERVER_IP, "prefix-length": 24}]
                        }
                    }]
                }
            },
            "ietf-ntp": {
                "ntp": {
                    "refclock-master": {"master-stratum": 8}
                }
            }
        })

        # Configure client with static IP
        client.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": client_data,
                        "type": "infix-if-type:ethernet",
                        "enabled": True,
                        "ipv4": {
                            "address": [{"ip": CLIENT_STATIC_IP, "prefix-length": 24}]
                        }
                    }]
                }
            }
        })

        until(lambda: iface.address_exist(client, client_data, CLIENT_STATIC_IP, proto="static"))
        print("Initial IP connectivity established")

    with test.step("Configure static DNS and NTP on client and verify"):
        client.put_config_dicts({
            "ietf-system": {
                "system": {
                    "ntp": {
                        "enabled": True,
                        "server": [{
                            "name": "static-ntp",
                            "udp": {"address": STATIC_NTP},
                            "iburst": True
                        }]
                    },
                    "dns-resolver": {
                        "server": [{
                            "name": "static-dns",
                            "udp-and-tcp": {"address": STATIC_DNS}
                        }]
                    }
                }
            }
        })

        print("Waiting for static servers to appear in operational state...")
        until(lambda: has_system_servers(client, [STATIC_DNS], [STATIC_NTP]),
              attempts=30)
        print("Static DNS and NTP servers verified")

    with test.step("Set up DHCP server with NTP and DNS options"):
        server.put_config_dicts({
            "infix-dhcp-server": {
                "dhcp-server": {
                    "subnet": [{
                        "subnet": "192.168.3.0/24",
                        "interface": server_data,
                        "pool": {
                            "start-address": "192.168.3.100",
                            "end-address":   "192.168.3.150"
                        },
                        "option": [
                            {"id": "router", "address": SERVER_IP},
                            {"id": "dns-server", "address": DHCP_DNS},
                            {"id": "ntp-server", "address": DHCP_NTP}
                        ]
                    }]
                }
            }
        })
        print("DHCP server configured on server DUT")

    with test.step("Configure client to use DHCP and verify combined servers"):
        # Explicitly delete static config using full path
        try:
            # Try to delete the entire ipv4 container from ietf-ip
            client.delete_xpath(f"ietf-interfaces:interfaces/interface[name='{client_data}']/ietf-ip:ipv4")
        except Exception:
            pass
        
        # Enable DHCP
        client.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": client_data,
                        "ipv4": {
                            "infix-dhcp-client:dhcp": {
                                "client-id": "client-1",
                                "option": [
                                    {"id": "dns-server"},
                                    {"id": "ntp-server"}
                                ]
                            }
                        }
                    }]
                }
            }
        })
        
        print("Waiting for DHCP lease...")
        until(lambda: any_dhcp_address(client, client_data), attempts=120)
        print("Client got DHCP address")

        print("Verifying combined DNS and NTP servers...")
        # Expected DNS: STATIC_DNS (1.1.1.1) AND DHCP_DNS (192.168.3.1)
        # Expected NTP: STATIC_NTP (2.2.2.2) AND DHCP_NTP (192.168.3.1)
        until(lambda: has_system_servers(client, [STATIC_DNS, DHCP_DNS], [STATIC_NTP, DHCP_NTP]),
              attempts=60)
        print("SUCCESS: Combined static and DHCP servers verified")

    test.succeed()
