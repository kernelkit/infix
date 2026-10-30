# Banana Pi BPI-R64

## Overview

<img align="right" src="banana_pi_bpi-r64_interface.jpg" alt="Board Overview" width=400 padding=10>

The Banana Pi BPI-R64 is a networking board based on the MediaTek MT7622
(dual Cortex-A53, AArch64) SoC.

### Hardware Features

- MediaTek MT7622 ARM Cortex-A53 dual-core processor @ 1.35 GHz
- 1 GB DDR3L RAM
- 8 GB eMMC storage
- microSD card slot
- MT7531 Gigabit Ethernet switch (4x LAN + 1x WAN)
- MT7622 WMAC built-in 2.4 GHz 802.11ac Wi-Fi
- USB 3.0 port
- 2x Mini PCIe slots

### Default Network Configuration

Infix comes preconfigured with:

- **LAN ports** (lan0-lan3): Bridged for internal networking
- **WAN port**: DHCP client enabled for internet connectivity
- **Wi-Fi** (wifi0-ap): Bridged to LAN (MT7622 WMAC, 2.4 GHz; or MT7615 PCIe card if fitted)

## Boot Switch Reference

<img align="right" src="bpi-r64-sw1.png" alt="SW1 Boot Switch" width=90 padding=10>

The BPI-R64 has a 2-position DIP switch (SW1) for selecting the boot device.
The MT7622 Boot ROM always tries SD first if a card is present, so you can
leave SW1 in the OFF (eMMC) position and simply insert or remove an SD card
to control boot device selection.

| SW1 | Boot device |
|-----|-------------|
| OFF | eMMC        |
| ON  | SD card     |

## Getting Started

### Quick Start with SD Card

1. **Flash the image to an SD card** (the filename includes the version, e.g.
   `infix-25.01-bpi-r64-sdcard.img`):

<img align="right" src="debug-uart.png" alt="Debug UART" width=100 padding=10>

   ```sh
   dd if=infix-*-bpi-r64-sdcard.img of=/dev/sdX bs=4M status=progress
   ```

2. **Insert SD card and power on**
3. **Connect console:** 115200 8N1 — use the dedicated Debug UART header
   just below the 40-pin GPIO header; pins are labeled GND, RX, TX on the board
4. **Default login:** `admin` / `admin`

## Installing to eMMC

Unlike the BPI-R3 (where SD and eMMC share a bus, requiring a NAND intermediate
step), the BPI-R64 has separate controllers for SD (mmc1/MSDC1) and eMMC
(mmc0/MSDC0).  You can write directly to eMMC while booted from SD.

### eMMC Boot ROM Behaviour

> [!IMPORTANT]
> The MT7622 Boot ROM reads BL2 from the eMMC BOOT0 hardware partition (offset
> 0), not from the User Data Area where the main disk image lives.  The
> installation procedure below writes BL2 to BOOT0 separately to handle this.

### Procedure

Boot from SD and write the eMMC image from U-Boot using a FAT32-formatted USB
drive.

#### Step 1: Boot from SD card

1. Insert SD card with Infix
2. Power on and break into U-Boot (press Ctrl-C during boot)

#### Step 2: Write the eMMC image from U-Boot

Place `infix-bpi-r64-emmc.img` and `bl2.img` on a FAT32-formatted USB drive,
then from the U-Boot prompt:

```
usb start
fatload usb 0:1 0x44000000 infix-bpi-r64-emmc.img
setexpr blocks ${filesize} / 0x200
mmc dev 0
mmc write 0x44000000 0x0 ${blocks}
```

#### Step 3: Write BL2 to eMMC BOOT0

```
fatload usb 0:1 0x44000000 bl2.img
mmc partconf 0 1 1 1
setexpr blkcnt ${filesize} + 0x1ff
setexpr blkcnt ${blkcnt} / 0x200
mmc write 0x44000000 0x0 ${blkcnt}
mmc partconf 0 1 1 0
```

#### Step 4: Boot from eMMC

1. Power off the board
2. Remove SD card and USB drive
3. Power on

## Giving the Board a Permanent MAC Address

The BPI-R64 has no EEPROM with a factory-programmed MAC address.  Out
of the box every Ethernet port therefore gets a random address on each
boot, and the hostname, which is derived from the base address, changes
with it.  The Wi-Fi radio carries the same MediaTek default address on
every board, so two boards cannot even connect to each other over
Wi-Fi: a client refuses to authenticate to an access point that has its
own address.

Store a product record as `/mnt/aux/vpd.bin`, from the Infix shell,
and the board keeps that identity across reboots and upgrades.  This
requires a boot loader from v26.11.0 or later:

```
echo '{"mac-address":"02:00:00:ba:64:00","serial-number":"banana64","vendor":"SinoVoip Co., Ltd.","product-name":"BPI-R64"}' \
  | onieprom -e > /mnt/aux/vpd.bin
```

Give each board its own base address and serial number, then reboot.
The base address becomes the hostname suffix, each wired port gets a
fixed offset from the base, and the Wi-Fi interfaces get addresses
derived from it.  With the record above the board is named
`bpi-ba-64-00`, `wan` is `02:00:00:ba:64:02`, `lan0` is
`02:00:00:ba:64:03` and `wifi0` is `06:00:00:ba:64:00`.  A port goes
back to its VPD address when a custom MAC address is removed from it.

| Offset | Port             |
|--------|------------------|
| +1     | CPU switch port  |
| +2     | `wan`            |
| +3..+6 | `lan0`..`lan3`   |
| +7     | Second CPU port  |

A locally administered address like `02:xx:xx:xx:xx:xx` is fine for a
lab.  For boards that will share a network with other equipment, use a
range you own.  Remove the file to return to random addresses.  See
[Vital Product Data](../../../doc/vpd.md#vpd-file-on-the-aux-partition)
for the record format and what else it can carry.

## Platform Notes

### Wi-Fi

The MT7622 SoC includes a built-in WMAC that provides 2.4 GHz 802.11b/g/n/ac
(2×2) — there is no 5 GHz capability from the onboard radio.

For dual-band operation, a PCIe card can be fitted in one of the two Mini PCIe
slots.  The [Banana Pi BPI-MT7615][bpi-mt7615] is a purpose-built dual-band
(2.4 + 5 GHz, 802.11ac 4×4) module designed for BPI router boards and is a
natural fit.  With it installed, Infix will use it in preference to the onboard
WMAC for the `wifi0-ap` interface.

[bpi-mt7615]: https://docs.banana-pi.org/en/BPI-MT7615/BananaPi_MT7615

### mmc0 = eMMC, mmc1 = SD

On MT7622, MSDC0 (mmc0) is the 8-bit eMMC controller and MSDC1 (mmc1) is the
4-bit SD card controller — the reverse of many other platforms.

## Building

```sh
# Bootloader only (SD)
make O=x-boot-bpir64-sd bpi_r64_sd_boot_defconfig && make O=x-boot-bpir64-sd

# Bootloader only (eMMC)
make O=x-boot-bpir64-emmc bpi_r64_emmc_boot_defconfig && make O=x-boot-bpir64-emmc

# Compose SD image (pass the Infix rootfs output directory)
utils/mkimage.sh -b x-boot-bpir64-sd -r x-aarch64/images bananapi-bpi-r64

# Compose eMMC image
utils/mkimage.sh -b x-boot-bpir64-emmc -r x-aarch64/images -t emmc bananapi-bpi-r64
```
