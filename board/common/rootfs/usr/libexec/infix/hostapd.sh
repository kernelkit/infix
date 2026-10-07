#!/bin/sh
# Run hostapd and, when stopped, hand the clients over first.
#
# A client with a good signal has no reason to roam, so when its access
# point goes away it only notices once the beacons stop, and then has to
# scan for a new network.  An 802.11v BSS transition request with
# disassociation imminent makes it roam to another access point while
# the radio is still up.  Clients without 802.11v are deauthenticated
# by hostapd on exit, as before.

# The request names no candidate, a node does not know the other nodes'
# access points, so the client has to scan for one: a few seconds on a
# real radio across three bands.  A client that roams never tells the old
# access point either, hostapd drops it from its station list when the
# timer runs out, so wait a little longer than that before giving up,
# but stay well inside the ten seconds the service gets to stop before
# it is killed.
TIMER=50	# beacon intervals (100 ms) until hostapd disassociates a client that stays
WAIT=7		# seconds to wait for the clients to leave

# The hostapd configs among the arguments, which also carry options
confs()
{
    for arg in $CONFS; do
	case $arg in
	    *.conf) echo "$arg" ;;
	esac
    done
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

handover()
{
    # Only clients that do 802.11v can be asked to move, and hostapd only
    # does so with bss_transition on
    grep -qs '^bss_transition=1' $(confs) || return 0

    num=0
    for bss in $(bsses); do
	for sta in $(hostapd_cli -i "$bss" list_sta 2>/dev/null); do
	    hostapd_cli -i "$bss" disassoc_imminent "$sta" $TIMER >/dev/null 2>&1
	    num=$((num + 1))
	done
    done
    if [ $num -eq 0 ]; then
	logger -t hostapd -p daemon.notice "stop: no stations on $(bsses | tr '\n' ' ')"
	return 0
    fi

    end=$(($(date +%s) + WAIT))
    while [ "$(date +%s)" -lt "$end" ]; do
	[ -z "$(stations)" ] && break
	sleep 0.2
    done

    left=$(stations | grep -c .)
    if [ "$left" -eq 0 ]; then
	logger -t hostapd -p daemon.notice "stop: asked $num station(s) to move, all left"
    else
	logger -t hostapd -p daemon.notice "stop: asked $num station(s) to move, $left still here"
    fi
}

stop()
{
    handover
    kill -TERM "$pid" 2>/dev/null
}

CONFS=$*
hostapd "$@" &
pid=$!
trap stop TERM INT
rc=0
while kill -0 "$pid" 2>/dev/null; do
    wait "$pid"
    rc=$?
done
exit $rc
