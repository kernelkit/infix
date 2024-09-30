setenv autoload no

if test -n "${ipaddr}" -a -n "${netmask}" -a -n "${serverip}" -a -n "${bootfile}" || dhcp; then
    setenv proto tftp
    setenv dltool tftpboot

    if setexpr proto sub "^(http|tftp)://.*" "\\1" "${bootfile}" && setexpr bootfile sub "^(http|tftp)://([^/]+?)/(.*)" "\2:\3"; then
	if test "${proto}" = "http"; then
	    setenv dltool wget
	fi
    fi

    if ${dltool} ${ramdisk_addr_r} "${bootfile}"; then
	setenv old_fdt_addr ${fdt_addr}
	if fdt addr ${ramdisk_addr_r}; then
	    fdt get value sqoffs /images/rootfs data-position
	    fdt get value sqsize /images/rootfs data-size
	    fdt addr ${old_fdt_addr}

	    setexpr sqaddr    ${ramdisk_addr_r} + ${sqoffs}
	    setexpr sqblkcnt  ${sqsize} / 0x200
	    setexpr sqkbsize  ${sqsize} / 0x400

	    setenv bootargs_root "root=/dev/ram0 ramdisk_size=0x${sqkbsize} initrd=0x${sqaddr},${sqsize}"
	    setenv prepared ok
	fi
    fi
fi
