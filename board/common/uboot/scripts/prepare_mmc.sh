setenv devtype mmc
setenv devnum 0
part start ${devtype} ${devnum} ${slot} devoffs

${devtype} dev ${devnum}
${devtype} read ${ramdisk_addr_r} ${devoffs} 0x10

setenv bootargs_root "root=/dev/dm-0 dm-mod.create=\"rootfs,,0,ro,0"

setenv old_fdt_addr ${fdt_addr}
if fdt addr ${ramdisk_addr_r}; then
    fdt get value sqoffs /images/rootfs data-position
    fdt get value sqsize /images/rootfs data-size
    fdt addr ${old_fdt_addr}

    setexpr sqaddr    ${ramdisk_addr_r} + ${sqoffs}
    setexpr sqblknr   ${sqoffs} / 0x200
    setexpr sqblkcnt  ${sqsize} / 0x200
    setexpr fitblkcnt ${sqblknr} + ${sqblkcnt}

    if ${devtype} read ${ramdisk_addr_r} ${devoffs} ${fitblkcnt}; then
	setexpr decsqblkcnt fmt %u ${sqblkcnt}
	setexpr decsqblknr  fmt %u ${sqblknr}
	setenv bootargs_root "${bootargs_root} ${decsqblkcnt} linear PARTLABEL=${slot} ${decsqblknr}\""
	setenv ramdisk "-"
	setenv prepared ok
    fi
fi
