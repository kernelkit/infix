Marvell CN9130-CRB
==================

## Build instructions

### Bootloader

Build the bootloader from the supplied `defconfig`. It might be useful
to build in a separate output directory if you want to build Infix
from the same working tree later:

    make O=$(pwd)/x-cn9130-boot cn9130_crb_boot_defconfig
    cd x-cn9130-boot
	make

The artifact of interest is called `flash-image.bin`, which will be
located in the `images/` directory once the build completes.


### Infix

> If you do not want to build Infix from source, feel free to use a pre-built [release]

The standard `aarch64_defconfig` is compatible with this board:

    make O=$(pwd)/x-aarch64 aarch64_defconfig
    cd x-aarch64
	make

Two artifacts from the `images/` directory of this build are required
to provision a new board:

- `rootfs.itb`: Netbootable image
- `infix-aarch64.pkg`: Standard upgrade bundle

## Provisioning

The overall provisioning flow, in which each step in described in
details in the following sections, is as follows:

- Strap board to boot from SPI FLASH
- Load `flash-image.bin` over UART
- Burn `flash-image.bin` to SPI FLASH
- Netboot `rootfs.itb`
- Run Infix's built-in provisioning script
- Reboot, now booting from the primary partition of the SD-card

#### Strap Board to Boot from SPI FLASH

By default, the board is strapped to boot from eMMC. However, Infix
assumes that the board will boot from SPI FLASH. Therefore we have to
ensure that the DIP switches of `SW2` on the board selects a
`BOOT_MODE` of `0x32`, meaning that the switches on positions 4, 3,
and 1 should be enabled (i.e. these need to be tied to ground):

```
.-----. .-----.                                    .-----.
|     | |     |                                    | PWR |
| SIM | | uSD |                                    |     |
|     | |     |                                    '-----'
'-----' '-----'

                               |
                               v
                    .-----. .-----.
                    | SW1 | | SW2 |
                    '-----' '-----'
                    .--------------------------.
                    |                          |
                    |                          |
                    |          CN9130          |
```

#### UART Boot U-Boot

Make sure that:
 - [mvebu64boot] is installed and available in your shell's `$PATH`
 - No other program is attached to `ttyUSB0`
 - No power is applied to the board

1. Start `mvebu64boot`:

        mvebu64boot -b /path/to/flash-image.bin /dev/ttyUSB0

2. Apply power

As soon as `mvebu64boot` completes, attach to the serial port,
e.g. using `screen(1)`, or `console(1)` and stop the normal boot
process by hitting any key.

#### Burn U-Boot to SPI FLASH

Make sure that:
- `eth1` is connected to a machine which serves `flash-image.bin` over
TFTP
- U-Boot can reach the TFTP server. If the neighboring machine is also
  set up as a DHCP server, simply run the command `dhcp -`

To burn the bootloader to SPI FLASH, run the `bubt` command:

    bubt flash-image.bin

Once the command completes, reset the board to verify that it can now
boot unassisted:

    reset

#### Boot up `rootfs.itb`

U-Boot will automatically fallback to netboot since the SD-card is
still blank. Make sure that the PC provides the path to `rootfs.itb`
in DHCP option 67 ("bootfile").

#### Install Firmware

Login as `admin`/`admin`, setup networking to the PC, ensure that the date on
the device is reasonably correct, and run the provisioning script:

    admin@infix:~$ sudo -i
    root@infix:~$ udhcpc -i e28
	root@infix:~$ date -us YYYY-MM-DD
	root@infix:~$ /libexec/infix/prod/provision tftp://<PC-IP>/infix-aarch64.pkg /dev/mmcblk0

After successful completion, the device is fully provisioned. On the
next boot, the device will boot of its own accord from the primary
SD-card partition.

> If possible, serve `infix-aarch64.pkg` over HTTP instead, as
> libcurl's TFTP implementation is quite slow.

[release]: https://github.com/kernelkit/infix/releases
[mvebu64boot]: https://github.com/addiva-elektronik/mvebu64boot
