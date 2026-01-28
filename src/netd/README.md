# netd - Network Daemon for Static Routes and RIP

A lightweight routing daemon that manages static routes and RIP routing protocol.

## Features

- **Static Routes** - IPv4 and IPv6 route management
- **RIP** - Routing Information Protocol (RIPv2) support
- **Dual Backend** - FRR integration or standalone Linux kernel routing
- **Simple Config** - INI-style section-based configuration format
- **Hot Reload** - SIGHUP support for configuration updates

## Building

### With FRR Integration (Default)

Full routing support with FRR gRPC northbound API:

```bash
./configure
make
make install
```

**Requirements:**
- FRR with gRPC northbound support
- protobuf >= 3.0.0
- grpc++ >= 1.16.0

**Note:** The FRR gRPC protocol definition (`grpc/frr-northbound.proto`) is included in the netd source tree.

**Features:**
- Static routes via FRR staticd
- RIP routing protocol
- OSPF (via separate FRR config)
- System command execution

### Standalone Linux Backend

Direct kernel routing without FRR:

```bash
./configure --without-frr
make
make install
```

**Requirements:**
- Linux kernel with rtnetlink support

**Features:**
- Static routes via rtnetlink
- No external dependencies

**Limitations:**
- No RIP/OSPF support
- No system commands

## Configuration

