# Scripting with RESTCONF

RESTCONF provides a programmatic interface to both configuration and
operational data over HTTPS.  This guide shows practical examples using
`curl` to interact with the RESTCONF API.

All examples use the following conventions:

- **Host**: `example.local` (replace with your device hostname/IP)
- **Credentials**: `admin:admin` (default username:password)
- **HTTPS**: Self-signed certificates require `-k` flag in curl

## Helper Script

To simplify RESTCONF operations, create a `curl.sh` wrapper script:

```bash
#!/bin/sh

AUTH=${AUTH:-admin:admin}
HOST=${HOST:-infix.local}

method=$1
path=$2
shift 2

set -x
exec curl \
     --insecure \
     --user ${AUTH} \
     --request ${method} \
     --header "Content-Type: application/yang-data+json" \
     --header "Accept: application/yang-data+json" \
     "$@" \
     https://${HOST}/restconf/ds/ietf-datastores:${path}
```

Make it executable:

```bash
~$ chmod +x curl.sh
```

This wrapper handles authentication, headers, and the base URL construction,
making commands much cleaner. You can override defaults with environment
variables:

```bash
~$ HOST=192.168.1.10 AUTH=admin:secret ./curl.sh GET running/...
```

The examples below show both raw `curl` commands and the equivalent using
`curl.sh` where applicable.

## Discovery & Common Patterns

Before working with specific configuration items, you often need to discover
what exists on the system. This section shows common discovery patterns and
practical workflows.

### Discovering Available Interfaces

**List all interface names:**

```bash
~$ ./curl.sh example.local GET operational/ietf-interfaces:interfaces 2>/dev/null | jq -r '.["ietf-interfaces:interfaces"]["interface"][].name'
lo
e0
e1
```

This is essential for automation - interface names vary by platform (eth0,
e1, enp0s3, etc.), so scripts should discover them rather than hardcode.

### Get API Capabilities

Discover what YANG modules are available:

```bash
~$ curl -kX GET -u admin:admin \
        -H 'Accept: application/yang-data+json' \
        https://example.local/restconf/data/ietf-yang-library:yang-library
```

This returns all supported YANG modules, revisions, and features.

### Get Entire Running Configuration

Useful for exploration or backup:

```bash
~$ ./curl.sh example.local GET running -o backup.json
```

### Common Workflow Patterns

#### Pattern 1: Find interface by IP address

Get all interfaces with IPs and search:

```bash
~$ ./curl.sh example.local GET operational/ietf-interfaces:interfaces 2>/dev/null \
     | jq -r '.["ietf-interfaces:interfaces"]["interface"][] | select(.["ietf-ip:ipv4"]["address"][]?.ip == "192.168.1.100") | .name'
```

#### Pattern 2: List all interfaces that are down

```bash
~$ ./curl.sh example.local GET operational/ietf-interfaces:interfaces 2>/dev/null \
     | jq -r '.["ietf-interfaces:interfaces"]["interface"][] | select(.["oper-status"] == "down") | .name'
```

#### Pattern 3: Get statistics for all interfaces

```bash
~$ ./curl.sh example.local GET operational/ietf-interfaces:interfaces 2>/dev/null \
     | jq -r '.["ietf-interfaces:interfaces"]["interface"][] | "\(.name): RX \(.statistics["in-octets"]) TX \(.statistics["out-octets"])"'
```

Output:

```
lo: RX 29320 TX 29320
e0: RX 1847392 TX 892341
e1: RX 0 TX 0
```

#### Pattern 4: Check if interface exists before configuring

```bash
~$ if ./curl.sh example.local GET running/ietf-interfaces:interfaces/interface=eth0 2>/dev/null | grep -q "ietf-interfaces:interface"; then
     echo "Interface eth0 exists"
   else
     echo "Interface eth0 not found"
   fi
```

## Configuration Operations

### Read Hostname

Example of fetching JSON configuration data:

**Using curl directly:**

```bash
~$ curl -kX GET -u admin:admin \
        -H 'Accept: application/yang-data+json' \
        https://example.local/restconf/data/ietf-system:system/hostname
{
  "ietf-system:system": {
    "hostname": "foo"
  }
}
```

**Using curl.sh:**

```bash
~$ ./curl.sh example.local GET running/ietf-system:system/hostname
{
  "ietf-system:system": {
    "hostname": "foo"
  }
}
```

### Set Hostname

Example of updating configuration with inline JSON data:

**Using curl directly:**

```bash
~$ curl -kX PATCH -u admin:admin \
     -H 'Content-Type: application/yang-data+json' \
     -d '{"ietf-system:system":{"hostname":"bar"}}' \
     https://example.local/restconf/data/ietf-system:system
```

**Using curl.sh:**

```bash
~$ ./curl.sh example.local PATCH running/ietf-system:system \
     -d '{"ietf-system:system":{"hostname":"bar"}}'
```

### Add IP Address to Interface

Add an IP address to the loopback interface:

```bash
~$ ./curl.sh example.local POST \
     running/ietf-interfaces:interfaces/interface=lo/ietf-ip:ipv4/address=192.168.254.254 \
     -d '{ "prefix-length": 32 }'
```

### Delete IP Address from Interface

Remove an IP address from the loopback interface:

```bash
~$ ./curl.sh example.local DELETE \
     running/ietf-interfaces:interfaces/interface=lo/ietf-ip:ipv4/address=192.168.254.254
```

### Copy Running to Startup

No copy command available yet to copy between datastores, and the
Rousette back-end also does not support "write-through" to the
startup datastore.

To save running-config to startup-config, fetch running to a local file
and then update startup with it:

**Using curl directly:**

