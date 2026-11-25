export PASW_DEV=/sys/bus/pci/devices/$(lspci -k | awk '/pasw-saline/ { printf("0000:%s", $1); }')/resource0
export PASW_OFFSET=0xc0000
export PASW_SPEC=/etc/pasw/saline-regs.yml
