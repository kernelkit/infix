#!/bin/sh
# Move dual-band clients from a 2.4 GHz access point to its 5/6 GHz twin.
#
# Usage: wifi-steer.sh <2.4 GHz bss> <twin bss>
#
# hostapd keeps a list of the clients each radio has seen lately.  A
# client connected on 2.4 GHz that the twin has seen is dual-band and in
# range of the twin, so ask it to move with an 802.11v BSS transition
# request naming the twin.  Clients that ignore the request stay: a
# client is asked again only after a cooldown, and one that has shrugged
# off a couple of requests is left alone for an hour.  A client with a
# weak 2.4 GHz signal is left alone too, 5 GHz would be worse, and
# nobody is sent to a twin that is down or still checking for radar.
bss=$1
twin=$2

PERIOD=15	# seconds between rounds
COOLDOWN=120	# seconds before asking the same client again
GIVEUP=2	# requests a client may shrug off before it is left alone ...
LONG=3600	# ... for this long
MIN_SIGNAL=-65	# dBm on 2.4 GHz below which a client is left alone

state=/run/wifi-steer/$bss
mkdir -p "$state"

# Neighbor report candidate for the twin: BSSID, BSSID information,
# operating class, channel and PHY type.  Taken from the twin's own
# neighbor entry when it keeps one (802.11k), else built from its status
# with the 20 MHz operating class of the channel, enough for the client
# to find the BSS and learn the rest from its beacons.
candidate()
{
    bssid=$(hostapd_cli -i "$twin" get_config 2>/dev/null | sed -n 's/^bssid=//p')
    [ -n "$bssid" ] || return 1

    nr=$(hostapd_cli -i "$twin" show_neighbor 2>/dev/null | \
	 awk -v b="$bssid" 'tolower($1) == tolower(b) { for (i = 2; i <= NF; i++) if ($i ~ /^nr=/) print substr($i, 4) }')
    if [ ${#nr} -ge 26 ]; then
	info=$(echo "$nr" | cut -c13-20)
	info=$(printf '%d' "0x$(echo "$info" | cut -c7-8)$(echo "$info" | cut -c5-6)$(echo "$info" | cut -c3-4)$(echo "$info" | cut -c1-2)")
	op=$(printf '%d' "0x$(echo "$nr" | cut -c21-22)")
	chan=$(printf '%d' "0x$(echo "$nr" | cut -c23-24)")
	phy=$(printf '%d' "0x$(echo "$nr" | cut -c25-26)")
	echo "$bssid,$info,$op,$chan,$phy"
	return 0
    fi

    status=$(hostapd_cli -i "$twin" status 2>/dev/null)
    freq=$(echo "$status" | sed -n 's/^freq=//p')
    chan=$(echo "$status" | sed -n 's/^channel=//p')
    [ -n "$freq" ] && [ -n "$chan" ] || return 1
    if [ "$freq" -ge 5925 ]; then
	op=131
    elif [ "$freq" -ge 5745 ]; then
	op=124
    elif [ "$freq" -ge 5500 ]; then
	op=121
    elif [ "$freq" -ge 5260 ]; then
	op=118
    elif [ "$freq" -ge 5180 ]; then
	op=115
    else
	op=81
    fi
    # BSSID information: AP reachable, same security and key scope.
    echo "$bssid,1151,$op,$chan,9"
}

while sleep $PERIOD; do
    stas=$(hostapd_cli -i "$bss" list_sta 2>/dev/null)
    [ -n "$stas" ] || continue
    # Nothing to move to while the twin is down or checking for radar
    hostapd_cli -i "$twin" status 2>/dev/null | grep -q '^state=ENABLED' || continue
    # hostapd_cli has no shorthand for the seen-on list, ask hostapd directly
    seen=$(hostapd_cli -i "$twin" raw TRACK_STA_LIST 2>/dev/null)
    [ -n "$seen" ] || continue
    cand=$(candidate) || continue
    now=$(date +%s)

    for sta in $stas; do
	echo "$seen" | grep -qi "^$sta " || continue
	read -r last tries < "$state/$sta" 2>/dev/null || { last=0; tries=0; }
	wait=$COOLDOWN
	[ "${tries:-0}" -lt $GIVEUP ] || wait=$LONG
	[ $((now - last)) -ge $wait ] || continue
	[ "${tries:-0}" -lt $GIVEUP ] || tries=0
	signal=$(hostapd_cli -i "$bss" sta "$sta" 2>/dev/null | sed -n 's/^signal=//p')
	if [ -n "$signal" ] && [ "$signal" -lt $MIN_SIGNAL ]; then
	    continue
	fi
	hostapd_cli -i "$bss" bss_tm_req "$sta" pref=1 abridged=1 valid_int=255 \
		    "neighbor=$cand" >/dev/null 2>&1
	echo "$now $((tries + 1))" > "$state/$sta"
	logger -t hostapd -p daemon.notice "$bss: asked $sta to move to $twin"
    done
done
