# -*- sh -*-
# REMEMBER TO UPDATE infix-interfaces ALSO IN confd.inc
MODULES=(
    "infix-interfaces@2024-09-23.yang -e vlan-filtering -e containers"
    "infix-containers@2024-03-27.yang"
)
