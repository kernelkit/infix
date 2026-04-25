# PTP — Precision Time Protocol

The Precision Time Protocol (PTP), defined in IEEE 1588-2019, synchronises
clocks across a network to sub-microsecond accuracy.  Where NTP (Network Time
Protocol) aims at millisecond accuracy over wide-area networks, PTP is
designed for local-area networks and relies on hardware timestamping in the
network interface to eliminate software-induced jitter.

PTP works by exchanging timestamped messages between devices.  A *grandmaster
clock* — elected by the **Best TimeTransmitter Clock Algorithm (BTCA)** based
on priority, clock class, and accuracy — distributes time to the rest of the
network.  Each synchronising device measures the one-way message delay to its
time-transmitter and continuously adjusts its local clock to compensate.

> [!NOTE]
> The IEEE 1588g-2022 amendment to IEEE 1588-2019 introduced the terms
> *timeTransmitter* and *timeReceiver* as replacements for the former
> *master* and *slave* terminology, and *Best TimeTransmitter Clock
> Algorithm (BTCA)* in place of *BMCA*.  This document uses the updated
> terms throughout.  You may even see the short forms transmitter and
> receiver here and in online documentation.

## Clock roles

Every device in a PTP network takes one of the following roles:

| Role                       | Description                                                                                 |
|----------------------------|---------------------------------------------------------------------------------------------|
| **Grandmaster (GM)**       | Network-wide time source; elected by BTCA                                                   |
| **Time-transmitter**       | Sends Sync messages downstream on a port                                                    |
| **Time-receiver**          | Synchronises to a time-transmitter on a port                                                |
| **Boundary Clock (BC)**    | Terminates PTP on each port; acts as time-receiver upstream and time-transmitter downstream |
| **Transparent Clock (TC)** | Passes PTP messages while correcting the residence-time delay accumulated in the device     |

An **Ordinary Clock (OC)** has a single PTP port and is either a
time-transmitter (acting as a grandmaster candidate) or a time-receiver
(a leaf node synchronising to the network).

## PTP profiles

A **PTP profile** (as defined in IEEE 1588-2019 §3.1) is a document that
specifies a consistent set of required, permitted, and prohibited PTP
options for a particular application domain — much like a dialect of the
protocol.  Examples from the standards world include profiles for power
utilities (IEC/IEEE C37.238), telecom (ITU-T G.8265.1), and
Time-Sensitive Networks.

Each profile sets a unique value in the `majorSdoId` field of PTP message
headers — a 4-bit identifier that lets devices distinguish traffic belonging
to different profiles on the same link.  Profile also determines the network
transport (UDP or Ethernet) and the delay measurement mechanism.

Currently, two profiles are supported via the `profile` leaf in `default-ds`:

| `profile`            | Standard          | majorSdoId | Transport | Delay          |
|----------------------|-------------------|:----------:|-----------|----------------|
| `ieee1588` (default) | IEEE 1588-2019    | `0x0`      | UDP/IPv4  | `e2e` or `p2p` |
| `ieee802-dot1as`     | IEEE 802.1AS-2020 | `0x1`      | L2        | `p2p`          |

The **gPTP** (generalized Precision Time Protocol) profile from IEEE 802.1AS-2020
is used in **TSN** (Time-Sensitive Networking) and **AVB** (Audio/Video Bridging)
applications.  Setting `profile ieee802-dot1as` applies all protocol-mandatory
settings automatically — Layer 2 transport, P2P delay measurement, 802.1AS
multicast addressing, path trace, follow-up information, and neighbour propagation
delay thresholds.  The user still configures `priority1`, `priority2`,
`domain-number`, `time-receiver-only`, and timer interval leaves.

The `ieee1588` profile leaves transport and delay mechanism user-configurable
per port.

## Delay mechanisms

PTP measures the link delay between neighbours using one of two mechanisms:

- **End-to-End (E2E)**: Each time-receiver measures the delay to the
  grandmaster by sending a `DELAY_REQ` message upstream.  Simple to
  configure; works with any network topology.
