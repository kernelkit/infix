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

## Naming Conventions

Like other interface types in Infix, WiFi components follow naming conventions
that allow the CLI to automatically infer types:

| **Name Pattern** | **Type**        | **Description**                          |
|------------------|-----------------|------------------------------------------|
| `radioN`         | WiFi Radio      | Hardware component for WiFi PHY          |
| `wifiN`          | WiFi Interface  | Virtual WiFi interface on a radio        |

Where `N` is a number (0, 1, 2, ...).

> [!TIP]
> Using these naming conventions simplifies configuration since type/class
> settings are automatically inferred.  For example, creating a hardware
> component named `radio0` automatically sets its class to `wifi`, enabling
> the `wifi-radio` configuration container.
>
> **Note:** This inference only works with the CLI.  Configuring WiFi over
> NETCONF or RESTCONF requires setting the class/type explicitly.

## Current Limitations

- USB hotplug is not supported - adapters must be present at boot
- Interface naming may be inconsistent with multiple USB Wi-Fi adapters
- AP and Station modes cannot be mixed on the same radio

## Supported Wi-Fi Adapters

Wi-Fi support is primarily tested with Realtek chipset-based adapters.

### Known Working Chipsets

- Built-in Wi-Fi on Banana Pi BPi-R3
- Built-in Wi-Fi on Raspberry Pi 4/CM4
- Realtek:
    - RTL8188CU
    - RTL8188FU
    - RTL8821CU

Other Realtek chipsets may work but are not tested.

> [!NOTE]
> Some Realtek chipsets require proprietary drivers not included in the
> standard kernel.
>
> - Firmware requirements vary by chipset
> - Check kernel logs if your adapter is not detected

### USB WiFi Dongles

USB WiFi dongles may be slow to initialize at boot due to firmware
loading. If your USB dongle is not detected reliably, configure a
`probe-timeout` on the radio to wait for the PHY:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit hardware component radio0 wifi-radio</b>
admin@example:/config/hardware/component/radio0/wifi-radio/> <b>set probe-timeout 30</b>
admin@example:/config/hardware/component/radio0/wifi-radio/> <b>leave</b>
</code></pre>

This waits up to 30 seconds for the radio PHY to appear before creating
WiFi interfaces. If the PHY is not detected within the timeout, a dummy
interface is created as a placeholder, allowing IP configuration to proceed.
Reboot when the radio becomes available.

## Radio Configuration

Before configuring WiFi interfaces, you must first configure the WiFi radio.
Radios are automatically discovered and named `radio0`, `radio1`, etc.

### Country Code ⚠

The radio defaults to "00" for World domain, but some systems may ship with a
factory default country code (typically "DE" for the BPi-R3).

> [!IMPORTANT] Legal notice!
> The `country-code` setting is **legally required** and determines
> which WiFi channels and power levels are permitted in your
> location. Using an incorrect country code may violate local wireless
> regulations.

**Common country codes, see [ISO 3166-1 alpha-2][1] for the complete list**:

- Europe:
    - DE: Germany
    - SE: Sweden
    - GB: UK
    - FR: France
    - ES: Spain
- Americas:
    - US: United States
    - CA: Canada
    - BR: Brazil
- Asia-Pacific:
	- JP: Japan
	- AU: Australia
	- CN: China

### Basic Radio Setup

Configure the radio with channel, power, and regulatory domain.

**For Station (client) mode:**

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit hardware component radio0 wifi-radio</b>
admin@example:/config/hardware/component/radio0/wifi-radio/> <b>set country-code DE</b>
admin@example:/config/hardware/component/radio0/wifi-radio/> <b>leave</b>
</code></pre>

**For Access Point mode:**

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit hardware component radio0 wifi-radio</b>
admin@example:/config/hardware/component/radio0/wifi-radio/> <b>set country-code DE</b>
admin@example:/config/hardware/component/radio0/wifi-radio/> <b>set band 5GHz</b>
admin@example:/config/hardware/component/radio0/wifi-radio/> <b>set channel 36</b>
admin@example:/config/hardware/component/radio0/wifi-radio/> <b>leave</b>
</code></pre>

**Key radio parameters:**

