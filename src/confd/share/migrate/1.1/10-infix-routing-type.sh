#!/bin/sh
# migrate ietf-routing-type => infix-routing-type

migrate_routing_type()
{
    file=$1
    match=$2
    replace=$3

    if jq -e '.["ietf-routing:routing"]?."control-plane-protocols"?."control-plane-protocol"?[] | length > 0' "$file" > /dev/null 2>&1; then
        jq --arg match "${match}" --arg replace "${replace}" '
            (.["ietf-routing:routing"]."control-plane-protocols"."control-plane-protocol"[] |
            select(.type == $match).type) |= $replace' "${file}" > "${file}.tmp"
	mv "${file}.tmp" "${file}"
    fi
}

migrate_routing_type "$1" "ietf-ospf:ospfv2" "infix-routing:ospfv2"
migrate_routing_type "$1" "static"           "infix-routing:static"
