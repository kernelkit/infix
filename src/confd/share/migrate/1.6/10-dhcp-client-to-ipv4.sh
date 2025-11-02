#!/bin/sh
# Migrate DHCP client configuration from standalone model to ietf-ip augment
# From: /infix-dhcp-client:dhcp-client/client-if[if-name]
# To: /ietf-interfaces:interfaces/interface[name]/ipv4/infix-dhcp-client:dhcp

file=$1
temp=${file}.tmp

jq '
if .["infix-dhcp-client:dhcp-client"] then
  .["infix-dhcp-client:dhcp-client"]."client-if" as $clients |

  ($clients // [] | map({
    key: ."if-name",
    value: {
      "infix-dhcp-client:dhcp": (
        . | del(."if-name") | del(.enabled)
      )
    }
  }) | from_entries) as $dhcp_configs |

  if .["ietf-interfaces:interfaces"] then
    .["ietf-interfaces:interfaces"].interface |= map(
      if $dhcp_configs[.name] then
        .["ietf-ip:ipv4"] += $dhcp_configs[.name]
      else
        .
      end
    )
  else
    .
  end |

  del(.["infix-dhcp-client:dhcp-client"])
else
  .
end
' "$file" > "$temp" && mv "$temp" "$file"