- **Peer-to-Peer (P2P)**: Each port measures its delay to its *immediate
  neighbour* independently using `PDELAY_REQ` messages.  Enables faster
  path-delay updates and is required by the gPTP profile.

## Data Sets

IEEE 1588 organises protocol state into named **Data Sets (DS)** — each a
collection of related attributes for one aspect of a PTP instance.  You
will encounter these directly in the CLI and in the `show ptp` output:

| Data Set         | CLI node       | Contents                                                 |
|------------------|----------------|----------------------------------------------------------|
| Default DS       | `default-ds`   | Instance identity, clock class, priority, domain number  |
| Current DS       | `current-ds`   | Live offset-from-GM, mean path delay, steps-removed      |
| Parent DS        | `parent-ds`    | Grandmaster identity and quality attributes              |
| Time Properties DS | `time-properties-ds` | UTC offset, leap-second flags, time source       |
| Port DS          | `port-ds`      | Per-port state, delay mechanism, message intervals       |

## Domains

A **PTP domain** (0–255) is a logical partition of the network.  Devices
only synchronise with others in the same domain.  Running multiple
instances on the same device — one per domain, or one per profile — is
fully supported; each instance is independent.

Each PTP instance is identified on the network by its
`(domain-number, profile)` pair, which must be unique across all instances
on a device.

> [!NOTE]
> The `show ptp` offset values reflect **PHC** (PTP Hardware Clock)
> synchronisation only.  A PHC is the hardware clock exposed by the network
> interface; it tracks the PTP grandmaster but is independent of the Linux
> system clock, which currently is **not** automatically adjusted.

## Ordinary Clock (time-receiver)

A typical time-receiver Ordinary Clock, synchronising on interface
`eth0` using the default IEEE 1588 profile:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit ptp instance 0</b>
admin@example:/config/ptp/instance/0/> <b>set default-ds domain-number 0</b>
admin@example:/config/ptp/instance/0/> <b>set default-ds time-receiver-only true</b>
admin@example:/config/ptp/instance/0/> <b>edit port 1</b>
admin@example:/config/ptp/…/0/port/1/> <b>set underlying-interface eth0</b>
admin@example:/config/ptp/…/0/port/1/> <b>leave</b>
</code></pre>

## Ordinary Clock (time-transmitter / grandmaster)

A grandmaster clock with high priority, domain 0:

<pre class="cli"><code>admin@example:/config/> <b>edit ptp instance 0</b>
admin@example:/config/ptp/instance/0/> <b>set default-ds domain-number 0</b>
admin@example:/config/ptp/instance/0/> <b>set default-ds priority1 1</b>
admin@example:/config/ptp/instance/0/> <b>set default-ds priority2 1</b>
admin@example:/config/ptp/instance/0/> <b>edit port 1</b>
admin@example:/config/ptp/…/0/port/1/> <b>set underlying-interface eth0</b>
admin@example:/config/ptp/…/0/port/1/> <b>leave</b>
</code></pre>

Lower `priority1` values win in the BTCA.  A clock with `priority1 1` will
be preferred over the default `128` in any compliant network.

## Boundary Clock

A Boundary Clock terminates PTP on each port and re-originates it.  Add one
port per interface:

<pre class="cli"><code>admin@example:/config/> <b>edit ptp instance 0</b>
admin@example:/config/ptp/instance/0/> <b>set default-ds instance-type bc</b>
admin@example:/config/ptp/instance/0/> <b>set default-ds domain-number 0</b>
admin@example:/config/ptp/instance/0/> <b>edit port 1</b>
admin@example:/config/ptp/…/0/port/1/> <b>set underlying-interface eth0</b>
admin@example:/config/ptp/…/0/port/1/> <b>end</b>
admin@example:/config/ptp/instance/0/> <b>edit port 2</b>
admin@example:/config/ptp/…/0/port/2/> <b>set underlying-interface eth1</b>
admin@example:/config/ptp/…/0/port/2/> <b>leave</b>
</code></pre>

