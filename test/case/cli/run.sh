#!/bin/sh

SCRIPT_PATH="$(dirname "$(readlink -f "$0")")"
ROOT_PATH="$SCRIPT_PATH/../../../"
CLI_OUTPUT_PATH="$SCRIPT_PATH/cli-output/"

CLI_PRETTY_TOOL="$ROOT_PATH/src/statd/python/cli_pretty/cli_pretty.py"
SR_EMULATOR_TOOL="$SCRIPT_PATH/sysrepo-emulator.sh"

CLI_OUTPUT_FILE="$(mktemp)"

TEST=1

cleanup() {
    rm -f "$CLI_OUTPUT_FILE"
}
trap cleanup EXIT

ok() {
    echo "ok $TEST - $1"
    TEST=$((TEST + 1))
}

fail() {
    echo "not ok $TEST - $1"
    exit 1
}

print_update_txt() {
    echo
    echo "# CLI output has changed. This might not be an error if you intentionally"
    echo "# changed something in yanger or cli-pretty. If you did, you need to update"
    echo "# the template file."
    echo
    echo "# Here's how you update the CLI output templates:"
    echo "# $SCRIPT_PATH/run.sh update <cli-pretty command>"
    echo
    echo "# Check the result"
    echo "# git diff"
    echo
    echo "# Then finish up by committing the new template"
    echo
}

if [ ! -e "$CLI_PRETTY_TOOL" ]; then
    echo "Error, cli-pretty tool not found"
    exit 1
fi

if [ $# -eq 2 ] && [ $1 = "update" ]; then
    if [ $2  = "show-interfaces" ]; then
      "$SR_EMULATOR_TOOL" | "$CLI_PRETTY_TOOL" "show-interfaces" > "$CLI_OUTPUT_PATH/show-interfaces.txt"
      for iface in "br0" "e0" "e1" "e2" "e3" "e4"; do
         "$SR_EMULATOR_TOOL" | "$CLI_PRETTY_TOOL" "show-interfaces" -n "$iface" \
             > "$CLI_OUTPUT_PATH/show-interface-${iface}.txt"
      done
    elif [ $2 = "show-routing-table" ]; then
      "$SR_EMULATOR_TOOL" | "$CLI_PRETTY_TOOL" "show-routing-table" -i "ipv4" > "$CLI_OUTPUT_PATH/show-routes-ipv4.txt"
      "$SR_EMULATOR_TOOL" | "$CLI_PRETTY_TOOL" "show-routing-table" -i "ipv6" > "$CLI_OUTPUT_PATH/show-routes-ipv6.txt"
    elif [ $2  = "show-bridge-mdb" ]; then
      "$SR_EMULATOR_TOOL" | "$CLI_PRETTY_TOOL" "show-bridge-mdb" > "$CLI_OUTPUT_PATH/show-bridge-mdb.txt"
    else
      echo "Unsupported cli-pretty command $2"
      exit 1
    fi
   echo "All files updated. Check git diff and commit if they look OK"
    exit 0
fi

echo "1..10"
echo "# Running:"

# Show interfaces
echo "# $SR_EMULATOR_TOOL | $CLI_PRETTY_TOOL show-interfaces"
"$SR_EMULATOR_TOOL" | "$CLI_PRETTY_TOOL" "show-interfaces" > "$CLI_OUTPUT_FILE"

if ! diff -u "$CLI_OUTPUT_PATH/show-interfaces.txt" "$CLI_OUTPUT_FILE"; then
    print_update_txt
    fail "\"show interfaces\" output has changed"
fi
ok "\"show interfaces\" output looks intact"

echo "# $SR_EMULATOR_TOOL | $CLI_PRETTY_TOOL show-bridge-mdb"
"$SR_EMULATOR_TOOL" | "$CLI_PRETTY_TOOL" "show-bridge-mdb" > "$CLI_OUTPUT_FILE"

if ! diff -u "$CLI_OUTPUT_PATH/show-bridge-mdb.txt" "$CLI_OUTPUT_FILE"; then
    print_update_txt
    fail "\"show bridge mdb\" output has changed"
fi
ok "\"show bridge mdb\" output looks intact"

# Show ipv4 routes
echo "# $SR_EMULATOR_TOOL | $CLI_PRETTY_TOOL show-routing-table -i ipv4"
"$SR_EMULATOR_TOOL" | "$CLI_PRETTY_TOOL" "show-routing-table" -i "ipv4" > "$CLI_OUTPUT_FILE"

if ! diff -u "$CLI_OUTPUT_PATH/show-routes-ipv4.txt" "$CLI_OUTPUT_FILE"; then
    print_update_txt
    fail "\"show routes ipv4\" output has changed"
fi
ok "\"show routes ipv4\" output looks intact"

# Show ipv6 routes
echo "# $SR_EMULATOR_TOOL | $CLI_PRETTY_TOOL show-routing-table -i ipv6"
"$SR_EMULATOR_TOOL" | "$CLI_PRETTY_TOOL" "show-routing-table" -i "ipv6" > "$CLI_OUTPUT_FILE"

if ! diff -u "$CLI_OUTPUT_PATH/show-routes-ipv6.txt" "$CLI_OUTPUT_FILE"; then
    print_update_txt
    fail "\"show routes ipv6\" output has changed"
fi
ok "\"show routes ipv6\" output looks intact"

# Show detailed interfaces
for iface in "br0" "e0" "e1" "e2" "e3" "e4"; do
    "$SR_EMULATOR_TOOL" | "$CLI_PRETTY_TOOL" "show-interfaces" -n "$iface" > "$CLI_OUTPUT_FILE"
    if ! diff -u "$CLI_OUTPUT_PATH/show-interface-${iface}.txt" "$CLI_OUTPUT_FILE"; then
        print_update_txt
        fail "\"show interface name $iface\" output has changed"
    fi
    ok "\"show interface name $iface\" output looks intact"
done
