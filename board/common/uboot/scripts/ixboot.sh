run prepare_aux
test -n "${BOOT_ORDER}" || setenv BOOT_ORDER "primary secondary net"
echo "Boot order: ${BOOT_ORDER}"

if test -n "${DEBUG}"; then
    setenv bootargs_log "debug"
else
    setenv bootargs_log "loglevel=4"
fi

for s in "${BOOT_ORDER}"; do
    setenv slot "${s}"

    echo "${slot}: Preparing..."
    setenv prepared

    if test "${slot}" = "primary"; then
	run prepare_primary
    elif test "${slot}" = "secondary"; then
	run prepare_secondary
    elif test "${slot}" = "net"; then
	run prepare_net
    fi

    if test "${prepared}" = "ok"; then
	echo "${slot}: Validating..."
	if iminfo ${ramdisk_addr_r}; then
	    echo "${slot}: Booting..."
	    setenv bootargs_rauc "rauc.slot=${slot}"
	    blkmap create boot
	    blkmap map boot 0 ${sqblkcnt} mem ${sqaddr}
	    source ${ramdisk_addr_r}:boot-script
	    blkmap destroy boot
	    echo "${slot}: ERROR: Boot failed"
	else
	    echo "${slot}: ERROR: Validation failed"
	fi
    else
	echo "${slot}: ERROR: Unable to use slot"
    fi
done

reset
