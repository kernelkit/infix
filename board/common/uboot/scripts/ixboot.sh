echo "Press Ctrl-C NOW to enter boot menu"
if sleep "${ixbootdelay}"; then
    run ixbootorder

    echo "FATAL: Exhausted all available boot sources, rebooting"
    sleep 1
    reset
fi

bootmenu
if test "${dev_mode}" != "yes"; then
    pause "Console shell access PROHIBITED. Press any key to reset..."
    reset
fi