> [!TIP]
> PTP port numbers are assigned sorted by `port-index`, so `port-index 1`
> becomes PTP port 1, `port-index 2` becomes PTP port 2, and so on.

## Transparent Clock

Transparent Clocks correct timestamps end-to-end without terminating PTP.
Use `instance-type p2p-tc` for a P2P TC (preferred in TSN networks) or
`instance-type e2e-tc` for an E2E TC:

<pre class="cli"><code>admin@example:/config/> <b>edit ptp instance 0</b>
admin@example:/config/ptp/instance/0/> <b>set default-ds instance-type p2p-tc</b>
admin@example:/config/ptp/instance/0/> <b>set default-ds domain-number 0</b>
admin@example:/config/ptp/instance/0/> <b>edit port 1</b>
admin@example:/config/ptp/…/0/port/1/> <b>set underlying-interface eth0</b>
admin@example:/config/ptp/…/0/port/1/> <b>end</b>
admin@example:/config/ptp/instance/0/> <b>edit port 2</b>
admin@example:/config/ptp/…/0/port/2/> <b>set underlying-interface eth1</b>
admin@example:/config/ptp/…/0/port/2/> <b>leave</b>
</code></pre>

> [!NOTE]
> For Transparent Clocks the delay mechanism is determined globally by the
> `instance-type` (`p2p-tc` → P2P, `e2e-tc` → E2E).  Per-port
> `delay-mechanism` settings have no effect for TC instances.

## gPTP / IEEE 802.1AS

The gPTP profile is used in TSN and AVB applications.  Setting
`profile ieee802-dot1as` applies all protocol-mandatory options from
IEEE 802.1AS-2020 automatically — Layer 2 transport, P2P delay
measurement, 802.1AS multicast addressing, and related protocol features.

<pre class="cli"><code>admin@example:/config/> <b>edit ptp instance 0</b>
admin@example:/config/ptp/instance/0/> <b>set default-ds profile ieee802-dot1as</b>
admin@example:/config/ptp/instance/0/> <b>set default-ds domain-number 0</b>
admin@example:/config/ptp/instance/0/> <b>set default-ds time-receiver-only true</b>
admin@example:/config/ptp/instance/0/> <b>edit port 1</b>
admin@example:/config/ptp/…/0/port/1/> <b>set underlying-interface eth0</b>
admin@example:/config/ptp/…/0/port/1/> <b>leave</b>
</code></pre>

> [!NOTE]
> The `ieee802-dot1as` profile enforces Layer 2 transport and P2P delay
> measurement globally, as required by IEEE 802.1AS-2020.  Per-port
> `delay-mechanism` settings have no effect for 802.1AS instances.

## Multiple Instances

Multiple PTP instances can run simultaneously, one per domain or profile
combination.  Each instance must have a unique `(domain-number, profile)`
pair and an independent set of ports:

<pre class="cli"><code>admin@example:/config/> <b>edit ptp instance 0</b>
admin@example:/config/ptp/instance/0/> <b>set default-ds domain-number 0</b>
admin@example:/config/ptp/instance/0/> <b>set default-ds profile ieee1588</b>
admin@example:/config/ptp/instance/0/> <b>edit port 1</b>
admin@example:/config/ptp/…/0/port/1/> <b>set underlying-interface eth0</b>
admin@example:/config/ptp/…/0/port/1/> <b>end</b>
admin@example:/config/ptp/instance/0/> <b>end</b>
admin@example:/config/ptp/> <b>edit instance 1</b>
admin@example:/config/ptp/instance/1/> <b>set default-ds domain-number 0</b>
admin@example:/config/ptp/instance/1/> <b>set default-ds profile ieee802-dot1as</b>
admin@example:/config/ptp/instance/1/> <b>edit port 1</b>
admin@example:/config/ptp/…/1/port/1/> <b>set underlying-interface eth1</b>
admin@example:/config/ptp/…/1/port/1/> <b>leave</b>
</code></pre>

## Port states

Each PTP port progresses through a state machine.  The current state is
shown in the `show ptp` port table:

