# Cellular Modem (WWAN)

Infix supports cellular modem connectivity via modems that expose a
QMI or MBIM control interface over USB.  Form factor is not
significant — USB dongles, mPCIe cards, and M.2 Key-B modules all
work as long as the modem chipset uses USB on the connector (the
typical case for 4G/LTE modems).  See *Supported Modems* below for
the exceptions.

Setup involves three configuration items:

- A `modem0` hardware component representing the physical modem
- A `sim0` hardware component representing the SIM card slot
- A `wwan0` network interface that references both and carries the
  bearer (APN) configuration

## Architecture

Infix uses a three-layer architecture for cellular modem support:

1. **Modem hardware component (modem0)**: Represents the physical modem
     - Configured via `ietf-hardware` module with class `infix-hardware:modem`
     - `admin-state` controls whether ModemManager and modemd are active
     - Holds physical-layer config: allowed bands, preferred mode, location
     - Auto-discovered into factory-default config when the modem is
       present at first boot

2. **SIM hardware component (sim0)**: Represents the SIM card slot
     - Configured via `ietf-hardware` module with class `infix-hardware:sim`
     - Holds PIN/PUK credentials and carrier profile
     - Auto-discovered into factory-default config alongside the modem

3. **Network interface (wwan0)**: Data bearer to the cellular network
     - Configured via `ietf-interfaces` module with type `infix-if-type:modem`
     - References a modem component and a SIM component
     - Holds bearer config: APN, IP type, roaming, route preference, authentication
     - Always added by the user; never auto-created

## Naming Conventions

| **Name Pattern** | **Type**             | **Description**                                 |
|------------------|----------------------|-------------------------------------------------|
| `modemN`         | Modem hardware       | Hardware component for the physical modem       |
| `simN`           | SIM hardware         | Hardware component for the SIM card slot        |
| `wwanN`          | Modem interface      | Network interface for cellular data             |

Where `N` is a number (0, 1, 2, ...).

> [!TIP]
> Using these naming conventions simplifies configuration since type and class
> are automatically inferred.  Creating a hardware component named `modem0`
> automatically sets its class to `infix-hardware:modem`, and creating an
> interface named `wwan0` automatically sets its type to `infix-if-type:modem`.
>
> **Note:** This inference only works via the CLI.  When configuring over
> NETCONF or RESTCONF the class and type must be set explicitly.

## Multi-Bearer (Multiple APNs)

Multiple wwan interfaces can reference the same modem component, each
with a different APN.  This is analogous to multi-SSID on a WiFi radio —
one hardware modem, multiple independent data connections.

Configure `wwan0` and `wwan1` both pointing to `modem0`:

```
edit interfaces interface wwan0 wwan modem modem0
edit interfaces interface wwan0 wwan bearer apn internet
edit interfaces interface wwan1 wwan modem modem0
edit interfaces interface wwan1 wwan bearer apn corporate.vpn.apn
```

## Current Limitations

- The modem must be present at boot — hot-plug is not supported
- If the modem is absent at boot (and no `probe-timeout` is set), a dummy
  `wwan0` placeholder is created immediately so IP configuration can proceed;
  a reboot is required once hardware is inserted

## Supported Modems

Modems exposing a CDC-WDM control interface over USB are supported,
regardless of physical form factor.  Two protocols are handled by
ModemManager:

- **MBIM** — Mobile Broadband Interface Model (e.g. Sierra Wireless, Quectel EM06/EM12)
- **QMI** — Qualcomm MSM Interface (e.g. Sierra Wireless EM7xxx, Quectel EM/RMxxx)

Most 4G/LTE modules — whether USB dongles, mPCIe cards, or M.2 Key-B
modules — use USB on the connector even when the slot also carries
PCIe lanes; from Infix's view they are all USB modems.  PCIe-only
modems (some 5G NR modules) are not currently supported, since the
modemd / ModemManager pipeline assumes a USB-attached control
interface.

## Step-by-step Setup

### 1 — Hardware Detection

At first boot, USB modems detected by the kernel are written to
`/run/system.json` and added as hardware components in the factory-
default configuration.  Verify the modem appears:

<pre class="cli"><code>admin@example:/> <b>show modem</b>
<span class="header">CELLULAR MODEMS                                               </span>
──────────────────────────────────────────────────────────────
<span class="title">Cellular Modems                                               </span>
Name                : modem0
Manufacturer        : Quectel
Model               : EM06-E
Firmware            : EM06ELAR04A07M4G
State               : registered
Signal              : 72%
RSRP                : -95 dBm
SINR                : 12.4 dB
Operator            : Tele2 (23002)
Network             : LTE
</code></pre>

If `modem0` does not appear:

- Verify the kernel sees the modem (`dmesg | grep -i mbim` or
  `dmesg | grep -i qmi`); without a kernel driver no further setup is
  possible.
- On boards with a custom factory configuration, or after replacing
  hardware on a running system, the components may need to be added
  manually — see Step 2.

