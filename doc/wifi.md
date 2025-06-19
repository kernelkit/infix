# Wi-Fi (Wireless LAN)

Infix includes built-in Wi-Fi client support for connecting to
wireless networks. When a compatible Wi-Fi adapter is detected, the
system automatically begins scanning for available networks.

## Current Limitations

- Only client mode is supported (no access point functionality)
- USB hotplug is not supported - adapters must be present at boot
- Interface naming may be inconsistent with multiple USB Wi-Fi adapters

## Supported Wi-Fi Adapters

Wi-Fi support is primarily tested with Realtek chipset-based adapters.

### Known Working Chipsets

- RTL8821CU
- Other Realtek chipsets may work but are not guaranteed


> [!NOTE]  Some Realtek chipsets require proprietary drivers not included in the standard kernel
>          Firmware requirements vary by chipset
>          Check kernel logs if your adapter is not detected

## Configuration

Add a supported Wi-Fi network device. To verify that it has been
detected, look for `wifi0` in `show interfaces`

```
admin@example:/>  show interfaces
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
admin@infix-00-00-00:/> show interfaces name wifi0
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
admin@example:/config/keystore/symmetric-key/example/> set key-format wifi-preshared-key-format
admin@example:/config/keystore/symmetric-key/example/> set cleartext-key mysecret
admin@example:/config/keystore/symmetric-key/example/> leave
admin@example:/>
```

Configure the Wi-Fi settings, set secret to the name selected above
for the symmetric key, in this case `example`.

WPA2 or WPA3 encryption will be automatically selected based on what
the access point supports. No manual selection is required unless
connecting to an open network. No support for certificate based
authentication yet.

Unencrypted network is also supported, to connect to an unencrypted
network (generally not recommended):
```
admin@example:/config/interface/wifi0/> set wifi encryption disabled
```

A valid `country-code` is also required for regulatory compliance, the
valid codes are documented in the YANG model `infix-wifi-country-codes`


```
admin@example:/> configure
admin@example:/config/> edit interface wifi0
admin@example:/config/interface/wifi0/>
admin@example:/config/interface/wifi0/> set wifi ssid ssid1
admin@example:/config/interface/wifi0/> set wifi secret example
admin@example:/config/interface/wifi0/> set wifi country-code SE
admin@example:/config/interface/wifi0/> leave
```

The Wi-Fi negotiation should now start immediately, provided that the
SSID and pre-shared key are correct. You can verify the connection by
running `show interfaces` again.


```
admin@example:/> show interfaces
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

## Troubleshooting Connection Issues

Use `show wifi scan wifi0` and `show interfaces` to verify signal strength
and connection status. If issues arise, try the following
troubleshooting steps:

1. **Verify signal strength**: Check that the target network shows "good" or "excellent" signal
2. **Check credentials**: Verify the preshared key in `ietf-keystore`
3. **Review logs**: Check system logs with `show log` for Wi-Fi related errors
4. **Regulatory compliance**: Ensure the country-code matches your location
5. **Hardware detection**: Confirm the adapter appears in `show interfaces`

If issues persist, check the system log for specific error messages that can help identify the root cause.
