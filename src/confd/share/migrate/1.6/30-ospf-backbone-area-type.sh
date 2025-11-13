#!/bin/sh
# Remove area-type from OSPF backbone area (0.0.0.0)
# The backbone area cannot be configured as stub or NSSA per RFC 2328/3101
# Silently drop any area-type setting for area 0.0.0.0

file=$1
temp=${file}.tmp

jq '
if .["ietf-routing:routing"]?."control-plane-protocols"?."control-plane-protocol" then
  .["ietf-routing:routing"]."control-plane-protocols"."control-plane-protocol" |= map(
    if .["ietf-ospf:ospf"]?.areas?.area then
      .["ietf-ospf:ospf"].areas.area |= map(
        if (.["area-id"] == "0.0.0.0") and .["area-type"] then
          # Remove area-type from backbone area
          del(.["area-type"])
        else
          .
        end
      )
    else
      .
    end
  )
else
  .
end
' "$file" > "$temp" && mv "$temp" "$file"
