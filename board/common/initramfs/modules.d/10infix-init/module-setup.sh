#!/bin/bash

progs="dmsetup kpartx jq lvm sgdisk sfdisk veritysetup"

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