| State                  | Meaning                                                        |
|------------------------|----------------------------------------------------------------|
| `initializing`         | Port is starting up, not yet ready to exchange messages        |
| `faulty`               | A fault condition has been detected on this port               |
| `disabled`             | Port is administratively disabled                              |
| `listening`            | Awaiting `ANNOUNCE` messages; BTCA has not yet resolved        |
| `pre-time-transmitter` | Transitioning towards time-transmitter state                   |
| `time-transmitter`     | Port is acting as time-transmitter on this link                |
| `passive`              | Another port on this device is already time-transmitter        |
| `uncalibrated`         | Receiving sync; local clock not yet locked to time-transmitter |
| `time-receiver`        | Port is locked and tracking its time-transmitter               |

A port in `uncalibrated` will typically transition to `time-receiver`
within a few seconds once the clock servo has converged.

## Monitoring

> [!TIP] Use the ++question++ key in the CLI
> The `show ptp` command has sub-commands — tap ++question++ after
> `show ptp` to see them, or use ++tab++ to complete.

### Show all PTP instances

<pre class="cli"><code>admin@example:/> <b>show ptp</b>
<b>PTP Instance 0</b>                          Ordinary Clock · domain 0
────────────────────────────────────────────────────────────────────
  Clock identity          : AA-BB-CC-FF-FE-00-11-22
  Grandmaster             : DD-EE-FF-FF-FE-33-44-55
  Priority1/Priority2     : 128 / 128
  GM Priority1/Priority2  : 1 / 1
  Clock class             : cc-time-receiver-only
  GM clock class          : cc-primary-sync
  Time source             : gnss
  PTP timescale           : yes
  UTC offset              : 37 s
  Time traceable          : yes
  Freq. traceable         : yes
  Offset from GM          : -42 ns
  Mean path delay         : 1250 ns
  Steps removed           : 1

────────────────────────────────────────────────────────────────────
Ports
<span class="header">PORT  INTERFACE          STATE                DELAY  LINK DELAY (ns)</span>
   1  eth0               <span class="ok">time-receiver</span>        E2E                  0

────────────────────────────────────────────────────────────────────
Message Statistics  (▼ rx  ▲ tx)
<span class="header">PORT  INTERFACE             SYNC ▼  SYNC ▲  ANN ▼  ANN ▲  PD ▼  PD ▲</span>
   1  eth0                      42       0     15      0     0     0

</code></pre>

Port state is colour-coded: green for `time-transmitter` and `time-receiver`
(actively synchronising), yellow for transient states (`listening`,
`uncalibrated`, `pre-time-transmitter`), and red for fault states (`faulty`,
`disabled`).  The *Message Statistics* section is omitted when no counts are
available.

### Show a specific instance

<pre class="cli"><code>admin@example:/> <b>show ptp 0</b>
</code></pre>

## Tuning port intervals

Adjust announcement, sync, and delay-request intervals per port.  Values
are expressed as log₂ of the interval in seconds (e.g. `-3` = 125 ms,
`0` = 1 s, `1` = 2 s):

<pre class="cli"><code>admin@example:/config/ptp/…/0/port/1/> <b>set port-ds log-announce-interval 0</b>
admin@example:/config/ptp/…/0/port/1/> <b>set port-ds log-sync-interval -3</b>
admin@example:/config/ptp/…/0/port/1/> <b>set port-ds log-min-delay-req-interval 0</b>
admin@example:/config/ptp/…/0/port/1/> <b>set announce-receipt-timeout 3</b>
</code></pre>

`announce-receipt-timeout` is a count of announce intervals, not a duration
in seconds.  With `log-announce-interval 0` (1 s) and
`announce-receipt-timeout 3`, a port waits 3 s without receiving an
`ANNOUNCE` before declaring the time-transmitter lost and returning to
`listening`.

## Message exchange

PTP distributes time using a small set of messages, all of which carry
hardware timestamps at the network interface:

