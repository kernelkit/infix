# Wi-Fi (Wireless LAN)

Infix includes comprehensive Wi-Fi support for both client (Station) and
Access Point modes. When a compatible Wi-Fi adapter is detected, the system
automatically creates a WiFi radio (PHY) in factory-config, that can
host virtual interfaces.

## Architecture Overview

Infix uses a two-layer WiFi architecture:

1. **WiFi Radio (PHY layer)**: Represents the physical wireless hardware
   - Configured via `ietf-hardware` module
   - Controls channel, transmit power, regulatory domain
   - One radio can host multiple virtual interfaces

2. **WiFi Interface (Network layer)**: Virtual interface on a radio
   - Configured via `infix-interfaces` module
   - Can operate in Station (client) or Access Point mode
   - Each interface references a parent radio

## Current Limitations

- USB hotplug is not supported - adapters must be present at boot
- Interface naming may be inconsistent with multiple USB Wi-Fi adapters
- AP and Station modes cannot be mixed on the same radio

## Supported Wi-Fi Adapters

Wi-Fi support is primarily tested with Realtek chipset-based adapters.

### Known Working Chipsets

- Built-in Wi-Fi on Banana Pi r3
- Built-in Wi-Fi on Raspberry Pi 4/CM4
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

## Discovering Available Networks (Scanning)

Before connecting to a WiFi network, you need to discover which networks
are available. Infix automatically scans for networks when a WiFi interface
is created with a radio reference.

### Enable Background Scanning

To enable scanning without connecting, configure the radio and create a WiFi
interface referencing it:

**Step 1: Configure the radio**

```
admin@example:/> configure
admin@example:/config/> edit hardware component radio0 wifi-radio
admin@example:/config/hardware/component/radio0/wifi-radio/> set country-code DE
admin@example:/config/hardware/component/radio0/wifi-radio/> leave
```

**Step 2: Create WiFi interface with radio reference only**

```
admin@example:/> configure
admin@example:/config/> edit interface wifi0
admin@example:/config/interface/wifi0/> set wifi radio radio0
admin@example:/config/interface/wifi0/> leave
```

The system will now start scanning in the background. The interface will
operate in scan-only mode until you configure a specific mode (station or
access-point).

### View Available Networks

Use `show interface` to see discovered networks and their signal strength:

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

