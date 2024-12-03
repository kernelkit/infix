#!/bin/sh
# Raw switch LED Control for systems that do not run iitod


LEDS=$(find /sys/class/leds -iname '*mdio-mii*')
LINK=$(find /sys/class/leds -iname '*mdio-mii*p')

# Disable ALL switch port LEDs
clear()
{
    for led in $LEDS; do
        echo 0 > "${led}/brightness"
    done
}

setup()
{
    for led in $LINK; do
        echo netdev > "${led}/trigger"
    done

    for led in $LINK; do
        cd "$led"
	# No sleep here, it's enough with the delay from previous loop
        echo 1 > link
        sleep 0.1
        echo 1 > rx
        sleep 0.1
        echo 1 > tx
        cd - >/dev/null
    done
}

leds()
{
    for led in $LINK; do
         printf "%3s: %s\n" "$(cat "$led/device_name" 2>/dev/null)" "$led"
    done
}

list()
{
    leds | sort | while read -r port path; do
        printf "%4s %s\n" "$port" "$(basename "$path")"

        aux=${path%%:tp}:aux
        if [ -e "$aux" ]; then
            printf "%3s: %s\n" "" "$(basename "$aux")"
        fi
    done
}

flash()
{
    sec=$1

    for led in $LEDS; do
        echo timer > "${led}/trigger"
    done

    for led in $LEDS; do
        echo 84 > "${led}/delay_on"
        echo 84 > "${led}/delay_off"
    done

    sleep "$sec"
    clear
}

usage()
{
    echo "usage: $0 [command]"
    echo
    echo "flash [SEC]  Flash all LEDs to locate device in rack, default: 5 sec"
    echo "list         List all LEDs"
    echo "setup        Set up and start normal operation"
    echo "start        Call at system init, clears all LEDs and sets up normal op"
    echo "stop         Clear all LEDs, may be called at system shutdown"
    echo
    echo "Please ensure no other tool or daemon is already managing the LEDs."
}

cmd=$1; shift
case $cmd in
    flash)
        flash ${1:-5}
        setup
        ;;
    help)
	usage
	exit 0
	;;
    list | ls)
        list
        ;;
    setup)
	setup
	;;
    start)
	initctl -nbq cond clear led
	clear
	setup
        ;;
    stop)
	clear
	;;
    *)
        usage
	exit 1
        ;;
esac
