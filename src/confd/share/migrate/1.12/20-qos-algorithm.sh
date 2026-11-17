#!/bin/sh
# The transmission selection algorithms are now identities of infix-qos.

file=$1
temp=${file}.tmp

jq '
  (.["ietf-interfaces:interfaces"]?.interface[]?["infix-interfaces:qos"],
   .["ietf-system:system"]?["infix-qos:qos-profile"][]?
   | .egress?["traffic-class"][]?.algorithm | strings)
  |= sub("^(ieee802-dot1q-types:)?(?<a>strict-priority|credit-based-shaper|enhanced-transmission-selection)$"; "infix-qos:\(.a)")
' "$file" > "$temp" && mv "$temp" "$file"
