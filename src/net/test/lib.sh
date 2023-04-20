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
if [ -n "$NET" ]; then
    # Verify we didn't find Samba net command, our net live in sbin
    if [ "$(dirname "$NET")" = "/usr/bin" ]; then
	NET=""
    fi
fi
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

# shellcheck disable=SC2120
init_next_gen()
{
    _=$((gen += 1))
    mkdir -p "$NET_DIR/$gen"
    echo $gen > "$NET_DIR/next"

    # shellcheck disable=SC2068
    for iface in $@; do
	create_iface_data "$iface"
    done
}

init_deps()
{
    parent=$1
    shift

    mkdir -p "$NET_DIR/$gen/$parent/deps"
    #shellcheck disable=SC2068
    for iface in $@; do
	ln -s "../../$iface" "$NET_DIR/$gen/$parent/deps/$iface"
    done
}

setup()
{
    say "Test start $(date)"
    init_next_gen

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

create_iface_data()
{
    ifname=$1
    ifdir="$NET_DIR/$gen/$ifname"

    mkdir -p "$ifdir/deps"
    echo "up" > "$ifdir/admin-state"
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

    create_iface_data $ifname

    if [ ! -f "$ifdir/init.ip" ]; then
	echo "link add $ifname type dummy" > "$ifdir/init.ip"
    fi
    if [ -n "$address" ]; then
	cat <<-EOF >>"$ifdir/init.ip"
		addr add $address dev $ifname
		link set $ifname up
		EOF
    fi
}

create_vlan_iface()
{
    ifname=$1
    link=$2
    vid=$3
    if [ $# -eq 4 ]; then
	address=$4
    else
	address=""
    fi
    ifdir="$NET_DIR/$gen/$ifname"

    mkdir -p "$ifdir/deps"
    ln -s "../../$link" "$ifdir/deps/$link"

    cat <<-EOF >"$ifdir/init.ip"
	link add $ifname link $link type vlan id $vid
	link set $ifname up
	EOF
    if [ -n "$address" ]; then
	cat <<-EOF >>"$ifdir/init.ip"
		addr add $address dev $ifname
		EOF
    fi

    echo "up" > "$ifdir/admin-state"
}

# shellcheck disable=SC2124
add_brport()
{
    brname=$1
    shift
    brports=$@
    brdir="$NET_DIR/$gen/$brname"

    mkdir -p "$brdir/deps"
    for port in $brports; do
	ln -s "../../$port" "$brdir/deps/$port"

	cat <<-EOF >> "$brdir/init.ip"
		# Attaching port $port to bridge $brname
		link set $port master $brname
		link set $port up
		EOF
    done
}

# shellcheck disable=SC2124
del_brport()
{
    brname=$1
    shift
    brports=$@
    brdir="$NET_DIR/$gen/$brname"

    for port in $brports; do
	cat <<-EOF >"$brdir/exit.ip"
		link set $port nomaster
		EOF
    done
}

# shellcheck disable=SC2124,SC2086
create_bridge()
{
    brname=$1
    bropts=$2
    shift 2
    brports=$@
    brdir="$NET_DIR/$gen/$brname"

    mkdir -p "$brdir/deps"
    cat <<-EOF > "$brdir/init.ip"
	link add $brname type bridge $bropts
	EOF

    add_brport "$brname" $brports
    cat <<-EOF >> "$brdir/init.ip"
	link set $brname up
	EOF

    echo "up" > "$brdir/admin-state"
}

# Create init.bridge commands to be run for $ifname on $brname
# shellcheck disable=SC2124,SC2086
bridge_init()
{
    brname=$1
    ifname=$2
    shift 2
    vlans=$@
    ifdir="$NET_DIR/$gen/$ifname"

    echo "" > "$ifdir/init.bridge"
    for vlan in $vlans; do
	echo "vlan add vid $vlan dev $brname self" >> "$ifdir/init.bridge"
    done
}

# shellcheck disable=SC2124
add_lagport()
{
    lagnm=$1
    shift
    lagports=$@
    lagdir="$NET_DIR/$gen/$lagnm"

    for port in $lagports; do
	ln -s "../../$port" "$lagdir/deps/$port"

	cat <<-EOF >>"$lagdir/init.ip"
		# Attaching port $port to $lagnm
		link set $port master $lagnm
		link set $port up
		EOF
    done
}

# shellcheck disable=SC2124
del_lagport()
{
    lagnm=$1
    shift
    lagports=$@
    lagdir="$NET_DIR/$gen/$lagnm"

    for port in $lagports; do
	cat <<-EOF >>"$lagdir/exit.ip"
		link set $port nomaster
		EOF
    done
}

# shellcheck disable=SC2124,SC2086
create_lag()
{
    lagnm=$1
    lagopts=$2
    shift 2
    ports=$@
    lagdir="$NET_DIR/$gen/$lagnm"

    mkdir -p "$lagdir/deps"
    cat <<-EOF > "$lagdir/init.ip"
	link add $lagnm type bond $lagopts
	link set $lagnm up
	EOF

    add_lagport "$lagnm" $ports
    echo "up" > "$lagdir/admin-state"
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
    if [ $# -gt 1 ]; then
	address=$2
    else
	address=""
    fi
    state=$(tr '[:lower:]' '[:upper:]' < "$NET_DIR/$gen/$ifname/admin-state")

    addr=$(ip -br -j addr show "$ifname" | jq -r '.[] | .addr_info[0].local')
    plen=$(ip -br -j addr show "$ifname" | jq -r '.[] | .addr_info[0].prefixlen')
    addr="$addr/$plen"
    updn=$(ip -br -j link show "$ifname" | jq -r '.[] | .flags[] | select(index("UP"))' | head -1)

#    echo "$state => $ifname: $updn $addr"
    assert "Verify $ifname state $state"      "$state"   = "$updn"
    if [ -n "$address" ]; then
	assert "Verify $ifname address $address"  "$address" = "$addr"
    fi
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

assert_bridge_ports()
{
    br="$1"
    val="$2"
    shift 2
    # shellcheck disable=SC2124
    ports=$@

    for port in $ports; do
	found=false
	for brport in $(bridge -j link |jq -r --arg br "$br" '.[] | select(.master == $br).ifname'); do
	    if [ "$port" = "$brport" ]; then
		found=true
		break;
	    fi
	done
	if [ "$val" = "false" ]; then
	    not="NOT "
	else
	    not=""
	fi
	assert "Port $port is ${not}a $br bridge port" "$found" = "$val"
    done
}

assert_lag_ports()
{
    lag="$1"
    val="$2"
    shift 2
    # shellcheck disable=SC2124
    ports=$@

    for port in $ports; do
	found=false

	if [ "$lag" = "$(ip -d -j link show $port |jq -r '.[].master')" ]; then
	    found=true
	fi
	if [ "$val" = "false" ]; then
	    not="NOT "
	else
	    not=""
	fi
	assert "Port $port is ${not}a $lag member port" "$found" = "$val"
    done
}

setup
