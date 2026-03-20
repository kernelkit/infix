# Keystore

The Infix keystore is a centralized storage system for cryptographic keys
used throughout the system.  It is based on the IETF standards [RFC 9641][1]
(Keystore) and [RFC 9640][2] (Cryptographic Types), with Infix extensions
for WiFi and WireGuard key formats.

## Overview

The keystore supports two types of cryptographic keys:

1. **Asymmetric Keys** — public/private key pairs used for:
    - SSH host authentication (RSA keys)
    - HTTPS/TLS certificates (X.509 keys)
    - WireGuard VPN tunnels (X25519 keys)

2. **Symmetric Keys** — shared secrets used for:
    - WiFi authentication (WPA2/WPA3 pre-shared keys)
    - WireGuard VPN pre-shared keys

All keys are stored under the `ietf-keystore` configuration path and can be
managed via CLI, NETCONF, or RESTCONF.

### Supported Formats

| **Asymmetric Key Format**                                | **Use Case**       | **Key Type** |
|----------------------------------------------------------|--------------------|--------------|
| `rsa-private-key-format` / `ssh-public-key-format`       | SSH host keys      | RSA          |
| `rsa-private-key-format` / `x509-public-key-format`      | TLS certificates   | RSA + X.509  |
| `x25519-private-key-format` / `x25519-public-key-format` | WireGuard VPN      | Curve25519   |

| **Symmetric Key Format**        | **Use Case**                          |
|-----------------------------|-----------------------------------|
| `passphrase-key-format`     | Human-readable passphrases (WiFi) |
| `octet-string-key-format`   | Raw symmetric keys (WireGuard)    |

## Asymmetric Keys

Asymmetric keys consist of a public/private key pair.  The public key can be
shared freely, while the private key must be kept secure.

### SSH Host Keys

SSH host keys identify the system during SSH and NETCONF connections.  The
default host key is automatically generated on first boot and stored in the
keystore with the name `genkey`.

See [SSH Management](management.md) for details on generating and importing
custom SSH host keys.

### TLS Certificates

TLS certificates are used by the web server (nginx) for HTTPS connections.
The default certificate is a self-signed certificate automatically generated
on first boot and stored in the keystore with the name `gencert`.  Like SSH
host keys, the certificate is regenerated on factory reset when its keys are
empty.

The web server's `certificate` leaf references which keystore entry to use:

```json
"infix-services:web": {
    "certificate": "gencert",
    "enabled": true
}
```

To use a custom (e.g., CA-signed) certificate, create a new asymmetric key
entry with `x509-public-key-format`, populate it with your certificate and
private key, then point the web `certificate` leaf to it:

```json
"ietf-keystore:keystore": {
    "asymmetric-keys": {
        "asymmetric-key": [
            {
                "name": "my-cert",
                "public-key-format": "infix-crypto-types:x509-public-key-format",
                "public-key": "<base64-encoded-cert>",
                "private-key-format": "infix-crypto-types:rsa-private-key-format",
                "cleartext-private-key": "<base64-encoded-key>",
                "certificates": {
                    "certificate": [
                        { "name": "ca-signed", "cert-data": "<base64-encoded-cert>" }
                    ]
                }
            }
        ]
    }
}
```

> [!NOTE]
> The `public-key` and `cert-data` fields contain base64-encoded PEM data
> with the `-----BEGIN/END-----` markers stripped.  The system reconstructs
> the PEM files when writing them to disk for nginx.

### WireGuard Keys

WireGuard uses X25519 elliptic curve cryptography for key exchange.  Each
WireGuard interface requires a public/private key pair stored as an asymmetric
key in the keystore.  Key pairs can be generated directly from the CLI:

<pre class="cli"><code>admin@example:/> <b>wireguard genkey</b>
Private: aMqBvZqkSP5JrqBvZqkSP5JrqBvZqkSP5JrqBvZqkSP=
Public:  bN1CwZ1lTP6KsrCwZ1lTP6KsrCwZ1lTP6KsrCwZ1lTP=
</code></pre>

See [WireGuard VPN](vpn-wireguard.md) for key generation and configuration
examples.

## Symmetric Keys

Symmetric keys are shared secrets where the same key must be configured on
all systems that need to communicate.

### WiFi Pre-Shared Keys