SSID                                    SECURITY                      SIGNAL
MyNetwork                               WPA2-Personal                 excellent
GuestWiFi                               WPA2-WPA3-Personal            good
CoffeeShop                              Open                          fair
IoT-Devices                             WPA2-Personal                 good
```

In the CLI, signal strength is reported as: excellent, good, fair or bad.
For precise RSSI values in dBm, use NETCONF or RESTCONF to access the
operational datastore directly.

### Connect to a Network

Once you've identified the desired network from the scan results, configure
station mode with the SSID and credentials. First, store your WiFi password
in the keystore:

```
admin@example:/> configure
admin@example:/config/> edit keystore symmetric-key my-wifi-key
admin@example:/config/keystore/…/my-wifi-key/> set key-format wifi-preshared-key-format
admin@example:/config/keystore/…/my-wifi-key/> set symmetric-key YourWiFiPassword
admin@example:/config/keystore/…/my-wifi-key/> leave
```

Then configure the WiFi interface for station mode:

```
admin@example:/> configure
admin@example:/config/> edit interface wifi0
admin@example:/config/interface/wifi0/> set wifi station ssid MyNetwork
admin@example:/config/interface/wifi0/> set wifi station security secret my-wifi-key
admin@example:/config/interface/wifi0/> leave
```

The interface will transition from scan-only mode to station mode and
attempt to connect to the specified network.

## Station Mode (Client)

Station mode connects to an existing Wi-Fi network. Before configuring station
mode, follow the "Discovering Available Networks (Scanning)" section above to
scan for available networks and identify the SSID you want to connect to.

### Step 1: Configure WiFi Password

Create a keystore entry for your WiFi password (8-63 characters):

```
admin@example:/> configure
admin@example:/config/> edit keystore symmetric-key my-wifi-key
admin@example:/config/keystore/…/my-wifi-key/> set key-format wifi-preshared-key-format
admin@example:/config/keystore/…/my-wifi-key/> set symmetric-key MyPassword123
admin@example:/config/keystore/…/my-wifi-key/> leave
```

### Step 2: Connect to Network

Configure station mode with the SSID and password to connect:

```
admin@example:/> configure
admin@example:/config/> edit interface wifi0
admin@example:/config/interface/wifi0/> set wifi station ssid MyHomeNetwork
admin@example:/config/interface/wifi0/> set wifi station security secret my-wifi-key
admin@example:/config/interface/wifi0/> leave
```

The connection attempt will start immediately. You can verify the connection status:

```
admin@example:/> show interface wifi0
name                : wifi0
type                : wifi
operational status  : up
physical address    : f0:09:0d:36:5f:86
SSID                : MyHomeNetwork
Signal              : excellent
```

**Station configuration parameters:**
- `radio`: Reference to the WiFi radio (mandatory) - already set during scanning
- `station ssid`: Network name to connect to (mandatory)
- `station security mode`: `auto` (default, WPA2/WPA3 auto-negotiation) or `disabled` (open network)
- `station security secret`: Reference to keystore entry (required unless mode is `disabled`)

> [!NOTE]
> The `auto` security mode automatically selects WPA3-SAE or WPA2-PSK based on
> what the access point supports, prioritizing WPA3 for better security.
> Certificate-based authentication (802.1X/EAP) is not yet supported.

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
admin@example:/config/> edit keystore symmetric-key guest-secret
admin@example:/config/keystore/…/guest-secret/> set key-format wifi-preshared-key-format
admin@example:/config/keystore/…/guest-secret/> set symmetric-key GuestPassword123
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

# Guest AP - Guest network (WPA2/WPA3 mixed for compatibility)
admin@example:/config/> edit interface wifi1
admin@example:/config/interface/wifi1/> set wifi radio radio0
admin@example:/config/interface/wifi1/> set wifi access-point ssid GuestNetwork
admin@example:/config/interface/wifi1/> set wifi access-point security mode wpa2-wpa3-personal
admin@example:/config/interface/wifi1/> set wifi access-point security secret guest-secret
admin@example:/config/interface/wifi1/> set custom-phys-address static 00:0c:43:26:60:01

# IoT AP - IoT devices (WPA2 for older device compatibility)
admin@example:/config/> edit interface wifi2
admin@example:/config/interface/wifi2/> set wifi radio radio0
admin@example:/config/interface/wifi2/> set wifi access-point ssid IoT-Devices
admin@example:/config/interface/wifi2/> set wifi access-point security mode wpa2-personal
admin@example:/config/interface/wifi2/> set wifi access-point security secret iot-secret
admin@example:/config/interface/wifi2/> set custom-phys-address static 00:0c:43:26:60:02
admin@example:/config/interface/wifi2/> leave
```

> [!IMPORTANT]
> **MAC Address Requirement for Multi-SSID:**
> When creating multiple AP interfaces on the same radio, you **must** configure
> a unique MAC address for each secondary interface (wifi1, wifi2, etc.) using
> `set custom-phys-address static <MAC>`. All interfaces on the same radio inherit
> the radio's hardware MAC address by default, which causes network conflicts. Only
> the primary interface (alphabetically first, e.g., wifi0) should use the default
> hardware MAC address.
>
> Choose MAC addresses from the same locally-administered range:
> - Primary (wifi0): Uses hardware MAC (e.g., `00:0c:43:26:60:00`)
> - Secondary (wifi1): `00:0c:43:26:60:01` (increment last octet)
> - Tertiary (wifi2): `00:0c:43:26:60:02` (increment last octet)

**Result:** Three SSIDs broadcasting simultaneously on radio0:
- `MainNetwork` (WPA3, most secure)
- `GuestNetwork` (WPA2/WPA3 mixed mode)
- `IoT-Devices` (WPA2 for compatibility)

All APs on the same radio share the same channel and physical layer settings
(configured at the radio level). Each AP can have its own:
- SSID (network name)
- Security mode and passphrase
- Hidden/visible SSID setting
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

admin@example:/config/> edit interface wifi0
admin@example:/config/interface/wifi0/> set bridge-port bridge br0
admin@example:/config/interface/wifi0/> leave
```

## Troubleshooting Connection Issues

Use `show interface wifi0` to verify signal strength and connection status.
If issues arise, try the following troubleshooting steps:

1. **Verify signal strength**: Check that the target network shows "good" or "excellent" signal in scan results
2. **Check credentials**: Verify the preshared key in the keystore matches the network password
3. **Review logs**: Check system logs with `show log` for Wi-Fi related errors
4. **Regulatory compliance**: Ensure the country-code on the radio matches your location
5. **Hardware detection**: Confirm the WiFi radio appears in `show hardware`

If issues persist, check the system log for specific error messages that can help identify the root cause.
