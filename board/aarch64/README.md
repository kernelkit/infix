aarch64
=======

Board Specific Documentation
----------------------------

- [Marvell CN9130-CRB](cn9130-crb/)
- [Microchip SparX-5i PCB135 (eMMC)](sparx5-pcb135/)
- [NanoPi R2S](r2s/)
- [Raspberry Pi 4 b](#raspberry-pi-4-b)

# Raspberry Pi 4 b

## Support level
Full support for base board but not any extension board on the
GPIOs.

### Touch screen
The [Raspberry Pi touch display v1][RPI-TOUCH] is supported, including
touch functionality. There are multiple touchscreens on the market for
Raspberry Pi, but only the official (first version with 800x480
resolution) is currently supported. Infix supplies all drivers
required to utilize the hardware, but you need to add the actual
graphical application in a container.

There are some important considerations you need to know about when
using Infix for graphical applications. The container needs access to
/dev/dri/ to be able to access the graphics card, and it also needs
access to /run/udev to be able to find the input devices.

Example of running Doom in Infix:

```cli
	admin@example:/> configure
	admin@example:/config/> edit container doom
	admin@example:/config/container/doom/> set image docker://mattiaswal/alpine-doom:latest
	admin@example:/config/container/doom/> set privileged
	admin@example:/config/container/doom/> edit mount udev
	admin@example:/config/container/doom/mount/udev/> set type bind
	admin@example:/config/container/doom/mount/udev/> set target /run/udev/
	admin@example:/config/container/doom/mount/udev/> set source /run/udev/
	admin@example:/config/container/doom/mount/udev/> end
	admin@example:/config/container/doom/mount/xorg.conf/> set content U2VjdGlvbiAiU2VydmVyTGF5b3V0IgogICAgSWRlbnRpZmllciAiRGVmYXVsdExheW91dCIKICAgIFNjcmVlbiAwICJTY3JlZW4wIiAwIDAKRW5kU2VjdGlvbgpTZWN0aW9uICJEZXZpY2UiCiAgICBJZGVudGlmaWVyICJpTVggTENEIgogICAgRHJpdmVyICJtb2Rlc2V0dGluZyIKICAgIEJ1c0lEICJwbGF0Zm9ybTozMmZjNjAwMC5kaXNwbGF5LWNvbnRyb2xsZXIiCiAgICBPcHRpb24gImttc2RldiIgIi9kZXYvZHJpL2NhcmQxIgpFbmRTZWN0aW9uCgpTZWN0aW9uICJTY3JlZW4iCiAgICBJZGVudGlmaWVyICJTY3JlZW4wIgogICAgRGV2aWNlICJpTVggTENEIgogICAgRGVmYXVsdERlcHRoIDI0CkVuZFNlY3Rpb24KCg==
	admin@example:/config/container/doom/mount/xorg.conf/> set target /etc/X11/xorg.conf
	admin@example:/config/container/doom/mount/xorg.conf/> end
	admin@example:/config/container/doom/> edit volume var
	admin@example:/config/container/doom/volume/var/> set target /var
	admin@example:/config/container/doom/volume/var/> leave
	admin@example:/>

```


[RPI-TOUCH]: https://www.raspberrypi.com/products/raspberry-pi-touch-display/
