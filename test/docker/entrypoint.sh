#!/bin/sh
# Run the command as the user who started the container, so that files
# written to bind mounts are owned by them.  test/env passes the caller's
# UID/GID when the runtime is Docker; the `infamy` user from the image is
# remapped to match and we drop to it, keeping only the capabilities the
# test suite needs.  Without them (rootless podman) root already is the
# caller, so just run as is.

set -e

if [ -n "$INFAMY_UID" ]; then
    if [ "$(id -u infamy):$(id -g infamy)" != "$INFAMY_UID:$INFAMY_GID" ]; then
	sed -i "s/^infamy:x:[0-9]*:[0-9]*:/infamy:x:$INFAMY_UID:$INFAMY_GID:/" /etc/passwd
	sed -i "s/^infamy:x:[0-9]*:/infamy:x:$INFAMY_GID:/" /etc/group
	chown -R infamy:infamy /home/infamy
    fi

    # /dev/kvm keeps its host group, let infamy use it
    if [ -c /dev/kvm ]; then
	gid=$(stat -c %g /dev/kvm)
	getent group "$gid" >/dev/null || addgroup -g "$gid" kvm-host
	addgroup infamy "$(getent group "$gid" | cut -d: -f1)"
    fi

    exec setpriv --reuid=infamy --regid=infamy --init-groups \
	 --inh-caps=+net_admin,+net_raw --ambient-caps=+net_admin,+net_raw \
	 -- "$@"
fi

exec "$@"
