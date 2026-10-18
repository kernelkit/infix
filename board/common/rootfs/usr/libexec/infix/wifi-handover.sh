#!/bin/sh
# Hand the clients over to another access point, then stop hostapd.
# Called by finit as the stop:script of hostapd, with $MAINPID set.
#
# A client with a good signal has no reason to roam, so when its access
# point goes away it only notices once the beacons stop, and then has to
# scan for a new network.  An 802.11v BSS transition request with
# disassociation imminent makes it roam to another access point while
# the radio is still up.  Clients without 802.11v are deauthenticated
# by hostapd on exit, as before.

# A client given candidates roams in well under a second, one without
# has to scan first, a few seconds on a real radio across three bands.
# A client that roams never tells the old access point either, hostapd
# drops it from its station list when the timer runs out, so wait a
# little longer than that before giving up, but stay well inside the
# ten seconds the service gets to stop before it is killed.
TIMER=50	# beacon intervals (100 ms) until hostapd disassociates a client that stays
WAIT=7		# seconds to wait for the clients to leave

log()
{
    logger -t wifi-handover -p daemon.notice "$*"
}

bsses()
{
    for sock in /run/hostapd/*; do
	[ -S "$sock" ] && echo "${sock##*/}"
    done
}

stations()
{
    for bss in $(bsses); do
	hostapd_cli -i "$bss" list_sta 2>/dev/null
    done
}

# Ask a station to leave, naming the other access points of the SSID
# wifi-neighbors.py has heard of: a client does not look for a new
# access point on a request without candidates.
ask_to_move()
{
    bss=$1
    sta=$2
    cands=""
    if [ -s /run/wifi-neighbors/$bss ]; then
	for cand in $(grep -E '^[0-9a-f]{2}(:[0-9a-f]{2}){5}(,[0-9]+){4}$' /run/wifi-neighbors/$bss); do
	    cands="$cands neighbor=$cand"
	done
    fi
    if [ -n "$cands" ]; then
	# An MBO transition reason makes a client take a candidate rather
	# than weigh it against this access point, which older clients do
	# even with disassociation imminent
	# shellcheck disable=SC2086
	hostapd_cli -i "$bss" bss_tm_req "$sta" disassoc_imminent=1 disassoc_timer=$TIMER \
		    pref=1 abridged=1 mbo=0:0:1 $cands >/dev/null 2>&1
    else
	hostapd_cli -i "$bss" disassoc_imminent "$sta" $TIMER >/dev/null 2>&1
    fi
}

handover()
{
    # Only clients that do 802.11v can be asked to move, and hostapd only
    # does so with bss_transition on
    grep -qs '^bss_transition=1' /etc/hostapd-*.conf || return 0

    num=0
    for bss in $(bsses); do
	for sta in $(hostapd_cli -i "$bss" list_sta 2>/dev/null); do
	    ask_to_move "$bss" "$sta"
	    num=$((num + 1))
	done
    done
    if [ $num -eq 0 ]; then
	log "no stations on $(bsses | tr '\n' ' ')"
	return 0
    fi

    end=$(($(date +%s) + WAIT))
    while [ "$(date +%s)" -lt "$end" ]; do
	[ -z "$(stations)" ] && break
	sleep 0.2
    done

    left=$(stations | grep -c .)
    if [ "$left" -eq 0 ]; then
	log "asked $num station(s) to move, all left"
    else
	log "asked $num station(s) to move, $left still here"
    fi
}

handover
kill -TERM "$MAINPID"
