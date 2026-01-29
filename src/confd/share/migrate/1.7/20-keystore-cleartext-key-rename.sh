#!/bin/sh
# Rename cleartext-key to symmetric-key and wrap in key-type container

file=$1
temp=${file}.tmp

jq '
if .["ietf-keystore:keystore"]?."symmetric-keys"?."symmetric-key" then
  .["ietf-keystore:keystore"]."symmetric-keys"."symmetric-key" |= map(
    if ."infix-keystore:cleartext-key" then
      # Rename cleartext-key to symmetric-key and wrap in key-type
      ."infix-keystore:cleartext-key" as $key_value |
      del(."infix-keystore:cleartext-key") | . + {
        "key-type": {
          "infix-keystore:symmetric-key": $key_value
        }
      }
    else
      .
    end
  )
else
  .
end
' "$file" > "$temp" && mv "$temp" "$file"
