# -*- sh -*-
# REMEMBER TO UPDATE infix-interfaces ALSO IN confd.inc
MODULES=(
    "infix-interfaces@2025-01-09.yang -e vlan-filtering -e containers"
    "infix-containers@2024-11-15.yang"
)
