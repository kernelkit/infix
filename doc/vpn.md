# VPN Configuration

A Virtual Private Network (VPN) creates encrypted tunnels over public networks,
enabling secure communication between remote locations or users.  Unlike plain
tunnels (GRE, VXLAN) that only provide encapsulation, VPNs add authentication
and encryption to protect data confidentiality and integrity.


## Configuring VPN

For detailed configuration instructions and examples, see:

- **[WireGuard VPN](vpn-wireguard.md)** - Complete guide to configuring
  WireGuard tunnels, including site-to-site, road warrior, and hub-and-spoke
  topologies.

## Understanding VPN Tunnels

VPN tunnels establish secure connections across untrusted networks by:

- **Authentication:** Verifying the identity of tunnel endpoints using
  cryptographic keys or certificates
- **Encryption:** Protecting data confidentiality with strong ciphers
- **Integrity:** Detecting tampering through message authentication codes

This makes VPNs essential for connecting sites over the internet, enabling
remote access for mobile users, and securing traffic in untrusted environments.

### VPN Deployment Models

VPNs are typically deployed in one of several models:

**Site-to-Site VPN**

![Site-to-Site VPN Topology](img/vpn-site-to-site.svg)
*Figure: Site-to-Site VPN connecting two office networks*

Connects entire networks across locations, creating a unified private network
over the internet.  Routers or firewalls at each site maintain persistent
tunnels, allowing seamless access between locations.

- Use case: Connecting branch offices to headquarters
- Characteristics: Always-on, connects networks not individual devices
- Example: Main office (192.168.1.0/24) ↔ Branch office (192.168.2.0/24)

**Remote Access VPN (Road Warrior)**

![Road Warrior VPN Topology](img/vpn-roadwarrior.svg)
*Figure: Mobile clients connecting to corporate network*

Enables individual users to securely access a private network from remote
locations.  Clients initiate connections as needed from dynamic IP addresses.

- Use case: Remote employees accessing corporate resources
- Characteristics: On-demand, handles dynamic endpoints and roaming
- Example: Mobile laptop ↔ Corporate network

**Hub-and-Spoke VPN**

![Hub-and-Spoke VPN Topology](img/vpn-hub-spoke.svg)
*Figure: Hub-and-Spoke topology with central hub routing traffic between spokes*

A central hub connects to multiple remote sites (spokes), routing traffic
between them.  Spokes don't connect directly to each other but communicate
through the hub.

- Use case: Central office connecting multiple remote locations
- Characteristics: Centralized control, simplified management
- Example: HQ ↔ (Branch A, Branch B, Branch C)

### VPN Protocol Comparison

Different VPN protocols offer varying trade-offs between security, performance,
and complexity:

| Protocol   | Complexity | Performance | Use Case                        |
|------------|------------|-------------|---------------------------------|
| WireGuard  | Simple     | Very High   | Modern deployments, all models  |
| IPsec      | Complex    | High        | Legacy systems, compliance reqs |
| OpenVPN    | Moderate   | Moderate    | Maximum compatibility           |

Infix supports WireGuard as its primary VPN solution, offering the best
balance of simplicity, security, and performance for modern networks.
