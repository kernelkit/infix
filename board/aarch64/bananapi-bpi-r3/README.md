# Banana Pi R3

## Support level
Full support for all Infix enabled features including switched ethernet ports, WiFi,
and SFP interfaces. The board includes comprehensive hardware support for
MediaTek MT7986 SoC features.

### Hardware features
The Banana Pi R3 is a high-performance networking board featuring:
- MediaTek MT7986 ARM Cortex-A53 quad-core processor
- 4x Gigabit LAN ports (lan1-lan4)
- 1x Gigabit WAN port
- 2x SFP ports (sfp1, sfp2) for fiber connectivity
- Dual WiFi interfaces (wifi0 for 2.4GHz, wifi1 for 5GHz)
- USB support
- SD card boot support

### Network configuration
The board comes preconfigured with:
- 4 switched LAN ports for internal networking
- Dedicated WAN port with DHCP client enabled
- SFP ports for high-speed fiber connections
- Dual WiFi interfaces for wireless connectivity

### Installing Infix on eMMC

> [!IMPORTANT]
> No standard eMMC image exist yet, you have to create it your own
> from the SD card image by replacing fip.bin in the SD card image to
> the eMMC variant.

This guide describes how to install Infix on the internal eMMC storage of your BPI-R3.
The installation process requires a FTDI cable for console access, SD
card with Infix, a USB drive with the necessary bootloaders and eMMC
system image, and involves configuring the board's boot switches.

#### Required files

Download the following files and place them on a FAT32-formatted USB drive:

1. **NAND bootloader files** (for initial setup):
   - [bpi-r3_spim-nand_bl2.img](https://github.com/frank-w/u-boot/releases/download/CI-BUILD-2025-10-bpi-2025.10-2025-10-13_1032/bpi-r3_spim-nand_bl2.img)
   - [bpi-r3_spim-nand_fip.bin](https://github.com/frank-w/u-boot/releases/download/CI-BUILD-2025-10-bpi-2025.10-2025-10-13_1032/bpi-r3_spim-nand_fip.bin)

2. **eMMC bootloader files**:
   - Download and extract [bpi-r3-emmc-boot-2025.01-latest.tar.gz](https://github.com/kernelkit/infix/releases/download/latest-boot/bpi-r3-emmc-boot-2025.01-latest.tar.gz)
   - This contains `bl2.img` and `fip.bin` for eMMC boot

3. **System image**:
   - `infix-bpi-r3-emmc.img` - The Infix system image for eMMC

#### Installation steps

**Step 1: Flash NAND bootloader**

Boot from SD card and break into U-Boot by pressing Ctrl-C during startup:

```
usb start
mtd erase spi-nand0
fatload usb 0:1 0x50000000 bpi-r3_spim-nand_bl2.img
mtd write spi-nand0 0x50000000 0x0 0x100000
fatload usb 0:1 0x50000000 bpi-r3_spim-nand_fip.bin
mtd write spi-nand0 0x50000000 0x380000 0x200000
```

Power off the board and set the boot switch to **0101** (NAND boot mode), then power on.

**Step 2: Flash system to eMMC**

From the U-Boot prompt:

```
usb start
fatload usb 0:1 0x50000000 infix-bpi-r3-emmc.img
setexpr blocks ${filesize} / 0x200
mmc write 0x50000000 0x0 ${blocks}
```

**Step 3: Configure eMMC bootloader**

Write the eMMC bootloader and configure the boot partition:

```
mmc partconf 0 1 1 1
mmc erase 0x0 0x400
fatload usb 0:1 0x50000000 bl2.img
mmc write 0x50000000 0x0 0x400
mmc partconf 0 1 1 0
mmc bootbus 0 0 0 0
```

Power off the board, set the boot switch to **0110** (eMMC boot mode), and power on.
Your BPI-R3 should now boot Infix from the internal eMMC storage.

### Pre-built images
SD card image: [infix-bpi-r3-sdcard.img](https://github.com/kernelkit/infix/releases/download/latest-boot/infix-bpi-r3-sdcard.img)
