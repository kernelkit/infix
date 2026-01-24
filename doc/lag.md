# Link Aggregation

A link aggregate, or *lag*, allows multiple physical interfaces to be
combined into a single logical interface, providing increased bandwidth
(in some cases) and redundancy (primarily).  Two modes of qualifying lag
member ports are available:

 1. **static**: Active members selected based on link status (carrier)
 2. **lacp:** IEEE 802.3ad Link Aggregation Control Protocol

In LACP mode, LACPDUs are exchanged by the link partners to qualify each
lag member, while in static mode only carrier is used.  This additional
exchange in LACP ensures traffic can be forwarded in both directions.

Traffic distribution, for both modes, across the active lag member ports
is determined by the hash policy[^1].  It uses an XOR of the source,
destination MAC addresses and the EtherType field.  This, IEEE
802.3ad-compliant, algorithm will place all traffic to a particular
network peer on the same link.  Meaning there is no increased bandwidth
for communication between two specific devices.

> [!TIP]
> Similar to other interface types, naming your interface `lagN`, where
> `N` is a number, allows the CLI to automatically infer the interface
> type as LAG.


## Basic Configuration

Creating a link aggregate interface and adding member ports:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interface lag0</b>
admin@example:/config/interface/lag0/> <b>set lag mode static</b>
admin@example:/config/interface/lag0/> <b>end</b>
admin@example:/config/> <b>set interface eth7 lag-port lag lag0</b>
admin@example:/config/> <b>set interface eth8 lag-port lag lag0</b>
admin@example:/config/> <b>leave</b>
</code></pre>

A static lag responds only to link (carrier) changes of member ports.
E.g., in this example egressing traffic is continuously distributed over
the two links until link down on one link is detected, triggering all
traffic to be steered to the sole remaining link.


## LACP Configuration

LACP mode provides dynamic negotiation of the link aggregate.  Key
settings include:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interface lag0</b>
admin@example:/config/interface/lag0/> <b>set lag mode lacp</b>
admin@example:/config/interface/lag0/> <b>set lag lacp mode passive</b>
admin@example:/config/interface/lag0/> <b>set lag lacp rate fast</b>
admin@example:/config/interface/lag0/> <b>set lag lacp system-priority 100</b>
</code></pre>

LACP mode supports two operational modes:

 - **active:** Initiates negotiation by sending LACPDUs (default)
 - **passive:** Waits for peer to initiate negotiation

> [!NOTE]
> At least one end of the link must be in active mode for negotiation to occur.

The LACP rate setting controls protocol timing:

 - **slow:** LACPDUs sent every 30 seconds, with 90 second timeout (default)
 - **fast:** LACPDUs sent every second, with 3 second timeout


## Link Flapping

To protect against link flapping, debounce timers can be configured to
delay link qualification.  Usually only the `up` delay is needed:

<pre class="cli"><code>admin@example:/config/interface/lag0/lag/link-monitor/> <b>edit debounce</b>
admin@example:/config/interface/lag0/lag/link-monitor/debounce/> <b>set up 500</b>
admin@example:/config/interface/lag0/lag/link-monitor/debounce/> <b>set down 200</b>
</code></pre>

## Operational Status, Overview

Like other interfaces, link aggregates are also available in the general
interfaces overview in the CLI admin-exec context.  Here is the above
static mode aggregate:

<pre class="cli"><code>admin@example:/> <b>show interfaces</b>
<span class="header">INTERFACE       PROTOCOL   STATE       DATA                                    </span>
lo              ethernet   UP          00:00:00:00:00:00
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
.
.
.
lag0            lag        UP          static: balance-xor, hash: layer2
│               ethernet   UP          00:a0:85:00:02:00
├ eth7          lag        ACTIVE
└ eth8          lag        ACTIVE
</code></pre>

Same aggregate, but in LACP mode:

<pre class="cli"><code>admin@example:/> <b>show interfaces</b>
<span class="header">INTERFACE       PROTOCOL   STATE       DATA                                    </span>
lo              ethernet   UP          00:00:00:00:00:00
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
.
.
.
lag0            lag        UP          lacp: active, rate: fast (1s), hash: layer2
│               ethernet   UP          00:a0:85:00:02:00
├ eth7          lag        ACTIVE      active, short_timeout, aggregating, in_sync, collecting, distributing
└ eth8          lag        ACTIVE      active, short_timeout, aggregating, in_sync, collecting, distributing
</code></pre>


