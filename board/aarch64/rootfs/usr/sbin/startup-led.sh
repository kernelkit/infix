#!/bin/sh
# Set the iitod "startup" condition once confd is ready, so the LAN and
# status LEDs light up per the product iitod.json rules.  /run is tmpfs,
# so this must run on every boot.
mkdir -p /run/finit/cond/run/startup
touch /run/finit/cond/run/startup/success
