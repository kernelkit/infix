#!/bin/sh
# The ietf-ospf and ietf-rip augments moved from infix-routing to
# infix-ospf and infix-rip.

file=$1
temp=${file}.tmp

jq '
  def moved($mod): walk(if type == "object"
                        then with_entries(.key |= sub("^infix-routing:"; $mod + ":"))
                        else . end);
  if .["ietf-routing:routing"]?."control-plane-protocols"?."control-plane-protocol" then
    .["ietf-routing:routing"]."control-plane-protocols"."control-plane-protocol" |=
      map(if .type == "infix-routing:ospfv2" or .type == "infix-routing:ospfv3"
          then moved("infix-ospf")
          elif .type == "infix-routing:ripv2" or .type == "infix-routing:ripng"
          then moved("infix-rip")
          else . end)
  else . end
' "$file" > "$temp" && mv "$temp" "$file"
