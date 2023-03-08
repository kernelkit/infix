#!/bin/sh
# Override boot mode based on user input from setup tool

set_cond()
{
    mkdir -p /run/finit/cond/boot
    ln -s /run/finit/cond/reconf "/run/finit/cond/boot/$1"
}

# Report to the setup and show tools the current boot mode
if /lib/infix/use-etc; then
    if [ -f /mnt/cfg/infix/.use_etc ]; then
	cat /mnt/cfg/infix/.use_etc > /tmp/.boot_mode
	if grep -qi profinet /mnt/cfg/infix/.use_etc; then
	    set_cond profinet
	else
	    set_cond etc
	fi
    else
	echo "native /etc"  > /tmp/.boot_mode
	set_cond etc
    fi
else
    echo "NETCONF" > /tmp/.boot_mode
    set_cond netconf
fi
