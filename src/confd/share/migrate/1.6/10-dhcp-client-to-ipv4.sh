#!/bin/sh
# Migrate DHCP client configuration from standalone model to ietf-ip augment
# From: /infix-dhcp-client:dhcp-client/client-if[if-name]
# To: /ietf-interfaces:interfaces/interface[name]/ipv4/infix-dhcp-client:dhcp
# Note: Drops configurations where enabled == false, since presence = enabled


file=$1
temp=${file}.tmp

jq '
if .["infix-dhcp-client:dhcp-client"] then
  # 1. Check global enabled flag
  if .["infix-dhcp-client:dhcp-client"].enabled == false then
    # Global dhcp-client disabled, drop entire configuration
    del(.["infix-dhcp-client:dhcp-client"])
  else
    # Global enabled (true or absent), process client-if entries
    .["infix-dhcp-client:dhcp-client"]."client-if" as $clients |

    # 2. Create a dictionary (map) of DHCP configs keyed by interface name
    ($clients // [] | map(
      # Only keep entries where enabled is TRUE or ABSENT
      select(.enabled == false | not) |
      {
        key: ."if-name",
        value: {
          # Construct the new presence container, stripping old keys
          "infix-dhcp-client:dhcp": (
            . | del(."if-name", .enabled)
          )
        }
      }
    ) | from_entries) as $dhcp_configs |

    # 3. Merge/Integrate DHCP config data into existing interfaces
    # Use "?" for optional chaining on "interfaces"
    .["ietf-interfaces:interfaces"]?.interface |= map(
      if $dhcp_configs[.name] then
        # Merge the new config into ipv4. Use // {} to ensure ipv4 exists if missing.
        .["ietf-ip:ipv4"] = (.["ietf-ip:ipv4"] // {}) + $dhcp_configs[.name]
      else
        .
      end
    ) |

    # 4. Cleanup the old container
    del(.["infix-dhcp-client:dhcp-client"])
  end
else
  # If no old DHCP config existed, return the input unchanged
  .
end
' "$file" > "$temp" && mv "$temp" "$file"
