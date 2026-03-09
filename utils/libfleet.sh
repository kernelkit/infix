# Fleet management via RESTCONF for Infix devices.
#
# Requires: curl, jq

FLEET_CONFIG_DIR="${HOME}/.config/infix"
FLEET_CONFIG="${FLEET_CONFIG_DIR}/config.json"
FLEET_BAR_WIDTH=20
FLEET_TIMEOUT=600
FLEET_POLL=1


# Ensure the config directory and file exist, with safe permissions.
fleet_config_init()
{
    if [ ! -d "$FLEET_CONFIG_DIR" ]; then
	mkdir -p "$FLEET_CONFIG_DIR"
    fi
    if [ ! -f "$FLEET_CONFIG" ]; then
	printf '{"devices":{}}\n' > "$FLEET_CONFIG"
	chmod 600 "$FLEET_CONFIG"
    fi
}

# Usage: fleet_auth <name>
#
# Print "user:password" for a device.  Falls back to $LLSSH_USER:$LLSSH_PASS
# (set by the -A flag) if no password is stored for the device.
fleet_auth()
{
    local name="$1"
    local user pass

    user=$(jq -r --arg n "$name" '.devices[$n].user // "admin"' "$FLEET_CONFIG")
    pass=$(jq -r --arg n "$name" '.devices[$n].password // ""'  "$FLEET_CONFIG")

    if [ -z "$pass" ]; then
	if [ -n "$LLSSH_PASS" ]; then
	    user="${LLSSH_USER:-admin}"
	    pass="$LLSSH_PASS"
	else
	    printf "Error: no password for '%s' (use -A or enroll with -w)\n" \
		   "$name" >&2
	    return 1
	fi
    fi

    printf "%s:%s" "$user" "$pass"
}

