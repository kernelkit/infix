Starfive VisionFive2
====================

The [VisionFive2][1] is a low-cost RISC-V 64-bit based platform, powered
by the Starfive JH7110 processor.

Infix runs from either SD card or eMMC, this guide only covers SD card.


How to Build
------------

```
$ make visionfive2_defconfig
$ make
```

Once the build has finished you will have `output/images/sdcard.img`
which you can flash to an SD card.

```
$ sudo dd if=output/images/sdcard.img of=/dev/mmcblk0 bs=1M status=progress oflag=direct
```

> **WARNING:** ensure `/dev/mmcblk0` really is the correct device for
> your SD card, and not used by the system!


Bootstrap Mode
--------------

Ensure the correct [bootstrap mode][3] for booting from SD card.  From
factory the board is preset to start from QSPI flash, which also has a
U-Boot that tries to load a Linux system from SD card, or fail-over to
load from the network.

| **Mode** | **`RGPIO_1`** | **`RGPIO_0`** |
|----------|---------------|---------------|
| QSPI NOR | 0 (L)         | 0 (L)         |
| SD card  | 0 (L)         | 1 (H)         |
| eMMC     | 1 (H)         | 0 (L)         |
| UART     | 1 (H)         | 1 (H)         |

> **Note:** the DIP switches are flipped 180° so don't get confused by
> the socket's `ON` label.  Look at the L and H markings on the board!


Booting the Board
-----------------

 1. Connect a TTL UART cable to pin 6 (GND), 8 (TX) and 10 (RX)
 2. Insert your SD card
 3. Power-up the board using an USB-C cable

You need a *stable power source*!  If you get the following on the
console at power on, try to power cycle the device again:

```
dwmci_s: Response Timeout.
dwmci_s: Response Timeout.
BOOT fail,Error is 0xffffffff
```

[1]: https://doc-en.rvspace.org/Doc_Center/visionfive_2.html
[2]: https://doc-en.rvspace.org/VisionFive2/Quick_Start_Guide/VisionFive2_SDK_QSG/recovering_bootloader%20-%20vf2.html
[3]: https://doc-en.rvspace.org/VisionFive2/Quick_Start_Guide/VisionFive2_SDK_QSG/boot_mode_settings.html
