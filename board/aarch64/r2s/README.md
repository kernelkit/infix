FriendlyELC NanoPi R2S
======================

The [NanoPi R2S][1] is a very low-cost 64-bit ARM min router, powered by
the Rockchip RK3328, quad-core Cortex-A53.

The R2S does not have any onboard eMMC, so the only way to boot Infix on
it is using and SD card.


LEDs
----

The front system LEDs work as follows in Infix:

| **Stage**      | **SYS** | **LAN** | **WAN** |
|----------------|---------|---------|---------|
| Power-on       | dimmed  | off     | off     |
| Factory reset  | on      | on      | on      |
| Linux loading  | on      | off     | off     |
| System loading | 1 Hz    | off     | off     |
| System up      | off     | on      | off     |
| WAN address    | off     | on      | on      |
| Locate         | 1 Hz    | 1 Hz    | 1 Hz    |
| Fail safe      | 5 Hz    | off     | off     |
| Panic          | 5 Hz    | 5 Hz    | 5 Hz    |

Powering on the device the SYS LED is turned on faintly (dimmed).  It
remains dimmed while U-Boot loads the kernel, and turns bright red when
the kernel starts.  It remains steady on until the system has started
the LED daemon, `iitod`, which sets it blinking at 1 Hz while the rest
of the system starts up.  When the system has come up successfully, the
SYS LED is turned off and the green LAN LED turns on.  The WAN LED will
turn on (green) when the WAN interface is up and has an IP address.

> Compared to the `x86_64` Qemu target, it takes a while to parse all
> YANG models and load `startup-config`, but the whole process should
> not take more than 60 seconds, and usually a lot less.

If a "find my device" function exists, it will blink all LEDs at 1 Hz.

If `startup-config` fails to load Ínfix reverts to `failure-config`,
putting the device in fail safe (or fail secure) mode.  Indicated by
the SYS LED blinking at 5 Hz instead of turning off.

If Infix for some reason also fails to load `failure-config`, then all
LEDs will blink at 5 Hz to clearly indicate something is very wrong.

In all error cases the console shows the problem.


Factory Reset
-------------

The reset button on the side can be used not only to safely reboot the
device, but can also be used to trigger a factory reset at power on.

At power-on, keep the reset button pressed for 10 seconds.  The system
LEDs (SYS, WAN, LAN) will all blink at 1 Hz, to help you count down the
seconds.  When the 10 seconds have passed all LEDs are turned off before
loading Linux.

When Linux boots up it confirms the factory reset by lighting up the
LEDs again, no blinking this time.  The LEDs stay on until all files and
directories on read/writable partitions (`/cfg` and `/var`) have been
safely erased.

The system then continues loading, turning off all LEDs except SYS,
which blinks calmly at 1 Hz as usual until the system has completed
loading, this time with a `startup-config` freshly restored from the
device's `factory-config`.


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
U-Boot environment (below), or use the `custom-phys-address` setting in
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
