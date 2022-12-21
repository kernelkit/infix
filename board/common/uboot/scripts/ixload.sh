${devtype} dev ${devnum}
${devtype} read ${ramdisk_addr_r} ${devoffs} 0x10

fdt addr ${ramdisk_addr_r}
fdt get value squashaddr /images/ramdisk data-position
fdt get value squashsize /images/ramdisk data-size

setexpr blkcnt ${squashaddr} + ${squashsize}
setexpr blkcnt ${blkcnt} + 0x1ff
setexpr blkcnt ${blkcnt} / 0x200

${devtype} read ${ramdisk_addr_r} ${devoffs} ${blkcnt}


