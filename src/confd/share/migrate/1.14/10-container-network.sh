#!/bin/sh
# container-network used to mean two things.  On a bridge (type bridge)
# the bridge stayed on the host and containers were connected to it with
# automatic VETH pairs, addresses from a pool, masquerading and published
# ports.  On any other interface (type host) the interface itself was
# moved into the container.  Only the latter remains, so:
#  - a container bridge is removed, with the references to it from
#    containers, bridge ports and firewall zones
#  - an interface that was moved into a container keeps the marker,
#    only the type and subnet leaves are dropped
#  - published ports and the ip, ip6 and mac options are dropped

file=$1
temp=${file}.tmp

jq '
  # A missing type was inferred from the type of the interface
  def is_bridge:
    .["infix-interfaces:container-network"] as $net
    | $net != null and
      (if $net.type then $net.type | endswith("bridge")
       else .type == "infix-if-type:bridge" end);

  [.["ietf-interfaces:interfaces"]?.interface[]? | select(is_bridge) | .name] as $bridges

  | if .["ietf-interfaces:interfaces"]?.interface then
      .["ietf-interfaces:interfaces"].interface |= map(
        select(is_bridge | not)
        | if [.["infix-interfaces:bridge-port"]?.bridge] | inside($bridges) then
            del(.["infix-interfaces:bridge-port"])
          else . end
        | if .["infix-interfaces:container-network"] then
            .["infix-interfaces:container-network"] |= del(.type, .subnet)
          else . end)
    else . end

  | if .["infix-firewall:firewall"]?.zone then
      .["infix-firewall:firewall"].zone |= map(
        if .interface then
          .interface -= $bridges
          | if .interface == [] then del(.interface) else . end
        else . end)
    else . end

  | if .["infix-containers:containers"]?.container then
      .["infix-containers:containers"].container |= map(
        if .network then
          del(.network.publish)
          | if .network.interface then
              .network.interface |= map(
                select([.name] | inside($bridges) | not)
                | if .option then
                    .option |= map(select(test("^(ip6?|mac)=") | not))
                    | if .option == [] then del(.option) else . end
                  else . end)
              | if .network.interface == [] then del(.network.interface) else . end
            else . end
          | if .network == {} then del(.network) else . end
        else . end)
    else . end
' "$file" > "$temp" && mv "$temp" "$file"