netd uses [libconfuse](https://github.com/martinh/libconfuse) for configuration parsing, providing a clean and structured format.

Configuration files are placed in `/etc/netd/conf.d/` with the `.conf` extension. Files are processed in alphabetical order.

### Configuration Format

The configuration uses libconfuse syntax with sections and key-value pairs:

```
route {
    prefix = "10.0.0.0/24"
    nexthop = "192.168.1.1"
    distance = 1
}

rip {
    enabled = true
    network = ["eth0", "eth1"]
}
```

### Static Routes

Define static routes using `route` sections. Multiple route sections can be specified.

**Route Section:**

```
route {
    prefix = "PREFIX/LEN"
    nexthop = "NEXTHOP"
    distance = DISTANCE
    tag = TAG
}
```

**Parameters:**
- `prefix` (required) - Network prefix with CIDR notation
  - IPv4: `"10.0.0.0/24"`
  - IPv6: `"2001:db8::/32"`
- `nexthop` (required) - Next hop specification
  - IP address: `"192.168.1.1"` or `"fe80::1"`
  - Interface name: `"eth0"`
  - Blackhole: `"blackhole"`, `"reject"`, or `"Null0"`
- `distance` (optional, default: 1) - Administrative distance (1-255)
- `tag` (optional, default: 0) - Route tag (0-4294967295), used for route filtering/redistribution

**Examples:**

IPv4 route via gateway:
```
route {
    prefix = "10.0.0.0/24"
    nexthop = "192.168.1.1"
    distance = 10
}
```

IPv6 route via interface:
```
route {
    prefix = "2001:db8::/32"
    nexthop = "eth0"
    distance = 1
}
```

Blackhole route:
```
route {
    prefix = "192.0.2.0/24"
    nexthop = "blackhole"
}
```

### RIP Configuration

Configure RIP routing protocol (requires FRR backend).

**RIP Section:**

```
rip {
    enabled = BOOL
    default-metric = VALUE
    distance = VALUE
    default-route = BOOL

    network = [LIST]
    passive = [LIST]
    neighbor = [LIST]
    redistribute = [LIST]

    timers {
        update = SECONDS
        invalid = SECONDS
        flush = SECONDS
    }

    debug-events = BOOL
    debug-packet = BOOL
    debug-kernel = BOOL

    system = [LIST]
}
```

**Parameters:**
- `enabled` (optional, default: false) - Enable RIP routing
- `default-metric` (optional, default: 1) - Default route metric (1-16)
- `distance` (optional, default: 120) - Administrative distance (1-255)
- `default-route` (optional, default: false) - Originate default route
- `network` (optional) - List of interfaces to enable RIP on
- `passive` (optional) - List of passive interfaces (receive only)
- `neighbor` (optional) - List of static RIP neighbor addresses
- `redistribute` (optional) - List of route types to redistribute
  - Valid types: `"connected"`, `"static"`, `"kernel"`, `"ospf"`
- `timers` (optional) - RIP timer configuration subsection
  - `update` (default: 30) - Update timer in seconds
  - `invalid` (default: 180) - Invalid timer in seconds
  - `flush` (default: 240) - Flush timer in seconds
- `debug-events` (optional, default: false) - Enable RIP event debugging
- `debug-packet` (optional, default: false) - Enable RIP packet debugging
- `debug-kernel` (optional, default: false) - Enable RIP kernel debugging
- `system` (optional) - List of system commands to execute after config application

**Examples:**

Basic RIP configuration:
```
rip {
    enabled = true
    network = ["eth0", "eth1"]
    redistribute = ["connected"]
}
```

RIP with passive interface:
```
rip {
    enabled = true
    network = ["eth0", "eth1"]
    passive = ["eth1"]
}
```

RIP with custom timers:
```
rip {
    enabled = true
    network = ["eth0"]
    timers {
        update = 15
        invalid = 90
        flush = 120
    }
}
```

### Configuration Files

Configuration files must be placed in `/etc/netd/conf.d/` with the `.conf` extension:

```bash
/etc/netd/conf.d/
├── 10-static.conf      # Static routes
├── 20-rip.conf         # RIP configuration
└── 99-local.conf       # Local overrides
```

Files are processed in alphabetical order. Use numeric prefixes to control processing order.

Lines starting with `#` are comments:

```
# This is a comment
route {
    # This is also a comment
    prefix = "10.0.0.0/24"  # Inline comment
    nexthop = "192.168.1.1"
}
```

### Reloading Configuration

Signal netd to reload configuration:

```bash
# Using killall
sudo killall -HUP netd
```

netd validates configuration on reload. Check syslog for errors.

## Architecture

```
┌─────────┐
│  confd  │ Writes /etc/netd/conf.d/confd.conf
└────┬────┘
     │ SIGHUP
     ▼
┌─────────┐
│  netd   │ Parses config files
└────┬────┘
     │
     ├──► FRR Backend (gRPC)
     │    ├─► mgmtd
     │    ├─► staticd (static routes)
     │    └─► ripd (RIP protocol)
     │
     └──► Linux Backend (rtnetlink)
          └─► Kernel routing table
```

### FRR Backend Flow

1. netd parses config files
2. Builds JSON config for FRR
3. Sends via gRPC to mgmtd
4. mgmtd distributes to backend daemons
5. Executes system commands (if any)

### Linux Backend Flow

1. netd parses config files
2. Opens rtnetlink socket
3. Sends RTM_NEWROUTE/RTM_DELROUTE
4. Kernel updates routing table

## Files

```
src/netd/
├── src/
│   ├── netd.c/h            - Main daemon and data structures
│   ├── config.c/h          - Config parser
│   ├── json_builder.c/h    - FRR JSON config builder
│   ├── grpc_backend.cc/h   - FRR gRPC backend
│   └── linux_backend.c/h   - Linux rtnetlink backend
├── grpc/
│   └── frr-northbound.proto - gRPC protocol definition (copied from FRR)
├── configure.ac            - Build configuration
├── Makefile.am             - Build rules
├── netd.conf               - Sample configuration
└── README.md               - This file
```

## API

### Configuration Format

- libconfuse syntax with sections
- `route { }` - Static route entries (multiple allowed)
- `rip { }` - RIP configuration (single section)

### Supported Routes

- IPv4 and IPv6
- Gateway, interface, blackhole nexthops
- Administrative distance
- Route tags

### RIP Features

- Network interfaces
- Passive interfaces
- Static neighbors
- Route redistribution
- Timer configuration
- Default route origination
- Debug commands

## Logging

netd logs to syslog facility `daemon`:

```bash
# Debug mode (stderr)
netd -d
```

Log levels:
- `INFO` - Configuration changes, route operations
- `ERROR` - Failures, errors
- `DEBUG` - Detailed operation info (with `-d`)

## Signal Handling

- `SIGHUP` - Reload configuration
- `SIGTERM` / `SIGINT` - Graceful shutdown

## License

BSD-3-Clause

## See Also

- FRR Documentation - https://docs.frrouting.org/
- rtnetlink(7) - Linux routing socket API
- libconfuse Documentation - https://github.com/martinh/libconfuse
