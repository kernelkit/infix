ixmsg()
{
    printf "\e[37;44m#!:   $@\e[0m\n"
}

die()
{
    echo "$@" >&2
    exit 1
}

load_cfg()
{
    local tmp=$(mktemp -p /tmp)

    grep "$1" $BR2_CONFIG >$tmp
    .  $tmp
    rm $tmp
}
