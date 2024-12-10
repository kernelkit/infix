# SSH Management

The default SSH hostkey is generated on first boot and is used in both
SSH and NETCONF (SSH transport). Custom keys can be added to the
configuration in `ietf-keystore`. The ony suuported hostkey type is
RSA for now, so the private must be `ietf-crypto-types:rsa-private-key-format` and the public key
`ietf-crypto-types:ssh-public-key-format`

## Use your own SSH hostkeys

Hostkeys can be generated with OpenSSL:
```bash
openssl genpkey -quiet -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -outform PEM > mykey
openssl rsa -RSAPublicKey_out < mykey > mykey.pyb
```
Store the keys in `ietf-keystore` _without_ the header and footer information
created by OpenSSL.

After the key has been stored in the keystore and given the name
_mykey_ it can be added to SSH configuration:

	admin@example:/> configure
	admin@example:/config/> edit ssh
	admin@example:/config/ssh/> set hostkey mykey
