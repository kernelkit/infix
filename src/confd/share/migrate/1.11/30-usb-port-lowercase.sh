#!/bin/sh
# USB ports are named usb, usb1, usb2 ..., lowercase like every other
# hardware component, and a port is one component whatever the number
# of root hubs behind it.  Before, each root hub was a component, the
# second one named after the first with a counter: USB and USB2, or
# USB1 and USB12.  Fold such an entry into its port, locked wins.
#
# On the device the port names come from the probe, off the device, as
# in make migrate-configs, the other configured names are the ports.

file=$1
temp=${file}.tmp
sysjson=${SYSTEM_JSON:-/run/system.json}

ports=$(jq -c '[ .["usb-ports"][]?.name ]' "$sysjson" 2>/dev/null)
[ -n "$ports" ] || ports='[]'

defs='
  def usb: .class == "infix-hardware:usb";
  def names: [ (.["ietf-hardware:hardware"].component // [])[] | select(usb) | .name | ascii_downcase ];
  def port_of($n; $cands):
    if ($ports | index($n)) then $n
    else ([ $cands[] | select(. != $n) | select(. as $p | $n | startswith($p) and (.[($p | length):] | test("^[0-9]+$"))) ]
          | sort_by(-length) | first // $n)
    end;
  def cands: if ($ports | length) > 0 then $ports else names end;
'

log()
{
    command -v logger >/dev/null && logger -t migrate -p "user.$1" "$file: $2"
}

jq -r --argjson ports "$ports" "$defs"'
  cands as $c | names[] | select(port_of(.; $c) != .) | "\(.) \(port_of(.; $c))"
' "$file" 2>/dev/null | while read -r alias port; do
    log notice "USB port $alias folded into $port"
done

if ! jq --argjson ports "$ports" "$defs"'
  def locked: (.state["admin-state"] // "locked") != "unlocked";
  if .["ietf-hardware:hardware"].component then
    cands as $c
    | .["ietf-hardware:hardware"].component |= reduce .[] as $comp ([];
        if ($comp | usb | not) then . + [$comp]
        else ($comp | .name |= ascii_downcase | .name = port_of(.name; $c)) as $u
          | (to_entries | map(select(.value | usb and .name == $u.name)) | first | .key) as $i
          | if $i == null then . + [$u]
            else .[$i].state["admin-state"] =
                   (if (.[$i] | locked) or ($u | locked) then "locked" else "unlocked" end)
            end
        end)
  else . end
' "$file" > "$temp" 2>/dev/null; then
    # Never leave the ports under their old names, confd would ignore them
    log warning "folding USB ports failed, renaming only"
    jq '
      if .["ietf-hardware:hardware"].component then
        .["ietf-hardware:hardware"].component |=
          map(if .class == "infix-hardware:usb" then .name |= ascii_downcase else . end)
      else . end
    ' "$file" > "$temp"
fi
mv "$temp" "$file"
