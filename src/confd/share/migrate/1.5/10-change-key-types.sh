#!/bin/sh
# Rename SSH key type in keystore, from ietf-crypto-types to infix-crypto-types
#

file=$1
temp=${file}.tmp

jq '.["ietf-keystore:keystore"]["asymmetric-keys"]["asymmetric-key"][] |= (
  .["public-key-format"] |= sub("ietf-crypto-types";"infix-crypto-types") |
  .["private-key-format"] |= sub("ietf-crypto-types";"infix-crypto-types"))' "$file" > "$temp" &&
    mv "$temp" "$file"

