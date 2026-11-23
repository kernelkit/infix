#!/bin/sh
# The whoami RPC tells a user its own NACM groups.  Permit it ahead of
# the rules in every rule-list that denies exec for all modules, e.g.
# the factory guest-acl, or for infix-system.

file=$1
temp=${file}.tmp

jq '
  def denies_exec:
    .action == "deny"
    and ((.["module-name"] // "*") | . == "*" or . == "infix-system")
    and (has("path") or has("rpc-name") or has("notification-name") | not)
    and ((.["access-operations"] // "*") | . == "*" or (split(" ") | index("exec") != null));

  if .["ietf-netconf-acm:nacm"]?["rule-list"]? then
    .["ietf-netconf-acm:nacm"]["rule-list"] |= map(
      (.rule // []) as $rules |
      if ($rules | any(denies_exec)) and ($rules | any(.name == "permit-whoami") | not) then
        .rule = [{
          "name": "permit-whoami",
          "module-name": "infix-system",
          "rpc-name": "whoami",
          "access-operations": "exec",
          "action": "permit",
          "comment": "Guests can ask who they are and what groups they are in."
        }] + $rules
      else . end)
  else . end
' "$file" > "$temp" && mv "$temp" "$file"
