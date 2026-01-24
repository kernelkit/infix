# WireGuard VPN

> [!NOTE]
> For a general introduction to VPN concepts and deployment models, see
> [VPN Configuration](vpn.md).

WireGuard is a modern, high-performance VPN protocol that uses state-of-the-art
cryptography.  It is significantly simpler and faster than traditional VPN
solutions like IPsec or OpenVPN, while maintaining strong security guarantees.

Key features of WireGuard:

- **Simple Configuration:** Minimal settings required compared to IPsec
- **High Performance:** Runs in kernel space with efficient cryptography
- **Strong Cryptography:** Uses Curve25519, ChaCha20, Poly1305, and BLAKE2
- **Roaming Support:** Seamlessly handles endpoint IP address changes
- **Dual-Stack:** Supports IPv4 and IPv6 for both tunnel endpoints and traffic

> [!IMPORTANT]
> When issuing `leave` to activate your changes, remember to also save
> your settings, `copy running-config startup-config`.  See the [CLI
> Introduction](cli/introduction.md) for a background.

## Key Management

WireGuard uses public-key cryptography similar to SSH.  Each WireGuard interface
requires a private key, and each peer is identified by its public key.

**Generate a WireGuard key pair using the `wg` command:**

```bash
admin@example:~$ wg genkey | tee privatekey | wg pubkey > publickey
admin@example:~$ cat privatekey
aMqBvZqkSP5JrqBvZqkSP5JrqBvZqkSP5JrqBvZqkSP=
admin@example:~$ cat publickey
bN1CwZ1lTP6KsrCwZ1lTP6KsrCwZ1lTP6KsrCwZ1lTP=
```

This generates a private key, saves it to `privatekey`, derives the public key,
and saves it to `publickey`.

**Import the private key into the keystore:**

```
admin@example:/> configure
admin@example:/config/> edit keystore asymmetric-key wg-site-a
admin@example:/config/keystore/asymmetric-key/wg-site-a/> set public-key-format x25519-public-key-format
admin@example:/config/keystore/asymmetric-key/wg-site-a/> set private-key-format x25519-private-key-format
admin@example:/config/keystore/asymmetric-key/wg-site-a/> set public-key bN1CwZ1lTP6KsrCwZ1lTP6KsrCwZ1lTP6KsrCwZ1lTP=
admin@example:/config/keystore/asymmetric-key/wg-site-a/> set private-key aMqBvZqkSP5JrqBvZqkSP5JrqBvZqkSP5JrqBvZqkSP=
admin@example:/config/keystore/asymmetric-key/wg-site-a/> leave
admin@example:/>
```

**Import peer public keys into the truststore:**

```
admin@example:/> configure
admin@example:/config/> edit truststore public-key-bag wg-peers public-key peer-b
admin@example:/config/truststore/…/peer-b/> set public-key-format x25519-public-key-format
admin@example:/config/truststore/…/peer-b/> set public-key PEER_PUBLIC_KEY_HERE
admin@example:/config/truststore/…/peer-b/> leave
admin@example:/>
```

> [!IMPORTANT]
> Keep private keys secure!  Never share your private key.  Only exchange
> public keys with peers.  Delete the `privatekey` file after importing it
> into the keystore.

## Point-to-Point Configuration

> [!TIP]
> If you name your WireGuard interface `wgN`, where `N` is a number, the
> CLI infers the interface type automatically.

A basic WireGuard tunnel between two sites:

**Site A configuration:**

```
admin@siteA:/> configure
admin@siteA:/config/> edit interface wg0
admin@siteA:/config/interface/wg0/> set wireguard listen-port 51820
admin@siteA:/config/interface/wg0/> set wireguard private-key wg-site-a
admin@siteA:/config/interface/wg0/> set ipv4 address 10.0.0.1 prefix-length 24
admin@siteA:/config/interface/wg0/> edit wireguard peer wg-peers peer-b
admin@siteA:/config/interface/…/wg-peers/peer/peer-b/> set endpoint 203.0.113.2
admin@siteA:/config/interface/…/wg-peers/peer/peer-b/> set endpoint-port 51820
admin@siteA:/config/interface/…/wg-peers/peer/peer-b/> set allowed-ips 10.0.0.2/32
admin@siteA:/config/interface/…/wg-peers/peer/peer-b/> set persistent-keepalive 25
admin@siteA:/config/interface/…/wg-peers/peer/peer-b/> leave
admin@siteA:/>
```

