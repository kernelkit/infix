if part number mmc 0 aux auxpart; then
    if load mmc 0:${auxpart} ${loadaddr} /uboot.env; then
	env import -b ${loadaddr} ${filesize} BOOT_ORDER DEBUG
    fi
fi
