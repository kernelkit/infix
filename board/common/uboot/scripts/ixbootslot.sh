echo "${slot}: Preparing..."
setenv prepared

if test "${slot}" = "primary"; then
    run ixprepblk
elif test "${slot}" = "secondary"; then
    run ixprepblk
elif test "${slot}" = "net"; then
    run ixprepdhcp
fi

if test "${prepared}" = "ok"; then
    echo "${slot}: Validating..."

    iminfo ${ramdisk_addr_r}
    imok=$?

    if test ${imok} -eq 0 || test "${dev_mode}" = "yes"; then
	if test ${imok} -ne 0; then
	    echo "DEVELOPER MODE ACTIVE, BOOTING UNTRUSTED IMAGE"
	fi

	echo "${slot}: Booting..."

	setenv bootargs_user "rauc.slot=${slot}"
	if test "${factory_reset}" = "yes"; then
	    setenv bootargs_user "${bootargs_user} finit.cond=factory-reset"
	fi
	if test "${dev_mode}" = "yes"; then
	    setenv bootargs_user "${bootargs_user} finit.cond=dev-mode"
	fi

	blkmap create boot
	blkmap get boot dev blkmapnum
	blkmap map boot 0 ${sqblkcnt} mem ${sqaddr}

	for conf in "${board}-syslinux.conf syslinux.conf"; do
	    if test -e blkmap ${blkmapnum} /boot/syslinux/${conf}; then
		sysboot blkmap ${blkmapnum} any ${scriptaddr} /boot/syslinux/${conf}
	    fi
	done

	blkmap destroy boot

	echo "${slot}: ERROR: Boot failed"
    else
	echo "${slot}: ERROR: Validation failed"
    fi
else
    echo "${slot}: ERROR: Unable to use slot"
fi
