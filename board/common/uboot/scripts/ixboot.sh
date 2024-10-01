echo "Press Ctrl-C NOW to override the default boot sequence"
if sleep "${ixbootdelay}"; then
    run ixbootorder

    echo "FATAL: Exhausted all available boot sources, rebooting"
    sleep 1
    reset
fi

if test "${dev_mode}" != "yes"; then
    bootmenu
    pause "Console shell access PROHIBITED. Press any key to reset..."
    reset
fi

echo
echo 'Run "bootmenu" to interactively select a boot device'
echo
