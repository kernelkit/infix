#!/bin/sh
# Migrate keystore symmetric key syntax to new YANG schema (RFC 9643)
# The 'cleartext-key' leaf has been renamed to 'cleartext-symmetric-key'
# Old: symmetric-key[]/key-type/cleartext-key
# New: symmetric-key[]/key-type/cleartext-symmetric-key

file=$1
temp=${file}.tmp

jq '
if .["ietf-keystore:keystore"]?."symmetric-keys"?."symmetric-key" then
  .["ietf-keystore:keystore"]."symmetric-keys"."symmetric-key" |= map(
    if ."key-type"?."cleartext-key" then
      # Rename cleartext-key to cleartext-symmetric-key
      ."key-type"."cleartext-key" as $key_value |
      ."key-type" |= (del(."cleartext-key") | . + {
        "cleartext-symmetric-key": $key_value
      })
    else
      .
    end
  )
else
  .
end
' "$file" > "$temp" && mv "$temp" "$file"