WiFi networks secured with WPA2 or WPA3 use pre-shared keys stored as
symmetric keys in the keystore with `passphrase-key-format`.  The
passphrase must be 8-63 printable ASCII characters.

Since symmetric keys are stored as binary (base64-encoded), the CLI
provides the `change` command to enter passphrases interactively:

<pre class="cli"><code>admin@example:/config/keystore/…/my-wifi-key/> <b>change cleartext-symmetric-key</b>
Passphrase: ************
Retype passphrase: ************
</code></pre>

See [WiFi](wifi.md) for complete configuration examples.

### WireGuard Pre-Shared Keys

WireGuard supports optional pre-shared keys (PSK) that add a layer of
symmetric encryption alongside Curve25519.  PSKs use the standard IETF
`octet-string-key-format` (32 random bytes).  This provides defense-in-depth
against future quantum computers that might break elliptic curve cryptography.
Note, however, that WireGuard’s authentication and initial key agreement
remain Curve25519-based, so PSKs only protect the session encryption,
not the handshake itself.

PSKs can be generated directly from the CLI:

<pre class="cli"><code>admin@example:/> <b>wireguard genpsk</b>
cO2DxZ2mUQ7LtsrDxZ2mUQ7LtsrDxZ2mUQ7LtsrDxZ2m=
</code></pre>

See [WireGuard VPN](vpn-wireguard.md) for PSK generation and usage examples.

## Viewing Keys

The `show keystore` command in admin-exec mode gives an overview of all
keys in the keystore.  Passphrases (WiFi passwords) are decoded and shown
in cleartext, while binary keys (WireGuard PSKs) are shown as base64:

<pre class="cli"><code>admin@example:/> <b>show keystore</b>
────────────────────────────────────────────────────────────────────────
<span class="title">Symmetric Keys</span>
<span class="header">NAME                         FORMAT        VALUE                        </span>
my-wifi-key                  passphrase    MySecretPassword
wg-psk                       octet-string  zYr83O4Ykj9i1gN+/aaosJxQx...

────────────────────────────────────────────────────────────────────────
<span class="title">Asymmetric Keys</span>
<span class="header">NAME                         TYPE    PUBLIC KEY                         </span>
genkey                       rsa     MIIBCgKCAQEAnj0YinjhYDgYbEGuh7...
gencert                      x509    MIIDXTCCAkWgAwIBAgIJAJC1HiIAZA...
wg-tunnel                    x25519  bN1CwZ1lTP6KsrCwZ1lTP6KsrCwZ1...
</code></pre>

To see the full (untruncated) details of a specific key, use the
`symmetric` or `asymmetric` qualifier with the key name:

<pre class="cli"><code>admin@example:/> <b>show keystore symmetric my-wifi-key</b>
name                : my-wifi-key
format              : passphrase
value               : MySecretPassword

admin@example:/> <b>show keystore asymmetric genkey</b>
name                : genkey
algorithm           : rsa
public key format   : ssh-public-key
public key          : MIIBCgKCAQEAnj0YinjhY...full key...IDAQAB
</code></pre>

> [!NOTE]
> The `show keystore` command is protected by NACM.  Only users in the
> `admin` group can view keystore data.  Operator-level users will see a
> message indicating that no keystore data is available.

The full configuration-mode view (including private keys) is still
available via `configure` and then `show keystore`:

<pre class="cli"><code>admin@example:/config/> <b>show keystore</b>
</code></pre>

> [!WARNING]
> The configuration-mode `show keystore` displays private keys in
> cleartext.  Be careful when viewing keys on shared screens or in
> logged sessions.  The admin-exec `show keystore` command never
> displays private keys.

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

Symmetric key values are stored as binary (base64-encoded).  The system
validates them based on their declared format:

- `passphrase-key-format`:  Used by WiFi, must decode to 8-63 ASCII characters
- `octet-string-key-format`: Used by Wireguard, must decode to exactly 32 bytes (256 bits)

## References

- [RFC 9641 - A YANG Data Model for a Keystore][1]
- [RFC 9640 - YANG Data Types and Groupings for Cryptography][2]
- [WiFi Documentation](wifi.md)
- [WireGuard VPN Documentation](vpn-wireguard.md)
- [SSH Management](management.md)
- [NACM Access Control](nacm.md)

[1]: https://datatracker.ietf.org/doc/html/rfc9641
[2]: https://datatracker.ietf.org/doc/html/rfc9640
