#!/bin/sh

set -e

fixup_owner()
{
    for dir in $HOST_CHOWN_PATH; do
	chown -R $HOST_CHOWN_UID:$HOST_CHOWN_GID $dir
    done
}

trap fixup_owner EXIT

"$@"
