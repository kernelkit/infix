echo "Booting from ${devtype}${devnum} (order: ${BOOT_ORDER})"

for s in "${BOOT_ORDER}"; do
    setenv slot "${s}"
    run ixbootslot
done
