#!/bin/sh
# Migrate infix-modem:modems config to ietf-hardware/ietf-interfaces.
#
# Old model (infix-modem:modems/modem[index]):
#   enabled, sim, pin, puk, carrier, preferred-mode, allowed-mode[],
#   band[], location{}, bearer[]{apn, ip-type, allow-roaming, username, password}
#
# New model:
#   ietf-hardware:hardware/component[modemN]  -- class infix-hardware:modem
#   ietf-hardware:hardware/component[simN]    -- class infix-hardware:sim
#   ietf-interfaces:interfaces/interface[wwanN] -- type infix-if-type:modem
#
# Dropped (no equivalent in new model):
#   bearer.apn-type, bearer.firewall-enabled, bearer.dns-enabled, bearer.default-route
#
# Credentials: plaintext password is moved to the keystore (base64-encoded)
# and referenced by name from bearer.authentication.password.

file=$1
temp=${file}.tmp

jq '
if ."infix-modem:modems" then
  ."infix-modem:modems".modem as $modems |

  # Flat list of all bearers across all modems, in order, with a global wwan index.
  ([ $modems[] | . as $m | (.bearer // [])[] | {modem: $m, bearer: .} ]
   | to_entries
   | map(.value + {wwan_index: .key})
  ) as $bearer_list |

  # Hardware components for each modem (modemN).
  ($modems | map(. as $m |
    {
      "name": ("modem" + ($m.index | tostring)),
      "class": "infix-hardware:modem",
      "state": {
        "admin-state": (if ($m.enabled // false) then "unlocked" else "locked" end)
      }
    } + (
      (if $m["preferred-mode"] then {"preferred-mode": $m["preferred-mode"]} else {} end) +
      (if (($m["allowed-mode"] // []) | length) > 0
        then {"allowed-mode": [$m["allowed-mode"][].mode]}
        else {} end) +
      (if (($m.band // []) | length) > 0
        then {"band": [$m.band[].band]}
        else {} end) +
      (if $m.location then
        {"location": (
          {"enabled": ($m.location.enabled // false)} +
          (if (($m.location.source // []) | length) > 0
            then {"source": [$m.location.source[].source]}
            else {} end)
        )}
      else {} end)
    | if . == {} then {} else {"infix-hardware:modem": .} end)
  )) as $modem_comps |

  # Hardware components for each SIM (simN, keyed by modem.sim index).
  ($modems | map(. as $m |
    select($m.sim != null) |
    {
      "name": ("sim" + ($m.sim | tostring)),
      "class": "infix-hardware:sim"
    } + (
      (if $m.pin    then {"pin":     $m.pin}    else {} end) +
      (if $m.puk    then {"puk":     $m.puk}    else {} end) +
      (if $m.carrier then {"carrier": $m.carrier} else {} end)
    | if . == {} then {} else {"infix-hardware:sim": .} end)
  )) as $sim_comps |

  # Keystore entries for bearers that have a non-empty password.
  ($bearer_list | map(
    select((.bearer.password // "") != "") |
    {
      "name": ("apn-wwan" + (.wwan_index | tostring) + "-pass"),
      "key-format": "infix-crypto-types:passphrase-key-format",
      "cleartext-symmetric-key": ((.bearer.password // "") | @base64)
    }
  )) as $ks_entries |

  # wwan interfaces, one per bearer.
  ($bearer_list | map(. as $entry |
    ($entry.modem) as $m |
    ($entry.bearer) as $b |
    ("wwan" + ($entry.wwan_index | tostring)) as $wname |
    ("apn-wwan" + ($entry.wwan_index | tostring) + "-pass") as $kname |
    {
      "name": $wname,
      "type": "infix-if-type:modem",
      "infix-interfaces:wwan": {
        "modem": ("modem" + ($m.index | tostring)),
        "sim":   ("sim"   + ($m.sim   | tostring)),
        "bearer": (
          (if $b.apn         then {"apn":     $b.apn}              else {} end) +
          (if $b["ip-type"]  then {"ip-type": $b["ip-type"]}       else {} end) +
          (if ($b["allow-roaming"] // false) then {"roaming": true} else {} end) +
          (if (($b.username // "") != "") then
            {"authentication": {"username": $b.username, "password": $kname}}
          else {} end)
        )
      }
    }
  )) as $wwan_ifaces |

  # Apply: delete old tree, inject new hardware components, keystore entries, interfaces.
  del(."infix-modem:modems")

  | .["ietf-hardware:hardware"]["component"] = (
      (.["ietf-hardware:hardware"]?.component // []) + $modem_comps + $sim_comps)

  | if ($ks_entries | length) > 0 then
      .["ietf-keystore:keystore"]["symmetric-keys"]["symmetric-key"] = (
        (.["ietf-keystore:keystore"]?."symmetric-keys"?."symmetric-key" // []) + $ks_entries)
    else . end

  | .["ietf-interfaces:interfaces"]["interface"] = (
      (.["ietf-interfaces:interfaces"]?.interface // []) + $wwan_ifaces)

else
  .
end
' "$file" > "$temp" && mv "$temp" "$file"
