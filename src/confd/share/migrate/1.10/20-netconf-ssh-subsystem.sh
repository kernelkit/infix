#!/bin/sh
# NETCONF is served by the SSH daemon as a subsystem and ietf-netconf-server
# is gone.  NETCONF is on by default, so only a configuration without a
# NETCONF endpoint needs ssh/netconf/enabled set to false.

file=$1
temp=${file}.tmp

jq '
(.["ietf-netconf-server:netconf-server"]?.listen?.endpoints?.endpoint // [] | length > 0) as $netconf |
del(.["ietf-netconf-server:netconf-server"]) |
if $netconf then
  .
else
  .["infix-services:ssh"].netconf.enabled = false
end
' "$file" > "$temp" && mv "$temp" "$file"