| Message                 | Timestamped | Purpose                                             |
|-------------------------|:-----------:|-----------------------------------------------------|
| `ANNOUNCE`              | No          | Advertises clock quality for BTCA election          |
| `SYNC`                  | Yes         | Carries transmitter timestamp to receivers          |
| `FOLLOW_UP`             | No          | Carries precise `t1` in two-step mode               |
| `DELAY_REQ`             | Yes         | Receiver-initiated E2E delay measurement            |
| `DELAY_RESP`            | No          | Time-transmitter reply to `DELAY_REQ`               |
| `PDELAY_REQ`            | Yes         | Initiates P2P neighbour-delay measurement           |
| `PDELAY_RESP`           | Yes         | Neighbour reply to `PDELAY_REQ`                     |
| `PDELAY_RESP_FOLLOW_UP` | No          | Carries precise `PDELAY_RESP` `t3` in two-step mode |

In **one-step** mode the timestamp is embedded directly into each `SYNC`
message as it leaves the wire, eliminating the need for `FOLLOW_UP`.
In **two-step** mode the `SYNC` carries a placeholder and the precise
transmit timestamp arrives in a subsequent `FOLLOW_UP`.  Hardware
timestamping gives high accuracy in both modes; one-step reduces message
overhead at the cost of more demanding hardware support.

## Message format

Every PTP message begins with a common 34-octet header, regardless of type.
The structure below follows the traditional IETF bit-field layout: each row
is four octets wide, bit 7 (MSB) is on the left and bit 0 (LSB) on the
right within each octet.

