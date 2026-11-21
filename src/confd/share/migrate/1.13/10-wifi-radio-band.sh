#!/bin/sh
# A Wi-Fi radio without a band no longer takes a channel, channel-width
# or legacy-rates.  They had no effect there, drop them.

file=$1
temp=${file}.tmp

jq '
  if .["ietf-hardware:hardware"].component then
    .["ietf-hardware:hardware"].component |= map(
      if has("infix-hardware:wifi-radio") and (.["infix-hardware:wifi-radio"] | has("band") | not) then
        .["infix-hardware:wifi-radio"] |= del(.channel, .["channel-width"], .["legacy-rates"])
      else . end)
  else . end
' "$file" > "$temp" && mv "$temp" "$file"
