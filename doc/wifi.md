# Wi-Fi (Wireless LAN)

Infix includes comprehensive Wi-Fi support for both client (Station) and
Access Point modes. When a compatible Wi-Fi adapter is detected, the system
automatically creates a WiFi radio (PHY) that can host virtual interfaces.

## Architecture Overview

Infix uses a two-layer WiFi architecture:

1. **WiFi Radio (PHY layer)**: Represents the physical wireless hardware
   - Configured via `infix-wifi-radio` module
   - Controls channel, transmit power, regulatory domain
   - One radio can host multiple virtual interfaces

2. **WiFi Interface (Network layer)**: Virtual interface on a radio
   - Configured via `infix-if-wifi` module
   - Can operate in Station (client) or Access Point mode
   - Each interface references a parent radio

## Current Limitations

- USB hotplug is not supported - adapters must be present at boot
- Interface naming may be inconsistent with multiple USB Wi-Fi adapters
- AP and Station modes cannot be mixed on the same radio

## Supported Wi-Fi Adapters

Wi-Fi support is primarily tested with Realtek chipset-based adapters.

### Known Working Chipsets

- RTL8821CU
- Other Realtek chipsets may work but are not guaranteed


> [!NOTE]  Some Realtek chipsets require proprietary drivers not included in the standard kernel
>          Firmware requirements vary by chipset
>          Check kernel logs if your adapter is not detected

## Radio Configuration

Before configuring WiFi interfaces, you must first configure the WiFi radio.
Radios are automatically discovered and named `radio0`, `radio1`, etc.

### Country Code and Regulatory Compliance

> [!IMPORTANT]
> The `country-code` setting is **legally required** and determines which WiFi channels and power levels are permitted in your location. Using an incorrect country code may violate local wireless regulations.

**Factory default**: Systems may ship with a default country code (typically "DE" for Germany in European builds or "00" for World domain). **You must configure the correct country code for your deployment location.**

**Common country codes**:
- Europe: DE (Germany), SE (Sweden), GB (UK), FR (France), ES (Spain)
- Americas: US (United States), CA (Canada), BR (Brazil)
- Asia-Pacific: JP (Japan), AU (Australia), CN (China)

