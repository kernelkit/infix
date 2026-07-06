#!/bin/sh

SCRIPT_PATH="$(dirname "$(readlink -f "$0")")"
JSON="$SCRIPT_PATH/json/firewall-overview.json"
CLI="$SCRIPT_PATH/../../../src/statd/python/cli_pretty/cli_pretty.py"

strip_ansi() {
    sed 's/\x1b\[[0-9;]*m//g'
}

echo "1..2"

OUT1="$(cat "$JSON" | "$CLI" show-firewall | strip_ansi)"
if printf '%s\n' "$OUT1" | grep -q "Address Sets" &&
   printf '%s\n' "$OUT1" | grep -q "allowed" &&
   printf '%s\n' "$OUT1" | grep -q "greylist" &&
   printf '%s\n' "$OUT1" | grep -q "ADDR SET" &&
   printf '%s\n' "$OUT1" | grep -q "trusted" &&
   printf '%s\n' "$OUT1" | grep -q "greylist" ; then
    echo "ok 1 - show-firewall includes address-set summaries"
else
    echo "not ok 1 - show-firewall missing address-set overview"
    exit 1
fi

OUT2="$(cat "$JSON" | "$CLI" show-firewall-zone trusted | strip_ansi)"
if printf '%s\n' "$OUT2" | grep -q "address-sets" &&
   printf '%s\n' "$OUT2" | grep -q "allowed, greylist" ; then
    echo "ok 2 - show-firewall-zone shows zone address-sets"
    exit 0
fi

echo "not ok 2 - show-firewall-zone missing zone address-sets"
exit 1
