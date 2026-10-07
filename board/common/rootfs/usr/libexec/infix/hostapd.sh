#!/bin/sh
# Run hostapd, learn the other nodes' access points, steer dual-band
# clients to 5 GHz while it runs, and hand the clients over when stopped.
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
	# shellcheck disable=SC2086
	hostapd_cli -i "$bss" bss_tm_req "$sta" disassoc_imminent=1 disassoc_timer=$TIMER \
		    pref=1 abridged=1 $cands >/dev/null 2>&1
    else
	hostapd_cli -i "$bss" disassoc_imminent "$sta" $TIMER >/dev/null 2>&1
    fi
}

handover()
{
    # Only clients that do 802.11v can be asked to move, and hostapd only
    # does so with bss_transition on
    grep -qs '^bss_transition=1' $(confs) || return 0

    num=0
    for bss in $(bsses); do
	for sta in $(hostapd_cli -i "$bss" list_sta 2>/dev/null); do
	    ask_to_move "$bss" "$sta"
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

# Band steering pairs, a 2.4 GHz BSS and the 5/6 GHz twin it defers to,
# from the no_probe_resp_if_seen_on directives in the radio configs.
pairs()
{
    for conf in "$@"; do
	case $conf in
	    *.conf) ;;
	    *) continue ;;
	esac
	awk -F= '/^(interface|bss)=/ { cur = $2 }
		 /^no_probe_resp_if_seen_on=/ { print cur, $2 }' "$conf"
    done
}

# Wait for the control socket of a BSS, up to ten seconds
wait_bss()
{
    i=0
    while [ ! -S /run/hostapd/$1 ] && [ $i -lt 50 ]; do
	sleep 0.2
	i=$((i + 1))
    done
    [ -S /run/hostapd/$1 ]
}

helpers()
{
    # Exchange access point lists with the other nodes once hostapd is up
    for conf in "$@"; do
	case $conf in *.conf) ;; *) continue ;; esac
	wait_bss "$(sed -n 's/^interface=//p' "$conf")" || continue
	/usr/libexec/infix/wifi-neighbors.py "$@" &
	echo $! >> /run/wifi-steer/pids
	break
    done

    # Steer dual-band clients to the higher band, one loop per pair
    pairs "$@" | while read -r bss twin; do
	wait_bss "$bss" || continue
	/usr/libexec/infix/wifi-steer.sh "$bss" "$twin" &
	echo $! >> /run/wifi-steer/pids
    done
}

stop()
{
    [ -f /run/wifi-steer/pids ] && kill $(cat /run/wifi-steer/pids) 2>/dev/null
    rm -rf /run/wifi-steer
    handover
    kill -TERM "$pid" 2>/dev/null
}

CONFS=$*
rm -rf /run/wifi-steer
mkdir -p /run/wifi-steer
trap stop TERM INT
hostapd "$@" &
pid=$!
helpers "$@" &
echo $! >> /run/wifi-steer/pids
rc=0
while kill -0 "$pid" 2>/dev/null; do
    wait "$pid"
    rc=$?
done
exit $rc
