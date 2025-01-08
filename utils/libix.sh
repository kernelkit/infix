
resolve_o()
{
    [ -n "$O" ] && return

    if [ -f ".config" ] && [ -d "output" ]; then
	# Buildroot
	O=./output
    elif [ -f "output/.config" ]; then
	# BR2_EXTERNAL
	O=./output
    elif [ -f ".config" ] && [ -d "host" ]; then
	# Called from inside output/ directory
	O=.
    else
	echo "*** Error: cannot find Buildroot output dir!" >&2
	exit 1
    fi
}

resolve_host_dir()
{
    [ -n "$HOST_DIR" ] && return

    resolve_o || exit 1

    if ! [ -d "$O/host" ]; then
	echo "*** Error: cannot find Buildroot host binaries dir!" >&2
	exit 1
    fi

    HOST_DIR="$O/host"
}
