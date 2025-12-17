# Raspberry Pi 3B/4B/CM4

## Overview

The Raspberry Pi is one of the most popular single-board computers with full
Infix support for networking features, making it an excellent platform for
learning, prototyping, and lightweight network applications.

### Hardware Features

**Raspberry Pi 3B:**

- Broadcom BCM2837B0 ARM Cortex-A53 quad-core processor @ 1.4 GHz
- 1 GB LPDDR2 RAM
- microSD card slot for storage
- 4x USB 2.0 ports
- Fast Ethernet (100 Mbps)
- Dual-band WiFi (2.4 GHz + 5 GHz) and Bluetooth 4.2
- HDMI port
- GPIO header (40-pin)

**Raspberry Pi 4B:**

- Broadcom BCM2711 ARM Cortex-A72 quad-core processor @ 1.5 GHz
- 1 GB, 2 GB, 4 GB, or 8 GB LPDDR4 RAM (depending on model)
- microSD card slot for storage
- 2x USB 3.0 + 2x USB 2.0 ports
- Gigabit Ethernet
- Dual-band WiFi (2.4 GHz + 5 GHz) and Bluetooth 5.0
- 2x micro-HDMI ports (up to 4K output)
- GPIO header (40-pin)
- PoE support (with add-on HAT)

**Compute Module 4 (CM4):**

- Same processor as Pi 4B
- Compact form factor for embedded applications
- Optional eMMC storage (0 GB, 8 GB, 16 GB, or 32 GB)
- Requires carrier board for I/O connectivity
- Various configurations tested and working

### Default Network Configuration

Infix comes preconfigured with:

- **Ethernet port**: DHCP client enabled for internet connectivity
- **WiFi interfaces** (wlan0, wlan1): Available for configuration as AP or client
- **GPIO**: Available but extension boards not currently supported

### Support Level

Full support for base board networking and core functionality. GPIO extension
boards (HATs) are not currently supported. Other Raspberry Pi boards of the
same generation may work but may require additional testing.

## Getting Started

### Quick Start with SD Card

The easiest way to get started is using a microSD card:

