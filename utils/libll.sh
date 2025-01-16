# Utility functions for communicating with IPv6 neighbors on the local
# LAN

# Usage: llping <iface>
#
# Send ping to all-hosts group, with link-local scope, on <iface>,
# suppressing loopbacked packets.
llping()
{
    local iface="$1"
    shift
    ping -L "$@" ff02::1%$iface
}


# Usage: llpeer <iface>
#
# Return the address of the first IPv6 neighbor to respond on
# <iface>'s local LAN. It is thus typically only useful in scenarios
# where only one neighbor exists
llpeer()
{
    local iface="$1"
    shift
    llping $iface -c1 "$@" | \
	awk '/bytes from/ { sub(/:$/, "", $4); print($4); exit(0); }'
}

llssh_expand()
{
    local old_ifs="$IFS"

    local orig="$1"
    local fmt="$2"

    local peer=
    local user=
    local iface=

    IFS=@
    set $1
    IFS="$old_ifs"
    case $# in
	1)
	    user="$LLSSH_USER"
	    iface="$1"
	    ;;
	2)
	    user="$1"
	    iface="$2"
	    ;;
	*)
	    echo "Error: Invalid peer \"$orig1\"" >&2
	    return 1
	    ;;
    esac

    if [ -d "/sys/class/net/$iface" ]; then
	peer="$(llpeer $iface -w3)"
    else
	peer="$iface"
    fi

    if [ ! "$peer" ]; then
	echo "Error: No peer responded on $iface" >&2
	return 1
    fi

    case "$fmt" in
	ssh)
	    ;;
	scp)
	    peer="[$peer]"
	    ;;
	*)
	    echo "Internal error: Invalid format \"$fmt\"" >&2
	    return 1
	    ;;
    esac

    [ "$user" ] && peer="$user@$peer"
    echo $peer
}

# Usage: llssh <destination> [<ssh-args>]
#
# ssh(1) to <destination>, but if the host matches an existing
# interface name, expand it to a neighboring host on that interface.
llssh()
{
    local remote="$1"
    shift

    local peer="$(llssh_expand $remote ssh)"
    [ "$peer" ] || return 1

    local sshpasscmd=
    [ "$LLSSH_PASS" ] && sshpasscmd="sshpass -p$LLSSH_PASS $LLSSHPASS_OPTS"

    $sshpasscmd ssh $LLSSH_OPTS $peer "$@"
}

llscp_expand()
{
    local orig="$1"
    local old_ifs="$IFS"

    local peer=

    IFS=:
    set $@
    IFS="$old_ifs"
    case $# in
	1)
	    echo "$1"
	    ;;
	2)
	    peer="$(llssh_expand $1 scp)"
	    [ "$peer" ] || return 1

	    echo "$peer:$2"
	    ;;
	*)
	    echo "Error: Invalid location \"$orig1\"" >&2
	    return 1
	    ;;
    esac
}

# Usage: llscp <src> <dst>
#
# scp(1) from <src> to <dst>, but expand any host specified in either
# <src> or <dst>, which matches an existing interface name, to a
# neighboring host on that interface.
llscp()
{
    local src="$(llscp_expand $1)"
    local dst="$(llscp_expand $2)"
    [ "$src" -a "$dst" ] || return 1
    shift 2

    local sshpasscmd=
    [ "$LLSSH_PASS" ] && sshpasscmd="sshpass -p$LLSSH_PASS $LLSSHPASS_OPTS"

    $sshpasscmd scp $LLSCP_OPTS "$src" "$dst"
}
