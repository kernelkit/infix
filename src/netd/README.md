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

Configuration files are placed in `/etc/netd/conf.d/` with `.conf` extension.

### Quick Start

```ini
[routes]
ip route 10.0.0.0/24 192.168.1.1 1
ipv6 route 2001:db8::/32 fe80::1 1

[rip]
network eth0
redistribute connected
```

See [CONFIGURATION.md](CONFIGURATION.md) for complete documentation.

### Sample Configuration

Copy and customize the sample:

```bash
sudo mkdir -p /etc/netd/conf.d
sudo cp netd.conf /etc/netd/conf.d/10-static.conf
sudo vi /etc/netd/conf.d/10-static.conf
```

### Reload

Signal netd to reload configuration:

```bash
sudo killall -HUP netd
# or
sudo initctl touch netd  # with Finit
```

## Running

### Foreground (Debug)

```bash
netd -d
```

### Background

With Finit init system:

```bash
sudo initctl start netd
```

With systemd:

```bash
sudo systemctl start netd
```

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
│   ├── netd.c              - Main daemon
│   ├── config.c            - Config parser
│   ├── route.c/h           - Route data structures
│   ├── rip.c/h             - RIP data structures
│   ├── grpc_backend.cc/h   - FRR gRPC backend
│   └── linux_backend.c/h   - Linux rtnetlink backend
├── grpc/
│   └── frr-northbound.proto - gRPC protocol definition (copied from FRR)
├── configure.ac            - Build configuration
├── Makefile.am             - Build rules
├── netd.conf               - Sample configuration
├── CONFIGURATION.md        - Config format documentation
└── README.md               - This file
```

## API

### Configuration Format

- Section-based INI format
- `[routes]` - Static routes
- `[rip]` - RIP configuration
- `[ospf]` - Reserved for future use

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
# View logs
sudo journalctl -u netd -f

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

- [CONFIGURATION.md](CONFIGURATION.md) - Configuration format details
- FRR Documentation - https://docs.frrouting.org/
- rtnetlink(7) - Linux routing socket API