- `country-code`: Two-letter [ISO 3166-1 alpha-2][1] code, determines allowed
  channels and maximum power. Examples: US, DE, GB, SE, FR, JP.  
  **⚠ Must match your physical location for legal compliance! ⚠**
- `band`: 2.4GHz, 5GHz, or 6GHz (required for AP mode). Automatically enables
  appropriate WiFi standards:
      - 2.4GHz: 802.11n/ax
      - 5GHz: 802.11n/ac/ax
      - 6GHz: 802.11ax
- `channel`: Channel number (1-196) or "auto".  When set to "auto", defaults to
  channel 6 for 2.4GHz, channel 36 for 5GHz, or channel 109 for 6GHz
- `probe-timeout`: Seconds to wait for PHY detection at boot (default: 0).  Set
  to a non-zero value (e.g., 30) for USB WiFi dongles that are slow to
  initialize due to firmware loading

> [!NOTE]
> TX power and channel width are automatically determined by the driver
> based on regulatory constraints, PHY mode, and hardware capabilities.

### WiFi 6 Support

WiFi 6 (802.11ax) is always enabled in AP mode on all bands, providing improved
performance through features like OFDMA, BSS Coloring, and beamforming.

**WiFi 6 Features (always enabled):**

- **OFDMA**: Better multi-user efficiency in dense environments
- **BSS Coloring**: Reduced interference from neighboring networks
- **Beamforming**: Improved signal quality and range

**Requirements:**

- Hardware must support 802.11ax
- Client devices must support WiFi 6 for full benefits
- Older WiFi 5/4 clients can still connect but won't use WiFi 6 features

## Discovering Available Networks

Before connecting to a WiFi network, you need to discover which networks
are available. Infix automatically scans for networks when a WiFi interface
is created with a radio reference.

### Enable Background Scanning

To enable scanning without connecting, configure the radio and create a WiFi
interface referencing it:

**Step 1: Configure the radio**

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit hardware component radio0 wifi-radio</b>
admin@example:/config/hardware/component/radio0/wifi-radio/> <b>set country-code DE</b>
admin@example:/config/hardware/component/radio0/wifi-radio/> <b>leave</b>
</code></pre>

**Step 2: Create WiFi interface with radio reference only**

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interface wifi0</b>
admin@example:/config/interface/wifi0/> <b>set wifi radio radio0</b>
admin@example:/config/interface/wifi0/> <b>leave</b>
</code></pre>

The system will now start scanning in the background. The interface will
operate in scan-only mode until you configure a specific mode (station or
access-point).

### View Available Networks

Use `show interface` to see discovered networks and their signal strength:

<pre class="cli"><code>admin@example:/> <b>show interface wifi0</b>
name                : wifi0
type                : wifi
index               : 3
mtu                 : 1500
operational status  : up
ip forwarding       : enabled
physical address    : f0:09:0d:36:5f:86
ipv4 addresses      : 192.168.1.100/24 (dhcp)
ipv6 addresses      :
in-octets           : 148388
out-octets          : 24555
mode                : station
ssid                : MyNetwork
signal              : -45 dBm (good)
rx bitrate          : 72.2 Mbps
tx bitrate          : 86.6 Mbps
──────────────────────────────────────────────────────────────────────
<span class="title">Available Networks</span>
<span class="header">SSID                 BSSID              SECURITY       SIGNAL  CHANNEL</span>
MyNetwork            b4:fb:e4:17:b6:a7  WPA2-Personal  good          6
GuestWiFi            c8:3a:35:12:34:56  WPA2-Personal  fair         11
CoffeeShop           00:1a:2b:3c:4d:5e  Open           bad           1
</code></pre>

In the CLI, signal strength is reported as: excellent, good, fair or bad.
For precise signal strength values in dBm, use NETCONF or RESTCONF to access
the `signal-strength` leaf in the operational datastore.

## Passphrase Requirements

To ensure your connection is secure and compatible with all network
hardware, your passphrase must meet the following criteria:

- Length: Between 8 and 63 characters
- Characters: Use only standard English keyboard characters
    - Allowed: Letters (A-Z, a-z), numbers (0-9), and common symbols (e.g., ! @ # $ % ^ & * ( ) _ + - = [ ] { } | ; : ' " , . < > / ? ~)
    - Spaces: Spaces are allowed, but not at the very beginning or very end of the passphrase
    - Prohibited: Emojis, accented characters (like á or ñ), and special "control" characters

> [!TIP] Why the limit?
> Standard WiFi security (WPA2/WPA3) requires a minimum of 8 characters to
> prevent "brute-force" hacking.  The character limit ensures your password
> works on older routers and various operating systems.  
> Tips for password strength, see [XKCD #936](https://xkcd.com/936/).

## Station Mode (Client)

Station mode connects to an existing Wi-Fi network. Before configuring station
mode, follow the "Discovering Available Networks (Scanning)" section above to
scan for available networks and identify the SSID you want to connect to.

### Step 1: Configure Password

Create a keystore entry for your WiFi password (8-63 characters):

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit keystore symmetric-key my-wifi-key</b>
admin@example:/config/keystore/…/my-wifi-key/> <b>set key-format passphrase-key-format</b>
admin@example:/config/keystore/…/my-wifi-key/> <b>change cleartext-symmetric-key</b>
Passphrase: ************
Retype passphrase: ************
admin@example:/config/keystore/…/my-wifi-key/> <b>leave</b>
</code></pre>

The `change` command prompts for the passphrase interactively and
handles the base64 encoding required by the keystore automatically.

### Step 2: Connect to Network

Configure station mode with the SSID and password to connect:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interface wifi0</b>
admin@example:/config/interface/wifi0/> <b>set wifi station ssid MyHomeNetwork</b>
admin@example:/config/interface/wifi0/> <b>set wifi station security secret my-wifi-key</b>
admin@example:/config/interface/wifi0/> <b>leave</b>
</code></pre>

The connection attempt will start immediately. You can verify the connection status:

<pre class="cli"><code>admin@example:/> <b>show interface wifi0</b>
name                : wifi0
type                : wifi
operational status  : up
physical address    : f0:09:0d:36:5f:86
mode                : station
ssid                : MyHomeNetwork
signal              : -52 dBm (good)
</code></pre>

**Station configuration parameters:**

- `radio`: Reference to the WiFi radio (mandatory) - already set during scanning
- `station ssid`: Network name to connect to (mandatory)
- `station security mode`:
      - `auto`: default, WPA2/WPA3 auto-negotiation
      - `disabled`: open network
- `station security secret`: Reference to keystore entry, required unless mode
  is `disabled`

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
create a keystore entry for your WiFi password and configure the AP interface:

**Step 1: Create keystore entry for the WiFi password**

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit keystore symmetric-key my-wifi-secret</b>
admin@example:/config/keystore/…/my-wifi-secret/> <b>set key-format passphrase-key-format</b>
admin@example:/config/keystore/…/my-wifi-secret/> <b>change cleartext-symmetric-key</b>
Passphrase: ************
Retype passphrase: ************
admin@example:/config/keystore/…/my-wifi-secret/> <b>end</b>
</code></pre>

**Step 2: Create the AP interface**

<pre class="cli"><code>admin@example:/config/> <b>edit interface wifi0</b>
admin@example:/config/interface/wifi0/> <b>set wifi radio radio0</b>
admin@example:/config/interface/wifi0/> <b>set wifi access-point ssid MyNetwork</b>
admin@example:/config/interface/wifi0/> <b>set wifi access-point security mode wpa2-personal</b>
admin@example:/config/interface/wifi0/> <b>set wifi access-point security secret my-wifi-secret</b>
admin@example:/config/interface/wifi0/> <b>leave</b>
</code></pre>

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

### SSID Hiding

To create a hidden network that doesn't broadcast its SSID:

<pre class="cli"><code>admin@example:/config/interface/wifi0/> <b>set wifi access-point hidden true</b>
</code></pre>

### Multi-SSID Configuration

Multiple AP interfaces on the same radio allow broadcasting multiple SSIDs,
each with independent security settings. This is useful for guest networks,
IoT devices, or segregating traffic into different VLANs.

**Step 1: Configure the radio** (shared by all APs)

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit hardware component radio0 wifi-radio</b>
admin@example:/config/hardware/component/radio0/wifi-radio/> <b>set country-code DE</b>
admin@example:/config/hardware/component/radio0/wifi-radio/> <b>set band 5GHz</b>
admin@example:/config/hardware/component/radio0/wifi-radio/> <b>set channel 36</b>
admin@example:/config/hardware/component/radio0/wifi-radio/> <b>leave</b>
</code></pre>

**Step 2: Configure keystore secrets**

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit keystore symmetric-key main-secret</b>
admin@example:/config/keystore/…/main-secret/> <b>set key-format passphrase-key-format</b>
admin@example:/config/keystore/…/main-secret/> <b>change cleartext-symmetric-key</b>
Passphrase: ************
Retype passphrase: ************
admin@example:/config/> <b>edit keystore symmetric-key guest-secret</b>
admin@example:/config/keystore/…/guest-secret/> <b>set key-format passphrase-key-format</b>
admin@example:/config/keystore/…/guest-secret/> <b>change cleartext-symmetric-key</b>
Passphrase: ************
Retype passphrase: ************
admin@example:/config/> <b>edit keystore symmetric-key iot-secret</b>
admin@example:/config/keystore/…/iot-secret/> <b>set key-format passphrase-key-format</b>
admin@example:/config/keystore/…/iot-secret/> <b>change cleartext-symmetric-key</b>
Passphrase: ************
Retype passphrase: ************
admin@example:/config/keystore/…/iot-secret/> <b>leave</b>
</code></pre>

**Step 3: Create multiple AP interfaces** (all on radio0)

<pre class="cli"><code>admin@example:/> <b>configure</b>
# Primary AP - Main network (WPA3 for maximum security)
admin@example:/config/> <b>edit interface wifi0</b>
admin@example:/config/interface/wifi0/> <b>set wifi radio radio0</b>
admin@example:/config/interface/wifi0/> <b>set wifi access-point ssid MainNetwork</b>
admin@example:/config/interface/wifi0/> <b>set wifi access-point security mode wpa3-personal</b>
admin@example:/config/interface/wifi0/> <b>set wifi access-point security secret main-secret</b>

# Guest AP - Guest network (WPA2/WPA3 mixed for compatibility)
admin@example:/config/> <b>edit interface wifi1</b>
admin@example:/config/interface/wifi1/> <b>set wifi radio radio0</b>
admin@example:/config/interface/wifi1/> <b>set wifi access-point ssid GuestNetwork</b>
admin@example:/config/interface/wifi1/> <b>set wifi access-point security mode wpa2-wpa3-personal</b>
admin@example:/config/interface/wifi1/> <b>set wifi access-point security secret guest-secret</b>
admin@example:/config/interface/wifi1/> <b>set custom-phys-address static 00:0c:43:26:60:01</b>

# IoT AP - IoT devices (WPA2 for older device compatibility)
admin@example:/config/> <b>edit interface wifi2</b>
admin@example:/config/interface/wifi2/> <b>set wifi radio radio0</b>
admin@example:/config/interface/wifi2/> <b>set wifi access-point ssid IoT-Devices</b>
admin@example:/config/interface/wifi2/> <b>set wifi access-point security mode wpa2-personal</b>
admin@example:/config/interface/wifi2/> <b>set wifi access-point security secret iot-secret</b>
admin@example:/config/interface/wifi2/> <b>set custom-phys-address static 00:0c:43:26:60:02</b>
admin@example:/config/interface/wifi2/> <b>leave</b>
</code></pre>

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

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interface br0</b>
admin@example:/config/interface/br0/> <b>set type bridge</b>

admin@example:/config/> <b>edit interface wifi0</b>
admin@example:/config/interface/wifi0/> <b>set bridge-port bridge br0</b>
admin@example:/config/interface/wifi0/> <b>leave</b>
</code></pre>

## Troubleshooting

Use `show interface wifi0` to verify signal strength and connection status.
If issues arise, try the following troubleshooting steps:

1. **Verify signal strength**: Check that the target network shows
   "good" or "excellent" signal in scan results
2. **Check credentials**: Use `show keystore symmetric <name>` to verify
   the passphrase matches the network password
3. **Review logs**: Check system logs with `show log` for Wi-Fi related
   errors
4. **Regulatory compliance**: Ensure the country-code on the radio
   matches your location
5. **Hardware detection**: Confirm the WiFi radio appears in `show
   hardware`

If issues persist, check the system log for specific error messages that
can help identify the root cause.

[1]: https://en.wikipedia.org/wiki/ISO_3166-1_alpha-2