## Operational Status, Detail

In addition to basic status shown in the interface overview, detailed
LAG status can be inspected:

<pre class="cli"><code>admin@example:/> <b>show interface lag0</b>
name                : lag0
index               : 25
mtu                 : 1500
operational status  : up
physical address    : 00:a0:85:00:02:00
lag mode            : static
lag type            : balance-xor
lag hash            : layer2
link debounce up    : 0 msec
link debounce down  : 0 msec
ipv4 addresses      :
ipv6 addresses      :
in-octets           : 0
out-octets          : 2142
</code></pre>

Same aggregate, but in LACP mode:

<pre class="cli"><code>admin@example:/> <b>show interface lag0</b>
name                : lag0
index               : 24
mtu                 : 1500
operational status  : up
physical address    : 00:a0:85:00:02:00
lag mode            : lacp
lag hash            : layer2
lacp mode           : active
lacp rate           : fast (1s)
lacp aggregate id   : 1
lacp system priority: 65535
lacp actor key      : 9
lacp partner key    : 9
lacp partner mac    : 00:a0:85:00:03:00
link debounce up    : 0 msec
link debounce down  : 0 msec
ipv4 addresses      :
ipv6 addresses      :
in-octets           : 100892
out-octets          : 111776
</code></pre>

Member ports provide additional status information:

 - Link failure counter: number of detected link failures
 - LACP state flags: various states of LACP negotiation:
   - `active`: port is actively sending LACPDUs
   - `short_timeout`: using fast rate (1s) vs. slow rate (30s)
   - `aggregating`: port is allowed to aggregate in this LAG
   - `in_sync`: port is synchronized with partner
   - `collecting`: port is allowed to receive traffic
   - `distributing`: port is allowed to send traffic
   - `defaulted`: using default partner info (partner not responding)
   - `expired`: partner info has expired (no LACPDUs received)
 - Aggregator ID: unique identifier for this LAG group
 - Actor state: LACP state flags for this port (local)
 - Partner state: LACP state flags from the remote port

Example member port status:

<pre class="cli"><code>admin@example:/> <b>show interface eth7</b>
name                : eth7
index               : 8
mtu                 : 1500
operational status  : up
physical address    : 00:a0:85:00:02:00
lag member          : lag0
lag member state    : active
lacp aggregate id   : 1
lacp actor state    : active, short_timeout, aggregating, in_sync, collecting, distributing
lacp partner state  : active, short_timeout, aggregating, in_sync, collecting, distributing
link failure count  : 0
ipv4 addresses      :
ipv6 addresses      :
in-octets           : 473244
out-octets          : 499037
</code></pre>


## Example: Switch Uplink with LACP

LACP mode provides the most robust operation, automatically negotiating
the link aggregate and detecting configuration mismatches.

A common use case is connecting a switch to an upstream device:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit interface lag0</b>
admin@example:/config/interface/lag0/> <b>set lag mode lacp</b>
</code></pre>

Enable fast LACP for quicker fail-over:

<pre class="cli"><code>admin@example:/config/interface/lag0/> <b>set lag lacp rate fast</b>
</code></pre>

Add uplink ports

<pre class="cli"><code>admin@example:/config/interface/lag0/> <b>end</b>
admin@example:/config/> <b>set interface eth7 lag-port lag lag0</b>
admin@example:/config/> <b>set interface eth8 lag-port lag lag0</b>
</code></pre>

Enable protection against "link flapping".

<pre class="cli"><code>admin@example:/config/interface/lag0/> <b>edit lag link-monitor</b>
admin@example:/config/interface/lag0/lag/link-monitor/> <b>edit debounce</b>
admin@example:/config/interface/lag0/lag/link-monitor/debounce/> <b>set up 500</b>
admin@example:/config/interface/lag0/lag/link-monitor/debounce/> <b>set down 200</b>
admin@example:/config/interface/lag0/lag/link-monitor/debounce/> <b>top</b>
</code></pre>

Add to bridge for switching

<pre class="cli"><code>admin@example:/config/interface/lag0/lag/link-monitor/debounce/> <b>end</b>
admin@example:/config/> <b>set interface lag0 bridge-port bridge br0</b>
admin@example:/config/> <b>leave</b>
</code></pre>

[^1]: `(source MAC XOR destination MAC XOR EtherType) MODULO num_links`
