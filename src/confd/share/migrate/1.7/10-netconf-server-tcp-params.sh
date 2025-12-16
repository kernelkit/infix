#!/bin/sh
# Migrate NETCONF server TCP parameters to new YANG schema (RFC 9643)
# The 'local-address' leaf has been moved into a 'local-bind' list
# Old: tcp-server-parameters/local-address
# New: tcp-server-parameters/local-bind[]/local-address

file=$1
temp=${file}.tmp

jq '
if .["ietf-netconf-server:netconf-server"]?.listen?.endpoints?.endpoint then
  .["ietf-netconf-server:netconf-server"].listen.endpoints.endpoint |= map(
    if .ssh?."tcp-server-parameters"?."local-address" then
      # Extract and remove the old local-address value, then add new structure
      .ssh."tcp-server-parameters"."local-address" as $addr |
      .ssh."tcp-server-parameters" |= (del(."local-address") | . + {
        "local-bind": [{
          "local-address": $addr
        }]
      })
    else
      .
    end
  )
else
  .
end
' "$file" > "$temp" && mv "$temp" "$file"
