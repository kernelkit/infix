#!/bin/sh
# Migrate self-signed HTTPS certificate from /cfg/ssl/ files into the
# ietf-keystore in startup-config.  Previously mkcert generated cert
# and key files on disk; now they are managed as a keystore entry
# called "gencert" alongside the SSH "genkey" entry.
#
# Also adds the "certificate": "gencert" leaf to the web container
# so nginx knows which keystore entry to use for TLS.
#
# After migration, /cfg/ssl/ is removed since cert/key are now stored
# in the keystore and written to /etc/ssl/ by confd at runtime.

file=$1
temp=${file}.tmp

LEGACY_DIR=/cfg/ssl
LEGACY_KEY=$LEGACY_DIR/private/self-signed.key
LEGACY_CRT=$LEGACY_DIR/certs/self-signed.crt

MKCERT_DIR=/tmp/ssl
MKCERT_KEY=$MKCERT_DIR/self-signed.key
MKCERT_CRT=$MKCERT_DIR/self-signed.crt

# Read PEM files, strip markers and newlines to get raw base64
read_pem() {
    grep -v -- '-----' "$1" | tr -d '\n'
}

if [ -f "$LEGACY_KEY" ] && [ -f "$LEGACY_CRT" ]; then
    priv_key=$(read_pem "$LEGACY_KEY")
    cert_data=$(read_pem "$LEGACY_CRT")
fi

# Fallback: generate a fresh certificate if legacy files were missing
# or unreadable, same as keystore.c does on first boot.
if [ -z "$priv_key" ] || [ -z "$cert_data" ]; then
    /usr/libexec/infix/mkcert
    if [ -f "$MKCERT_KEY" ] && [ -f "$MKCERT_CRT" ]; then
        priv_key=$(read_pem "$MKCERT_KEY")
        cert_data=$(read_pem "$MKCERT_CRT")
        rm -rf "$MKCERT_DIR"
    fi
fi

# If we still have no cert data, leave keys empty and let confd
# generate on boot via keystore_update().
if [ -z "$priv_key" ]; then
    priv_key=""
    cert_data=""
fi

jq --arg priv "$priv_key" --arg cert "$cert_data" '
# Add gencert entry to keystore if not already present
if .["ietf-keystore:keystore"]?."asymmetric-keys"?."asymmetric-key" then
  if (.["ietf-keystore:keystore"]."asymmetric-keys"."asymmetric-key" | map(select(.name == "gencert")) | length) == 0 then
    .["ietf-keystore:keystore"]."asymmetric-keys"."asymmetric-key" += [{
      "name": "gencert",
      "public-key-format": "infix-crypto-types:x509-public-key-format",
      "public-key": $cert,
      "private-key-format": "infix-crypto-types:rsa-private-key-format",
      "cleartext-private-key": $priv,
      "certificates": {
        "certificate": [{
          "name": "self-signed",
          "cert-data": $cert
        }]
      }
    }]
  else
    .
  end
else
  .
end |

# Add certificate reference to web container
if .["infix-services:web"] then
  if .["infix-services:web"].certificate then
    .
  else
    .["infix-services:web"].certificate = "gencert"
  end
else
  .
end
' "$file" > "$temp" && mv "$temp" "$file"

# Cert/key now live in the keystore, wipe the legacy on-disk copy
rm -rf "$LEGACY_DIR"
