# -*- sh -*-
MODULES=(
    "infix-routing -e ospf -e rip -e bfd"
    "ietf-ospf@2022-10-19.yang -e auto-cost -e bfd -e explicit-router-id -e hybrid-interface"
    "ietf-rip@2020-02-20.yang"
    "iana-bfd-types@2021-10-21.yang"
    "ietf-bfd-types@2022-09-22.yang"
    "ietf-bfd@2022-09-22.yang"
    "ietf-bfd-ip-sh@2022-09-22.yang"
    "infix-ospf@2026-10-26.yang"
    "infix-rip@2026-10-26.yang"
)
