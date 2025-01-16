set -e

usage()
{
    local me="$(basename $0)"

    cat <<EOF
usage: $me [<command>]

Verify that yanger produces the expected operational data for
$yang_models, and that cli_pretty produces the expected output for
$cli_commands.

  Commands:
    If no command is specified it defaults to "check"

    gen
      (Re)generate system state, operational.json and expected
      cli_pretty output based on $gen_test

    check
      Check yanger and CLI output

    yanger gen
      Run $gen_test and capture the system status from the Infix
      device behind $gen_iface

    yanger check
      Verify that yanger produces the operational data previously
      recorded in operational.json

    yanger live
      Produce the operational data for $yang_models from the Infix
      device behind $gen_iface, and write it to stdout

    cli gen
      Update the expected output of $cli_commands from the current
      operational.json

    cli check
      Verify that cli_pretty produces the expected output for
      $cli_commands, based on the current operational.json

EOF
}

casedir=$(readlink -f $(dirname $0))
ixdir=$(readlink -f $casedir/../../../../)

YANGER=${YANGER:-$ixdir/src/statd/python/yanger/yanger}
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

prepare_infamy_test()
{
    $ixdir/test/case/meta/wait.py && \
	$ixdir/test/case/$1
}

yanger_exec()
{
    local opts=
    while getopts "w:t:" opt; do
	case ${opt} in
	    w)
		opts="$opts -w \"$OPTARG\""
		;;
	    t)
		opts="$opts -t \"$OPTARG\""
		;;
	esac
    done
    shift $((OPTIND - 1))

    eval "$YANGER" $opts "$1"
}

yanger_gen()
{
    local operfiles=

    if type gen_exec &>/dev/null; then
	gen_exec
	return
    fi

    if ! [ "$gen_test" ] && [ "$gen_iface" ]; then
	echo "!!! Not implemented" >&2
	false
    fi

    echo ">>> RUNNING $gen_test" >&2
    prepare_infamy_test "$gen_test"
    set $yang_models
    while [ $# -gt 0 ]; do
	echo ">>> CAPTURING $1 from $gen_iface" >&2
	yanger_exec \
	    -t "$casedir/system" \
	    -w "$wrapper" \
	    $1 \
	    >"$casedir/$1.json"
 	operfiles="$operfiles $casedir/$1.json"
	echo ">>> OK" >&2
	shift
    done

    echo ">>> GENERATING operational.json" >&2
    cat $operfiles | jq --slurp --sort-keys \
       'reduce .[] as $item ({}; . * $item)' \
       >"$casedir/operational.json"
    echo ">>> OK" >&2
}

yanger_check()
{
    local status=
    local diff=$(mktemp)

    set $yang_models
    while [ $# -gt 0 ]; do
	status="ok"
	if ! diff -up \
	     "$casedir/$1.json" \
	     <(yanger_exec -t "$casedir/system" $1) \
	     >"$diff"; then
	    cat $diff | sed 's/^/# /'
	    status="not ok"
	fi

	rm $diff
	step "$status" \
	     "yanger output of \"$1\" matches $1.json"
	shift
    done
}

yanger_cat()
{
    if [ $# -eq 0 ]; then
	set $yang_models
    fi

    while [ $# -gt 0 ]; do
	yanger_exec -t "$casedir/system" $1
	shift
    done
}

yanger_live()
{
    if [ $# -eq 0 ]; then
	set $yang_models
    fi

    while [ $# -gt 0 ]; do
	yanger_exec -w "$wrapper" $1
	shift
    done
}

yanger()
{
    local cmd=check

    if [ $# -gt 0 ]; then
	cmd="$1"
	shift
    fi

    case "$cmd" in
	cat|gen|live)
	    "yanger_$cmd" "$@"
	    return
	    ;;
	check)
	    yanger_check "$@"
	    plan
    esac

    false
}


cli_exec()
{
    $CLI -t $(echo "$1" | tr '_' ' ')
}

cli_gen()
{
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
    echo ">>> Capturing live state" >&2
    yanger_live >$json
    echo ">>> OK" >&2

    if [ $# -eq 0 ]; then
	set $cli_commands
    fi

    while [ $# -gt 0 ]; do
	echo ">>> Running $1" >&2
	cli_exec "$1" <"$casedir/cli/$1"
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
	    yanger gen && cli gen
	    ;;
	check)
	    yanger_check && cli_check
	    plan
	    ;;
	yanger|cli)
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
