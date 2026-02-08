# NTP Server

The NTP (Network Time Protocol) server provides accurate time synchronization
for network clients.  It supports both standalone operation with a local
reference clock and hybrid mode where it synchronizes with upstream servers
while serving time to downstream clients.

> [!NOTE]
> The NTP server is mutually exclusive with the NTP client in system
> configuration context.

## Standalone Mode

Configure a standalone NTP server using only a local reference clock:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit ntp</b>
admin@example:/config/ntp/> <b>leave</b>
</code></pre>

When setting up NTP via the CLI the system automatically configures a local
reference clock. The default [stratum](#ntp-stratum-levels) is 16 (unsynchronized),
which is suitable for isolated networks. For production use, configure a specific
stratum level:

<pre class="cli"><code>admin@example:/config/> <b>edit ntp</b>
admin@example:/config/ntp/> <b>set refclock-master master-stratum 10</b>
admin@example:/config/ntp/> <b>leave</b>
</code></pre>

## GPS Reference Clock

A GPS/GNSS receiver can be used as an NTP reference clock source,
providing stratum 1 time derived from the GPS satellite constellation.
This requires a GPS hardware component to be configured first, see
[Hardware — GPS/GNSS Receivers](hardware.md#gpsgnss-receivers).

### Basic setup

Add a GPS receiver as a reference clock source:

<pre class="cli"><code>admin@example:/config/> <b>edit ntp</b>
admin@example:/config/ntp/> <b>edit refclock-master source gps0</b>
admin@example:/config/ntp/refclock-master/source/gps0/> <b>set poll 2</b>
admin@example:/config/ntp/refclock-master/source/gps0/> <b>set precision 0.1</b>
admin@example:/config/ntp/refclock-master/source/gps0/> <b>end</b>
admin@example:/config/ntp/> <b>leave</b>
</code></pre>

Tunable parameters:

| Parameter   | Default | Description                                        |
|-------------|--------:|----------------------------------------------------|
| `poll`      |     `2` | Polling interval in log2 seconds (2 = 4s)          |
| `precision` |   `0.1` | Assumed precision in seconds (0.1 = 100ms)         |
| `refid`     |  `"GPS"`| Reference identifier (e.g., `GPS`, `GNSS`, `GLO`) |
| `prefer`    | `false` | Prefer this source over other reference clocks     |
| `pps`       | `false` | Enable PPS for microsecond-level accuracy          |
| `offset`    |   `0.0` | Constant offset correction in seconds              |
| `delay`     |   `0.0` | Assumed maximum delay from the receiver             |

### PPS (Pulse Per Second)

When the GPS receiver provides a PPS signal, enable the `pps` option for
microsecond-level accuracy.  With PPS, the GPS time provides the initial
lock and the PPS edges discipline the clock:

<pre class="cli"><code>admin@example:/config/ntp/> <b>edit refclock-master source gps0</b>
admin@example:/config/ntp/refclock-master/source/gps0/> <b>set pps true</b>
admin@example:/config/ntp/refclock-master/source/gps0/> <b>set precision 0.000001</b>
admin@example:/config/ntp/refclock-master/source/gps0/> <b>end</b>
admin@example:/config/ntp/> <b>leave</b>
</code></pre>

### Monitoring

The `show ntp` command shows the GPS receiver as the reference clock source:

<pre class="cli"><code>admin@example:/> <b>show ntp</b>
Mode                : Server (GPS reference clock: gps0)
Port                : 123
Stratum             : 1
Ref time (UTC)      : Sun Feb 08 19:44:36 2026
</code></pre>

Use `show ntp source` to see GPS reference clock details:

<pre class="cli"><code>admin@example:/> <b>show ntp source</b>
Reference Clock     : gps0 (u-blox)
Status              : selected
Fix Mode            : 3D
Satellites          : 9/17 (used/visible)
</code></pre>

## Server Mode

Synchronize from upstream NTP servers while serving time to clients:

<pre class="cli"><code>admin@example:/config/> <b>edit ntp</b>
admin@example:/config/ntp/> <b>edit unicast-configuration 0.pool.ntp.org type uc-server</b>
admin@example:/config/ntp/…/0.pool.ntp.org/type/uc-server/> <b>set iburst true</b>
admin@example:/config/ntp/…/0.pool.ntp.org/type/uc-server/> <b>end</b>
admin@example:/config/ntp/> <b>edit unicast-configuration 1.pool.ntp.org type uc-server</b>
admin@example:/config/ntp/…/1.pool.ntp.org/type/uc-server/> <b>set iburst true</b>
admin@example:/config/ntp/…/1.pool.ntp.org/type/uc-server/> <b>end</b>
admin@example:/config/ntp/> <b>leave</b>
</code></pre>

The `unicast-configuration` uses a composite key with both address and type.
Both hostnames and IP addresses are supported.  The `iburst` option enables
fast initial synchronization.

## Peer Mode

In peer mode, two NTP servers synchronize with each other bidirectionally.
Each server acts as both client and server to the other:

**First peer:**

<pre class="cli"><code>admin@peer1:/config/> <b>edit ntp</b>
admin@peer1:/config/ntp/> <b>edit unicast-configuration 192.168.1.2 type uc-peer</b>
admin@peer1:/config/ntp/…/192.168.1.2/type/uc-peer/> <b>end</b>
admin@peer1:/config/ntp/> <b>set refclock-master master-stratum 8</b>
admin@peer1:/config/ntp/> <b>leave</b>
</code></pre>

**Second peer:**

<pre class="cli"><code>admin@peer2:/config/> <b>edit ntp</b>
admin@peer2:/config/ntp/> <b>edit unicast-configuration 192.168.1.1 type uc-peer</b>
admin@peer2:/config/ntp/…/192.168.1.1/type/uc-peer/> <b>end</b>
admin@peer2:/config/ntp/> <b>set refclock-master master-stratum 8</b>
admin@peer2:/config/ntp/> <b>leave</b>
</code></pre>

This configuration provides mutual synchronization between peers. If one peer
fails, the other continues to serve time to clients.

> [!NOTE]
> The `iburst` and `burst` options are not supported in peer mode.

### Peer Selection in Symmetric Mode

When both peers have the same stratum (as in the example above where both are
stratum 8), NTP's clock selection algorithm uses the **Reference ID** as the
tie-breaker. The Reference ID is typically derived from the peer's IP address
when using a local reference clock.

This means the peer with the **numerically lower IP address** will be selected
as the sync source by the other peer. In the example above:

- peer1 (192.168.1.1) has a lower Reference ID
- peer2 (192.168.1.2) will select peer1 as sync source

This behavior is deterministic and ensures stable clock selection. If you need
a specific peer to be selected, configure it with a lower stratum level than
the other peer.

## Timing Configuration

### Poll Intervals

Control how often the NTP server polls upstream sources:

<pre class="cli"><code>admin@example:/config/ntp/> <b>edit unicast-configuration 0.pool.ntp.org type uc-server</b>
admin@example:/config/ntp/…/0.pool.ntp.org/type/uc-server/> <b>set minpoll 4</b>
admin@example:/config/ntp/…/0.pool.ntp.org/type/uc-server/> <b>set maxpoll 10</b>
admin@example:/config/ntp/…/0.pool.ntp.org/type/uc-server/> <b>end</b>
</code></pre>

Poll intervals are specified as powers of 2:
- `minpoll 4` = poll every 2^4 = 16 seconds (minimum polling rate)
- `maxpoll 10` = poll every 2^10 = 1024 seconds (maximum polling rate)
- Defaults: minpoll 6 (64 seconds), maxpoll 10 (1024 seconds)

Use shorter intervals (minpoll 2-4) for faster convergence in test environments
or peer configurations. Use defaults for production servers.

### Fast Initial Synchronization

The `makestep` directive is automatically configured with safe defaults (1.0
seconds threshold, 3 updates limit) when creating an NTP server. This is
critical for embedded systems without RTC that boot with epoch time.

To customize the values:

<pre class="cli"><code>admin@example:/config/ntp/> <b>edit makestep</b>
admin@example:/config/ntp/makestep/> <b>set threshold 2.0</b>
admin@example:/config/ntp/makestep/> <b>set limit 1</b>
admin@example:/config/ntp/makestep/> <b>end</b>
</code></pre>

- **threshold** - If clock offset exceeds this (in seconds), step immediately
  instead of slewing slowly
- **limit** - Number of updates during which stepping is allowed. After this,
  only gradual slewing is used for security

With these defaults, a device booting at epoch time (1970-01-01) will sync to
correct time within seconds instead of hours.

## Monitoring

For a quick overview:

To view the sources being used by the NTP client, run:

<pre class="cli"><code>admin@example:/> <b>show ntp</b>
Mode                : Client
Stratum             : 3
Ref time (UTC)      : Sat Jan 24 23:41:42 2026

<span class="header">ADDRESS         MODE    STATE     STRATUM  POLL</span>
147.78.228.41   server  outlier         2   64s
192.168.0.1     server  unusable        0  128s
176.126.86.247  server  selected        2   64s
</code></pre>

Check NTP source status:

<pre class="cli"><code>admin@example:/> <b>show ntp source</b>
<span class="header">MS  Name/IP address  Stratum  Poll  Reach  LastRx          Last sample</span>
^+  147.78.228.41          2     6    007      15  -431us +/- 33.573ms
^*  176.126.86.247         2     6    007      14   -389us +/- 4.307ms
</code></pre>

For detailed information about a specific source:

<pre class="cli"><code>admin@example:/> <b>show ntp source 176.126.86.247</b>
Address             : 176.126.86.247
Mode                : Server (client mode) [^]
State               : Selected sync source [*]
Configured          : Yes
Stratum             : 2
Poll interval       : 7 (2^7 seconds = 128s)
Reachability        : 377 (octal) = 11111111b
Last RX             : 75s ago
Offset              : +2.0us (+0.002000ms)
Delay               : 4.270ms (0.004270s)
Dispersion          : 205.0us (0.205000ms)
</code></pre>

## NTP Stratum Levels

NTP uses a hierarchical system called **stratum** to indicate distance from
authoritative time sources:

- **Stratum 0**: Reference clocks (atomic clocks)
- **Stratum 1**: Servers directly connected to stratum 0 (e.g., GPS receivers)
- **Stratum 2-15**: Servers that sync from lower stratum (each hop adds one)
- **Stratum 16**: Unsynchronized (invalid)

The default stratum (16) is not suitable for distributing time in isolated
networks, so when setting up an NTP server remember to adjust this value.
Use, e.g., `10`, this is a safe, low-priority value that ensures clients will
prefer upstream-synchronized servers (stratum 1-9) while still having a
fallback time source in isolated networks.