1. **Download the SD card image:** [infix-rpi64-sdcard.img][2]
2. **Flash the image to a microSD card:** see [this guide][0]
3. **Boot the board:**
   - Insert the microSD card into your Raspberry Pi
   - Connect an Ethernet cable to your network
   - Connect power (see [Power Supply Requirements](#power-supply-requirements))
   - The board will boot automatically
4. **Connect and login:**
   - SSH to the DHCP-assigned IP address
   - Default login: `admin` / `admin`

> [!NOTE]
> Raspberry Pi 3B and 4B boot with a factory configuration (`factory-config.cfg`)
> that enables DHCP client on the Ethernet port. This means you can access
> the device over the network without needing a serial console. Simply find
> the assigned IP address and SSH in!
>
> **Compute Module 4 (CM4)** and some other variants do not have this factory
> configuration, so you'll need to use a serial console for initial setup or
> configure the carrier board accordingly.

### First Boot Notes

On first boot, Infix will:

- Obtain an IP address via DHCP on the Ethernet port
- Generate unique SSH host keys
- Initialize the configuration system

You can find the assigned IP address by:

- Checking your DHCP server/router's client list
- Using network scanning tools like `nmap` or `arp-scan`
- Connecting via serial console and running `ip addr` (if needed)

## Hardware-Specific Features

### WiFi Configuration

Both Pi 3B and Pi 4B include dual-band WiFi that can be configured as a
client (station mode). See the [Infix WiFi documentation][9] for detailed
configuration examples.

To configure WiFi as a client, first store your WiFi password in the keystore:

```
admin@infix:/> configure
admin@infix:/config/> edit keystore symmetric-key mywifi
admin@infix:/config/keystore/…/mywifi/> set key-format wifi-preshared-key-format
admin@infix:/config/keystore/…/mywifi/> set cleartext-symmetric-key YourWiFiPassword
admin@infix:/config/keystore/…/mywifi/> leave
```

Then configure the WiFi interface using the keystore reference:

```
admin@infix:/> configure
admin@infix:/config/> edit interface wifi0
admin@infix:/config/interface/wifi0/> set ipv4 dhcp-client
admin@infix:/config/interface/wifi0/> set wifi ssid YourNetworkName
admin@infix:/config/interface/wifi0/> set wifi secret mywifi
admin@infix:/config/interface/wifi0/> set wifi country-code US
admin@infix:/config/interface/wifi0/> leave
```

> [!NOTE]
> The WiFi password (8-63 characters) is stored securely in the keystore as
> `mywifi` (or any name you choose), which is then referenced in the WiFi
> configuration. The country-code must match your location for regulatory
> compliance (e.g., US, SE, DE, JP).

### Touch Screen Support

The [Raspberry Pi Touch Display v1][10] (800x480 resolution) is supported on
the Pi 4B, including touch functionality. To use graphical applications with
the touch screen, you need to run them in a container with proper device
access.

#### Requirements for Graphical Applications

Containers need:
- Access to `/dev/dri/` for graphics card access
- Access to `/run/udev` for input device detection
- Privileged mode or specific capabilities

#### Example: Running Doom with Touch Screen

```
admin@infix:/> configure
admin@infix:/config/> edit container doom
admin@infix:/config/container/doom/> set image docker://mattiaswal/alpine-doom:latest
admin@infix:/config/container/doom/> set privileged
admin@infix:/config/container/doom/> edit mount udev
admin@infix:/config/container/…/udev/> set type bind
admin@infix:/config/container/…/udev/> set target /run/udev/
admin@infix:/config/container/…/udev/> set source /run/udev/
admin@infix:/config/container/…/udev/> end
admin@infix:/config/container/doom/> edit mount xorg.conf
admin@infix:/config/container/…/xorg.conf/> set content U2VjdGlvbiAiT3V0cHV0Q2xhc3MiCiAgSWRlbnRpZmllciAidmM0IgogIE1hdGNoRHJpdmVyICJ2YzQiCiAgRHJpdmVyICJtb2Rlc2V0dGluZyIKICBPcHRpb24gIlByaW1hcnlHUFUiICJ0cnVlIgpFbmRTZWN0aW9uCg==
admin@infix:/config/container/…/xorg.conf/> set target /etc/X11/xorg.conf
admin@infix:/config/container/…/xorg.conf/> end
admin@infix:/config/container/doom/> edit volume var
admin@infix:/config/container/…/var/> set target /var
admin@infix:/config/container/…/var/> leave
```

> [!NOTE]
> The `xorg.conf` [content mount][3] is a useful Infix feature that allows you
> to keep all configuration in a single file. The data is base64 encoded, and
> everything can be set up remotely [using `curl`][4] if preferred.

### Power Supply Requirements

Proper power supply is critical for stable operation:

- **Raspberry Pi 4B:** 5V/3A USB-C power supply (official recommended)
- **Raspberry Pi 3B:** 5V/2.5A micro-USB power supply
- **Compute Module 4:** Power requirements depend on carrier board

Inadequate power can cause:
- Random reboots or crashes
- USB device failures
- Network disconnections
- Corrupted storage

<img align="right" src="gpio-pinout.png" alt="GPIO Pinout" width=300 padding=10>

### Serial Console Access (Optional)

A serial console is useful for debugging but not required for Pi 3B/4B, since
the factory configuration enables network access via DHCP. For CM4 and other
variants without factory configuration, serial console access is required for
initial setup.

To connect via serial:

1. Connect a USB-to-TTL serial adapter (3.3V) to GPIO pins:
   - GND (black wire) → Pin 6 (Ground)
   - TX → Pin 8 (GPIO 14, RXD)
   - RX → Pin 10 (GPIO 15, TXD)
   - **VCC (red wire) → Leave disconnected** (Pi has its own power supply)
2. Use 115200 baud, 8 data bits, no parity, 1 stop bit (115200 8N1)
3. Connect using `screen`, `minicom`, or similar terminal emulator

The image shows the standard 40-pin GPIO header pinout. For serial console:
- **Pin 6** (black) = Ground
- **Pin 8** (orange) = TxD (UART) - connect to RX on your adapter
- **Pin 10** (orange) = RxD (UART) - connect to TX on your adapter

> [!WARNING]
> Use only 3.3V serial adapters. 5V adapters will damage your Raspberry Pi!

## Troubleshooting

### Board won't boot

- Verify the power supply meets requirements (see [Power Supply Requirements](#power-supply-requirements))
- Check that the SD card is properly seated
- Try re-flashing the SD card image
- Look for the green LED activity indicator (should flash during boot)
- Connect via serial console to see boot messages if needed

### Can't find IP address

- Ensure Ethernet cable is properly connected (look for link LED activity)
- Verify your DHCP server is running and has available addresses
- Check your router/DHCP server's client list for new devices
- Use `nmap` or `arp-scan` to scan your network
- Connect via serial console and run `ip addr` to see the assigned address

### WiFi not working

- Verify WiFi regulatory domain is set correctly
- Check that SSID and password are correct
- Ensure WiFi channel is supported in your region
- Try connecting to a 2.4 GHz network first (better compatibility)

### SD card corruption

If the system becomes unresponsive or won't boot:

- Always use proper shutdown procedures (don't just pull power)
- Use a quality SD card (Class 10, A1, or better recommended)
- Consider using a UPS or battery backup for critical deployments
- Verify power supply meets requirements (see [Power Supply Requirements](#power-supply-requirements))

### Container/Docker issues

- Verify adequate RAM is available (check with `free -h`)
- Ensure sufficient SD card space (check with `df -h`)
- For Pi 3B, consider using lighter containers due to limited RAM

## Additional Resources

- [Infix Documentation][1]
- [Flashing SD Card Guide][0]
- [Infix Container Documentation][3]
- [Scripting with RESTCONF][4]
- [Release Downloads][8]
- [Official Raspberry Pi Documentation][11]

## Building Custom Images

See the main Infix documentation for building from source. To build a custom
Raspberry Pi image:

```bash
# Build the bootloader (only needed once or when bootloader changes)
make O=x-boot rpi64_boot_defconfig
make O=x-boot

# Build main system
make aarch64_defconfig
make

# Create SD card image
./utils/mkimage.sh -od raspberrypi-rpi64
```

The resulting image will be in `output/images/infix-rpi64-sdcard.img`.

### Customizing the Build

You can customize the build by:
- Modifying `board/aarch64/raspberrypi-rpi64/config.txt` for boot configuration
- Adding packages to the Buildroot configuration
- Customizing the device tree in `board/aarch64/raspberrypi-rpi64/dts/`

## Performance Notes

### Raspberry Pi 4B vs 3B

The Pi 4B offers significant improvements over the 3B:
- 3x faster processor
- Up to 8x more RAM
- Gigabit Ethernet (vs 100 Mbps)
- USB 3.0 for faster storage and peripherals
- Better thermal performance

For network-intensive applications, the Pi 4B is strongly recommended.

### Raspberry Pi as a Router

While capable of basic routing tasks, be aware of limitations:
- Single Ethernet port (consider USB Ethernet adapters for multi-port setups)
- CPU-based packet processing (no hardware offload)
- Best suited for home/lab use rather than high-throughput production

For applications requiring multiple ports or high performance, consider
dedicated networking hardware like the [Banana Pi R3][12].

[0]: https://kernelkit.org/posts/flashing-sdcard/
[1]: https://kernelkit.org/infix/latest/
[2]: https://github.com/kernelkit/infix/releases/download/latest-boot/infix-rpi64-sdcard.img
[3]: https://kernelkit.org/infix/latest/container/#content-mounts
[4]: https://kernelkit.org/infix/latest/scripting-restconf/
[8]: https://github.com/kernelkit/infix/releases/tag/latest-boot
[9]: https://kernelkit.org/infix/latest/networking/#wifi
[10]: https://www.raspberrypi.com/products/raspberry-pi-touch-display/
[11]: https://www.raspberrypi.com/documentation/
[12]: https://kernelkit.org/infix/latest/hardware/#banana-pi-bpi-r3
