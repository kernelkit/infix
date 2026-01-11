#!/bin/sh
# DHCPv6 client state update script for odhcp6c
# This script expects a system with resolvconf (openresolv) and iproute2

[ -z "$1" ] && echo "Error: should be called from odhcp6c" && exit 1

interface="$1"
state="$2"
RESOLV_CONF="/run/resolvconf/interfaces/${interface}-ipv6.conf"
NTPFILE="/run/chrony/dhcp-sources.d/${interface}-ipv6.sources"

[ -n "$metric" ] || metric=5

log()
{
	logger -I $$ -t odhcp6c -p user.notice "${interface}: $*"
}

dbg()
{
	logger -I $$ -t odhcp6c -p user.debug "${interface}: $*"
}

err()
{
	logger -I $$ -t odhcp6c -p user.err "${interface}: $*"
}

teardown_interface()
{
	ip -6 route flush dev "$interface"
	ip -6 address flush dev "$interface" scope global
}

setup_interface()
{
	# Merge RA addresses with DHCP addresses
	for entry in $RA_ADDRESSES; do
		duplicate=0
		addr="${entry%%/*}"
		for dentry in $ADDRESSES; do
			daddr="${dentry%%/*}"
			[ "$addr" = "$daddr" ] && duplicate=1
		done
		[ "$duplicate" = "0" ] && ADDRESSES="$ADDRESSES $entry"
	done

	# Add addresses
	for entry in $ADDRESSES; do
		addr="${entry%%,*}"
		entry="${entry#*,}"
		preferred="${entry%%,*}"
		entry="${entry#*,}"
		valid="${entry%%,*}"

		ip -6 address add "$addr" dev "$interface" preferred_lft "$preferred" valid_lft "$valid" proto dhcp
		log "assigned address $addr (preferred=$preferred, valid=$valid)"
	done

	# Add routes from RA
	for entry in $RA_ROUTES; do
		addr="${entry%%,*}"
		entry="${entry#*,}"
		gw="${entry%%,*}"
		entry="${entry#*,}"
		valid="${entry%%,*}"
		entry="${entry#*,}"
		metric="${entry%%,*}"

		if [ -n "$gw" ]; then
			ip -6 route add "$addr" via "$gw" metric "$metric" dev "$interface" from "::/128"
		else
			ip -6 route add "$addr" metric "$metric" dev "$interface"
		fi

		# Add routes for delegated prefixes
		for prefix in $PREFIXES; do
			paddr="${prefix%%,*}"
			[ -n "$gw" ] && ip -6 route add "$addr" via "$gw" metric "$metric" dev "$interface" from "$paddr"
		done
	done
}

handle_prefixes()
{
	# $PREFIXES format: "prefix/len,preferred,valid[,class=N][,excluded=...] ..."
	for entry in $PREFIXES; do
		addr="${entry%%,*}"
		entry="${entry#*,}"
		preferred="${entry%%,*}"
		entry="${entry#*,}"
		valid="${entry%%,*}"

		log "received delegated prefix $addr (preferred=$preferred, valid=$valid)"

		# Add unreachable route to prevent routing loops
		ip -6 route add unreachable "$addr" 2>/dev/null

		# Future: Distribute to downstream interfaces
	done
}

handle_dns()
{
	truncate -s 0 "$RESOLV_CONF"

	# Combine DHCPv6 DNS ($RDNSS) and RA DNS ($RA_DNS), deduplicating
	all_dns=""
	for server in $RDNSS $RA_DNS; do
		# Simple deduplication: only add if not already in list
		case " $all_dns " in
			*" $server "*) ;;
			*) all_dns="$all_dns $server" ;;
		esac
	done

	# Domain search list (DHCPv6 option 24)
	if [ -n "$DOMAINS" ]; then
		dbg "adding search domains: $DOMAINS"
		echo "search $DOMAINS # $interface" >> "$RESOLV_CONF"
	fi

	# DNS servers
	for server in $all_dns; do
		[ -z "$server" ] && continue
		dbg "adding dns $server"
		echo "nameserver $server # $interface" >> "$RESOLV_CONF"
	done

	if [ -n "$all_dns" ]; then
		resolvconf -u
	fi
}

