#!/bin/sh
# Migrate infix-shell-type:bash -> infix-system:bash

file=$1
temp=$1.tmp

jq '.["ietf-system:system"].authentication.user |= map(.["infix-system:shell"] |= gsub("infix-shell-type:"; ""))' "$file" > "$temp"
mv "$temp" "$file"