### 2 — Hardware Components

If `show modem` already lists `modem0` and `sim0`, the components were
auto-discovered into the factory-default configuration and you can skip
ahead to Step 3.

Otherwise, add them manually.  The class is inferred from the component
name (`modemN` → `infix-hardware:modem`, `simN` → `infix-hardware:sim`):

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit hardware component modem0</b>
admin@example:/config/hardware/…/modem0/> <b>leave</b>
admin@example:/config/> <b>edit hardware component sim0</b>
admin@example:/config/hardware/…/sim0/> <b>leave</b>
admin@example:/config/> <b>leave</b>
</code></pre>

The component is created with `admin-state unlocked` by default, which
causes confd to start ModemManager and modemd.  To take the modem
offline cleanly without removing the configuration, set `admin-state
locked` — both services are stopped and the bearer is torn down before
the modem goes offline:

<pre class="cli"><code>admin@example:/config/> <b>edit hardware component modem0</b>
admin@example:/config/hardware/…/modem0/> <b>set state admin-state locked</b>
admin@example:/config/hardware/…/modem0/> <b>leave</b>
</code></pre>

#### Slow USB Modems

USB modems can be slow to enumerate at boot — the kernel wwan interface
may not appear until several seconds after confd starts applying
configuration.  The `probe-timeout` leaf inside the `modem` hardware
configuration container controls how long confd waits; it **defaults to
30 seconds** when the container is present.

To enable the timeout, create the modem configuration container (this also
lets you configure bands, preferred mode, etc.):

<pre class="cli"><code>admin@example:/config/> <b>edit hardware component modem0 modem</b>
admin@example:/config/hardware/…/modem/> <b>leave</b>
</code></pre>

With `probe-timeout` at its default of 30, confd waits up to 30 seconds
for the wwan interface to appear before proceeding — for most modems it
will be ready in 2–5 seconds.  If the modem has not appeared within the
timeout, a dummy placeholder interface is created and a reboot is required
for the real interface to take over.  Set `probe-timeout 0` to disable
waiting entirely.

### 3 — Configure the Bearer (APN)

Bearer configuration lives on the `wwan0` interface.  Reference the modem
hardware component and set the APN:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interfaces interface wwan0</b>
admin@example:/config/interfaces/…/wwan0/> <b>set wwan modem modem0</b>
admin@example:/config/interfaces/…/wwan0/> <b>set wwan sim sim0</b>
admin@example:/config/interfaces/…/wwan0/> <b>set wwan bearer apn internet</b>
admin@example:/config/interfaces/…/wwan0/> <b>leave</b>
</code></pre>

The modem will connect automatically once the bearer is configured and
the hardware is unlocked.

**Key bearer parameters:**

- `apn`: Access Point Name — required, provided by your operator
  (e.g. `internet`, `data.vodafone.com`, `web.tele2.se`)
- `route-preference`: Administrative distance for the default route (default: `200`).
  Higher value = lower priority.  Default 200 places cellular behind wired
  Ethernet (distance 5) and WiFi automatically, making it a natural failover
- `roaming`: Allow data when roaming on a foreign network (default: `false`)
- `ip-type`: `ipv4`, `ipv6`, or `ipv4v6` dual-stack (default: `ipv4v6`)

### 4 — Configure Authentication

Most consumer APNs connect without credentials.  If your operator requires
authentication, first store the password in the keystore, then reference
it from the bearer:

<pre class="cli"><code>admin@example:/config/> <b>edit keystore symmetric-keys symmetric-key apn-pass</b>
admin@example:/config/keystore/…/apn-pass/> <b>set cleartext-symmetric-key mypassword</b>
admin@example:/config/keystore/…/apn-pass/> <b>up 4</b>
admin@example:/config/> <b>edit interfaces interface wwan0 wwan bearer</b>
admin@example:/config/interfaces/…/wwan/bearer/> <b>edit authentication</b>
admin@example:/config/interfaces/…/wwan/bearer/authentication/> <b>set username myuser</b>
admin@example:/config/interfaces/…/wwan/bearer/authentication/> <b>set password apn-pass</b>
admin@example:/config/interfaces/…/wwan/bearer/authentication/> <b>leave</b>
</code></pre>

The `authentication` container is a presence container — creating it
enables authentication.  The `password` leaf is a reference to a symmetric
key in the keystore, not the plaintext password itself.

The authentication protocol defaults to `chap`.  To use PAP instead:

<pre class="cli"><code>admin@example:/config/interfaces/…/wwan/bearer/authentication/> <b>set type pap</b>
</code></pre>

### 5 — Configure SIM PIN

If the SIM requires a PIN to unlock, configure it on the SIM hardware component:

<pre class="cli"><code>admin@example:/config/> <b>edit hardware component sim0</b>
admin@example:/config/hardware/…/sim0/> <b>set sim pin 1234</b>
admin@example:/config/hardware/…/sim0/> <b>leave</b>
</code></pre>

