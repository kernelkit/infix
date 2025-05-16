# -*- sh -*-
# REMEMBER TO UPDATE infix-interfaces ALSO IN confd.inc
MODULES=(
    "infix-interfaces@2025-01-09.yang -e vlan-filtering -e containers"
    "infix-containers@2025-05-14.yang"
)
