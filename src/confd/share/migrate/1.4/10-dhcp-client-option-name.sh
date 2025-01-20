#!/bin/sh
# Rename DHCP client option attribute 'name' -> 'id' to make DHCP server.

file=$1
temp=${file}.tmp

jq '(
    .["infix-dhcp-client:dhcp-client"]?."client-if"[]?.option[]? |=
    if has("name") then
        { "id": .name } + . | del(.name)
    else
        .
    end
)' "$file" > "$temp" &&
    mv "$temp" "$file"
