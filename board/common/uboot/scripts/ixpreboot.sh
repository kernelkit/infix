setenv dev_mode no
setenv factory_reset no

echo -n "dev-mode:      "
run ixbtn-devmode
echo -n "factory-reset: "
run ixbtn-factory

setenv valid_media no
setenv ixmenu_n 0

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
	setenv "boot_${tgt}_primary" "setenv devtype ${devtype}; setenv devnum ${devnum}; setenv slot primary; run ixbootslot"
	setenv "bootmenu_${ixmenu_n}" "Boot primary ${tgt} partition=run boot_${tgt}_primary"
	setexpr ixmenu_n ${ixmenu_n} + 1

	setenv "boot_${tgt}_secondary" "setenv devtype ${devtype}; setenv devnum ${devnum}; setenv slot secondary; run ixbootslot"
	setenv "bootmenu_${ixmenu_n}" "Boot secondary ${tgt} partition=run boot_${tgt}_secondary"
	setexpr ixmenu_n ${ixmenu_n} + 1

	if load ${devtype} ${devnum}:${auxpart} ${loadaddr} /uboot.env; then
	    env import -b ${loadaddr} ${filesize} BOOT_ORDER DEBUG
	fi

	if test -n "${DEBUG}"; then
	    setenv bootargs_log "debug"
	else
	    setenv bootargs_log "loglevel=4"
	fi

	test -n "${BOOT_ORDER}" || setenv BOOT_ORDER "primary secondary net"
	echo "Valid boot device found on ${tgt} (order: ${BOOT_ORDER})"

	for slot in "${BOOT_ORDER}"; do
	    if test "${slot}" = "primary"; then
		setenv ixbootorder "${ixbootorder}run boot_${tgt}_primary; "
	    elif test "${slot}" = "secondary"; then
		setenv ixbootorder "${ixbootorder}run boot_${tgt}_secondary; "
	    elif test "${slot}" = "net"; then
		setenv ixbootorder "${ixbootorder}run boot_net; "
	    fi
	done

	setenv valid_media yes
    fi
done

setenv "boot_net" "setenv slot net; run ixbootslot"
setenv "bootmenu_${ixmenu_n}" "Boot over network=run boot_net"
setexpr ixmenu_n ${ixmenu_n} + 1

setenv "bootmenu_${ixmenu_n}" "Factory reset=run ixfactory; echo; pause; bootmenu"
setexpr ixmenu_n ${ixmenu_n} + 1

if test "${valid_media}" = "no"; then
    echo "NO BOOTABLE MEDIA FOUND, falling back to netboot"
    setenv bootargs_log "debug"
    setenv ixbootorder "run boot_net"
fi
