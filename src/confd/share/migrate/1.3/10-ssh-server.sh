#!/bin/sh
# SSH is now configurable, add default settings to configuration

file=$1
temp=${file}.tmp
jq '
if has("infix-services:ssh") then
  .
else
  .["infix-services:ssh"] = {
    "enabled": true,
    "hostkey": ["genkey"],
    "listen": [
      {"name": "ipv4", "address": "0.0.0.0", "port": 22},
      {"name": "ipv6", "address": "::", "port": 22}
    ]
  }
end' $file > temp.json
mv temp.json $file
