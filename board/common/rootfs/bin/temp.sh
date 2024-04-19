#!/bin/sh

tstamp()
{
	if [ "$1" = "head" ]; then
		printf '%9s' time
	else
		printf '%9s' $(date +%T)
	fi
}

thermal()
{
	for th in /sys/class/thermal/thermal_zone*; do
		if [ "$1" = "head" ]; then
			printf '%5s' $(cat $th/type | \
				sed -e 's/-thermal//' -e 's/ap-cpu/cpu/' -e 's/-ic//')
		else
			printf '%5d' \
				$((($(cat $th/temp) + 500) / 1000))
		fi
	done
}


hwmon()
{
	for hw in /sys/class/hwmon/*; do
		[ -f $hw/temp1_input ] || continue

		if [ "$1" = "head" ]; then
			printf '%5s' \
				$(cat $hw/name | sed -e 's/cp0configspacef2000000mdio12a200switch0mdio0/p/' -e 's/f212a600mdiomii0/xp/')
		else
			printf '%5d' \
				$((($(cat $hw/temp1_input) + 500) / 1000))
		fi
	done
}

xphys()
{
	for xphy in 4 5; do
		mdio f212a6* $xphy:31 0xf08a 0x4d00

		if [ "$1" = "head" ]; then
			printf '%5s' \
				p$((xphy + 5))
		else
			printf '%5d' \
				$(($(mdio f212a6* $xphy:31 0xf08a) & 0xff - 75))
		fi
	done
}

if [ "$1" != "-H" ]; then
#	tstamp head
	thermal head
	hwmon head
#	xphys head
	echo
fi

while :; do
#	tstamp
	thermal
	hwmon
#	xphys
	echo

	if [ "$1" == "-n" ]; then
		sleep ${2:-10}
	else
		break
	fi
done