```bash
~$ curl -kX GET -u admin:admin -o running-config.json \
        -H 'Accept: application/yang-data+json'       \
         https://example.local/restconf/ds/ietf-datastores:running

~$ curl -kX PUT -u admin:admin -d @running-config.json \
        -H 'Content-Type: application/yang-data+json'  \
        https://example.local/restconf/ds/ietf-datastores:startup
```

**Using curl.sh:**

```bash
~$ ./curl.sh example.local GET running -o running-config.json
~$ ./curl.sh example.local PUT startup -d @running-config.json
```

## Operational Data

### Read Interface Configuration

Get the running configuration for the loopback interface:

```bash
~$ ./curl.sh example.local GET running/ietf-interfaces:interfaces/interface=lo
```

### Read Interface Operational State

Get operational data (state, statistics, etc.) for an interface:

```bash
~$ ./curl.sh example.local GET operational/ietf-interfaces:interfaces/interface=lo
```

This includes administrative and operational state, MAC address, MTU, and
statistics counters.

### Read Interface Statistics

Extract specific statistics using `jq`:

```bash
~$ ./curl.sh example.local GET operational/ietf-interfaces:interfaces/interface=eth0 2>/dev/null \
     | jq -r '.["ietf-interfaces:interfaces"]["interface"][0]["statistics"]["in-octets"]'
```

### List All Interfaces

Get operational data for all interfaces:

```bash
~$ ./curl.sh example.local GET operational/ietf-interfaces:interfaces
```

### Read Routing Table

Get the IPv4 routing table:

```bash
~$ ./curl.sh example.local GET operational/ietf-routing:routing/ribs/rib=ipv4-default
```

### Read OSPF State

Get OSPF operational data (neighbors, routes, etc.):

```bash
~$ ./curl.sh example.local GET operational/ietf-routing:routing/control-plane-protocols/control-plane-protocol=ietf-ospf:ospfv2,default
```

Or get just the neighbor information:

```bash
~$ ./curl.sh example.local GET operational/ietf-routing:routing/control-plane-protocols/control-plane-protocol=ietf-ospf:ospfv2,default/ietf-ospf:ospf/areas/area=0.0.0.0/interfaces
```

## System Operations (RPCs)

### Factory Reset

Reset the system to factory defaults:

```bash
~$ curl -kX POST -u admin:admin \
        -H "Content-Type: application/yang-data+json" \
        https://example.local/restconf/operations/ietf-factory-default:factory-reset
curl: (56) OpenSSL SSL_read: error:0A000126:SSL routines::unexpected eof while reading, errno 0
```

> **Note:** The connection error is expected - the device resets immediately.

### System Reboot

Reboot the system:

```bash
~$ curl -kX POST -u admin:admin \
        -H "Content-Type: application/yang-data+json" \
        https://example.local/restconf/operations/ietf-system:system-restart
```

### Set Date and Time

Example of an RPC that takes input/arguments:

```bash
~$ curl -kX POST -u admin:admin \
        -H "Content-Type: application/yang-data+json" \
        -d '{"ietf-system:input": {"current-datetime": "2024-04-17T13:48:02-01:00"}}' \
        https://example.local/restconf/operations/ietf-system:set-current-datetime
```

Verify the change with SSH:

```bash
~$ ssh admin@example.local 'date'
Wed Apr 17 14:48:12 UTC 2024
```

## Advanced Examples

### Makefile for Common Operations

Create a `Makefile` to simplify common operations:

```makefile
HOST ?= infix.local

lo-running:
	./curl.sh $(HOST) GET running/ietf-interfaces:interfaces/interface=lo

lo-operational:
	./curl.sh $(HOST) GET operational/ietf-interfaces:interfaces/interface=lo

lo-add-ip:
	./curl.sh $(HOST) POST \
	    running/ietf-interfaces:interfaces/interface=lo/ietf-ip:ipv4/address=192.168.254.254 \
	    -d '{ "prefix-length": 32 }'

lo-del-ip:
	./curl.sh $(HOST) DELETE \
	    running/ietf-interfaces:interfaces/interface=lo/ietf-ip:ipv4/address=192.168.254.254

%-stats:
	@./curl.sh $(HOST) GET operational/ietf-interfaces:interfaces/interface=$* 2>/dev/null \
	    | jq -r '.["ietf-interfaces:interfaces"]["interface"][0]["statistics"]["in-octets"]'

%-monitor:
	while sleep 0.2; do make -s HOST=$(HOST) $*-stats; done \
	    | ttyplot -t "$(HOST):$* in-octets" -r
```

Usage examples:

```bash
# Get loopback operational state
~$ make lo-operational

# Add IP to loopback
~$ make lo-add-ip

# Get eth0 statistics
~$ make eth0-stats

# Monitor eth0 traffic in real-time (requires ttyplot)
~$ make eth0-monitor
```

You can override the host:

```bash
~$ make HOST=192.168.1.10 lo-operational
```

### Monitoring Interface Traffic

The `%-monitor` target demonstrates real-time monitoring by polling
interface statistics and piping to `ttyplot` for visualization. Install
`ttyplot` with:

```bash
~$ sudo apt install ttyplot
```

Then monitor any interface:

```bash
~$ make eth0-monitor
```

This creates a live ASCII graph of incoming octets on `eth0`.

## References

- [RESTCONF Protocol (RFC 8040)](https://datatracker.ietf.org/doc/html/rfc8040)
- [YANG Data Modeling Language (RFC 7950)](https://datatracker.ietf.org/doc/html/rfc7950)
- [ietf-interfaces YANG module](https://datatracker.ietf.org/doc/html/rfc8343)
- [ietf-routing YANG module](https://datatracker.ietf.org/doc/html/rfc8349)
- [ietf-system YANG module](https://datatracker.ietf.org/doc/html/rfc7317)
