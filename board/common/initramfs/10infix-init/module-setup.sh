#!/bin/bash

progs="blkid ddi dmsetup kpartx jq lvm openssl sgdisk veritysetup"

check() {
    require_binaries $progs || return 1

    return 0
}

depends() {
    return 0
}

install() {
    inst_multiple $progs
    inst '/etc/os-release'
    inst '/etc/rauc/keys/*'

    cp "$moddir/init" "${initdir?}/init"
}
