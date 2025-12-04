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
reference clock with [stratum](#ntp-stratum-levels) 10.

View the configuration:

```
admin@example:/> show running-config
  "ietf-ntp:ntp": {
    "refclock-master": {
      "master-stratum": 10
    }
  }
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
fast initial synchronization. The local reference clock (stratum 10) is
automatically configured as a fallback.

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

### Initial Synchronization

Enable clock stepping for systems that boot with incorrect time:

```
admin@example:/config/ntp/> edit makestep
admin@example:/config/ntp/makestep/> set threshold 1.0
admin@example:/config/ntp/makestep/> set limit 3
admin@example:/config/ntp/makestep/> end
```

The `makestep` directive is automatically configured with safe defaults (1.0
seconds threshold, 3 updates limit) when creating an NTP server. This is
critical for embedded systems without RTC that boot with epoch time.

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

- **Stratum 0**: Reference clocks (atomic clocks, GPS receivers)
- **Stratum 1**: Servers directly connected to stratum 0
- **Stratum 2-15**: Servers that sync from lower stratum (each hop adds one)
- **Stratum 16**: Unsynchronized (invalid)

**Default Stratum 10**: Infix uses stratum 10 as the default for local
reference clocks. This is a safe, low-priority value that ensures clients
will prefer upstream-synchronized servers (stratum 1-9) while still having
a fallback time source in isolated networks.
