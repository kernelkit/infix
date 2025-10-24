# Raspberry Pi 3B/4B

## Support level

Full support for base board but not any extension board on the GPIOs.
Other RPi boards of the same generation may work as well, but may need
some additional testing/work.  A few CM4 variants have been tested and
seem to work as expected, but YMMV as always.

### Touch screen

The [Raspberry Pi touch display v1][0] is supported on the 4B, including
touch functionality.  There are multiple touchscreens on the market for
Raspberry Pi, but currently only the official first version with 800x480
resolution is supported.  Infix supplies all drivers required to utilize
the hardware, but you need to add the actual graphical application in a
container.

There are some important considerations you need to know about when
using Infix for graphical applications.  The container needs access to
`/dev/dri/` to be able to access the graphics card, it also need access
to `/run/udev` to be able to find the input devices.

Example of running Doom in Infix:

```
admin@example:/> configure
admin@example:/config/> edit container doom
admin@example:/config/container/doom/> set image docker://mattiaswal/alpine-doom:latest
admin@example:/config/container/doom/> set privileged
admin@example:/config/container/doom/> edit mount udev
admin@example:/config/container/doom/mount/udev/> set type bind
admin@example:/config/container/doom/mount/udev/> set target /run/udev/
admin@example:/config/container/doom/mount/udev/> set source /run/udev/
admin@example:/config/container/doom/mount/udev/> end
admin@example:/config/container/doom/mount/xorg.conf/> set content U2VjdGlvbiAiT3V0cHV0Q2xhc3MiCiAgSWRlbnRpZmllciAidmM0IgogIE1hdGNoRHJpdmVyICJ2YzQiCiAgRHJpdmVyICJtb2Rlc2V0dGluZyIKICBPcHRpb24gIlByaW1hcnlHUFUiICJ0cnVlIgpFbmRTZWN0aW9uCg==
admin@example:/config/container/doom/mount/xorg.conf/> set target /etc/X11/xorg.conf
admin@example:/config/container/doom/mount/xorg.conf/> end
admin@example:/config/container/doom/> edit volume var
admin@example:/config/container/doom/volume/var/> set target /var
admin@example:/config/container/doom/volume/var/> leave
admin@example:/>
```

> [!NOTE]
> The `xorg.conf` [content mount][2] is a nifty detail of Infix that
> allows you to keep all the relevant configuration in a single file.
> The deta is "simply" `base64` encoded, so you do not really need the
> features of the Infix CLI, everything can be set up remotely [using
> `curl`][3] if you like.

### Pre-built images

Pre-built SD card images are available here: [infix-rpi4-sdcard.img][sdcard]

[0]: https://www.raspberrypi.com/products/raspberry-pi-touch-display/
[1]: https://github.com/kernelkit/infix/releases/download/latest-boot/infix-rpi4-sdcard.img
[2]: https://kernelkit.org/infix/latest/container/#content-mounts
[3]: https://kernelkit.org/infix/latest/scripting-restconf/
