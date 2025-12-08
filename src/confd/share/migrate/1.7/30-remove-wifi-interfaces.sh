#!/bin/sh
# Remove interfaces of type infix-if-type:wifi
# Wi-Fi support has been refactored and all radio
# settings have been moved to ietf-hardware.
file=$1
temp=${file}.tmp

jq '
if .["ietf-interfaces:interfaces"]?.interface then
  .["ietf-interfaces:interfaces"].interface |= map(
    select(.type != "infix-if-type:wifi")
  )
else
  .
end
' "$file" > "$temp" && mv "$temp" "$file"
