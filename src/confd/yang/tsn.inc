# -*- sh -*-
#
# IEEE 802.1 TSN shapers, installed on products with the hardware.
MODULES=(
	"ieee802-dot1q-cbsa@2024-08-07.yang"
	"ieee802-dot1dc-cbsa-if@2024-09-26.yang"
	"ieee802-dot1q-sched@2023-10-22.yang -e scheduled-traffic"
	"ieee802-dot1dc-sched-if@2024-09-26.yang"
	"infix-tsn@2026-10-22.yang"
)