**Site B configuration:**

```
admin@siteB:/> configure
admin@siteB:/config/> edit interface wg0
admin@siteB:/config/interface/wg0/> set wireguard listen-port 51820
admin@siteB:/config/interface/wg0/> set wireguard private-key wg-site-b
admin@siteB:/config/interface/wg0/> set ipv4 address 10.0.0.2 prefix-length 24
admin@siteB:/config/interface/wg0/> edit wireguard peer wg-peers peer-a
admin@siteB:/config/interface/…/wg-peers/peer/peer-a/> set endpoint 203.0.113.1
admin@siteB:/config/interface/…/wg-peers/peer/peer-a/> set endpoint-port 51820
admin@siteB:/config/interface/…/wg-peers/peer/peer-a/> set allowed-ips 10.0.0.1/32
admin@siteB:/config/interface/…/wg-peers/peer/peer-a/> set persistent-keepalive 25
admin@siteB:/config/interface/…/wg-peers/peer/peer-a/> leave
admin@siteB:/>
```

This creates an encrypted tunnel with Site A at 10.0.0.1 and Site B at 10.0.0.2.

## Understanding Allowed IPs

The `allowed-ips` setting in WireGuard serves two critical purposes:

1. **Ingress Filtering:** Only packets with source IPs in the allowed list
   are accepted from the peer
2. **Cryptokey Routing:** Determines which peer receives outbound packets
   for a given destination

Think of `allowed-ips` as a combination of firewall rules and routing table.

![WireGuard Keys and Allowed IPs](img/wireguard-allowed-ips.svg)
*Figure: WireGuard key exchange and allowed-ips configuration between two peers*

For a simple point-to-point tunnel, you typically allow only the peer's
tunnel IP address (e.g., `10.0.0.2/32`).  For site-to-site VPNs connecting
entire networks, include the remote network prefixes:

```
admin@example:/config/interface/…/wg-peers/peer/peer-a/> set allowed-ips 10.0.0.2/32
admin@example:/config/interface/…/wg-peers/peer/peer-a/> set allowed-ips 192.168.2.0/24
```

This allows traffic to/from the peer at 10.0.0.2 and routes traffic destined
for 192.168.2.0/24 through this peer.

