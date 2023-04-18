#!/bin/sh

# Session set from Makefile before calling unshare -mrun
if [ -z "$SESSION" ]; then
    SESSION=$(mktemp -d)
    TMPSESS=1
fi

if [ -n "$DEBUG" ]; then
    DEBUG="-v -d"
else
    DEBUG=""
fi

# Test name, used everywhere as /tmp/$NM/foo
NM=$(basename "$0" .sh)
NET_DIR="${SESSION}/${NM}"
export NET_DIR

gen=-1

NET=$(command -v net)
[ -n "$NET" ] || NET=../src/net

# Exit immediately on error, treat unset variables as error
set -eu

color_reset='\e[0m'
fg_red='\e[1;31m'
fg_green='\e[1;32m'
fg_yellow='\e[1;33m'
log()
{
    test=$(basename "$0" ".sh")
    printf "\e[2m[%s]\e[0m %b%b%b %s\n" "$test" "$1" "$2" "$color_reset" "$3"
}

sep()
{
    printf "\e[2m――――――――――――――――――――――――――――――――――――――――――――――――――――――――――――――――――――――――\e[0m\n"
}

say()
{
    log "$fg_yellow" "•" "$@"
}

skip()
{
    log "$fg_yellow" − "$*"
    exit 77
}

fail()
{
    log "$fg_red" ✘ "$*"
    exit 99
}

assert()
{
    __assert_msg=$1
    shift

    if [ ! "$@" ]; then
	log "$fg_red" ✘ "$__assert_msg ($*)"
	return 1
    fi

    log "$fg_green" ✔ "$__assert_msg"
    return 0
}

signal()
{
    echo
    if [ "$1" != "EXIT" ]; then
	print "Got signal, cleaning up"
    fi

    rm -rf "${NET_DIR}"
    if [ -n "$TMPSESS" ] && [ -d "$SESSION" ]; then
	rm -rf "$SESSION"
    fi
}

# props to https://stackoverflow.com/a/2183063/1708249
# shellcheck disable=SC2064
trapit()
{
    func="$1" ; shift
    for sig ; do
	trap "$func $sig" "$sig"
    done
}

create_ng()
{
    _=$((gen += 1))
    mkdir -p "$NET_DIR/$gen"
    echo $gen > "$NET_DIR/next"
}

setup()
{
    say "Test start $(date)"
    create_ng

    # Runs once when including lib.sh
    mkdir -p "${NET_DIR}"
    trapit signal INT TERM QUIT EXIT

    ip link set lo up
    sep
}

netdo()
{
    if [ -n "$DEBUG" ]; then
	tree "$NET_DIR/"
	echo "Calling: $NET $DEBUG apply"
    fi

    # shellcheck disable=SC2086
    $NET $DEBUG apply

    if [ -n "$DEBUG" ]; then
	ip link
	ip addr
	tree "$NET_DIR/"
    fi
}

netdown()
{
    # shellcheck disable=SC2086,SC2068
    $NET $DEBUG down $@

    if [ -n "$DEBUG" ]; then
	ip link
	ip addr
    fi
}

netup()
{
    # shellcheck disable=SC2086,SC2068
    $NET $DEBUG up $@

    if [ -n "$DEBUG" ]; then
	ip link
	ip addr
    fi
}

create_iface()
{
    ifname=$1
    ifdir="$NET_DIR/$gen/$ifname"
    if [ $# -eq 2 ]; then
	address=$2
    else
	address=""
    fi

    mkdir -p "$ifdir/deps"
    touch "$ifdir/init.ip"
    if [ -n "$address" ]; then
	cat <<-EOF >"$ifdir/init.ip"
		link add $ifname type dummy
		addr add $address dev $ifname
		link set $ifname up
		EOF
    fi
    echo "up" > "$ifdir/admin-state"
}

remove_iface()
{
    ifname=$1

    cat <<-EOF >"$NET_DIR/$gen/$ifname/exit.ip"
	link del $ifname
EOF
}

assert_iface()
{
    ifname=$1
    address=$2
    state=$(tr '[:lower:]' '[:upper:]' < "$NET_DIR/$gen/$ifname/admin-state")

    addr=$(ip -br -j addr show "$ifname" | jq -r '.[] | .addr_info[0].local')
    plen=$(ip -br -j addr show "$ifname" | jq -r '.[] | .addr_info[0].prefixlen')
    addr="$addr/$plen"
    updn=$(ip -br -j link show "$ifname" | jq -r '.[] | .flags[] | select(index("UP"))' | head -1)

#    echo "$state => $ifname: $updn $addr"
    assert "Verify $ifname state $state"      "$state"   = "$updn"
    assert "Verify $ifname address $address"  "$address" = "$addr"
}

assert_noiface()
{
    ifname=$1
    rc=true

    for iface in $(ip -j -br link show |jq -r '.[] .ifname'); do
	[ "$iface" = "$ifname" ] || continue

	rc=false
	break
    done

    assert "Verify $ifname has been removed" $rc
}

assert_iface_flag()
{
    found=false
    ifname=$2
    flag=$3
    msg=$1
    val=$4

    for f in $(ip -j -br link show "$ifname" |jq -r '.[] .flags[]'); do
#	echo "$ifname: FLAG $f ..."
	[ "$f" = "$flag" ] || continue
	found=true
	break
    done

#    echo "FLAG $flag found $found, expected $val"
    assert "$msg" "$found" = "$val"
}

setup
