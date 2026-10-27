Banana Pi BPI-F3
================

The [BPI-F3][1] is a SpacemiT K1 (8x RISC-V X60) board with two GbE
ports, eMMC, and a microSD slot.  Infix runs from SD card.

| **Component**   | **Source**                                       |
|-----------------|--------------------------------------------------|
| FSBL (DDR init) | SpacemiT `uboot-2022.10`, `k1-bl-v2.2.y`         |
| OpenSBI         | upstream v1.8.1                                  |
| U-Boot          | `kkit-u-boot-2026.10.y`, `spacemit_k1_defconfig` |


How to Build
------------

```
$ make O=x-boot bpi_f3_boot_defconfig
$ make O=x-boot
$ make riscv64_defconfig
$ make
$ ./utils/mkimage.sh -b x-boot -r output bananapi-bpi-f3
$ sudo dd if=x-boot/images/infix-bpi-f3-sdcard.img of=/dev/mmcblk0 bs=1M status=progress oflag=direct
```


Booting the Board
-----------------

 1. Connect a 3.3V TTL UART to the debug header, 115200 8N1
 2. Insert the SD card and power on


Limitations
-----------

Linux 6.18:

 - SD card support needs backported device tree and driver changes
 - Reboot and power-off need a backported P1 PMIC driver
 - SD card limited to 3.3V signalling (`no-1-8-v`), no UHS modes
 - PDMA disabled in the board DT, its probe hangs the boot
 - No temperature sensors, the K1 thermal driver landed in v7.2
 - No PCIe, so no Wi-Fi or NVMe, the K1 PCIe drivers landed after 6.18
 - No USB, the K1 USB2 PHY driver landed after 6.18
 - The P1 RTC cannot be read at boot, the clock starts from the build default

Boot loader:

 - Vendor FSBL (U-Boot 2022.10 SPL) for DDR init
 - U-Boot has no Ethernet driver for the K1, so there is no netboot
 - U-Boot has no reset driver for the K1, `reset` does nothing
 - SD card image only, no eMMC image yet

[1]: https://docs.banana-pi.org/en/BPI-F3/BananaPi_BPI-F3
