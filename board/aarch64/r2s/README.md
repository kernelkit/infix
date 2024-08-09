FriendlyELC NanoPi R2S
======================

The [NanoPi R2S][1] is a very low-cost 64-bit ARM min router, powered by
the Rockchip RK3328, quad-core Cortex-A53.

The R2S does not have any onboard eMMC, so the only way to boot Infix on
it is using and SD card.


How to Build
------------

```
$ make r2s_defconfig
$ make
```

Once the build has finished you will have `output/images/sdcard.img`
which you can flash to an SD card.

```
$ sudo dd if=output/images/sdcard.img of=/dev/mmcblk0 bs=1M status=progress oflag=direct
```

> **WARNING:** ensure `/dev/mmcblk0` really is the correct device for
> your SD card, and not used by the system!


Booting the Board
-----------------

 1. Connect a TTL cable to three UART pins, GND is closest to the edge
 2. Insert the flashed SD card
 3. Power-up the board using an USB-C cable (ensure good power source!)

Worth noting, unlike many other boards, the Rockchip family of chipsets
runs the UART at 1500000 bps (1.5 Mbps) 8N1.


Secure Boot
-----------

Like other Infix builds, the R2S enjoys secure boot.  Please note,
however that the default signing keys are the public!

Also, default builds allow modifying and saving the U-Boot environment
(see below), which you may want to disable to secure the device.  The
device also runs in *developer mode*, allowing full U-Boot shell access,
which you may also want to disable in a full production setting.


Caveat
------

Most (all?) of these boards do not have any Vital Product Data (VPD)
EEPROM mounted.  This means they do not come with unique MAC addresses
allocated to the two Ethernet ports.

The bootloader (U-Boot) default environment for the board is usually
what provides a default, the same default MAC addresses to Linux:

 - 4a:dc:d8:20:0d:84
 - 4a:dc:d8:20:0d:85

This is important in case you want to run multiple R2S devices on the
same LAN.  Meaning you either have to change the MAC address in the
U-Boot environment (below), or modify your `phys-address` setting in
Infix for the interface(s).

Break into U-Boot using Ctrl-C at power-on, preferably when the text
`Press Ctrl-C NOW to enter boot menu` is displayed.  Exit the menu to
get to the prompt:

```
(r2s) printenv
...
eth1addr=4a:dc:d8:20:0d:84
ethact=ethernet@ff540000
ethaddr=4a:dc:d8:20:0d:85
ethprime=eth0
...
```

Here we change both addresses, using the *Locally Administered* bit:

```
(r2s) setenv eth1addr 02:00:c0:ff:ee:01
(r2s) setenv ethaddr 02:00:c0:ff:ee:00
(r2s) saveenv
```

Boot the system, log into Linux, and inspect the MAC addresses:

```
admin@infix-00-00-00:~$ ip -br l
lo      UP     00:00:00:00:00:00 <LOOPBACK,UP,LOWER_UP>
eth0    UP     02:00:c0:ff:ee:00 <BROADCAST,MULTICAST,UP,LOWER_UP>
eth1    UP     02:00:c0:ff:ee:01 <BROADCAST,MULTICAST,UP,LOWER_UP>
```

[1]: https://wiki.friendlyelec.com/wiki/index.php/NanoPi_R2S
