if dhcp ${ramdisk_addr_r}; then

    setexpr rdsize ${filesize} + 0x3ff

    setenv old_fdt_addr ${fdt_addr}
    if fdt addr ${ramdisk_addr_r}; then
	fdt get value sqoffs /images/rootfs data-position
	fdt get value sqsize /images/rootfs data-size
	fdt addr ${old_fdt_addr}

	setexpr sqaddr    ${ramdisk_addr_r} + ${sqoffs}
	setexpr sqblknr   ${sqoffs} / 0x200
	setexpr sqblkcnt  ${sqsize} / 0x200
	setexpr rdsize    ${sqsize} / 0x400

	setenv bootargs_root "root=/dev/ram0 ramdisk_size=0x${rdsize}"
	setenv ramdisk "${sqaddr}:${sqsize}"
	setenv prepared ok
    fi
fi
