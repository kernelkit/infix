if load ${devtype} ${devnum}:${auxpart} ${ramdisk_addr_r} /${slot}.itbh; then
    setenv old_fdt_addr ${fdt_addr}
    if fdt addr ${ramdisk_addr_r}; then
	fdt get value sqoffs /images/rootfs data-position
	fdt get value sqsize /images/rootfs data-size
	fdt addr ${old_fdt_addr}

	setexpr sqaddr    ${ramdisk_addr_r} + ${sqoffs}
	setexpr sqblkcnt  ${sqsize} / 0x200

	if part start ${devtype} ${devnum} ${slot} devoffs; then
	    if ${devtype} read ${sqaddr} ${devoffs} ${sqblkcnt}; then
		setenv bootargs_root "root=PARTLABEL=${slot}"
		setenv prepared ok
	    fi
	fi
    fi
fi
