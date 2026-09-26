#!/bin/sh
# ietf-hardware component names are now restricted to a safe character set:
# letters, digits, '_', '.', '-' and '@', not starting with '.' or '-'.
# Sanitize any configured component name that would no longer validate
# and update the leaves that reference it: the Wi-Fi radio of an interface
# and the GPS receiver of an NTP reference-clock source.  Probed names
# (radioN, gpsN) never match, so this is a safety net for hand-edited
# configs.

file=$1
temp=${file}.tmp

# Everything the identifier type does not allow.
bad='[^a-zA-Z0-9_.@-]'

jq --arg bad "$bad" '
    ["ietf-hardware:hardware", "component"] as $hw
  | if getpath($hw) == null then . else
      ( [ getpath($hw)[]
          | (.name | gsub($bad; "") | sub("^[.-]+"; "")) as $new
          | select($new != .name and $new != "")
          | { key: .name, value: $new } ]
        | from_entries ) as $ren
      | if ($ren | length) == 0 then . else
          setpath($hw; getpath($hw)
            | map(if $ren[.name] != null then .name = $ren[.name] else . end))
        | ["ietf-interfaces:interfaces", "interface"] as $ifs
        | (if getpath($ifs) != null then
             setpath($ifs; getpath($ifs) | map(
               ((.["infix-interfaces:wifi"] // {}) | .radio // null) as $r
               | if $r != null and $ren[$r] != null
                 then .["infix-interfaces:wifi"].radio = $ren[$r]
                 else . end))
           else . end)
        | ["ietf-ntp:ntp", "refclock-master", "infix-ntp:source"] as $src
        | (if getpath($src) != null then
             setpath($src; getpath($src) | map(
               if $ren[.receiver] != null then .receiver = $ren[.receiver] else . end))
           else . end)
        end
    end
' "$file" > "$temp" && mv "$temp" "$file"
