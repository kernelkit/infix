# Hardware information and status

The hardware infomation and status is handled by the YANG model [IETF hardware][ietf-hardware].
with deviations and augmentations in _infix-hardware_.

## USB Ports
For infix to be able to control the USB port(s), a device tree modification is needed (see _alder.dtsi_ for full example).
```
 chosen {
   infix {
     usb-ports = <&cp0_usb3_1>;
	 usb-port-names = "USB";
   };
 };
```
Two USB ports are also exposed in QEMU for test purpose.

All USB ports in the system will be disabled during boot due to the file _board/common/rootfs/etc/modprobe.d/usbcore.conf_.
If you not want Infix to controll the USB port(s), remove the file or manually enable the USB bus, here is an example:

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
An USB port can be in two states _unlocked_ and _locked_. When a port is locked,
all connected devices will get power, but never authorized by Linux to use.

### Configure USB port
> **Note:** You can only configure an USB ports that exist in _show hardware_ in admin exec.

```
 root@example:/> configure
 root@example:/config/> set hardware component USB state admin-state unlocked
 root@example:/config/> leave
 root@example:/>
```

[ietf-hardware]:         https://www.rfc-editor.org/rfc/rfc8348.html