# Usage: fleet_resolve <target>
#
# Print the device name(s) matching <target>, one per line.  <target> may be
# a device name or a profile name.  Exits non-zero with a message if neither.
fleet_resolve()
{
    local target="$1"
    local names

    fleet_config_init

    if jq -e --arg n "$target" '.devices[$n]' "$FLEET_CONFIG" >/dev/null 2>&1; then
	printf "%s\n" "$target"
	return 0
    fi

    names=$(jq -r --arg p "$target" \
	       '.devices | to_entries[]
		| select(.value.profile == $p) | .key' \
	       "$FLEET_CONFIG")
    if [ -n "$names" ]; then
	printf "%s\n" "$names"
	return 0
    fi

    printf "Error: '%s' is not a known device name or profile\n" "$target" >&2
    return 1
}

# Usage: fleet_rc_get <address> <auth> <path>
#
# RESTCONF GET, returns response body.
fleet_rc_get()
{
    local addr="$1" auth="$2" path="$3"

    curl -ks \
	 -u "$auth" \
	 -H "Accept: application/yang-data+json" \
	 "https://${addr}${path}"
}

# Usage: fleet_rc_post <address> <auth> <path> [<json-body>]
#
# RESTCONF POST, returns response body.
fleet_rc_post()
{
    local addr="$1" auth="$2" path="$3" data="${4:-}"

    if [ -n "$data" ]; then
	curl -ks -X POST \
	     -u "$auth" \
	     -H "Content-Type: application/yang-data+json" \
	     -H "Accept: application/yang-data+json" \
	     -d "$data" \
	     "https://${addr}${path}"
    else
	curl -ks -X POST \
	     -u "$auth" \
	     -H "Content-Type: application/yang-data+json" \
	     -H "Accept: application/yang-data+json" \
	     "https://${addr}${path}"
    fi
}

# Usage: fleet_rc_error <response>
#
# If <response> contains a RESTCONF error, print the message and return 0.
# Returns 1 (no error) otherwise.
fleet_rc_error()
{
    local resp="$1"
    local msg

    msg=$(printf "%s" "$resp" | \
	      jq -r 'if ."ietf-restconf:errors" then
			 ."ietf-restconf:errors".error[0]["error-message"]
			 // "unknown error"
		     else empty end' 2>/dev/null)
    if [ -n "$msg" ]; then
	printf "%s" "$msg"
	return 0
    fi
    return 1
}

# Usage: fleet_draw_bar <name> <pct> <msg>
#
# Print one progress bar line, clearing to end of line first.
fleet_draw_bar()
{
    local name="$1" pct="$2" msg="$3"
    local filled empty bar i

    filled=$(( pct * FLEET_BAR_WIDTH / 100 ))
    empty=$(( FLEET_BAR_WIDTH - filled ))
    bar=""

    i=0
    while [ "$i" -lt "$filled" ]; do
	bar="${bar}="
	i=$(( i + 1 ))
    done

    if [ "$filled" -lt "$FLEET_BAR_WIDTH" ]; then
	bar="${bar}>"
	empty=$(( empty - 1 ))
    fi

    i=0
    while [ "$i" -lt "$empty" ]; do
	bar="${bar} "
	i=$(( i + 1 ))
    done

    msg=$(printf "%.40s" "$msg")
    printf "\033[K%-16s [%s] %3d%% %s\n" "$name" "$bar" "$pct" "$msg"
}

# Usage: _fleet_upgrade_one <name> <addr> <auth> <url> <tmpdir>
#
# Background worker: POST install-bundle then poll installer state, writing
# results to <tmpdir>/<name>.{pct,msg,err}.
_fleet_upgrade_one()
{
    local name="$1" addr="$2" auth="$3" url="$4" tmpdir="$5"
    local resp err state pct msg elapsed

    set +e  # handle errors explicitly within this background worker

    resp=$(fleet_rc_post "$addr" "$auth" \
		"/restconf/operations/infix-system:install-bundle" \
		"{\"infix-system:input\":{\"url\":\"$url\"}}")

    if err=$(fleet_rc_error "$resp"); then
	printf "%s" "$err" > "${tmpdir}/${name}.err"
	return 1
    fi

    elapsed=0
    while [ "$elapsed" -lt "$FLEET_TIMEOUT" ]; do
	state=$(fleet_rc_get "$addr" "$auth" \
		    "/restconf/data/ietf-system:system-state/infix-system:software/installer")

	pct=$(printf "%s" "$state" | \
		  jq -r '.["infix-system:installer"].progress.percentage // 0')
	msg=$(printf "%s" "$state" | \
		  jq -r '.["infix-system:installer"].progress.message // ""')
	err=$(printf "%s" "$state" | \
		  jq -r '.["infix-system:installer"]["last-error"] // ""')

	[ "$pct" = "null" ] && pct=0
	[ "$msg" = "null" ] && msg=""
	[ "$err" = "null" ] && err=""

	if [ -n "$err" ]; then
	    printf "%s" "$err" > "${tmpdir}/${name}.err"
	    return 1
	fi

	# Atomic update of status files
	printf "%s" "$pct" > "${tmpdir}/${name}.pct.tmp" \
	    && mv "${tmpdir}/${name}.pct.tmp" "${tmpdir}/${name}.pct"
	printf "%s" "$msg" > "${tmpdir}/${name}.msg.tmp" \
	    && mv "${tmpdir}/${name}.msg.tmp" "${tmpdir}/${name}.msg"

	[ "$pct" -ge 100 ] && return 0

	sleep "$FLEET_POLL"
	elapsed=$(( elapsed + FLEET_POLL ))
    done

    printf "Timeout after %d seconds" "$elapsed" > "${tmpdir}/${name}.err"
    return 1
}

# Usage: fleet_upgrade <name|profile> <url>
#
# Upgrade one device or all devices in a profile, in parallel, showing a
# live per-device progress bar for each.
fleet_upgrade()
{
    local target="$1" url="$2"

    if [ -z "$target" ] || [ -z "$url" ]; then
	printf "Usage: ixll fleet upgrade <name|profile> <url>\n" >&2
	return 1
    fi

    fleet_config_init

    local devices name addr auth tmpdir ndevices first pct msg err all_done

    if ! devices=$(fleet_resolve "$target"); then
	return 1
    fi

    tmpdir=$(mktemp -d)
    active=""   # space-separated list of devices actually started

    # Initialise status files and launch one background poller per device.
    for name in $devices; do
	addr=$(jq -r --arg n "$name" '.devices[$n].address' "$FLEET_CONFIG")
	if ! auth=$(fleet_auth "$name" 2>/dev/null); then
	    printf "Skipping %s: no credentials\n" "$name" >&2
	    continue
	fi

	printf "0"         > "${tmpdir}/${name}.pct"
	printf "Starting." > "${tmpdir}/${name}.msg"
	printf ""          > "${tmpdir}/${name}.err"
	active="${active} ${name}"

	_fleet_upgrade_one "$name" "$addr" "$auth" "$url" "$tmpdir" &
    done

    active="${active# }"    # strip leading space
    ndevices=$(printf "%s\n" "$active" | wc -w)

    if [ "$ndevices" -eq 0 ]; then
	printf "No devices to upgrade.\n" >&2
	rm -rf "$tmpdir"
	return 1
    fi

    # Display loop: redraw all bars in place until every device is done.
    # Uses $active (not $devices) so skipped devices don't affect line count.
    first=1
    while true; do
	[ "$first" -eq 0 ] && printf "\033[%dA" "$ndevices"
	first=0
	all_done=1

	for name in $active; do
	    pct=$(cat "${tmpdir}/${name}.pct" 2>/dev/null || printf "0")
	    msg=$(cat "${tmpdir}/${name}.msg" 2>/dev/null || printf "Starting.")
	    err=$(cat "${tmpdir}/${name}.err" 2>/dev/null || printf "")

	    if [ -n "$err" ]; then
		printf "\033[K%-16s \033[31m[FAILED]\033[0m %s\n" "$name" "$err"
	    elif [ "$pct" -ge 100 ] 2>/dev/null; then
		printf "\033[K%-16s \033[32m[done]\033[0m\n" "$name"
	    else
		fleet_draw_bar "$name" "$pct" "$msg"
		all_done=0
	    fi
	done

	[ "$all_done" -eq 1 ] && break
	sleep 1
    done

    wait || true
    rm -rf "$tmpdir"
}

# Usage: fleet_backup [-o <dir>] <name|profile>
#
# Save the startup-config from one device or all devices in a profile.
# Single-device target: saved to current directory.
# Profile target: saved to ~/.config/infix/<profile>/<name>/.
fleet_backup()
{
    local outdir=

    OPTIND=1
    while getopts "o:" opt; do
	case $opt in
	    o) outdir="$OPTARG" ;;
	    *) ;;
	esac
    done
    shift $(( OPTIND - 1 ))

    local target="$1"
    if [ -z "$target" ]; then
	printf "Usage: ixll fleet backup [-o <dir>] <name|profile>\n" >&2
	return 1
    fi

    fleet_config_init

    # Determine if target is a single enrolled device or a profile.
    local single_device=0
    if jq -e --arg n "$target" '.devices[$n]' "$FLEET_CONFIG" \
	    >/dev/null 2>&1; then
	single_device=1
    fi

    local devices name addr auth profile iso_date outfile resp errmsg
    if ! devices=$(fleet_resolve "$target"); then
	return 1
    fi

    for name in $devices; do
	addr=$(jq -r --arg n "$name" '.devices[$n].address' "$FLEET_CONFIG")
	if ! auth=$(fleet_auth "$name" 2>/dev/null); then
	    printf "Skipping %s: no credentials\n" "$name" >&2
	    continue
	fi
	profile=$(jq -r --arg n "$name" '.devices[$n].profile // ""' \
		     "$FLEET_CONFIG")
	iso_date=$(date +%Y-%m-%dT%H:%M:%S)

	if [ -n "$outdir" ]; then
	    outfile="${outdir}/startup-config-${iso_date}.cfg"
	elif [ "$single_device" -eq 1 ]; then
	    outfile="startup-config-${iso_date}.cfg"
	else
	    outfile="${FLEET_CONFIG_DIR}/${profile}/${name}/startup-config-${iso_date}.cfg"
	    mkdir -p "$(dirname "$outfile")"
	fi

	printf "Backing up %-16s ... " "$name"

	resp=$(fleet_rc_get "$addr" "$auth" "/restconf/ds/ietf-datastores:startup")

	if errmsg=$(fleet_rc_error "$resp"); then
	    printf "\033[31mFAILED\033[0m: %s\n" "$errmsg"
	    continue
	fi

	printf "%s" "$resp" | jq . > "$outfile"
	printf "saved to %s\n" "$outfile"
    done
}

