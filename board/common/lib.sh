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
# shellcheck disable=SC1090
load_cfg()
{
    tmp=$(mktemp -p /tmp)

    grep -E "${1}.*=" "$BR2_CONFIG" >"$tmp"
    .  "$tmp"

    rm "$tmp"
}