See [ISO 3166-1 alpha-2](https://en.wikipedia.org/wiki/ISO_3166-1_alpha-2) for the complete list.

### Basic Radio Setup

Configure the radio with channel, power, and regulatory domain.

**For Station (client) mode:**
```
admin@example:/> configure
admin@example:/config/> edit hardware component radio0 wifi-radio
admin@example:/config/hardware/component/radio0/wifi-radio/> set country-code DE
admin@example:/config/hardware/component/radio0/wifi-radio/> leave
```

**For Access Point mode:**
```
admin@example:/> configure
admin@example:/config/> edit hardware component radio0 wifi-radio
admin@example:/config/hardware/component/radio0/wifi-radio/> set country-code DE
admin@example:/config/hardware/component/radio0/wifi-radio/> set band 5GHz
admin@example:/config/hardware/component/radio0/wifi-radio/> set channel 36
admin@example:/config/hardware/component/radio0/wifi-radio/> leave
```

**Key radio parameters:**
- `country-code`: Two-letter ISO 3166-1 code - determines allowed channels and maximum power. Examples: US, DE, GB, SE, FR, JP. **Must match your physical location for legal compliance.**
- `band`: 2.4GHz, 5GHz, or 6GHz (required for AP mode). Band selection automatically enables appropriate WiFi standards (2.4GHz: 802.11n, 5GHz: 802.11n/ac, 6GHz: 802.11n/ac/ax)
- `channel`: Channel number (1-196) or "auto" (required for AP mode). When set to "auto", defaults to channel 6 for 2.4GHz, channel 36 for 5GHz, or channel 109 for 6GHz
- `enable-wifi6`: Boolean (default: false). Opt-in to enable WiFi 6 (802.11ax) on 2.4GHz and 5GHz bands. The 6GHz band always uses WiFi 6 regardless of this setting

> [!NOTE]
> TX power and channel width are automatically determined by the driver based on regulatory constraints, PHY mode, and hardware capabilities.

### WiFi 6 (802.11ax) Support

WiFi 6 (802.11ax) provides improved performance in congested environments through
features like OFDMA, Target Wake Time, and BSS Coloring. By default, WiFi 6 is
only enabled on the 6GHz band (WiFi 6E requirement).

To enable WiFi 6 on 2.4GHz or 5GHz bands:

```
admin@example:/> configure
admin@example:/config/> edit hardware component radio0 wifi-radio
admin@example:/config/hardware/component/radio0/wifi-radio/> set country-code DE
admin@example:/config/hardware/component/radio0/wifi-radio/> set band 5GHz
admin@example:/config/hardware/component/radio0/wifi-radio/> set channel 36
admin@example:/config/hardware/component/radio0/wifi-radio/> set enable-wifi6 true
admin@example:/config/hardware/component/radio0/wifi-radio/> leave
```

**WiFi 6 Benefits:**
- **OFDMA**: Better multi-user efficiency in dense environments
- **Target Wake Time**: Improved battery life for client devices
- **1024-QAM**: Higher throughput with strong signal conditions
- **BSS Coloring**: Reduced interference from neighboring networks

**Requirements:**
- Hardware must support 802.11ax
- Client devices must support WiFi 6 for full benefits
- Older WiFi 5/4 clients can still connect but won't use WiFi 6 features

> [!NOTE]
> The 6GHz band always uses WiFi 6 (802.11ax) regardless of the `enable-wifi6`
> setting, as WiFi 6E requires 802.11ax support.

## Station Mode (Client)

Station mode connects to an existing Wi-Fi network. To verify that a
compatible adapter has been detected, look for a radio (e.g., `radio0`) in
`show hardware` or interfaces in `show interface`

```
admin@example:/>  show interface
INTERFACE       PROTOCOL   STATE       DATA
lo              loopback   UP
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
e1              ethernet   UP          02:00:00:00:00:01
                ipv6                   fe80::ff:fe00:1/64 (link-layer)
                ipv6                   fec0::ff:fe00:1/64 (link-layer)
wifi0           ethernet   DOWN        f0:09:0d:36:5f:86
                wifi                   ssid: ------, signal: ------

```
Add the new Wi-Fi interface to the configuration to start scanning.
```
admin@example:/config/> set interface wifi0
admin@example:/config/> leave
```
Now the system will now start scanning in the background. To
see the result read the operational datastore for interface `wifi0` or
use the CLI

```
admin@example:/> show interface wifi0
name                : wifi0
type                : wifi
index               : 3
mtu                 : 1500
operational status  : down
physical address    : f0:09:0d:36:5f:86
ipv4 addresses      :
ipv6 addresses      :
SSID                : ----
Signal              : ----

SSID                                    ENCRYPTION                    SIGNAL
ssid1                                   WPA2-Personal                 excellent
ssid2                                   WPA2-Personal                 excellent
ssid3                                   WPA2-Personal                 excellent
ssid4                                   WPA2-Personal                 good
ssid5                                   WPA2-Personal                 good
ssid6                                   WPA2-Personal                 good
```

In the CLI, signal strength is reported as: excellent, good, poor or
bad. For precise values, use NETCONF or RESTCONF, where the RSSI (in
dBm) is available in the operational datastore.

Configure your Wi-Fi secret in the keystore, it should be between 8
and 63 characters

```
admin@example:/> configure
admin@example:/config/> edit keystore symmetric-key example
admin@example:/config/keystore/…/example/> set key-format wifi-preshared-key-format
admin@example:/config/keystore/…/example/> set symmetric-key mysecret
admin@example:/config/keystore/…/example/> leave
admin@example:/>
```

Configure the Wi-Fi interface to reference the radio and connect to a network:

```
admin@example:/> configure
admin@example:/config/> edit interface wifi0
admin@example:/config/interface/wifi0/> set wifi radio radio0
admin@example:/config/interface/wifi0/> set wifi station ssid ssid1
admin@example:/config/interface/wifi0/> set wifi station security secret example
admin@example:/config/interface/wifi0/> leave
```

**Station configuration parameters:**
- `radio`: Reference to the WiFi radio (mandatory) - must reference a hardware component with class 'wifi' (e.g., radio0)
- `station ssid`: Network name to connect to
- `station security mode`: `auto` (default, WPA2/WPA3 auto-negotiation) or `disabled` (open network)
- `station security secret`: Reference to keystore entry (required when security is not disabled)

WPA2 or WPA3 security will be automatically selected based on what
the access point supports. The default `auto` security mode tries WPA3-SAE first,
then falls back to WPA2-PSK for maximum compatibility and security.

> [!NOTE]  Certificate-based authentication (802.1X/EAP) is not yet supported.

The Wi-Fi negotiation should now start immediately, provided that the
SSID and pre-shared key are correct. You can verify the connection by
running `show interface` again.


```
admin@example:/> show interface
INTERFACE       PROTOCOL   STATE       DATA
lo              loopback   UP
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
e1              ethernet   UP          02:00:00:00:00:01
                ipv6                   fe80::ff:fe00:1/64 (link-layer)
                ipv6                   fec0::ff:fe00:1/64 (link-layer)
wifi0           ethernet   UP          f0:09:0d:36:5f:86
                wifi                   ssid: ssid1, signal: excellent

admin@example:/>
```

## Access Point Mode

Access Point (AP) mode allows your device to create a WiFi network that
other devices can connect to. APs are configured as virtual interfaces on
a WiFi radio.

### Basic AP Configuration

First, ensure the radio is configured (see Radio Configuration above). Then
create an AP interface:

```
admin@example:/> configure
admin@example:/config/> edit interface wifi0
admin@example:/config/interface/wifi0/> set wifi radio radio0
admin@example:/config/interface/wifi0/> set wifi access-point ssid MyNetwork
admin@example:/config/interface/wifi0/> set wifi access-point security mode wpa2-personal
admin@example:/config/interface/wifi0/> set wifi access-point security secret example
admin@example:/config/interface/wifi0/> leave
```

> [!NOTE]
> Using `wifiN` as the interface name automatically sets the type to WiFi.
> Alternatively, you can use any name and explicitly set `type wifi`.

**Access Point configuration parameters:**
- `radio`: Reference to the WiFi radio (mandatory)
- `access-point ssid`: Network name (SSID) to broadcast
- `access-point hidden`: Set to `true` to hide SSID (optional, default: false)
- `access-point security mode`: Security mode (see below)
- `access-point security secret`: Reference to keystore entry (for secured networks)

**Security modes:**
- `open`: No encryption (not recommended)
- `wpa2-personal`: WPA2-PSK (most compatible)
- `wpa3-personal`: WPA3-SAE (more secure, requires WPA3-capable clients)
- `wpa2-wpa3-personal`: Mixed mode (maximum compatibility)

### Hidden Network (SSID Hiding)

To create a hidden network that doesn't broadcast its SSID:

```
admin@example:/config/interface/wifi0/> set wifi access-point hidden true
```

### Multi-SSID Configuration

Multiple AP interfaces on the same radio allow broadcasting multiple SSIDs,
each with independent security settings. This is useful for guest networks,
IoT devices, or segregating traffic into different VLANs.

**Step 1: Configure the radio** (shared by all APs)

```
admin@example:/> configure
admin@example:/config/> edit hardware component radio0 wifi-radio
admin@example:/config/hardware/component/radio0/wifi-radio/> set country-code DE
admin@example:/config/hardware/component/radio0/wifi-radio/> set band 5GHz
admin@example:/config/hardware/component/radio0/wifi-radio/> set channel 36
admin@example:/config/hardware/component/radio0/wifi-radio/> leave
```

**Step 2: Configure keystore secrets**

```
admin@example:/> configure
admin@example:/config/> edit keystore symmetric-key main-secret
admin@example:/config/keystore/…/main-secret/> set key-format wifi-preshared-key-format
admin@example:/config/keystore/…/main-secret/> set symmetric-key MyMainPassword
admin@example:/config/keystore/…/main-secret/> up
admin@example:/config/> edit keystore symmetric-key guest-secret
admin@example:/config/keystore/…/guest-secret/> set key-format wifi-preshared-key-format
admin@example:/config/keystore/…/guest-secret/> set symmetric-key GuestPassword123
admin@example:/config/keystore/…/guest-secret/> up
admin@example:/config/> edit keystore symmetric-key iot-secret
admin@example:/config/keystore/…/iot-secret/> set key-format wifi-preshared-key-format
admin@example:/config/keystore/…/iot-secret/> set symmetric-key IoTDevices2025
admin@example:/config/keystore/…/iot-secret/> leave
```

**Step 3: Create multiple AP interfaces** (all on radio0)

```
admin@example:/> configure
# Primary AP - Main network (WPA3 for maximum security)
admin@example:/config/> edit interface wifi0
admin@example:/config/interface/wifi0/> set wifi radio radio0
admin@example:/config/interface/wifi0/> set wifi access-point ssid MainNetwork
admin@example:/config/interface/wifi0/> set wifi access-point security mode wpa3-personal
admin@example:/config/interface/wifi0/> set wifi access-point security secret main-secret
admin@example:/config/interface/wifi0/> up

# Guest AP - Guest network (WPA2/WPA3 mixed for compatibility)
admin@example:/config/> edit interface wifi1
admin@example:/config/interface/wifi1/> set wifi radio radio0
admin@example:/config/interface/wifi1/> set wifi access-point ssid GuestNetwork
admin@example:/config/interface/wifi1/> set wifi access-point security mode wpa2-wpa3-personal
admin@example:/config/interface/wifi1/> set wifi access-point security secret guest-secret
admin@example:/config/interface/wifi1/> up

# IoT AP - IoT devices (WPA2 for older device compatibility)
admin@example:/config/> edit interface wifi2
admin@example:/config/interface/wifi2/> set wifi radio radio0
admin@example:/config/interface/wifi2/> set wifi access-point ssid IoT-Devices
admin@example:/config/interface/wifi2/> set wifi access-point security mode wpa2-personal
admin@example:/config/interface/wifi2/> set wifi access-point security secret iot-secret
admin@example:/config/interface/wifi2/> leave
```

**Result:** Three SSIDs broadcasting simultaneously on radio0:
- `MainNetwork` (WPA3, most secure)
- `GuestNetwork` (WPA2/WPA3 mixed mode)
- `IoT-Devices` (WPA2 for compatibility)

All APs on the same radio share the same channel and physical layer settings
(configured at the radio level). Each AP can have its own:
- SSID (network name)
- Security mode and passphrase
- Hidden/visible SSID setting
- VLAN assignment
- Bridge membership

You can verify the configuration with `show hardware component radio0` to see
radio settings, and `show interface` to see all active AP interfaces.

> [!IMPORTANT]
> AP and Station modes cannot be mixed on the same radio. All virtual interfaces
> on a radio must be the same mode (all APs or all Stations).

### AP as Bridge Port

WiFi AP interfaces can be added to bridges to integrate wireless devices
into your LAN:

```
admin@example:/> configure
admin@example:/config/> edit interface br0
admin@example:/config/interface/br0/> set type bridge
admin@example:/config/interface/br0/> up

admin@example:/config/> edit interface wifi0
admin@example:/config/interface/wifi0/> set bridge-port bridge br0
admin@example:/config/interface/wifi0/> leave
```

## Troubleshooting Connection Issues

Use `show wifi scan wifi0` and `show interface` to verify signal strength
and connection status. If issues arise, try the following
troubleshooting steps:

1. **Verify signal strength**: Check that the target network shows "good" or "excellent" signal
2. **Check credentials**: Verify the preshared key in `ietf-keystore`
3. **Review logs**: Check system logs with `show log` for Wi-Fi related errors
4. **Regulatory compliance**: Ensure the country-code matches your location
5. **Hardware detection**: Confirm the adapter appears in `show interface`

If issues persist, check the system log for specific error messages that can help identify the root cause.
