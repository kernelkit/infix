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
Radios are automatically discovered and named `phy0`, `phy1`, etc.

### Basic Radio Setup

Configure the radio with channel, power, and regulatory domain:

```
admin@example:/> configure
admin@example:/config/> edit wifi-radio phy0
admin@example:/config/wifi-radio/phy0/> set country-code US
admin@example:/config/wifi-radio/phy0/> set channel 6
admin@example:/config/wifi-radio/phy0/> set channel-width 20
admin@example:/config/wifi-radio/phy0/> set txpower auto
admin@example:/config/wifi-radio/phy0/> set phy-mode auto
admin@example:/config/wifi-radio/phy0/> leave
```

**Key radio parameters:**
- `country-code`: Two-letter ISO 3166-1 code (mandatory) - determines allowed channels and power
- `channel`: Channel number (1-196) or "auto" for automatic selection
- `channel-width`: 20, 40, 80, or 160 MHz
- `txpower`: Power in dBm (1-30) or "auto" for maximum allowed
- `phy-mode`: PHY standard (ieee80211b/g/a/n/ac/ax) or "auto"
- `band`: 2.4GHz, 5GHz, 6GHz, or "auto"

## Station Mode (Client)

Station mode connects to an existing Wi-Fi network. To verify that a
compatible adapter has been detected, look for a radio (e.g., `phy0`) in
`show wifi-radio` or interfaces in `show interface`

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
admin@infix-00-00-00:/> show interface wifi0
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
admin@example:/config/keystore/…/example/> set cleartext-key mysecret
admin@example:/config/keystore/…/example/> leave
admin@example:/>
```

Configure the Wi-Fi interface to reference the radio and connect to a network:

```
admin@example:/> configure
admin@example:/config/> edit interface wifi0
admin@example:/config/interface/wifi0/> set wifi radio phy0
admin@example:/config/interface/wifi0/> set wifi station ssid ssid1
admin@example:/config/interface/wifi0/> set wifi station encryption type preshared-key
admin@example:/config/interface/wifi0/> set wifi station encryption secret example
admin@example:/config/interface/wifi0/> leave
```

**Station configuration parameters:**
- `radio`: Reference to the WiFi radio (mandatory)
- `station ssid`: Network name to connect to
- `station encryption type`: `preshared-key` or `disabled` (open network)
- `station encryption secret`: Reference to keystore entry

WPA2 or WPA3 encryption will be automatically selected based on what
the access point supports. No manual selection is required.

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
admin@example:/config/> edit interface wlan0
admin@example:/config/interface/wlan0/> set type wifi
admin@example:/config/interface/wlan0/> set wifi radio phy0
admin@example:/config/interface/wlan0/> set wifi access-point ssid MyNetwork
admin@example:/config/interface/wlan0/> set wifi access-point security mode wpa2-personal
admin@example:/config/interface/wlan0/> set wifi access-point security secret example
admin@example:/config/interface/wlan0/> leave
```

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
admin@example:/config/interface/wlan0/> set wifi access-point hidden true
```

### Multi-SSID Configuration

Multiple AP interfaces on the same radio allow broadcasting multiple SSIDs,
each with independent security settings. This is useful for guest networks
or segregating traffic.

```
admin@example:/> configure
# Primary AP - Main network
admin@example:/config/> edit interface wlan0
admin@example:/config/interface/wlan0/> set type wifi
admin@example:/config/interface/wlan0/> set wifi radio phy0
admin@example:/config/interface/wlan0/> set wifi access-point ssid MainNetwork
admin@example:/config/interface/wlan0/> set wifi access-point security mode wpa3-personal
admin@example:/config/interface/wlan0/> set wifi access-point security secret main-secret
admin@example:/config/interface/wlan0/> up

