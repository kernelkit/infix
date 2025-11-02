#!/bin/sh
# Migrate IPv4 autoconf from enabled leaf to presence container
# Remove the "enabled" leaf, keeping the container only if it was enabled

file=$1
temp=${file}.tmp

jq '
if .["ietf-interfaces:interfaces"] then
  .["ietf-interfaces:interfaces"].interface |= map(
    if .["ietf-ip:ipv4"]?."infix-ip:autoconf" then
      if .["ietf-ip:ipv4"]."infix-ip:autoconf".enabled == false then
        # Remove autoconf container if it was disabled
        .["ietf-ip:ipv4"] |= del(."infix-ip:autoconf")
      else
        # Keep autoconf but remove the enabled leaf
        .["ietf-ip:ipv4"]."infix-ip:autoconf" |= del(.enabled)
      end
    else
      .
    end
  )
else
  .
end
' "$file" > "$temp" && mv "$temp" "$file"
