Raspberry Pi 2 Model B
======================

The [Raspberry Pi 2 Model B][1] is a 32-bit ARM single-board computer,
powered by the Broadcom BCM2836 quad-core Cortex-A7 processor @ 900 MHz
with 1 GB RAM.

The board features:

- 4x USB 2.0 ports
- Fast Ethernet (100 Mbps)
- microSD card slot for storage
- HDMI port
- GPIO header (40-pin)

> [!NOTE]
> Revision 1.2 of the Pi 2B actually uses a BCM2837 (Cortex-A53) underclocked
> and without WiFi, making it very similar to the Pi 3B hardware-wise but
> running in 32-bit mode.  This revision is not supported.

How to Build
------------

Since there are no pre-built images for ARM32, you need to build both Infix
and the bootloader from source.


1. Clone the repository

        git clone https://github.com/kernelkit/infix.git
        cd infix

2. Build the bootloader (in separate tree)

        make O=x-boot rpi2_boot_defconfig
        make O=x-boot

3. Build Infix (in another tree)

        make O=x-arm32 aarch32_defconfig
        make O=x-arm32

4. Create the SD card image

        ./utils/mkimage.sh -b x-boot -r x-arm32 raspberrypi-rpi2

The resulting image can be found in `x-boot/images/infix-arm-sdcard.img`

Flashing to SD Card
-------------------

[Flash the image][0] to a microSD card (at least 4 GB):

```bash
sudo dd if=x-boot/images/infix-arm-sdcard.img of=/dev/mmcblk0 \
        bs=1M status=progress oflag=direct
```

You can also use `bmaptool`:

```bash
sudo bmaptool copy x-boot/images/infix-rpi2-sdcard.img /dev/mmcblk0
```

> [!WARNING]
> Ensure `/dev/mmcblk0` is the correct device for your SD card and not used by
> the host system! Use `lsblk` to verify.

Booting the Board
-----------------

1. Insert the flashed SD card into the Raspberry Pi
2. Connect an Ethernet cable (DHCP will be used to get an IP address)
3. Power up the board using a 5V/2.5A micro-USB power supply

The board will boot and obtain an IP address via DHCP on the Ethernet port.
Find the assigned IP and SSH in with the default login credentials, user/pass:
`admin` / `admin`.

Console Port (Optional)
-----------------------

A serial console can be useful for debugging. Connect a USB-to-TTL
serial adapter (3.3V) to GPIO pins:

- GND → Pin 6, ground
- TxD → Pin 8, GPIO 14
- RxD → Pin 10, GPIO 15

Serial settings: 115200 8N1

> [!WARNING]
> Use only 3.3V serial adapters. 5V adapters will damage your Raspberry Pi!

[0]: https://kernelkit.org/posts/flashing-sdcard/
[1]: https://www.raspberrypi.com/products/raspberry-pi-2-model-b/
