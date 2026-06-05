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
admin@example:/config/hardware/component/radio0/wifi-radio/> <b>set channel-width 80MHz</b>
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
- `channel`: Channel number (1-233) or "auto".  When set to "auto", defaults to
  channel 6 for 2.4GHz, channel 36 for 5GHz, or channel 37 for 6GHz
- `channel-width`: AP channel bandwidth.  Supported values are `auto`, `20MHz`,
  `40MHz`, `80MHz`, and `160MHz`.  Wider channels require matching hardware,
  regulatory approval, and are only available on 5GHz/6GHz where supported.
- `legacy-rates`: Allow legacy 802.11b rates (1, 2, 5.5, 11 Mbps) on 2.4GHz
  (default: disabled).  Slow 802.11b clients consume excessive airtime and
  degrade throughput for all stations, so the rates are normally suppressed.
  Enable only when old 2.4GHz-only IoT devices need them to associate.  No
  effect on 5GHz/6GHz.
- `probe-timeout`: Seconds to wait for PHY detection at boot (default: 0).  Set
  to a non-zero value (e.g., 30) for USB WiFi dongles that are slow to
  initialize due to firmware loading

> [!NOTE]
> TX power is still determined by the driver based on regulatory
> constraints and hardware capabilities.  Channel width can now be set
> explicitly for AP mode, or left at `auto` to let the driver choose.

### Bands and Channels

Each band strikes a different balance between range and capacity.  The
`country-code` decides which channels are legal in your location; the
lists below are the common allocations, and your regulatory domain may
allow fewer.

**2.4 GHz**

Channels 1-13 are available in most of the world, 1-11 in the US and
Canada, and 14 in Japan (802.11b only).  At 20 MHz only three channels
avoid overlap: 1, 6, and 11.  A 40 MHz channel takes up most of the
band, so it is seldom worth using here.

Drawbacks:

- This is the most crowded band.  It is shared with Bluetooth, Zigbee,
  cordless phones, microwave ovens, and most of the neighboring Wi-Fi.
- Narrow channels and constant contention hold real throughput well
  below 5 and 6 GHz.
- The upside is range: 2.4 GHz reaches further and passes through walls
  better, which keeps it useful for distant clients and 2.4 GHz-only
  IoT devices.

**5 GHz**

UNII-1 (channels 36-48) and UNII-3 (149-165) need no radar checks.
UNII-2 (channels 52-64 and 100-144) shares spectrum with radar and
requires DFS.  ETSI regions such as the EU do not include UNII-3, so the
only non-DFS 5 GHz channels there are 36-48.  This band supports 20, 40,
80, and 160 MHz, so it is the one to use for wide, fast channels.

Drawbacks:

- Shorter range than 2.4 GHz, and a weaker signal through walls and
  floors.
- A DFS channel must be monitored for radar for 60 seconds (up to 10
  minutes near some weather radars) before the AP may transmit, which
  delays start-up.  If radar appears later, the AP has to leave the
  channel within 10 seconds and avoid it for 30 minutes, dropping
  clients during the move.
- The widest 80 and 160 MHz channels almost always sit on DFS spectrum,
  so the same radar rules apply to them.

**6 GHz**

The FCC regions open 59 channels (1, 5, 9 ... 233) across 5925-7125 MHz.
ETSI regions, including the EU, currently open only the lower part,
5945-6425 MHz (channels 1-93), for indoor use.  Clients find networks on
the 15 Preferred Scanning Channels (5, 21, 37 ... 229) spaced every
80 MHz, and `auto` selects channel 37.  There is no DFS in 6 GHz, so
there is no radar start-up delay.

Drawbacks:

- The shortest range and the weakest wall penetration of the three
  bands.
- Only Wi-Fi 6E and newer clients can use it; older phones and IoT
  devices cannot see the band at all.
- AP operation requires WPA3-Personal (SAE) with management frame
  protection, so WPA2-only and open networks are rejected.
- Indoor power limits cap coverage further.

### WiFi 6 Support

WiFi 6 (802.11ax) is always enabled in AP mode on all bands, providing improved
performance through features like OFDMA, BSS Coloring, and beamforming.

**WiFi 6 Features (always enabled in AP mode on supported radios):**

- **OFDMA**: Better multi-user efficiency in dense environments
- **BSS Coloring**: Reduced interference from neighboring networks
- **Beamforming**: Improved signal quality and range

**Requirements:**

- Hardware must support 802.11ax
- Client devices must support WiFi 6 for full benefits
- Older WiFi 5/4 clients can still connect but won't use WiFi 6 features