handle_ntp()
{
	# DHCPv6 option 56 (NTP server) is provided as $OPTION_56 in hex format
	# Format: sub-option-code (2 bytes) + length (2 bytes) + data
	# Sub-option 1 = NTP server address (16 bytes IPv6)
	#
	# This is complex to parse in shell. For now, we attempt basic parsing
	# and fall back to logging a warning if the format is unexpected.

	if [ -n "$OPTION_56" ]; then
		# Remove all non-hex characters (spaces, colons, etc.) and convert to lowercase
		hex=$(echo "$OPTION_56" | tr -d '[:space:]:-' | tr '[:upper:]' '[:lower:]')

		truncate -s 0 "$NTPFILE"
		ntp_found=0

		# Parse option 56: iterate through sub-options
		# Each sub-option: 2 bytes code + 2 bytes length + data
		pos=0
		while [ $pos -lt ${#hex} ]; do
			# Need at least 4 hex chars (2 bytes) for sub-option code
			[ $((${#hex} - pos)) -lt 4 ] && break

			# Extract sub-option code (2 bytes = 4 hex chars)
			subopt_code=$(echo "$hex" | cut -c $((pos+1))-$((pos+4)))
			pos=$((pos + 4))

			# Need 4 more hex chars for length
			[ $((${#hex} - pos)) -lt 4 ] && break

			# Extract length (2 bytes = 4 hex chars)
			subopt_len_hex=$(echo "$hex" | cut -c $((pos+1))-$((pos+4)))
			subopt_len=$(printf "%d" "0x$subopt_len_hex")
			pos=$((pos + 4))

			# Sub-option 1 = NTP server address (should be 16 bytes for IPv6)
			if [ "$subopt_code" = "0001" ] && [ "$subopt_len" -eq 16 ]; then
				# Extract 16 bytes (32 hex chars) for IPv6 address
				addr_hex=$(echo "$hex" | cut -c $((pos+1))-$((pos+32)))

				# Convert hex to IPv6 address format
				# Format: 0123456789abcdef0123456789abcdef -> 0123:4567:89ab:cdef:0123:4567:89ab:cdef
				ipv6=$(echo "$addr_hex" | sed 's/\(....\)\(....\)\(....\)\(....\)\(....\)\(....\)\(....\)\(....\)/\1:\2:\3:\4:\5:\6:\7:\8/')

				dbg "got NTP server $ipv6"
				echo "server $ipv6 iburst" >> "$NTPFILE"
				ntp_found=1
			fi

			# Skip this sub-option's data
			pos=$((pos + subopt_len * 2))
		done

		if [ "$ntp_found" -eq 1 ]; then
			chronyc reload sources >/dev/null 2>&1
		else
			dbg "option 56 received but no NTP server addresses found (consider using option 31/SNTP)"
		fi
	fi
}

log "state: $state"

(
	flock 9
	case "$state" in
		started)
			# Initial state - clean up any stale config
			teardown_interface
			;;

		bound)
			# Fresh lease - tear down and set up from scratch
			teardown_interface
			setup_interface
			handle_prefixes
			handle_dns
			handle_ntp
			;;

		informed|updated|rebound|ra-updated)
			# Update existing configuration
			setup_interface
			[ -n "$PREFIXES" ] && handle_prefixes
			handle_dns
			handle_ntp
			;;

		unbound|stopped)
			# Lost server or client stopped
			teardown_interface
			rm -f "$RESOLV_CONF"
			rm -f "$NTPFILE"
			resolvconf -u
			chronyc reload sources >/dev/null 2>&1
			;;
	esac
) 9>/tmp/odhcp6c.lock.${interface}
rm -f /tmp/odhcp6c.lock.${interface}

# Run hooks
HOOK_DIR="/usr/libexec/odhcp6c.d"
for hook in "${HOOK_DIR}/"*; do
	[ -f "${hook}" -a -x "${hook}" ] || continue
	"${hook}" "$interface" "$state"
done

exit 0