### 6 — Verify Connectivity

Once connected, the `wwan0` interface receives an IP address from the
carrier and modemd installs the default route:

<pre class="cli"><code>admin@example:/> <b>show interface wwan0</b>
name                : wwan0
type                : modem
index               : 5
mtu                 : 1500
operational status  : up
ip forwarding       : enabled
physical address    : 12:34:56:78:9a:bc
ipv4 addresses      : 10.142.87.33/30 (wwan)
ipv6 addresses      : 2001:db8:1:2::1/64 (wwan)
in-octets           : 84213
out-octets          : 31456
</code></pre>

Check the full modem state including signal quality and registration:

<pre class="cli"><code>admin@example:/> <b>show modem modem0</b>
name                : modem0
class               : infix-hardware:modem
admin-state         : unlocked
oper-state          : enabled
──────────────────────────────────────────────────────────────
Manufacturer        : Quectel
Model               : EM06-E
Firmware            : EM06ELAR04A07M4G
IMEI                : 352753090141905
IMSI                : 240021234567890
ICCID               : 8946020000001234567
State               : connected
Signal              : 72%
RSRP                : -95 dBm
RSRQ                : -11 dB
SINR                : 12.4 dB
Registration        : home
Operator            : Tele2 (23002)
Network             : LTE
</code></pre>

## Cellular Failover

The default `route-preference` (200) is deliberately higher than distances
used by wired Ethernet (udhcpc default: 5) and WiFi.  When wired
connectivity is available, it takes precedence automatically.  Cellular
becomes the active path only if higher-priority routes are withdrawn.

A default route via the bearer is always installed when the bearer
connects.  To adjust the failover priority between two cellular modems, or
to make cellular preferred over WiFi, set `route-preference` explicitly:

<pre class="cli"><code>admin@example:/config/> <b>edit interfaces interface wwan0 wwan bearer</b>
admin@example:/config/interfaces/…/wwan/bearer/> <b>set route-preference 100</b>
admin@example:/config/interfaces/…/wwan/bearer/> <b>leave</b>
</code></pre>

Lower `route-preference` = higher priority.

## Roaming

Data roaming is disabled by default.  To allow the modem to connect when
on a foreign (roaming) network:

<pre class="cli"><code>admin@example:/config/> <b>edit interfaces interface wwan0 wwan bearer</b>
admin@example:/config/interfaces/…/wwan/bearer/> <b>set roaming true</b>
admin@example:/config/interfaces/…/wwan/bearer/> <b>leave</b>
</code></pre>

> [!IMPORTANT]
> Enabling roaming may incur significant charges depending on your
> mobile subscription.  Check with your operator before enabling.

## Management Commands

### Restart Bearer

Disconnect and reconnect all bearers without resetting the modem hardware.
Use this after changing APN or authentication settings:

<pre class="cli"><code>admin@example:/> <b>modem restart modem0</b>
</code></pre>

### Reset Modem

Factory-reset the modem firmware.  This clears all modem-internal settings
and takes longer than a restart.  Only use this if the modem is in a bad
state that a bearer restart cannot fix:

<pre class="cli"><code>admin@example:/> <b>modem reset modem0</b>
</code></pre>

### Send SMS

Send an SMS message via the signalling plane.  No active data bearer is
required — the modem only needs to be registered on the network:

<pre class="cli"><code>admin@example:/> <b>modem sms modem0 +46701234567 "Hello from Infix"</b>
</code></pre>

> [!NOTE]
> Some SIM cards have Fixed Dialing Number (FDN) enabled, which restricts
> outgoing SMS and calls to a pre-configured whitelist.  If `modem sms`
> fails, check whether FDN is active with `mmcli -m 0` and look for
> `enabled locks: fixed-dialing` in the output.

## Troubleshooting

**Modem not detected (`show modem` shows no modem entry)**

- Verify the modem is connected and recognized by the kernel: check
  `dmesg` for `cdc_mbim` or `qmi_wwan` driver messages
- Confirm `/sys/class/usbmisc/` contains a `cdc-wdm*` entry
- The modem must be present at boot — hotplug after boot is not supported

**`wwan0` shows as `down` or has no IP address**

- Check `show modem modem0` — the state should show
  `registered` or `connected`, not `failed`
- Verify the APN is correct for your operator
- Check system logs with `show log` for modemd or ModemManager messages
- If the state shows `failed`, the modem may need a SIM card inserted or
  a PIN unlocked (configure `hardware component sim0 sim pin`)

**`wwan0` interface is a dummy (no data flows, no carrier address)**

- The modem was not enumerated by the kernel before confd applied config
- Create the modem hardware configuration container (see Step 2) — this
  enables the default 30-second probe-timeout so confd waits for the wwan
  interface before falling back to a dummy placeholder

**High latency or poor signal**

- Use `show modem modem0` to check signal quality and RSRP
- Signal below -110 dBm RSRP typically indicates poor coverage
- Consider repositioning the antenna or the device