> [!IMPORTANT]
> 6 GHz AP operation requires WPA3-Personal (SAE) with mandatory
> management frame protection.  Open networks and WPA2-only AP
> configurations are not valid on 6 GHz.

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
name               : wifi0
type               : wifi
index              : 3
mtu                : 1500
operational status : up
ip forwarding      : enabled
physical address   : f0:09:0d:36:5f:86
ipv4 addresses     : 192.168.1.100/24 (dhcp)
ipv6 addresses     :
in-octets          : 148388
out-octets         : 24555
mode               : station
ssid               : MyNetwork
signal             : -45 dBm (good)
rx bitrate         : 72.2 Mbps
tx bitrate         : 86.6 Mbps
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
name               : wifi0
type               : wifi
operational status : up
physical address   : f0:09:0d:36:5f:86
mode               : station
ssid               : MyHomeNetwork
signal             : -52 dBm (good)
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
- `auto`: WPA2/WPA3 transitional mode on 2.4/5 GHz, WPA3-only on 6 GHz
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

## Fast Roaming Between Access Points

Fast roaming enables seamless client handoff between access points through
802.11k/r/v standards. These features can be enabled individually based on
your requirements.

### 802.11r - Fast BSS Transition

Enable 802.11r for fast handoff (<50ms) between APs with the same SSID:

```
admin@example:/config/interface/wifi0/> set wifi access-point roaming dot11r
admin@example:/config/interface/wifi0/> set wifi access-point roaming dot11r mobility-domain 4f57
```

**Requirements:**
- All APs in roaming group must have **identical** SSID
- All APs must have **identical** passphrase (same keystore secret)
- All APs must use the **same mobility-domain** identifier

**Mobility Domain Options:**
- Explicit 4-character hex value (e.g., `4f57`) - default if not specified
- `hash` - Automatically derive from SSID using MD5 (OpenWrt-compatible)

Using `hash` allows multiple APs with the same SSID to automatically share
the same mobility domain without manual configuration:

```
admin@example:/config/interface/wifi0/> set wifi access-point roaming dot11r mobility-domain hash
```

The NAS-Identifier (Network Access Server Identifier) is a string that
uniquely identifies each AP within the 802.11r mobility domain.  APs
exchange this identifier during fast BSS transition so they can look up
the correct PMK-R1 key for the roaming client.  It must be unique per AP
BSS and stable across reboots.

```
admin@example:/config/interface/wifi0/> set wifi access-point roaming dot11r nas-identifier auto
```

`auto` derives the identifier as:

`<interface-name>-<hostname>.<mobility-domain>`

Or set an explicit string:

```
admin@example:/config/interface/wifi0/> set wifi access-point roaming dot11r nas-identifier ap01.wifi0.4f57
```

### 802.11k - Radio Resource Management

Enable 802.11k for client neighbor discovery and better roaming decisions:

```
admin@example:/config/interface/wifi0/> set wifi access-point roaming dot11k
```

Enables neighbor reports and beacon reports, allowing clients to discover
nearby APs before roaming.

### 802.11v - BSS Transition Management

Enable 802.11v for network-assisted roaming:

```
admin@example:/config/interface/wifi0/> set wifi access-point roaming dot11v
```

Allows APs to suggest better APs to clients, improving roaming decisions.

#### Band Steering (MBO)

Enabling `dot11v` also turns on MBO (Multi-Band Operation), advertised in
beacons and association responses.  MBO lets a dual-band client see that
the same SSID exists on another band and decide for itself when to move,
while 802.11v BSS Transition Management lets the AP suggest a better
target.

On top of the client-cooperative hints, the AP applies active steering:
on a 2.4 GHz access-point it suppresses probe responses to clients that
were recently seen on the same SSID on the 5/6 GHz band, nudging
dual-band clients onto the higher band.  MBO is **enabled by default**
whenever `dot11v` is enabled:

```
admin@example:/config/interface/wifi0/> set wifi access-point roaming dot11v
```

To turn it off while keeping BSS Transition Management:

```
admin@example:/config/interface/wifi0/> set wifi access-point roaming dot11v band-steering false
```

> [!NOTE]
> Band steering only matters when the same SSID is offered on two or more
> bands (one access-point per radio).  On a single-band network there is
> no other band to move to, so it has no effect.

### Opportunistic Key Caching (OKC)

OKC reduces re-authentication time for roaming clients that do not
support 802.11r.  The AP caches the PMK from previous associations and
shares it with other APs in the same mobility group.  It is **enabled by
default** and only activates when both AP and client support it:

```
admin@example:/config/interface/wifi0/> set wifi access-point roaming okc false
```

### Recommended Configuration

For optimal roaming experience, enable all three features:

```
admin@example:/config/interface/wifi0/> set wifi access-point roaming dot11k
admin@example:/config/interface/wifi0/> set wifi access-point roaming dot11r
admin@example:/config/interface/wifi0/> set wifi access-point roaming dot11v
admin@example:/config/interface/wifi0/> set wifi access-point roaming dot11r mobility-domain 4f57
```

Or use `hash` for automatic mobility domain derivation from SSID:

```
admin@example:/config/interface/wifi0/> set wifi access-point roaming dot11k
admin@example:/config/interface/wifi0/> set wifi access-point roaming dot11r
admin@example:/config/interface/wifi0/> set wifi access-point roaming dot11v
admin@example:/config/interface/wifi0/> set wifi access-point roaming dot11r mobility-domain hash
```

