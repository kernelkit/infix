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
    fi

    ../net "$DEBUG" apply

    if [ -n "$DEBUG" ]; then
	ip link
	ip addr
	tree "$NET_DIR/"
    fi
}

setup
