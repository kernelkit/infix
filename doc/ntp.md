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

```
admin@example:/> configure
admin@example:/config/> edit ntp
admin@example:/config/ntp/> leave
```

When setting up NTP via the CLI the system automatically configures a local
reference clock. The default [stratum](#ntp-stratum-levels) is 16 (unsynchronized),
which is suitable for isolated networks. For production use, configure a specific
stratum level:

```
admin@example:/config/> edit ntp
admin@example:/config/ntp/> set refclock-master master-stratum 10
admin@example:/config/ntp/> leave
```

## Server Mode

Synchronize from upstream NTP servers while serving time to clients:

```
admin@example:/config/> edit ntp
admin@example:/config/ntp/> edit unicast-configuration 0.pool.ntp.org type uc-server
admin@example:/config/ntp/…/0.pool.ntp.org/type/uc-server/> set iburst true
admin@example:/config/ntp/…/0.pool.ntp.org/type/uc-server/> end
admin@example:/config/ntp/> edit unicast-configuration 1.pool.ntp.org type uc-server
admin@example:/config/ntp/…/1.pool.ntp.org/type/uc-server/> set iburst true
admin@example:/config/ntp/…/1.pool.ntp.org/type/uc-server/> end
admin@example:/config/ntp/> leave
```

The `unicast-configuration` uses a composite key with both address and type.
Both hostnames and IP addresses are supported.  The `iburst` option enables
fast initial synchronization.

## Peer Mode

In peer mode, two NTP servers synchronize with each other bidirectionally.
Each server acts as both client and server to the other:

**First peer:**

```
admin@peer1:/config/> edit ntp
admin@peer1:/config/ntp/> edit unicast-configuration 192.168.1.2 type uc-peer
admin@peer1:/config/ntp/…/192.168.1.2/type/uc-peer/> end
admin@peer1:/config/ntp/> set refclock-master master-stratum 8
admin@peer1:/config/ntp/> leave
```

**Second peer:**

```
admin@peer2:/config/> edit ntp
admin@peer2:/config/ntp/> edit unicast-configuration 192.168.1.1 type uc-peer
admin@peer2:/config/ntp/…/192.168.1.1/type/uc-peer/> end
admin@peer2:/config/ntp/> set refclock-master master-stratum 8
admin@peer2:/config/ntp/> leave
```

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

```
admin@example:/config/ntp/> edit unicast-configuration 0.pool.ntp.org type uc-server
admin@example:/config/ntp/…/0.pool.ntp.org/type/uc-server/> set minpoll 4
admin@example:/config/ntp/…/0.pool.ntp.org/type/uc-server/> set maxpoll 10
admin@example:/config/ntp/…/0.pool.ntp.org/type/uc-server/> end
```

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

```
admin@example:/config/ntp/> edit makestep
admin@example:/config/ntp/makestep/> set threshold 2.0
admin@example:/config/ntp/makestep/> set limit 1
admin@example:/config/ntp/makestep/> end
```

- **threshold** - If clock offset exceeds this (in seconds), step immediately
  instead of slewing slowly
- **limit** - Number of updates during which stepping is allowed. After this,
  only gradual slewing is used for security

With these defaults, a device booting at epoch time (1970-01-01) will sync to
correct time within seconds instead of hours.

## Monitoring

Check NTP server statistics:

```
admin@example:/> show ntp server
NTP SERVER CONFIGURATION
Local Stratum       : 10

SERVER STATISTICS
Packets Received    : 142
Packets Sent        : 142
Packets Dropped     : 0
Send Failures       : 0
```

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
