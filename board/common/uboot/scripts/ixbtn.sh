setenv dev_mode no
setenv factory_reset no

echo -n "dev-mode:      "
run ixdevmode
echo -n "factory-reset: "
run ixfactory

if test "${dev_mode}" = "yes"; then
    sleep 1 && run ixboot
else
    run ixboot
fi
