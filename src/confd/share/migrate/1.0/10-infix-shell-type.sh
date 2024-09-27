#!/bin/sh
# Migrate infix-shell-type:bash -> infix-system:bash

file=$1
temp=${file}.tmp

if jq -e '.["ietf-system:system"]?.authentication?.user? | length > 0' "$file" > /dev/null 2>&1; then
    jq '.["ietf-system:system"].authentication.user |= map(.["infix-system:shell"] |= gsub("infix-shell-type:"; ""))' "$file" > "$temp"
    mv "$temp" "$file"
fi
