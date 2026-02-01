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

# Usage: llscan [-a] [<iface>]
#
# Discover Infix devices (or all mDNS devices with -a) on the LAN.
# Prefers mDNS-SD via avahi-browse; falls back to IPv6 link-local
# multicast ping if avahi-browse is not available and an interface
# is specified.
llscan()
{
    local all=0
    local iface=

    while [ $# -gt 0 ]; do
	case "$1" in
	    -a)
		all=1
		shift
		;;
	    *)
		iface="$1"
		shift
		;;
	esac
    done

    if [ "$iface" ] && [ ! -d "/sys/class/net/$iface" ]; then
	echo "Error: Interface \"$iface\" does not exist." >&2
	return 1
    fi

    if command -v avahi-browse >/dev/null 2>&1; then
	llscan_mdns "$all"
    elif [ "$iface" ]; then
	echo "Note: avahi-browse not found, falling back to link-local scan." >&2
	llscan_ll "$iface"
    else
	cat >&2 <<-EOF
	Error: avahi-browse not found.

	Install avahi-utils to scan for Infix devices via mDNS:
	  sudo apt install avahi-utils

	Or specify an interface to use IPv6 link-local fallback:
	  $(basename $0) scan <iface>
	EOF
	return 1
    fi
}

llscan_mdns()
{
    local all="$1"
    local flags="-tarp"

    if avahi-browse --help 2>&1 | grep -q -- '-k'; then
	flags="-tarpk"
    fi

    avahi-browse $flags | awk -F';' -v show_all="$all" '
    $1 == "=" {
	host = $7
	proto = $3
	addr = $8
	txt  = $10

	on = ""; ov = ""; product = ""; serial = ""; devid = ""
	n = split(txt, parts, "\" \"")
	for (i = 1; i <= n; i++) {
	    gsub(/"/, "", parts[i])
	    if      (parts[i] ~ /^on=/)       { split(parts[i], kv, "="); on      = kv[2] }
	    else if (parts[i] ~ /^ov=/)       { split(parts[i], kv, "="); ov      = kv[2] }
	    else if (parts[i] ~ /^product=/)  { split(parts[i], kv, "="); product = kv[2] }
	    else if (parts[i] ~ /^serial=/)   { split(parts[i], kv, "="); serial  = kv[2] }
	    else if (parts[i] ~ /^deviceid=/) { split(parts[i], kv, "="); devid   = kv[2] }
	}

	if (!show_all && on != "Infix") next

	# Use deviceid (MAC) as unique key; fall back to hostname
	key = devid ? devid : host

	if (!product) product = on ? on : "-"
	if (!ov)      ov      = "-"
	if (!serial || serial == "null") serial = "-"

	# Prefer IPv4 for display address
	if (proto == "IPv4") {
	    ipv4[key] = addr
	} else if (proto == "IPv6" && !ipv6[key]) {
	    ipv6[key] = addr
	}

	if (!seen[key]++) {
	    keys[++ndevs]  = key
	    hosts[key]     = host
	    products[key]  = product
	    versions[key]  = ov
	    serials[key]   = serial
	} else if (length(host) > length(hosts[key])) {
	    # Prefer the unique hostname (e.g., infix-c0-ff-ee.local)
	    # over the generic one (e.g., infix.local)
	    hosts[key] = host
	}
    }

    END {
	if (ndevs == 0) {
	    if (show_all)
		print "No mDNS devices found." | "cat >&2"
	    else
		print "No Infix devices found. Use -a to show all mDNS devices." | "cat >&2"
	    exit 1
	}

	fmt = "%-26s %-18s %-24s %-22s %s\n"
	hdr = sprintf(fmt, "HOSTNAME", "ADDRESS", "PRODUCT", "VERSION", "SERIAL")
	sub(/\n$/, "", hdr)
	printf "\033[7m%s\033[0m\n", hdr

	for (i = 1; i <= ndevs; i++) {
	    k = keys[i]
	    a = (ipv4[k] ? ipv4[k] : (ipv6[k] ? ipv6[k] : "-"))
	    p = products[k]; if (length(p) > 23) p = substr(p, 1, 22) "~"
	    v = versions[k]; if (length(v) > 21) v = substr(v, 1, 20) "~"
	    printf fmt, hosts[k], a, p, v, serials[k]
	}

	printf "\n%d device(s) found.\n", ndevs
    }
    '
}

llscan_ll()
{
    local iface="$1"

    printf "Scanning %s for IPv6 link-local neighbors ...\n\n" "$iface"
    printf "\033[7m%-6s %s\033[0m\n" "#" "ADDRESS"

    llping "$iface" -c3 -w3 2>/dev/null | awk '
    /bytes from/ {
	sub(/:$/, "", $4)
	addr = $4
	if (!seen[addr]++) {
	    printf "%-6d %s\n", ++n, addr
	}
    }
    END {
	printf "\n%d neighbor(s) found on '"$iface"'.\n", n+0
    }
    '
}
