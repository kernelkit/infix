ixmsg()
{
    printf "\e[37;44m#!:   $@\e[0m\n"
}

die()
{
    echo "$@" >&2
    exit 1
}

# Find all matching key=value assignments in output/.config
# E.g., load_cfg DISK_IMAGE sets the following variables:
#
#      DISK_IMAGE=y
#      DISK_IMAGE_SIZE="512"
#      etc.
#
# Nested variables, like INFIX_COMPATIBLE="${INFIX_IMAGE_ID}"
# are handled by sourcing the file in a subshell.
#
# shellcheck disable=SC1090
load_cfg()
{
    tmp=$(mktemp -p /tmp)
    (
	. "$BR2_CONFIG" 2>/dev/null

	# Set *all* matching variables
	set | grep -E "^${1}[^=]*=" | while IFS= read -r line; do
            echo "$line"
	done
    ) > "$tmp"

    .  "$tmp"
    rm "$tmp"
}
