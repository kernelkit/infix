#!/bin/sh

KDIR=$1
KTAG=$2
PDIR=$3

usage()
{
    cat <<EOF
usage: $0 <kernel-dir> [<kernel-tag>] [<patch-dir>]

Synchronize patches directory with changes from a GIT versioned kernel
tree.

  <kernel-dir>  Path to kernel tree.

  <kernel-tag>  Base tag from which to generate patches. Default: "v"
                followed by the value of BR2_LINUX_KERNEL_VERSION in
                the current configuration.

  <patch-dir>   Path to kernel patches directory. Default: the value of
                BR2_EXTERNAL_INFIX_PATH in the current configuration,
                followed by "/patches/linux".
EOF
}

getconfig()
{
    [ -f .config ] || return 1

    grep "$1=" .config | sed -e "s/$1=\"\([^\"]\+\)\"/\1/"
}

if [ $# -lt 1 ]; then
    usage
    exit 1
fi

if ! [ "$KTAG" ]; then
     ixkver=$(getconfig BR2_LINUX_KERNEL_VERSION)
    [ "$ixkver" ] && KTAG="v$ixkver"
else
    ixkver=${KTAG#v}
fi

if ! [ "$KTAG" ]; then
    echo "Kernel tag was not supplied, and could not be inferred" >&2
    usage
    exit 1
fi

if ! [ "$PDIR" ]; then
    ixdir=$(getconfig BR2_EXTERNAL_INFIX_PATH)
    [ "$ixdir" ] && PDIR="$ixdir/patches/linux/$ixkver"
fi

if ! [ "$PDIR" ]; then
    echo "Patch directory was not supplied, and could not be inferred" >&2
    usage
    exit 1
fi

KDIR=$(readlink -f $KDIR)
PDIR=$(readlink -f $PDIR)

git ls-files --error-unmatch $PDIR 1>/dev/null 2>&1 && git -C $PDIR rm *.patch
git -C $KDIR format-patch -o $PDIR $KTAG..HEAD
git -C $PDIR add *.patch
