#!/bin/sh
# Migrate phys-address -> custom-phys-address with static option

file=$1
temp=${file}.tmp

jq '(
    .["ietf-interfaces:interfaces"].interface[] |=
    if has("phys-address") then
        .["infix-interfaces:custom-phys-address"] = { "static": .["phys-address"] } | del(.["phys-address"])
    else
        .
    end
)' "$file" > "$temp" &&
mv "$temp" "$file"