> [!NOTE]
> When routing traffic to networks behind WireGuard peers, you also need
> to configure static routes pointing to the WireGuard interface.  See
> [Static Routes](networking.md#routing-support) for more information.

## Peer Configuration and Key Bags

WireGuard peer configuration supports a two-level hierarchy that allows
efficient management of multiple peers with shared settings.

**Public Key Bags** group related peers together (e.g., all mobile clients,
all branch offices) and allow you to configure default settings that apply
to all peers in the bag.  Individual peers can then override these defaults
when needed.

Settings that support bag-level defaults and per-peer overrides:

- `endpoint` - Remote peer's IP address
- `endpoint-port` - Remote peer's UDP port (defaults to 51820 at bag level)
- `persistent-keepalive` - Keepalive interval in seconds
- `preshared-key` - Optional pre-shared key for additional quantum resistance
- `allowed-ips` - IP addresses allowed to/from this peer

> [!IMPORTANT]
> **Key Bag Configuration Rules:**
> - **Single key in bag:** You can use bag-level settings (endpoint, allowed-ips, etc.)
>   without specifying individual peer configurations. All settings apply to that one peer.
> - **Multiple keys in bag:** You MUST provide individual `peer` configuration for
>   each key in the bag. Settings like `endpoint` and `allowed-ips` must be unique
>   per peer and cannot be shared at the bag level when multiple peers exist.
>
> This prevents configuration errors where multiple peers would incorrectly share
> the same endpoint address or allowed-ips ranges.

**Example with bag-level defaults:**

```
admin@example:/> configure
admin@example:/config/> edit interface wg0
admin@example:/config/interface/wg0/> set wireguard listen-port 51820
admin@example:/config/interface/wg0/> set wireguard private-key wg-key
admin@example:/config/interface/wg0/> set ipv4 address 10.0.0.1 prefix-length 24

# Configure defaults for all peers in the 'branch-offices' bag
admin@example:/config/interface/wg0/> edit wireguard peers branch-offices
admin@example:/config/interface/…/wireguard/peers/branch-offices/> set endpoint-port 51820
admin@example:/config/interface/…/wireguard/peers/branch-offices/> set persistent-keepalive 25
admin@example:/config/interface/…/wireguard/peers/branch-offices/> end

# Configure peer-specific settings (inherits endpoint-port and keepalive from bag)
admin@example:/config/interface/…/wireguard/peers/branch-offices/> edit peer office-east
admin@example:/config/interface/…/branch-offices/peer/office-east/> set endpoint 203.0.113.10
admin@example:/config/interface/…/branch-offices/peer/office-east/> set allowed-ips 10.0.0.10/32
admin@example:/config/interface/…/branch-offices/peer/office-east/> set allowed-ips 192.168.10.0/24
admin@example:/config/interface/…/branch-offices/peer/office-east/> end

# Another peer with an override for persistent-keepalive
admin@example:/config/interface/…/wireguard/peers/branch-offices/> edit peer office-west
admin@example:/config/interface/…/branch-offices/peer/office-west/> set endpoint 203.0.113.20
admin@example:/config/interface/…/branch-offices/peer/office-west/> set allowed-ips 10.0.0.20/32
admin@example:/config/interface/…/branch-offices/peer/office-west/> set allowed-ips 192.168.20.0/24
admin@example:/config/interface/…/branch-offices/peer/office-west/> set persistent-keepalive 10
admin@example:/config/interface/…/branch-offices/peer/office-west/> leave
admin@example:/>
```

In this example:
- Both peers inherit `endpoint-port 51820` and `persistent-keepalive 25` from the bag
- `office-west` overrides the keepalive to 10 seconds while `office-east` uses the default 25
- Each peer has its own `endpoint` and `allowed-ips` configuration

This approach simplifies management when you have many peers with similar
configurations - set the common defaults once at the bag level, then only
specify per-peer differences.

## Site-to-Site VPN

![Site-to-Site VPN Topology](img/vpn-site-to-site.svg)
*Figure: Site-to-Site VPN connecting two office networks*

A site-to-site VPN connects entire networks across locations, creating a unified
private network over the internet.  This allows devices in one location to
seamlessly access resources in another as if they were on the same local network.

This is the point-to-point configuration shown earlier, extended with routing
to allow access to networks behind each peer.  Configure the WireGuard tunnel
as shown in [Point-to-Point Configuration](#point-to-point-configuration),
then add the remote network to `allowed-ips` and configure static routes.

## Road Warrior VPN

![Road Warrior VPN Topology](img/vpn-roadwarrior.svg)
*Figure: Mobile clients connecting to corporate network*

For mobile clients or peers without fixed IPs, omit the `endpoint` setting.
WireGuard learns the peer's endpoint from authenticated incoming packets:

```
admin@hub:/> configure
admin@hub:/config/> edit interface wg0 wireguard peers wg-peers peer mobile-client
admin@hub:/config/interface/…/wg-peers/peer/mobile-client/> set allowed-ips 10.0.0.10/32
admin@hub:/config/interface/…/wg-peers/peer/mobile-client/> leave
admin@hub:/>
```

The mobile client configures the hub's endpoint normally.  The hub learns
and tracks the mobile client's changing IP address automatically.

## Hub-and-Spoke Topology

![Hub-and-Spoke VPN Topology](img/vpn-hub-spoke.svg)
*Figure: Hub-and-Spoke topology with central hub routing traffic between spokes*

WireGuard excels at hub-and-spoke (star) topologies where multiple remote
sites connect to a central hub.

**Hub configuration:**

```
admin@hub:/> configure
admin@hub:/config/> edit interface wg0
admin@hub:/config/interface/wg0/> set ipv4 address 10.0.0.1 prefix-length 24
admin@hub:/config/interface/wg0/> set wireguard listen-port 51820
admin@hub:/config/interface/wg0/> set wireguard private-key wg-hub
admin@hub:/config/interface/wg0/> edit wireguard peers wg-peers

# Spoke 1
admin@hub:/config/interface/…/wireguard/peers/wg-peers/> edit peer spoke1
admin@hub:/config/interface/…/wg-peers/peer/spoke1/> set allowed-ips 10.0.0.2/32
admin@hub:/config/interface/…/wg-peers/peer/spoke1/> set allowed-ips 192.168.1.0/24
admin@hub:/config/interface/…/wg-peers/peer/spoke1/> end

# Spoke 2
admin@hub:/config/interface/…/wireguard/peers/wg-peers/> edit peer spoke2
admin@hub:/config/interface/…/wg-peers/peer/spoke2/> set allowed-ips 10.0.0.3/32
admin@hub:/config/interface/…/wg-peers/peer/spoke2/> set allowed-ips 192.168.2.0/24
admin@hub:/config/interface/…/wg-peers/peer/spoke2/> leave

# Add routes for spoke networks
admin@hub:/> configure
admin@hub:/config/> edit routing control-plane-protocol static name default
admin@hub:/config/routing/…/static/name/default/> set ipv4 route 192.168.1.0/24 wg0
admin@hub:/config/routing/…/static/name/default/> set ipv4 route 192.168.2.0/24 wg0
admin@hub:/config/routing/…/static/name/default/> leave
admin@hub:/>
```

**Spoke 1 configuration:**

```
admin@spoke1:/> configure
admin@spoke1:/config/> edit interface wg0
admin@spoke1:/config/interface/wg0/> set wireguard listen-port 51820
admin@spoke1:/config/interface/wg0/> set wireguard private-key wg-spoke1
admin@spoke1:/config/interface/wg0/> set ipv4 address 10.0.0.2 prefix-length 24
admin@spoke1:/config/interface/wg0/> edit wireguard peers wg-peers peer hub
admin@spoke1:/config/interface/…/wg-peers/peer/hub/> set endpoint 203.0.113.1
admin@spoke1:/config/interface/…/wg-peers/peer/hub/> set endpoint-port 51820
admin@spoke1:/config/interface/…/wg-peers/peer/hub/> set allowed-ips 10.0.0.1/32
admin@spoke1:/config/interface/…/wg-peers/peer/hub/> set allowed-ips 10.0.0.3/32
admin@spoke1:/config/interface/…/wg-peers/peer/hub/> set allowed-ips 192.168.0.0/24
admin@spoke1:/config/interface/…/wg-peers/peer/hub/> set allowed-ips 192.168.2.0/24
admin@spoke1:/config/interface/…/wg-peers/peer/hub/> set persistent-keepalive 25
admin@spoke1:/config/interface/…/wg-peers/peer/hub/> leave
admin@spoke1:/> configure
admin@spoke1:/config/> edit routing control-plane-protocol static name default
admin@spoke1:/config/routing/…/static/name/default/> set ipv4 route 192.168.0.0/24 wg0
admin@spoke1:/config/routing/…/static/name/default/> set ipv4 route 192.168.2.0/24 wg0
admin@spoke1:/config/routing/…/static/name/default/> leave
admin@spoke1:/>
```

This configuration allows Spoke 1 to reach both the hub network (192.168.0.0/24)
and Spoke 2's network (192.168.2.0/24) via the hub, enabling spoke-to-spoke
communication through the central hub.

## Persistent Keepalive

The `persistent-keepalive` setting sends periodic packets to keep the tunnel
active through NAT devices and firewalls:

```
admin@example:/config/interface/…/wg-peers/peer/hub/> set persistent-keepalive 25
```

This is particularly important when:

- The peer is behind NAT
- Intermediate firewalls have connection timeouts
- You need the tunnel to remain ready for bidirectional traffic

A value of 25 seconds is recommended for most scenarios.  Omit this setting
for peers with public static IPs that initiate connections.

> [!NOTE]
> Only the peer behind NAT needs `persistent-keepalive` configured.  The
> peer with a public IP learns the NAT endpoint from incoming packets.

## IPv6 Endpoints

WireGuard fully supports IPv6 for tunnel endpoints:

```
admin@example:/> configure
admin@example:/config/> edit interface wg0
admin@example:/config/interface/wg0/> set wireguard listen-port 51820
admin@example:/config/interface/wg0/> set wireguard private-key wg-key
admin@example:/config/interface/wg0/> set ipv4 address 10.0.0.1 prefix-length 24
admin@example:/config/interface/wg0/> set ipv6 address fd00::1 prefix-length 64
admin@example:/config/interface/wg0/> edit wireguard peers wg-peers peer remote
admin@example:/config/interface/…/wg-peers/peer/remote/> set endpoint 2001:db8::2
admin@example:/config/interface/…/wg-peers/peer/remote/> set endpoint-port 51820
admin@example:/config/interface/…/wg-peers/peer/remote/> set allowed-ips 10.0.0.2/32
admin@example:/config/interface/…/wg-peers/peer/remote/> set allowed-ips fd00::2/128
admin@example:/config/interface/…/wg-peers/peer/remote/> leave
admin@example:/>
```

WireGuard can carry both IPv4 and IPv6 traffic regardless of whether the
tunnel endpoints use IPv4 or IPv6.

## Monitoring WireGuard Status

Check WireGuard interface status and peer connections:

```
admin@example:/> show interfaces
wg0             wireguard  UP          2 peers (1 up)
                ipv4                   10.0.0.1/24 (static)
                ipv6                   fd00::1/64 (static)

admin@example:/> show interfaces wg0
name                : wg0
type                : wireguard
index               : 12
operational status  : up
peers               : 2

  Peer 1:
    status            : UP
    endpoint          : 203.0.113.2:51820
    latest handshake  : 2025-12-09T10:23:45+0000
    transfer tx       : 125648 bytes
    transfer rx       : 98432 bytes

  Peer 2:
    status            : DOWN
    endpoint          : 203.0.113.3:51820
    latest handshake  : 2025-12-09T09:15:22+0000
    transfer tx       : 45120 bytes
    transfer rx       : 32768 bytes
```

The connection status shows `UP` if a handshake occurred within the last 3
minutes, indicating an active tunnel.  The `latest handshake` timestamp shows
when the peers last successfully authenticated and exchanged keys.

## Post-Quantum Security (Preshared Keys)

WireGuard supports optional preshared keys (PSK) that add an extra layer of
symmetric encryption alongside Curve25519.  This provides defense-in-depth
against future quantum computers that might break elliptic curve cryptography.

PSKs protect your data from "harvest now, decrypt later" attacks - adversaries
recording traffic today would still need the PSK even if they break Curve25519
later.  However, peer authentication still relies on Curve25519, so PSKs don't
provide complete post-quantum security.

**Generate a preshared key using `wg genpsk`:**

```bash
admin@example:~$ wg genpsk > preshared.key
admin@example:~$ cat preshared.key
cO2DxZ2mUQ7LtsrDxZ2mUQ7LtsrDxZ2mUQ7LtsrDxZ2m=
```

**Import the preshared key into the keystore:**

```
admin@example:/> configure
admin@example:/config/> edit keystore symmetric-key wg-psk
admin@example:/config/keystore/symmetric-key/wg-psk/> set key-format wireguard-symmetric-key-format
admin@example:/config/keystore/symmetric-key/wg-psk/> set key cO2DxZ2mUQ7LtsrDxZ2mUQ7LtsrDxZ2mUQ7LtsrDxZ2m=
admin@example:/config/keystore/symmetric-key/wg-psk/> end
admin@example:/config/interface/wg0/> edit wireguard peers wg-peers peer remote
admin@example:/config/interface/…/wg-peers/peer/remote/> set preshared-key wg-psk
admin@example:/config/interface/…/wg-peers/peer/remote/> leave
admin@example:/>
```

The preshared key must be securely shared between both peers and configured
on both sides.

> [!IMPORTANT]
> Preshared keys must be kept secret and exchanged through a secure channel,
> just like passwords.  Delete the `preshared.key` file after importing it
> into both peer keystores.
