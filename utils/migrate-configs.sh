#!/bin/sh
# Run static .cfg files in br2-external trees through confd migrate.

infix=$(dirname "$(dirname "$(readlink -f "$0")")")
migrate="$infix/src/confd/bin/migrate"
scripts="$infix/src/confd/share/migrate"

usage()
{
    cat <<EOF
Usage: $(basename "$0") [-h] [DIR...]

Run all static configuration files, *-config.cfg, found in each DIR
through the confd migrate script.  The files are edited in place, use
'git diff' to review the result.

DIR defaults to the br2-external trees listed in \$BR2_EXTERNAL, which
for a spin includes both Infix and the spin itself.
EOF
}

version()
{
    jq -r '.["infix-meta:meta"].version // "0.0"' "$1" 2>/dev/null
}

cleanup()
{
    rm -f "$list" "$tmp"
}

while getopts "h" opt; do
    case $opt in
	h)
	    usage
	    exit 0
	    ;;
	*)
	    usage >&2
	    exit 1
	    ;;
    esac
done
shift $((OPTIND - 1))

if [ $# -eq 0 ]; then
    # shellcheck disable=SC2046
    set -- $(echo "$BR2_EXTERNAL" | tr ':' ' ')
fi
if [ $# -eq 0 ]; then
    usage >&2
    exit 1
fi

for cmd in jq git; do
    if ! command -v "$cmd" >/dev/null; then
	echo "Error: $cmd not found, please install it, e.g., 'sudo apt install $cmd'" >&2
	exit 1
    fi
done

list=$(mktemp)
tmp=$(mktemp)
trap cleanup INT HUP TERM EXIT

rc=0
for dir in "$@"; do
    if ! git -C "$dir" -c core.quotepath=off ls-files -co --exclude-standard \
	 -- '*-config.cfg' > "$list" 2>/dev/null; then
	echo "Error: $dir is not a git repository, skipping." >&2
	rc=1
	continue
    fi

    echo "Migrating static configs in $dir"
    while IFS= read -r file; do
	cfg="$dir/$file"
	[ -f "$cfg" ] || continue

	if ! STATIC_CONFIG=1 FACTORY_CONFIG="$cfg" sh "$migrate" -e -q -s "$scripts" "$cfg" > "$tmp"; then
	    echo "  $file: failed"
	    rc=1
	    continue
	fi

	# No output, already at latest version
	if [ ! -s "$tmp" ]; then
	    echo "  $file: $(version "$cfg"), up to date"
	    continue
	fi

	old=$(version "$cfg")
	cat "$tmp" > "$cfg"
	echo "  $file: $old -> $(version "$cfg")"
    done < "$list"
done

exit $rc
