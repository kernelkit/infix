#!/bin/sh
# Background WAN interface monitor.  Lights up WAN LED
# while the interface has a DHCP address.

LED_FILE="/run/led/wan-up"
PID_FILE="/run/$(basename "$0").pid"

check_wan()
{
    ip_info=$(ip a show wan)

    if echo "$ip_info" | grep -q "inet .* proto dhcp"; then
        [ ! -f "$LED_FILE" ] && touch "$LED_FILE"
    else
        [ -f "$LED_FILE" ] && rm "$LED_FILE"
    fi
}

cleanup()
{
    rm -f "$LED_FILE"
    rm -f "$PID_FILE"
    exit 0
}

trap 'cleanup' TERM INT HUP QUIT
echo $$ > "$PID_FILE"

remaining_time=$((1800 - $(awk '{print int($1)}' /proc/uptime)))
[ "$remaining_time" -lt 0 ] && remaining_time=0

while [ "$remaining_time" -gt 0 ]; do
    check_wan
    sleep 1
    remaining_time=$((remaining_time - 1))
done

while :; do
    check_wan
    sleep 5
done
