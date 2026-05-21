#!/bin/sh
# Migrate fixed-speed ethernet configs to auto-negotiation/advertised-pmd-types.
#
# IEEE Std 802.3.2-2025 obsoleted the ieee802-ethernet-interface speed leaf,
# leaving no standards-blessed config-true location to pin a port to a fixed
# speed.  Infix expresses the same intent via a new infix-augmented
# 'advertised-pmd-types' leaf-list inside the auto-negotiation container: when
# the list names exactly one PMD type, the link comes up at that mode against
# any cooperating peer — the standards-correct interpretation of "fixed speed".
#
# Rewrites
#   ethernet { auto-negotiation { enable false; } speed S; duplex D; }
# into
#   ethernet { auto-negotiation { enable false;
#                                  advertised-pmd-types [PMD]; } duplex D; }
# where PMD is derived from (S, D) using a static lookup table.  D and
# enable=false are preserved verbatim — confd's apply path treats
# enable=false plus a single advertised-pmd-types entry as "force
# autoneg off and pin to this speed/duplex", matching the legacy
# semantics for link partners that don't run auto-negotiation.
#
# Interfaces that don't disable auto-negotiation, or that lack a speed leaf,
# are left untouched.

file=$1
temp=${file}.tmp

#
# The (speed Gb/s, copper-T-implicit-duplex) → PMD table below is a subset
# of the canonical map in src/statd/python/yanger/ietf_interfaces/ethernet.py
# (_LINK_MODES, key by (port, speed, duplex)).  Migrate only covers the
# copper-T cases that the old legacy syntax supported in the first place;
# fiber/DAC pinning was never expressible via the deprecated speed leaf.
#
jq '
def speed_to_pmd:
    if .   == "0.01" then "ieee802-ethernet-phy-type:pmd-type-10BASE-T"
    elif . == "0.1"  then "ieee802-ethernet-phy-type:pmd-type-100BASE-TX"
    elif . == "1.0"  then "ieee802-ethernet-phy-type:pmd-type-1000BASE-T"
    elif . == "2.5"  then "ieee802-ethernet-phy-type:pmd-type-2.5GBASE-T"
    elif . == "5.0"  then "ieee802-ethernet-phy-type:pmd-type-5GBASE-T"
    elif . == "10.0" then "ieee802-ethernet-phy-type:pmd-type-10GBASE-T"
    else null
    end;

(.["ietf-interfaces:interfaces"].interface // [])
|= [ .[] |
     . as $iface
     | if ($iface["ieee802-ethernet-interface:ethernet"]?
              ["auto-negotiation"]?.enable == false)
          and ($iface["ieee802-ethernet-interface:ethernet"]?.speed != null)
       then
           ($iface["ieee802-ethernet-interface:ethernet"].speed | speed_to_pmd) as $pmd
           | if $pmd == null then
                 $iface     # leave unmappable speeds alone; admin must fix
             else
                 $iface
                 | .["ieee802-ethernet-interface:ethernet"]
                       ["auto-negotiation"]
                            += {"infix-ethernet-interface:advertised-pmd-types": [$pmd]}
                 | del(.["ieee802-ethernet-interface:ethernet"].speed)
             end
       else
           $iface
       end ]
' "$file" > "$temp" && mv "$temp" "$file"
