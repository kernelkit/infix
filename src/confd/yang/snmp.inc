# -*- sh -*-
# No features enabled: UDP is the only transport we serve, and proxy,
# TLS/SSH/TSM transports and notification filters stay out of the tree.
MODULES=(
	"ietf-snmp@2014-12-10.yang"
	"infix-snmp@2026-09-22.yang"
)
