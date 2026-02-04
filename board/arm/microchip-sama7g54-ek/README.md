Microchip SAMA7G54-EK
=====================

The [SAMA7G54-EK][1] is an evaluation kit for the Microchip SAMA7G5 series,
featuring an ARM Cortex-A7 processor @ 800 MHz with 512 MB DDR3L RAM.

The board features:

- 2x MACB Gigabit Ethernet ports
- 8 GB eMMC (SDMMC0) + microSD card slot (SDMMC1)
- QSPI NOR flash
- USB 2.0 host ports
- CAN bus interfaces
- Flexcom serial ports (UART, SPI, I2C)

How to Build
------------

Since there are no pre-built images for ARM 32-bit, you need to build
both Infix and the bootloader from source.

### SD Card

1. Build the bootloader

        make O=x-boot-sama7g54 sama7g54_ek_sd_boot_defconfig
        make O=x-boot-sama7g54

2. Build Infix

        make O=x-arm arm_defconfig
        make O=x-arm

3. Create the SD card image

        ./utils/mkimage.sh -b x-boot-sama7g54 -r x-arm microchip-sama7g54-ek

### eMMC

1. Build the bootloader

        make O=x-boot-sama7g54-emmc sama7g54_ek_emmc_boot_defconfig
        make O=x-boot-sama7g54-emmc

2. Build Infix (same as above, skip if already built)

        make O=x-arm arm_defconfig
        make O=x-arm

3. Create the eMMC image

        ./utils/mkimage.sh -b x-boot-sama7g54-emmc -r x-arm -t emmc microchip-sama7g54-ek

Flashing to SD Card
-------------------

[Flash the image][0] to a microSD card (at least 2 GB):

```bash
sudo dd if=x-boot-sama7g54/images/infix-arm-sdcard.img of=/dev/sdX \
        bs=1M status=progress oflag=direct
```

You can also use `bmaptool` for faster writes:

```bash
sudo bmaptool copy x-boot-sama7g54/images/infix-arm-sdcard.img /dev/sdX
```

> [!WARNING]
> Ensure `/dev/sdX` is the correct device for your SD card and not used
> by the host system!  Use `lsblk` to verify.

Flashing to eMMC
-----------------

The SAMA7G5EK has an on-board 8 GB eMMC (SDMMC0/mmc0).  Jumper J22
controls if booting from onboard storage is allowed or not; open means
allowed.  When open, the SW4 button can also prevent booting from eMMC
if held at power on.

The easiest method is to flash from a running SD card system:

1. Boot the board from SD card
2. Transfer the eMMC image to the board (USB drive, network, etc.)
3. Flash the eMMC:

        sudo bmaptool copy emmc.img /dev/mmcblk0

4. Power off, remove SD card, and boot from eMMC

Booting the Board
-----------------

1. Insert the flashed SD card (or ensure eMMC is flashed)
2. Connect an Ethernet cable
3. Power up the board
4. Find the assigned IP and SSH in, default login: `admin` / `admin`

Console Port
------------

The debug console is on Flexcom3 (active by default):

- Baud rate: 115200
- Data bits: 8
- Parity: None
- Stop bits: 1

Connect a USB-to-serial adapter to the board's debug UART header.

> [!WARNING]
> Use only 3.3V serial adapters.

[0]: https://kernelkit.org/posts/flashing-sdcard/
[1]: https://www.microchip.com/en-us/development-tool/EV21H18A