# Usage: fleet_reboot <name|profile>
#
# Reboot one device or all devices in a profile (fire-and-forget).
fleet_reboot()
{
    local target="$1"

    if [ -z "$target" ]; then
	printf "Usage: ixll fleet reboot <name|profile>\n" >&2
	return 1
    fi

    fleet_config_init

    local devices name addr auth
    if ! devices=$(fleet_resolve "$target"); then
	return 1
    fi

    for name in $devices; do
	addr=$(jq -r --arg n "$name" '.devices[$n].address' "$FLEET_CONFIG")
	if ! auth=$(fleet_auth "$name" 2>/dev/null); then
	    printf "Skipping %s: no credentials\n" "$name" >&2
	    continue
	fi

	printf "Rebooting %-16s ... " "$name"
	fleet_rc_post "$addr" "$auth" \
		      "/restconf/operations/ietf-system:system-restart" \
		      >/dev/null 2>&1
	printf "done\n"
    done
}

# Usage: fleet_enroll [-d] [-p <profile>] [-u <user>] [-w <password>]
#                     <name> [<address>]
#
# Enroll a device, or delete it with -d.
fleet_enroll()
{
    local delete=0 profile= user=admin password=

    OPTIND=1
    while getopts "dp:u:w:" opt; do
	case $opt in
	    d) delete=1 ;;
	    p) profile="$OPTARG" ;;
	    u) user="$OPTARG" ;;
	    w) password="$OPTARG" ;;
	    *)
		printf "Usage: ixll fleet enroll [-d] [-p profile] [-u user] [-w password] <name> [<address>]\n" >&2
		return 1
		;;
	esac
    done
    shift $(( OPTIND - 1 ))

    local name="$1" address="$2"

    if [ -z "$name" ]; then
	printf "Usage: ixll fleet enroll [-d] [-p profile] [-u user] [-w password] <name> [<address>]\n" >&2
	return 1
    fi

    fleet_config_init

    if [ "$delete" -eq 1 ]; then
	if ! jq -e --arg n "$name" '.devices[$n]' "$FLEET_CONFIG" \
		>/dev/null 2>&1; then
	    printf "Error: device '%s' not found\n" "$name" >&2
	    return 1
	fi
	jq --arg n "$name" 'del(.devices[$n])' \
	   "$FLEET_CONFIG" > "${FLEET_CONFIG}.tmp" \
	    && mv "${FLEET_CONFIG}.tmp" "$FLEET_CONFIG"
	printf "Device '%s' removed.\n" "$name"
	return 0
    fi

    if [ -z "$address" ]; then
	printf "Usage: ixll fleet enroll [-p profile] [-u user] [-w password] <name> <address>\n" >&2
	return 1
    fi

    jq --arg n "$name" \
       --arg a "$address" \
       --arg u "$user" \
       --arg p "${profile:-}" \
       --arg w "${password:-}" \
       '.devices[$n] = {address: $a, user: $u}
	| if $p != "" then .devices[$n].profile = $p else . end
	| if $w != "" then .devices[$n].password = $w else . end' \
       "$FLEET_CONFIG" > "${FLEET_CONFIG}.tmp" \
	&& mv "${FLEET_CONFIG}.tmp" "$FLEET_CONFIG"

    printf "Device '%s' enrolled at %s.\n" "$name" "$address"
}

