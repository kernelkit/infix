set -e

usage()
{
    local me="$(basename $0)"

    cat <<EOF
usage: $me [<command>]

Verify that cli_pretty produces the expected output for $cli_commands,
from the operational data in operational.json.

  Commands:
    If no command is specified it defaults to "check"

    gen
      Run $gen_test, read $oper_xpaths from the operational
      datastore of the Infix device behind $gen_iface into
      operational.json, and update the expected cli_pretty output

    check
      Verify that cli_pretty produces the expected output for
      $cli_commands, based on the current operational.json

    oper gen
      Like gen, without updating the expected cli_pretty output

    oper live
      Read $oper_xpaths from the device behind $gen_iface, and write
      it to stdout

    cli gen
      Update the expected output of $cli_commands from the current
      operational.json

    cli check
      Same as check

    cli live
      Run $cli_commands on operational data read from the device
      behind $gen_iface

EOF
}

casedir=$(readlink -f $(dirname $0))
ixdir=$(readlink -f $casedir/../../../../)

CLI=${CLI:-$ixdir/src/statd/python/cli_pretty/cli_pretty.py}

wrapper="$ixdir/utils/ixll -A ssh $gen_iface sudo"

# Make sure topology matchings are stable since we capture system
# states by referencing a _physical_ interface, which must match the
# expected _logical_ node.
export PYTHONHASHSEED=0

exitcode=0
n_steps=0
step()
{
    local status="$1"
    shift

    if [ "$status" != "ok" ]; then
	exitcode=1
    fi

    n_steps=$((n_steps+1))
    echo "$status $n_steps - $*"
}

plan()
{
    echo "$n_steps..$n_steps"
    exit $exitcode
}

oper_live()
{
    local files=
    local json=

    for xpath in $oper_xpaths; do
	json=$(mktemp)
	$wrapper copy operational -x "$xpath" >"$json"
	files="$files $json"
    done

    cat $files | jq --slurp --sort-keys 'reduce .[] as $item ({}; . * $item)'
    rm $files
}

oper_gen()
{
    if ! [ "$gen_test" ] || ! [ "$gen_iface" ]; then
	echo "!!! $(basename $casedir) has no gen_test, operational.json is hand written" >&2
	false
    fi

    echo ">>> RUNNING $gen_test" >&2
    $ixdir/test/case/$gen_test

    echo ">>> READING $oper_xpaths from $gen_iface" >&2
    oper_live >"$casedir/operational.json"
    echo ">>> OK" >&2
}

oper()
{
    local cmd=live

    if [ $# -gt 0 ]; then
	cmd="$1"
	shift
    fi

    case "$cmd" in
	gen|live)
	    "oper_$cmd" "$@"
	    return
	    ;;
    esac

    false
}


cli_exec()
{
    $CLI -t $(echo "$1" | tr '_' ' ')
}

cli_gen()
{
    [ "$cli_commands" ] || return 0

    mkdir -p "$casedir/cli"

    set $cli_commands
    while [ $# -gt 0 ]; do
	cli_exec "$1" <"$casedir/operational.json" >"$casedir/cli/$1"
	shift
    done
}

cli_check()
{
    local diff=
    local status=

    [ "$cli_commands" ] || return 0

    set $cli_commands
    while [ $# -gt 0 ]; do
	diff="$(mktemp)"
	status="ok"
	if ! diff -up \
	     "$casedir/cli/$1" \
	     <(cli_exec "$1" <"$casedir/operational.json") \
	     >"$diff"; then
	    cat $diff | sed 's/^/# /'
	    status="not ok"
	fi
	rm $diff
	step "$status" \
	     "cli output of \"$1\" matches cli/$1"

	shift
    done
}

cli_cat()
{
    if [ $# -eq 0 ]; then
	set $cli_commands
    fi

    while [ $# -gt 0 ]; do
	echo ">>> Running $1" >&2
	cli_exec "$1" <"$casedir/operational.json"
	shift
    done
}

cli_live()
{
    local json=$(mktemp)
    echo ">>> Reading live operational data" >&2
    oper_live >$json
    echo ">>> OK" >&2

    if [ $# -eq 0 ]; then
	set $cli_commands
    fi

    while [ $# -gt 0 ]; do
	echo ">>> Running $1" >&2
	cli_exec "$1" <$json
	shift
    done

    rm $json
}

cli()
{
    local cmd=check

    if [ $# -gt 0 ]; then
	cmd="$1"
	shift
    fi

    case "$cmd" in
	cat|gen|live)
	    "cli_$cmd" "$@"
	    return
	    ;;
	check)
	    cli_check "$@"
	    plan
	    ;;
    esac

    false
}

main()
{
    local cmd=check

    if [ $# -gt 0 ]; then
	cmd="$1"
	shift
    fi

    case $cmd in
	gen)
	    oper_gen && cli_gen
	    ;;
	check)
	    cli_check
	    plan
	    ;;
	oper|cli)
	    "$cmd" "$@"
	    ;;
	-h|--help|help)
	    usage
	    ;;
	*)
	    echo "!!! Unknown command \"$cmd\"" >&2
	    usage >&2
	    exit 1
	    ;;
    esac
}
