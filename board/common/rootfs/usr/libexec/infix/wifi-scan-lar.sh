#!/bin/sh
# Scan once on AP radios with the lar-needs-scan quirk, e.g. Intel AX210,
# so their firmware learns the country from nearby APs before hostapd.

for conf in /etc/hostapd-*.conf; do
    radio=${conf#/etc/hostapd-}
    radio=${radio%.conf}

    [ "$(/usr/libexec/infix/has-quirk lar-needs-scan "$radio")" = true ] || continue

    logger -t wifi -p user.notice "$radio has LAR enabled, starting neighbor scan ..."
    iw phy "$radio" interface add "scan-$radio" type managed || continue
    ip link set "scan-$radio" up
    # The combined scan can spin on netlink forever, the scan takes seconds
    timeout 15 iw dev "scan-$radio" scan >/dev/null 2>&1
    iw dev "scan-$radio" del
done

exit 0
