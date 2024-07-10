# Hardware Information and Status

The hardware infomation and status is handled by the YANG model [IETF
hardware][1], with deviations and augmentations in _infix-hardware_.

## USB Ports

For Infix to be able to control USB port(s), a device tree modification
is needed (see _alder.dtsi_ for full example).

```
 chosen {
   infix {
     usb-ports = <&cp0_usb3_1>;
	 usb-port-names = "USB";
   };
 };
```

Two USB ports are also exposed in QEMU for test purpose.

All USB ports in the system will be disabled during boot due to the file
`board/common/rootfs/etc/modprobe.d/usbcore.conf`.  If you do not want
Infix to control USB port(s), remove the file or manually enable the USB
bus, here is an example:

```
# Enable the bus
echo 1 > /sys/bus/usb/devices/usb1/authorized
```

And then enable sub-devices (e.g. USB memory)

```
# Enable a device plugged into usb1
echo 1 >  /sys/bus/usb/devices/usb1/1-1/authorized
```

### Current status

```
admin@example:/> show hardware
 USB PORTS
 NAME                STATE
 USB                 unlocked
```

An USB port can be in two states _unlocked_ and _locked_. When a port is
locked, all connected devices will get power, but never authorized by
Linux to use.

### Configure USB port

> **Note:** You can only configure USB ports known to the system.  See
> `show hardware` in admin-exec context.  (Use `do` prefix in configure
> context.)

```
admin@example:/> configure
admin@example:/config/> set hardware component USB state admin-state unlocked
admin@example:/config/> leave
admin@example:/>
```

### Using a USB Stick

With the USB port unlocked, a memory stick can be used to expand the
storage of the device.  Plug it in, either directly, or via a USB hub.
All partitions of *VFAT* or *exFAT* type are automatically mounted to
the `/media/<LABLEL>` directory, e.g., `/media/memorystick`, or if a
partition does not have a label, using the kernel device name, e.g.,
`/media/sda`.

Depending on the partition type the the media is either mounted in
`flush` or `sync` mode to ensure files are properly flushed to the
backing store of the memory stick.  This to protect against users
yanking out a device before file(s) have been written to it.

The only way currently to safely "eject" a USB memory stick is to use
`umount` command from a UNIX shell, which explicitly synchronizes any
cached data to disk before returning the prompt:

```
admin@example:~$ sudo umount /media/log
```


[1]:  https://www.rfc-editor.org/rfc/rfc8348.html