# Secondary AP - Guest network
admin@example:/config/> edit interface wlan1
admin@example:/config/interface/wlan1/> set type wifi
admin@example:/config/interface/wlan1/> set wifi radio phy0
admin@example:/config/interface/wlan1/> set wifi access-point ssid GuestNetwork
admin@example:/config/interface/wlan1/> set wifi access-point security mode wpa2-personal
admin@example:/config/interface/wlan1/> set wifi access-point security secret guest-secret
admin@example:/config/interface/wlan1/> leave
```

All APs on the same radio share the same channel and physical layer settings
(configured at the radio level). Each AP can have its own:
- SSID
- Security mode and passphrase
- VLAN assignment
- Bridge membership

> [!IMPORTANT] AP and Station modes cannot be mixed on the same radio. All
> virtual interfaces on a radio must be the same mode (all APs or all Stations).

### AP as Bridge Port

WiFi AP interfaces can be added to bridges to integrate wireless devices
into your LAN:

```
admin@example:/> configure
admin@example:/config/> edit interface br0
admin@example:/config/interface/br0/> set type bridge
admin@example:/config/interface/br0/> up

admin@example:/config/> edit interface wlan0
admin@example:/config/interface/wlan0/> set bridge-port bridge br0
admin@example:/config/interface/wlan0/> leave
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

## Migration Guide: Old to New WiFi Model

> [!WARNING] This is a **breaking change**. The WiFi configuration model has been
> completely redesigned and existing configurations must be manually migrated.

### What Changed

**Old Model (Station only):**
```
interface wifi0
  wifi
    ssid MyNetwork
    country-code US
    encryption
      type preshared-key
      secret my-secret
```

**New Model (Radio + Interface):**
```
wifi-radio phy0
  country-code US
  channel auto
  channel-width 20

interface wifi0
  type wifi
  wifi
    radio phy0
    station
      ssid MyNetwork
      encryption
        type preshared-key
        secret my-secret
```

### Key Differences

1. **Radio Configuration Separated**: Physical layer settings (channel, power,
   country-code) moved to `wifi-radio` module

2. **Explicit Mode Selection**: Must choose `station` or `access-point` mode
   (old model only supported station)

3. **Radio Reference Required**: All WiFi interfaces must reference a radio
   via the `radio` leaf

4. **Country Code Location**: Moved from interface to radio configuration

5. **Multi-SSID Support**: Multiple interfaces can share one radio

### Migration Steps

1. **Identify your WiFi adapters and radios:**
   ```
   admin@example:/> show interfaces
   # Look for wifi interfaces (wifi0, wifi1, etc.)
   # Radios are typically phy0, phy1, etc.
   ```

2. **Create radio configuration:**
   ```
   admin@example:/> configure
   admin@example:/config/> edit wifi-radio phy0
   admin@example:/config/wifi-radio/phy0/> set country-code <YOUR-CODE>
   admin@example:/config/wifi-radio/phy0/> set channel auto
   admin@example:/config/wifi-radio/phy0/> set channel-width 20
   ```

3. **Update interface configuration:**
   ```
   # For Station mode (client):
   admin@example:/config/> edit interface wifi0
   admin@example:/config/interface/wifi0/> delete wifi country-code
   admin@example:/config/interface/wifi0/> set wifi radio phy0
   # Move ssid and encryption under station:
   admin@example:/config/interface/wifi0/> set wifi station ssid <SSID>
   admin@example:/config/interface/wifi0/> set wifi station encryption type preshared-key
   admin@example:/config/interface/wifi0/> set wifi station encryption secret <SECRET>

   # For AP mode:
   admin@example:/config/> edit interface wlan0
   admin@example:/config/interface/wlan0/> set wifi radio phy0
   admin@example:/config/interface/wlan0/> set wifi access-point ssid <SSID>
   admin@example:/config/interface/wlan0/> set wifi access-point security mode wpa2-personal
   admin@example:/config/interface/wlan0/> set wifi access-point security secret <SECRET>
   ```

4. **Apply configuration:**
   ```
   admin@example:/config/> leave
   ```

### Compatibility

- **No automatic migration**: Existing WiFi configurations will fail validation
  and must be manually updated
- **CLI changes**: Commands have changed to reflect the new structure
- **Operational data**: Station scan results and status available in same location
- **AP operational data**: New - includes connected stations list

### Benefits of New Model

- **Access Point support**: Create WiFi networks
- **Multi-SSID**: Multiple networks on one radio
- **Better separation**: PHY and network layers properly separated
- **Regulatory compliance**: Clearer country-code management
- **Flexibility**: Easier to add new features (mesh, WDS, etc.)