```
         7 6 5 4 3 2 1 0 7 6 5 4 3 2 1 0 7 6 5 4 3 2 1 0 7 6 5 4 3 2 1 0
        +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
  0-3   |trSpec |msgType|  rsv  |  ver  |         messageLength         |
        +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
  4-7   |  domainNumber |  minorSdoId   |             flags             |
        +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
 8-15   |                                                               |
        +                        correctionField                        +
        |                                                               |
        +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
16-19   |                      messageTypeSpecific                      |
        +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
20-27   |                                                               |
        +                         clockIdentity                         +
        |                                                               |
        +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
28-31   |          portNumber           |           sequenceId          |
        +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
32-33   |  controlField | logMsgIntvl   |
        +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

- **`trSpec`** (`transportSpecific`, bits 7–4 of octet 0): 4-bit profile
  identifier.  `0x0` = IEEE 1588, `0x1` = gPTP (802.1AS).  Set implicitly
  by the `profile` configuration leaf.
- **`msgType`** (`messageType`, bits 3–0 of octet 0): `0x0` SYNC ·
  `0x1` DELAY_REQ · `0x2` PDELAY_REQ · `0x3` PDELAY_RESP ·
  `0x8` FOLLOW_UP · `0x9` DELAY_RESP · `0xA` PDELAY_RESP_FOLLOW_UP ·
  `0xB` ANNOUNCE.
- **`rsv`** (reserved, bits 7–4 of octet 1): Set to zero; ignored on
  receipt.
- **`ver`** (`versionPTP`, bits 3–0 of octet 1): PTP version; `2` for
  IEEE 1588-2008 and IEEE 1588-2019.
- **`messageLength`** (octets 2–3): Total message length in octets,
  including the header.
- **`domainNumber`** (octet 4): PTP domain; receivers silently discard
  messages that do not match their configured domain.
- **`minorSdoId`** (octet 5): Reserved in IEEE 1588-2008; carries a
  profile sub-identifier in IEEE 1588-2019.
- **`flags`** (octets 6–7): Per-message flags — includes the two-step
  flag (set when a FOLLOW_UP will follow a SYNC), UTC offset valid, and
  leap-second indicators.
- **`correctionField`** (octets 8–15): Accumulated path correction in
  nanoseconds × 2¹⁶.  Transparent Clocks add their measured residence
  time and link delay here as they forward each message, so the final
  time-receiver can subtract the total accumulated delay.
- **`messageTypeSpecific`** (octets 16–19): Reserved in IEEE 1588-2008;
  carries message-type-specific data in IEEE 1588-2019.
- **`clockIdentity`** (octets 20–27): EUI-64 identity of the sending
  clock — the value shown as "Clock identity" in `show ptp`.
- **`portNumber`** (octets 28–29): Port number of the sender within its
  clock; together with `clockIdentity` it forms the unique
  `sourcePortIdentity`.
- **`sequenceId`** (octets 30–31): Increments with each message; used to
  match a DELAY_REQ to its DELAY_RESP.
- **`controlField`** (octet 32): Deprecated in PTPv2; set to fixed
  values per message type for backward compatibility with PTPv1.
- **`logMsgIntvl`** (`logMessageInterval`, octet 33): Log₂ of the
  expected interval between messages of this type; `0x7F` means not
  applicable.

The `transportSpecific` and `domainNumber` fields are the quickest way to
verify on the wire that a device is using the profile and domain you
configured.

### Decoding with Wireshark

Wireshark decodes PTP messages automatically, expanding every header field
and message-type-specific payload in the packet tree.  PTP travels over
two UDP ports — 319 for event messages (SYNC, DELAY_REQ, PDELAY_REQ and
their responses) and 320 for general messages (ANNOUNCE, FOLLOW_UP) — as
well as directly over Ethernet (EtherType `0x88F7`) when layer-2 transport
is in use.

Use the display filter `ptp` to isolate PTP traffic:

```
ptp
```

To narrow down to a specific domain or profile (exact field names can be
verified in Wireshark via **View → Internals → Supported Protocols**,
filtering for `ptp`):

```
ptp.v2.domainnumber == 0
ptp.v2.transportspecific == 1
```

This makes it straightforward to confirm which grandmaster a port is
tracking, verify that `correctionField` is being updated by a Transparent
Clock, or diagnose why the BTCA is not electing the expected grandmaster.

## Glossary

| Abbreviation | Expansion                            | Notes                                                             |
|--------------|--------------------------------------|-------------------------------------------------------------------|
| AVB          | Audio/Video Bridging                 | IEEE 802.1 precursor to TSN; real-time AV over Ethernet           |
| IETF         | Internet Engineering Task Force      | Standards body; defines RFC for layer-3 and up                    |
| UDP          | User Datagram Protocol               | IP transport used by PTP; port 319 (event) and 320 (general)      |
| EUI-64       | Extended Unique Identifier (64-bit)  | IEEE identifier format used as `clockIdentity` in PTP             |
| EtherType    | Ethernet frame type field            | `0x88F7` identifies PTP over layer-2 Ethernet                     |
| BC           | Boundary Clock                       | Terminates and re-originates PTP on each port                     |
| BTCA         | Best TimeTransmitter Clock Algorithm | Elects the GM; replaces BMCA from IEEE 1588-2008                  |
| CMLDS        | Common Mean Link Delay Service       | IEEE 1588-2019 §16.6; shared delay service for multiple instances |
| DS           | Data Set                             | Named attribute collection in IEEE 1588 (default-ds, port-ds, …)  |
| E2E          | End-to-End                           | Delay mechanism: measures path from GM to time-receiver           |
| GM           | Grandmaster                          | PTP network-wide time source, elected by BTCA                     |
| gPTP         | generalized Precision Time Protocol  | IEEE 802.1AS profile; used in TSN and AVB                         |
| NTP          | Network Time Protocol                | Millisecond-accuracy time protocol for wide-area use              |
| OC           | Ordinary Clock                       | Single-port PTP clock; time-transmitter or time-receiver          |
| P2P          | Peer-to-Peer                         | Delay mechanism: measures delay to immediate neighbour            |
| PHC          | PTP Hardware Clock                   | Hardware clock in the NIC used for PTP timestamping               |
| PTP          | Precision Time Protocol              | IEEE 1588 sub-microsecond clock synchronisation protocol          |
| SDO          | Standards Development Organization   | Body that defines a PTP profile; encoded in `sdo-id`              |
| TC           | Transparent Clock                    | Forwards PTP messages, correcting for residence-time delay        |
| TSN          | Time-Sensitive Networking            | IEEE 802.1 standard set for deterministic Ethernet                |
