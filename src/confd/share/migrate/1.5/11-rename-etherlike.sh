#!/bin/sh
# Rename interface type etherlike => ethernet
#

file=$1
temp=${file}.tmp

jq '(.["ietf-interfaces:interfaces"].interface[] | select(.type == "infix-if-type:etherlike") .type) |= "infix-if-type:ethernet"'  "$file" > "$temp" &&
    mv "$temp" "$file"
