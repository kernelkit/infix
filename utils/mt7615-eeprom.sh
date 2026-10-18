#!/bin/sh
# Show or fix up the EEPROM image of an MT7615 WiFi card
set -e

size=19880			# EEPROM buffer of the mt7615 driver

usage()
{
	cat <<EOF
Usage: ${0##*/} [-b BAND] [-m MAC] FILE

Show or fix up an MT7615 EEPROM image.  Cards with an unprogrammed
EEPROM get a random MAC address and a single radio.  Start from the
EEPROM the driver uses today:

  cat /sys/kernel/debug/ieee80211/radio0/mt76/eeprom > mt7615.bin

Without options, show the chip ID, MAC address, and band selection of
FILE.  With options, change FILE in place, setting also the chip ID the
driver checks for.

Options:
  -b BAND  dual  One radio, 2.4 or 5 GHz
           2g    One radio, 2.4 GHz only
           5g    One radio, 5 GHz only
           dbdc  Two radios, 2.4 and 5 GHz at the same time, MT7615D only
  -m MAC   MAC address of the first radio, the second radio derives its own
  -h       This help text
EOF
}

die()
{
	echo "${0##*/}: $*" >&2
	exit 1
}

peek()
{
	od -An -tu1 -j "$1" -N 1 "$file" | tr -d ' '
}

poke()
{
	off=$1
	shift
	for val; do
		# shellcheck disable=SC2059
		printf "\\$(printf %03o "$val")" | dd of="$file" bs=1 seek="$off" conv=notrunc 2>/dev/null
		off=$((off + 1))
	done
}

show()
{
	chip=$(od -An -tx1 -N 2 "$file" | awk '{ print $2 $1 }')
	mac=$(od -An -tx1 -j 4 -N 6 "$file" | sed 's/^ *//; s/ /:/g')
	case $((($(peek 62) >> 4) & 3)) in
		0) band="dual" ;;
		1) band="5g" ;;
		2) band="2g" ;;
		3) band="dbdc" ;;
	esac

	echo "chip-id : 0x$chip"
	echo "mac     : $mac"
	echo "band    : $band"
}

while getopts "b:hm:" opt; do
	case $opt in
		b) band=$OPTARG ;;
		m) mac=$OPTARG ;;
		h) usage; exit 0 ;;
		*) usage >&2; exit 1 ;;
	esac
done
shift $((OPTIND - 1))

file=$1
[ -n "$file" ] || { usage >&2; exit 1; }
[ -f "$file" ] || die "$file: no such file"
[ "$(wc -c < "$file")" -ge $size ] || die "$file: too small, the driver needs $size bytes"

if [ -n "$band" ]; then
	case $band in
		dual) sel=0 ;;
		5g)   sel=1 ;;
		2g)   sel=2 ;;
		dbdc) sel=3 ;;
		*)    die "invalid band: $band" ;;
	esac
fi

if [ -n "$mac" ]; then
	echo "$mac" | grep -Eqi '^([0-9a-f]{2}:){5}[0-9a-f]{2}$' || die "invalid MAC address: $mac"
	[ $((0x${mac%%:*} & 1)) -eq 0 ] || die "invalid MAC address: $mac is multicast"
fi

if [ -n "$band" ] || [ -n "$mac" ]; then
	poke 0 0x15 0x76
	[ -z "$sel" ] || poke 62 $((($(peek 62) & ~0x30) | (sel << 4)))
	if [ -n "$mac" ]; then
		oifs=$IFS
		IFS=:
		# shellcheck disable=SC2086
		set -- $mac
		IFS=$oifs
		poke 4 "0x$1" "0x$2" "0x$3" "0x$4" "0x$5" "0x$6"
	fi
fi

show
