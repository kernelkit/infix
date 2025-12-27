#!/bin/sh
# Rename cleartext-symmetric-key to symmetric-key

file=$1
temp=${file}.tmp

jq '
if .["ietf-keystore:keystore"]?."symmetric-keys"?."symmetric-key" then
  .["ietf-keystore:keystore"]."symmetric-keys"."symmetric-key" |= map(
    if ."key-type"?."cleartext-key" then
      # Rename cleartext-key to cleartext-symmetric-key
      ."key-type"."cleartext-key" as $key_value |
      ."key-type" |= (del(."cleartext-key") | . + {
        "symmetric-key": $key_value
      })
    else
      .
    end
  )
else
  .
end
' "$file" > "$temp" && mv "$temp" "$file"
