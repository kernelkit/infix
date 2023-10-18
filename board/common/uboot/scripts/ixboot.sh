for tgt in "${boot_targets}"; do
    if test "${tgt}" = "mmc0"; then
	setenv devtype "mmc"
	setenv devnum 0
	mmc dev 0
    elif test "${tgt}" = "mmc1"; then
	setenv devtype "mmc"
	setenv devnum 1
	mmc dev 1
    else
	setenv devtype "${tgt}"
	setenv devnum 0
    fi

    if part number ${devtype} ${devnum} aux auxpart; then
	if load ${devtype} ${devnum}:${auxpart} ${loadaddr} /uboot.env; then
	    env import -b ${loadaddr} ${filesize} BOOT_ORDER DEBUG
	fi

	test -n "${BOOT_ORDER}" || setenv BOOT_ORDER "primary secondary net"

	if test -n "${DEBUG}"; then
	    setenv bootargs_log "debug"
	else
	    setenv bootargs_log "loglevel=4"
	fi

	run ixbootmedia
    fi
done

reset
