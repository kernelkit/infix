#!/bin/sh
# The port's qos egress rate-limit is now qos egress-shaper.

file=$1
temp=${file}.tmp

jq '
  if .["ietf-interfaces:interfaces"]?.interface then
    .["ietf-interfaces:interfaces"].interface |= map(
      if .["infix-interfaces:qos"]?.egress?["rate-limit"] then
        .["infix-interfaces:qos"]["egress-shaper"] = .["infix-interfaces:qos"].egress["rate-limit"]
        | del(.["infix-interfaces:qos"].egress["rate-limit"])
        | if .["infix-interfaces:qos"].egress == {} then del(.["infix-interfaces:qos"].egress) else . end
      else . end)
  else . end
' "$file" > "$temp" && mv "$temp" "$file"
