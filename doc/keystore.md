# Keystore

The Infix keystore is a centralized storage system for cryptographic keys
used throughout the system.  It is based on the IETF standards [RFC 9641][1]
(Keystore) and [RFC 9640][2] (Cryptographic Types), with Infix extensions
for WiFi and WireGuard key formats.

## Overview

The keystore supports two types of cryptographic keys:

1. **Asymmetric Keys** — public/private key pairs used for:
    - SSH host authentication (RSA keys)
    - WireGuard VPN tunnels (X25519 keys)

2. **Symmetric Keys** — shared secrets used for:
    - WiFi authentication (WPA2/WPA3 pre-shared keys)
    - WireGuard VPN pre-shared keys

All keys are stored under the `ietf-keystore` configuration path and can be
managed via CLI, NETCONF, or RESTCONF.

### Supported Formats

| Asymmetric Key Format                                    | Use Case      | Key Type   |
|----------------------------------------------------------|---------------|------------|
| `rsa-private-key-format` / `ssh-public-key-format`       | SSH host keys | RSA        |
| `x25519-private-key-format` / `x25519-public-key-format` | WireGuard VPN | Curve25519 |

| Symmetric Key format             | Use Case       | Key Length             |
|----------------------------------|----------------|------------------------|
| `wifi-preshared-key-format`      | WiFi WPA2/WPA3 | 8-63 characters        |
| `wireguard-symmetric-key-format` | WireGuard PSK  | 44 characters (base64) |

## Asymmetric Keys

Asymmetric keys consist of a public/private key pair.  The public key can be
shared freely, while the private key must be kept secure.

### SSH Host Keys

SSH host keys identify the system during SSH and NETCONF connections.  The
default host key is automatically generated on first boot and stored in the
keystore with the name `genkey`.

See [SSH Management](management.md) for details on generating and importing
custom SSH host keys.

### WireGuard Keys

WireGuard uses X25519 elliptic curve cryptography for key exchange.  Each
WireGuard interface requires a public/private key pair stored as an asymmetric
key in the keystore.

See [WireGuard VPN](vpn-wireguard.md) for key generation and configuration
examples.

## Symmetric Keys

Symmetric keys are shared secrets where the same key must be configured on
all systems that need to communicate.

### WiFi Pre-Shared Keys

WiFi networks secured with WPA2 or WPA3 use pre-shared keys stored as
symmetric keys in the keystore.  The key must be 8-63 printable ASCII
characters.

See [WiFi](wifi.md) for complete configuration examples.

### WireGuard Pre-Shared Keys

WireGuard supports optional pre-shared keys (PSK) that add a layer of
symmetric encryption alongside Curve25519.  This provides defense-in-depth
against future quantum computers that might break elliptic curve cryptography.
Note, however, that WireGuard’s authentication and initial key agreement
remain Curve25519-based, so PSKs only protect the session encryption,
not the handshake itself.

See [WireGuard VPN](vpn-wireguard.md) for PSK generation and usage examples.

## Viewing Keys

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>show keystore</b>
asymmetric-keys {
  asymmetric-key genkey {
    public-key-format ssh-public-key-format;
    public-key MIIBCgKCAQEAm6uCENSafz7mIfIJ8O.... AQAB;
    private-key-format rsa-private-key-format;
    cleartext-private-key MIIEvAIBADANBgkqhkiG9w0BAQEFAASCBKYw...b7dyPr4mpHg==;
  }
}
</code></pre>

> [!WARNING]
> The `show keystore` command displays private keys in cleartext.  Be careful
> when viewing keys on shared screens or in logged sessions.

To list only asymmetric or symmetric keys:

<pre class="cli"><code>admin@example:/config/> <b>show keystore asymmetric-keys</b>
admin@example:/config/> <b>show keystore symmetric-keys</b>
</code></pre>

## Deleting Keys

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>delete keystore asymmetric-key mykey</b>
admin@example:/config/> <b>leave</b>
</code></pre>

> [!CAUTION]
> Deleting a key that is referenced by a service (SSH, WireGuard, WiFi) will
> cause that service to fail.  Verify the key is not in use before deletion.

## Security Considerations

The keystore is protected by NACM (Network Access Control Model) rules.
Only users in the `admin` group can view or modify cryptographic keys.
See [NACM](nacm.md) for details on access control.

Private keys are stored in cleartext in the configuration database.
Configuration files and backups containing the keystore should be treated
as sensitive and protected accordingly.

### Key Validation

The keystore validates symmetric keys based on their declared format:

- **WiFi PSK**: Must be 8-63 characters
- **WireGuard PSK**: Must be exactly 44 characters (base64-encoded)

Invalid keys are rejected at configuration time.

## References

- [RFC 9641 - A YANG Data Model for a Keystore][1]
- [RFC 9640 - YANG Data Types and Groupings for Cryptography][2]
- [WiFi Documentation](wifi.md)
- [WireGuard VPN Documentation](vpn-wireguard.md)
- [SSH Management](management.md)
- [NACM Access Control](nacm.md)

[1]: https://datatracker.ietf.org/doc/html/rfc9641
[2]: https://datatracker.ietf.org/doc/html/rfc9640