Repeat for all APs that should participate in the roaming group.

> [!NOTE]
> Not all client devices support all roaming features. Modern devices typically
> support 802.11k/r/v, but older devices may only support basic roaming without
> fast transition.

## 802.11s Mesh Point Mode

IEEE 802.11s is a wireless mesh networking standard operating at Layer 2.
Mesh nodes form peer links directly with each other and route traffic
using HWMP (Hybrid Wireless Mesh Protocol), which is built into the
Linux mac80211 subsystem.  There is no central controller; nodes
discover peers and find paths on their own.

The standard defines two node roles:

- **Mesh Point (MP)** - a basic mesh node that forwards traffic within
  the mesh
- **Mesh Portal (MPP)** - a mesh node that bridges traffic between the
  mesh and an external network (e.g., a wired LAN)

In practice, a node bridging the mesh interface to a LAN acts as a mesh
portal.

> [!NOTE]
> Not all WiFi hardware supports 802.11s mesh.  The driver must implement
> mesh point mode in mac80211.  Check your adapter's capabilities with
> `iw phy <phy> info` and look for "mesh point" under "Supported interface
> modes".

### 802.11s vs EasyMesh

|                             | **802.11s**              | **EasyMesh**                   |
|-----------------------------|--------------------------|--------------------------------|
| **Standard**                | IEEE (open, ratified)    | Wi-Fi Alliance (certification) |
| **Topology**                | Peer-to-peer, any-to-any | Controller-based tree          |
| **Single point of failure** | None                     | Controller                     |
| **Multi-hop**               | True N-hop               | Limited (1-2 hops)             |
| **Vendor lock-in**          | None                     | Common                         |
| **Linux support**           | Kernel-native (mac80211) | Requires proprietary firmware  |

Infix uses 802.11s because it runs entirely in the kernel with no
proprietary components.

### Mesh configuration

A mesh point requires the radio to have `band`, `channel`, and a valid
`country-code` configured. Mesh and AP modes cannot coexist on the same
radio.

**Step 1: Configure the radio**

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit hardware component radio1 wifi-radio</b>
admin@example:/config/hardware/component/radio1/wifi-radio/> <b>set country-code DE</b>
admin@example:/config/hardware/component/radio1/wifi-radio/> <b>set band 5GHz</b>
admin@example:/config/hardware/component/radio1/wifi-radio/> <b>set channel 36</b>
admin@example:/config/hardware/component/radio1/wifi-radio/> <b>leave</b>
</code></pre>

**Step 2: Create keystore entry for mesh security**

All mesh links use WPA3-SAE encryption. All nodes in the same mesh
network must share the same passphrase:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit keystore symmetric-key mesh-secret</b>
admin@example:/config/keystore/…/mesh-secret/> <b>set key-format passphrase-key-format</b>
admin@example:/config/keystore/…/mesh-secret/> <b>change cleartext-symmetric-key</b>
Passphrase: ************
Retype passphrase: ************
admin@example:/config/keystore/…/mesh-secret/> <b>end</b>
</code></pre>

**Step 3: Configure the mesh interface**

<pre class="cli"><code>admin@example:/config/> <b>edit interface wifi-mesh</b>
admin@example:/config/interface/wifi-mesh/> <b>set type wifi</b>
admin@example:/config/interface/wifi-mesh/> <b>set wifi radio radio1</b>
admin@example:/config/interface/wifi-mesh/> <b>set wifi mesh-point mesh-id my-mesh</b>
admin@example:/config/interface/wifi-mesh/> <b>set wifi mesh-point security secret mesh-secret</b>
admin@example:/config/interface/wifi-mesh/> <b>leave</b>
</code></pre>

**Mesh parameters:**

- `mesh-id`: Network identifier, 1-32 characters.  All nodes in the mesh
  must use the same mesh ID
- `forwarding`: L2 mesh forwarding (default: true).  When enabled, the
  interface can be added to a bridge as a mesh portal
- `security secret`: Keystore reference for the WPA3-SAE passphrase

### Mesh portal (bridge integration)

To connect the wireless mesh to a wired LAN, add the mesh interface to
a bridge:

<pre class="cli"><code>admin@example:/config/> <b>edit interface wifi-mesh</b>
admin@example:/config/interface/wifi-mesh/> <b>set bridge-port bridge br0</b>
admin@example:/config/interface/wifi-mesh/> <b>leave</b>
</code></pre>

### Mesh with roaming APs

You can combine 802.11s mesh backhaul with roaming-enabled access
points.  Each node has a mesh interface for backhaul on one radio and
AP interfaces for clients on another:

![802.11s mesh backhaul with roaming-enabled access points](img/wifi-mesh-roaming.svg)

With 802.11r/k/v roaming enabled on the APs (same SSID, same
passphrase, same mobility domain), clients hand off between nodes while
the mesh carries backhaul traffic.

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
