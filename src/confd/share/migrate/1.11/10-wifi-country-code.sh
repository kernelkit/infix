#!/bin/sh
# Move the WiFi country code from each radio to the box-wide
# hardware/wifi/country-code leaf.
#
# The regulatory domain is one setting in the kernel, so a code per radio
# was misleading, and it was only ever applied once the radio had an
# interface.  The first radio naming a real country wins.  A radio left
# at the world domain ("00") sets nothing, that is the default without
# the leaf.
#
# The leaf only takes the codes the regulatory database knows.  The old
# per-radio leaf accepted any ISO code, and a code the database lacks
# never did anything, so such a code becomes the world domain here.

file=$1
temp=${file}.tmp
yang=${WIFI_COUNTRY_CODES_YANG:-$(ls /usr/share/yang/modules/confd/infix-wifi-country-codes@*.yang 2>/dev/null | tail -1)}

codes=$(jq -r '[ (.["ietf-hardware:hardware"].component // [])[]
                 | .["infix-hardware:wifi-radio"]?["country-code"]? // empty
                 | select(. != "00") ] | join(" ")' "$file")

# The radios could disagree, the kernel has only one domain, so whichever
# daemon started last won.  Say which one the migration keeps.
case $(echo "$codes" | tr ' ' '\n' | sort -u | wc -l) in
    0|1) ;;
    *)
        logger -t migrate -p user.warning \
            "$file: radios have different country codes ($codes), keeping the first radio's"
        ;;
esac

cc=
for code in $codes; do
    if [ -n "$yang" ] && ! grep -q "enum \"$code\"" "$yang"; then
        logger -t migrate -p user.warning \
            "$file: WiFi country code $code is not in the regulatory database, using the world domain"
        continue
    fi
    cc=$code
    break
done

jq --arg cc "$cc" '
  if $cc != "" then
    .["ietf-hardware:hardware"]["infix-hardware:wifi"] = {"country-code": $cc}
  else . end
  | if .["ietf-hardware:hardware"].component then
      .["ietf-hardware:hardware"].component |=
          [ .[] | del(.["infix-hardware:wifi-radio"]["country-code"]) ]
    else . end
' "$file" > "$temp" && mv "$temp" "$file"
