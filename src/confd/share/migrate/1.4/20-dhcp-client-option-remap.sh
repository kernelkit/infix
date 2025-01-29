#!/bin/sh
# Remap DHCP client options to match DHCP server nomenclature.

file=$1
temp=${file}.tmp

jq '(
    .["infix-dhcp-client:dhcp-client"]?."client-if"[]?.option[]? |=
    if has("id") then
        .id = ({
            "subnet"         : "netmask",
            "dns"            : "dns-server",
            "ntpsrv"         : "ntp-server",
            "clientid"       : "client-id",
            "staticroutes"   : "classless-static-route",
            "msstaticroutes" : "ms-classless-static-route"
        }[.id] // .id)
    else
        .
    end
)' "$file" > "$temp" &&
    mv "$temp" "$file"
