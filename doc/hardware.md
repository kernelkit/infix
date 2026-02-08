# Hardware Information and Status

The hardware infomation and status is handled by the YANG model [IETF
hardware][1], with deviations and augmentations in _infix-hardware_.

## GPS/GNSS Receivers

Infix supports GPS/GNSS receivers for hardware status monitoring and NTP
time synchronization.  USB GPS receivers using the USB ACM interface are
supported (e.g., u-blox).  When connected, devices are automatically
discovered and named `gps0`, `gps1`, etc.

### Current status

<pre class="cli"><code>admin@example:/> <b>show hardware</b>
<span class="header">HARDWARE COMPONENTS                                           </span>
──────────────────────────────────────────────────────────────
<span class="title">GPS/GNSS Receivers                                           </span>
Name                : gps0
Device              : /dev/gps0
Driver              : u-blox
Status              : Active
Fix                 : 3D
Satellites          : 10/15 (used/visible)
Position            : 59.334567N 18.063456E 42.3m
PPS                 : Available
</code></pre>

Available information per receiver:

| Field      | Description                                       |
|------------|---------------------------------------------------|
| Name       | Component name (`gps0`, `gps1`, ...)              |
| Device     | Device path (`/dev/gps0`)                         |
| Driver     | Protocol driver (e.g., `u-blox`, `NMEA`, `SiRF`)  |
| Status     | `Active` or `Inactive`                            |
| Fix        | `NONE`, `2D`, or `3D`                             |
| Satellites | Used/visible count                                |
| Position   | Latitude, longitude, altitude (when fix acquired) |
| PPS        | Pulse Per Second signal availability              |

### Configure GPS receiver

GPS receivers are hardware components with class `gps`.  The class is
auto-inferred from the component name, similar to WiFi radios (`radioN`):

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>set hardware component gps0</b>
admin@example:/config/> <b>leave</b>
</code></pre>

To use a GPS receiver as an NTP reference clock source, see
[NTP — GPS Reference Clock](ntp.md#gps-reference-clock).


## USB Ports

For Infix to be able to control USB port(s), a device tree modification
is needed (see _alder.dtsi_ for full example).

```json
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

```bash
# Enable the bus
echo 1 > /sys/bus/usb/devices/usb1/authorized
```

And then enable sub-devices (e.g. USB memory)

```bash
# Enable a device plugged into usb1
echo 1 >  /sys/bus/usb/devices/usb1/1-1/authorized
```

### Current status

<pre class="cli"><code>admin@example:/> <b>show hardware</b>
<span class="header">HARDWARE COMPONENTS                                           </span>
──────────────────────────────────────────────────────────────
<span class="title">Board Information                                            </span>
Model               : FriendlyElec NanoPi R2S
Manufacturer        : FriendlyElec
Serial Number       : 9d1fbfdab6d171ce
Base MAC Address    : 4a:dc:d8:20:0d:85
──────────────────────────────────────────────────────────────
<span class="title">USB Ports                                                    </span>
<span class="header">NAME                                         STATE     OPER   </span>
USB                                          unlocked  enabled
──────────────────────────────────────────────────────────────
<span class="title">Sensors                                                      </span>
<span class="header">NAME                          VALUE                 STATUS    </span>
soc                           44.1 °C               ok        
soc                           44.5 °C               ok        
</code></pre>

An USB port can be in two states _unlocked_ and _locked_. When a port is
locked, all connected devices will get power, but never authorized by
Linux to use.

### Configure USB port

> [!NOTE]
> You can only configure USB ports known to the system.  See the CLI
> command `show hardware` in admin-exec context.  (Use `do` prefix in
> configure context.)

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>set hardware component USB state admin-state unlocked</b>
admin@example:/config/> <b>leave</b>
admin@example:/>
</code></pre>

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

<pre class="cli"><code>admin@example:~$ <b>sudo umount /media/log</b>
</code></pre>

[1]:  https://www.rfc-editor.org/rfc/rfc8348.html
