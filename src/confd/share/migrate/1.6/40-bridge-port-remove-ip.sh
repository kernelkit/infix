#!/bin/sh
# Remove ietf-ip:ipv4 and ietf-ip:ipv6 from bridge port interfaces.
# Bridge ports should not have IP addresses; the IP address should
# be configured on the bridge interface itself.
file=$1
temp=${file}.tmp

jq '
if .["ietf-interfaces:interfaces"]?.interface then
  .["ietf-interfaces:interfaces"].interface |= map(
    if .["infix-interfaces:bridge-port"] and .type != "infix-if-type:bridge" then
      del(.["ietf-ip:ipv4"], .["ietf-ip:ipv6"])
    else
      .
    end
  )
else
  .
end
' "$file" > "$temp" && mv "$temp" "$file"
