#!/bin/sh
# Migrate symmetric keys from v1.6 (v25.11) to IETF standard format.
#
# v1.6 had WiFi keys stored as:
#   - infix-keystore:cleartext-key  (type string, plaintext)
#   - infix-keystore:key-format     wifi-preshared-key-format
#
# v1.7 uses the IETF standard leaf and updated format names:
#   - cleartext-symmetric-key       (type binary, base64-encoded)
#   - key-format                    passphrase-key-format

file=$1
temp=${file}.tmp

jq '
if .["ietf-keystore:keystore"]?."symmetric-keys"?."symmetric-key" then
  .["ietf-keystore:keystore"]."symmetric-keys"."symmetric-key" |= map(
    if ."infix-keystore:key-format" then
      del(."infix-keystore:key-format") |
      . + { "key-format": "infix-crypto-types:passphrase-key-format" }
    else
      .
    end |

    if ."infix-keystore:cleartext-key" then
      ."infix-keystore:cleartext-key" as $val |
      del(."infix-keystore:cleartext-key") |
      . + { "cleartext-symmetric-key": ($val | @base64) }
    else
      .
    end
  )
else
  .
end
' "$file" > "$temp" && mv "$temp" "$file"
