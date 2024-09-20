#!/bin/sh
# migrate ietf-routing-type => infix-routing-type
file=$1
temp=$1.tmp

jq '(.["ietf-routing:routing"]."control-plane-protocols"."control-plane-protocol"[] | select(.type == "ietf-ospf:ospfv2").type) |= "infix-routing:ospfv2"'  "$file" > "$temp"
jq '(.["ietf-routing:routing"]."control-plane-protocols"."control-plane-protocol"[] | select(.type == "static").type) |= "infix-routing:static"' "$temp" > "$file"
