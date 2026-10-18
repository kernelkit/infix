#!/bin/sh
# NETCONF is served by the SSH daemon as a subsystem and ietf-netconf-server
# is gone.  NETCONF is on by default, so only a configuration without a
# NETCONF endpoint needs ssh/netconf/enabled set to false.  Also run for
# 1.11, a configuration without ietf-netconf-server is left as is.

file=$1
temp=${file}.tmp

jq '
if has("ietf-netconf-server:netconf-server") | not then
  .
else
  (.["ietf-netconf-server:netconf-server"]?.listen?.endpoints?.endpoint // [] | length > 0) as $netconf |
  del(.["ietf-netconf-server:netconf-server"]) |
  if $netconf then
    .
  else
    .["infix-services:ssh"].netconf.enabled = false
  end
end
' "$file" > "$temp" && mv "$temp" "$file"