# Usage: fleet_list
#
# List all enrolled devices.
fleet_list()
{
    fleet_config_init

    local count
    count=$(jq '.devices | length' "$FLEET_CONFIG")

    if [ "$count" -eq 0 ]; then
	printf "No devices enrolled. Use 'ixll fleet enroll' to add one.\n"
	return 0
    fi

    printf "\033[7m%-12s %-16s %-22s %-8s %s\033[0m\n" \
	   "PROFILE" "NAME" "ADDRESS" "USER" "PASS"

    jq -r '.devices | to_entries[] |
	   [(.value.profile // "(none)"),
	    .key,
	    .value.address,
	    (.value.user // "admin"),
	    (if .value.password then "*" else "-" end)] |
	   join("\t")' "$FLEET_CONFIG" | \
    while IFS=$(printf '\t') read -r profile name address user pass; do
	printf "%-12s %-16s %-22s %-8s %s\n" \
	       "$profile" "$name" "$address" "$user" "$pass"
    done
}

# Usage: fleet_main <subcommand> [<args>]
#
# Dispatch fleet subcommands.
fleet_main()
{
    if [ $# -lt 1 ]; then
	printf "Usage: ixll fleet <enroll|list|upgrade|backup|reboot>\n" >&2
	return 1
    fi

    local subcmd="$1"
    shift

    case "$subcmd" in
	enroll)  fleet_enroll  "$@" ;;
	list)    fleet_list    "$@" ;;
	upgrade) fleet_upgrade "$@" ;;
	backup)  fleet_backup  "$@" ;;
	reboot)  fleet_reboot  "$@" ;;
	*)
	    printf "Unknown fleet command '%s'\n" "$subcmd" >&2
	    return 1
	    ;;
    esac
}
