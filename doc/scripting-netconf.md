# Scripting with NETCONF

NETCONF (Network Configuration Protocol) provides a standardized mechanism for
managing network devices using XML-based RPC operations over SSH (port 830).
This guide shows practical examples for interacting with Infix using NETCONF.

NETCONF offers robust capabilities for network automation:

- **Transactional operations**: Validate before commit, rollback on error
- **Fine-grained locking**: Prevent concurrent configuration conflicts
- **Structured data**: XML with YANG schema validation
- **Standardized operations**: Get, edit-config, copy-config, etc.

## NETCONF vs RESTCONF

Both protocols use the same YANG data models, but differ in approach:

| Feature    | NETCONF            | RESTCONF                        |
|------------|--------------------|---------------------------------|
| Transport  | SSH                | HTTPS                           |
| Encoding   | XML                | JSON/XML                        |
| Operations | RPC-based          | REST/HTTP methods               |
| Best for   | Automation scripts | Web integration, simple queries |

Choose NETCONF when you need:

- Transactional configuration changes
- Configuration validation before commit
- Locking to prevent concurrent changes
- Integration with existing NETCONF tooling

Choose RESTCONF for:

- Simple queries and updates
- Web-based applications
- When you prefer JSON over XML
- RESTful API patterns

## Quick Start with netopeer2-cli

`netopeer2-cli` is an interactive NETCONF client, useful for learning and
testing. Install it:

```bash
~$ sudo apt install netopeer2-cli
```

Connect to your Infix device:

```bash
~$ netopeer2-cli
> connect --host example.local --login admin
admin@example.local password:
> status
Current NETCONF session:
  ID          : 1
  Host        : example.local
  Port        : 830
  Transport   : SSH
  Capabilities: 35
```

### Basic Operations in netopeer2-cli

**Get entire configuration:**

```
> get-config --source running
```

**Get specific subtree (hostname):**

```
> get-config --source running --filter-xpath /system/hostname
<data xmlns="urn:ietf:params:xml:ns:netconf:base:1.0">
  <system xmlns="urn:ietf:params:xml:ns:yang:ietf-system">
    <hostname>example</hostname>
  </system>
</data>
```

**Get operational state:**

```
> get --filter-xpath /interfaces
```

**Edit configuration:**

```
> edit-config --target candidate --config=/tmp/config.xml
> commit
```

**Disconnect:**

```
> disconnect
> quit
```

## Discovery & Common Patterns

Before working with specific configuration items, you often need to discover
what exists on the system. This section shows common discovery patterns and
practical workflows.

### Discovering Available Interfaces

**List all interface names:**

```xml
<rpc message-id="1" xmlns="urn:ietf:params:xml:ns:netconf:base:1.0">
  <get>
    <filter type="subtree">
      <interfaces xmlns="urn:ietf:params:xml:ns:yang:ietf-interfaces">
        <interface>
          <name/>
        </interface>
      </interfaces>
    </filter>
  </get>
</rpc>
```

Using netopeer2-cli:

```
> get --filter-xpath /interfaces/interface/name
```

This returns all interface names, useful for iterating through interfaces
in scripts.

### Get All YANG Capabilities

Discover which YANG modules and features are available:

```
> status
```

Or programmatically via the `<hello>` message capabilities received during
connection establishment.

### Get Entire Running Configuration

Useful for exploration or backup:

```xml
<rpc message-id="2" xmlns="urn:ietf:params:xml:ns:netconf:base:1.0">
  <get-config>
    <source>
      <running/>
    </source>
  </get-config>
</rpc>
```

Using netopeer2-cli:

```
> get-config --source running
```

### Common Workflow Patterns

#### Pattern 1: Find interface by IP address

Get all interfaces with their IPs, then filter:

```
> get --filter-xpath /interfaces
```

Parse the XML output to find which interface has the desired IP.

#### Pattern 2: Check which interfaces are down

```
> get --filter-xpath /interfaces/interface/oper-status
```

Look for interfaces with `<oper-status>down</oper-status>`.

#### Pattern 3: Get interface statistics for monitoring

```xml
<rpc message-id="3" xmlns="urn:ietf:params:xml:ns:netconf:base:1.0">
  <get>
    <filter type="subtree">
      <interfaces xmlns="urn:ietf:params:xml:ns:yang:ietf-interfaces">
        <interface>
          <statistics/>
        </interface>
      </interfaces>
    </filter>
  </get>
</rpc>
```

Returns in-octets, out-octets, in-errors, out-errors for all interfaces.

## Scripting with netconf-client

[netconf-client](https://github.com/wires-se/netconf-client) is a lightweight
Python-based NETCONF client designed for scripting and automation.

### Installation

```bash
~$ pip install netconf-client
```

Or clone and install from source:

```bash
~$ git clone https://github.com/wires-se/netconf-client.git
~$ cd netconf-client
~$ pip install .
```

### Basic Usage

The client provides a simple command-line interface:

```bash
~$ netconf-client --host example.local --user admin --password admin <operation>
```

Common operations:

- `get-config` - Retrieve configuration
- `edit-config` - Modify configuration
- `get` - Retrieve operational state
- `copy-config` - Copy between datastores
- `lock/unlock` - Lock datastores
- `commit` - Commit candidate configuration

### Python API

You can also use netconf-client as a Python library:

```python
from netconf_client.client import NetconfClient

# Connect
client = NetconfClient(
    host='example.local',
    username='admin',
    password='admin'
)

# Get configuration
config = client.get_config(source='running')
print(config)

# Edit configuration
xml_config = """
<config>
  <system xmlns="urn:ietf:params:xml:ns:yang:ietf-system">
    <hostname>newhostname</hostname>
  </system>
</config>
"""
client.edit_config(target='candidate', config=xml_config)
client.commit()

# Close connection
client.close()
```

## Configuration Examples

### Read Hostname

**XML request:**

```xml
<rpc message-id="1" xmlns="urn:ietf:params:xml:ns:netconf:base:1.0">
  <get-config>
    <source>
      <running/>
    </source>
    <filter type="subtree">
      <system xmlns="urn:ietf:params:xml:ns:yang:ietf-system">
        <hostname/>
      </system>
    </filter>
  </get-config>
</rpc>
```

**Using netopeer2-cli:**

```
> get-config --source running --filter-xpath /system/hostname
```

**Using netconf-client:**

```bash
~$ netconf-client --host example.local --user admin \
     get-config --source running --xpath /system/hostname
```

**Response:**

```xml
<rpc-reply message-id="1" xmlns="urn:ietf:params:xml:ns:netconf:base:1.0">
  <data>
    <system xmlns="urn:ietf:params:xml:ns:yang:ietf-system">
      <hostname>example</hostname>
    </system>
  </data>
</rpc-reply>
```

### Set Hostname

**XML request:**

```xml
<rpc message-id="2" xmlns="urn:ietf:params:xml:ns:netconf:base:1.0">
  <edit-config>
    <target>
      <candidate/>
    </target>
    <config>
      <system xmlns="urn:ietf:params:xml:ns:yang:ietf-system">
        <hostname>newhostname</hostname>
      </system>
    </config>
  </edit-config>
</rpc>
```

**Using netopeer2-cli:**

Save the config to `/tmp/hostname.xml`:

```xml
<config>
  <system xmlns="urn:ietf:params:xml:ns:yang:ietf-system">
    <hostname>newhostname</hostname>
  </system>
</config>
```

Then apply:

```
> edit-config --target candidate --config=/tmp/hostname.xml
> commit
```

**Using netconf-client:**

```bash
~$ cat > hostname.xml <<EOF
<config>
  <system xmlns="urn:ietf:params:xml:ns:yang:ietf-system">
    <hostname>newhostname</hostname>
  </system>
</config>
EOF

~$ netconf-client --host example.local --user admin \
     edit-config --target candidate --config hostname.xml

~$ netconf-client --host example.local --user admin commit
```

### Add IP Address to Interface

Save the config to `ip-config.xml`:

```xml
<config>
  <interfaces xmlns="urn:ietf:params:xml:ns:yang:ietf-interfaces">
    <interface>
      <name>eth0</name>
      <ipv4 xmlns="urn:ietf:params:xml:ns:yang:ietf-ip">
        <address>
          <ip>192.168.1.100</ip>
          <prefix-length>24</prefix-length>
        </address>
      </ipv4>
    </interface>
  </interfaces>
</config>
```

**Using netconf-client:**

```bash
~$ netconf-client --host example.local --user admin \
     edit-config --target candidate --config ip-config.xml
~$ netconf-client --host example.local --user admin commit
```

### Copy Running to Startup

**XML request:**

```xml
<rpc message-id="3" xmlns="urn:ietf:params:xml:ns:netconf:base:1.0">
  <copy-config>
    <source>
      <running/>
    </source>
    <target>
      <startup/>
    </target>
  </copy-config>
</rpc>
```

**Using netopeer2-cli:**

```
> copy-config --source running --target startup
```

**Using netconf-client:**

```bash
~$ netconf-client --host example.local --user admin \
     copy-config --source running --target startup
```

## Operational Data Examples

### Read Interface State

**XML request:**

```xml
<rpc message-id="4" xmlns="urn:ietf:params:xml:ns:netconf:base:1.0">
  <get>
    <filter type="subtree">
      <interfaces xmlns="urn:ietf:params:xml:ns:yang:ietf-interfaces">
        <interface>
          <name>eth0</name>
        </interface>
      </interfaces>
    </filter>
  </get>
</rpc>
```

**Using netopeer2-cli:**

```
> get --filter-xpath /interfaces/interface[name='eth0']
```

**Using netconf-client:**

```bash
~$ netconf-client --host example.local --user admin \
     get --xpath "/interfaces/interface[name='eth0']"
```

This returns operational state including admin/oper status, statistics,
MAC address, MTU, and IP addresses.

### Read All Interfaces

**Using netopeer2-cli:**

```
> get --filter-xpath /interfaces
```

**Using netconf-client:**

```bash
~$ netconf-client --host example.local --user admin \
     get --xpath /interfaces
```

### Read Routing Table

**Using netconf-client:**

```bash
~$ netconf-client --host example.local --user admin \
     get --xpath "/routing/ribs/rib[name='ipv4-default']"
```

### Read OSPF Neighbors

**Using netconf-client:**

```bash
~$ netconf-client --host example.local --user admin \
     get --xpath "/routing/control-plane-protocols/control-plane-protocol[type='ietf-ospf:ospfv2'][name='default']/ietf-ospf:ospf"
```

## Advanced Scripting

### Python Script: Backup Configuration

```python
#!/usr/bin/env python3
from netconf_client.client import NetconfClient
from datetime import datetime
import sys

def backup_config(host, user, password, output_file):
    try:
        # Connect
        client = NetconfClient(host=host, username=user, password=password)

        # Get running config
        config = client.get_config(source='running')

        # Save to file with timestamp
        timestamp = datetime.now().strftime('%Y%m%d-%H%M%S')
        filename = f"{output_file}-{timestamp}.xml"

        with open(filename, 'w') as f:
            f.write(config)

        print(f"Configuration backed up to {filename}")

        client.close()
        return 0

    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        return 1

if __name__ == '__main__':
    if len(sys.argv) != 5:
        print(f"Usage: {sys.argv[0]} <host> <user> <password> <output_prefix>")
        sys.exit(1)

    sys.exit(backup_config(sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]))
```

Usage:

```bash
~$ chmod +x backup.py
~$ ./backup.py example.local admin admin config-backup
Configuration backed up to config-backup-20250102-143022.xml
```

### Python Script: Monitor Interface Statistics

```python
#!/usr/bin/env python3
from netconf_client.client import NetconfClient
import xml.etree.ElementTree as ET
import time
import sys

def get_interface_stats(client, interface):
    """Get interface statistics"""
    xpath = f"/interfaces/interface[name='{interface}']/statistics"
    data = client.get(filter_xpath=xpath)

    # Parse XML to extract counters
    root = ET.fromstring(data)
    ns = {'if': 'urn:ietf:params:xml:ns:yang:ietf-interfaces'}

    stats = {}
    for stat in root.findall('.//if:statistics/*', ns):
        stats[stat.tag.split('}')[1]] = int(stat.text)

    return stats

def monitor_interface(host, user, password, interface, interval=5):
    """Monitor interface statistics"""
    client = NetconfClient(host=host, username=user, password=password)

    print(f"Monitoring {interface} on {host} (Ctrl-C to stop)")
    print(f"{'Time':<20} {'RX Packets':<15} {'TX Packets':<15} {'RX Bytes':<15} {'TX Bytes':<15}")
    print("-" * 80)

    try:
        while True:
            stats = get_interface_stats(client, interface)
            timestamp = time.strftime('%Y-%m-%d %H:%M:%S')

            print(f"{timestamp:<20} "
                  f"{stats.get('in-unicast-pkts', 0):<15} "
                  f"{stats.get('out-unicast-pkts', 0):<15} "
                  f"{stats.get('in-octets', 0):<15} "
                  f"{stats.get('out-octets', 0):<15}")

            time.sleep(interval)

    except KeyboardInterrupt:
        print("\nMonitoring stopped")
    finally:
        client.close()

if __name__ == '__main__':
    if len(sys.argv) < 5:
        print(f"Usage: {sys.argv[0]} <host> <user> <password> <interface> [interval]")
        sys.exit(1)

    interval = int(sys.argv[5]) if len(sys.argv) > 5 else 5
    monitor_interface(sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4], interval)
```

Usage:

```bash
~$ chmod +x monitor.py
~$ ./monitor.py example.local admin admin eth0 2
Monitoring eth0 on example.local (Ctrl-C to stop)
Time                 RX Packets      TX Packets      RX Bytes        TX Bytes
--------------------------------------------------------------------------------
2025-01-02 14:35:10  12453          8932           1847392        892341
2025-01-02 14:35:12  12489          8967           1851204        895673
...
```

### Shell Script: Batch Configuration

```bash
#!/bin/bash
# Apply configuration to multiple devices

DEVICES="device1.local device2.local device3.local"
USER="admin"
PASSWORD="admin"
CONFIG_FILE="$1"

if [ -z "$CONFIG_FILE" ]; then
    echo "Usage: $0 <config.xml>"
    exit 1
fi

for device in $DEVICES; do
    echo "Configuring $device..."

    # Edit candidate
    netconf-client --host "$device" --user "$USER" --password "$PASSWORD" \
        edit-config --target candidate --config "$CONFIG_FILE"

    if [ $? -eq 0 ]; then
        # Commit if edit succeeded
        netconf-client --host "$device" --user "$USER" --password "$PASSWORD" commit
        echo "  ✓ $device configured successfully"
    else
        echo "  ✗ $device configuration failed"
    fi
done
```

## Other NETCONF Tools

### ncclient (Python)

Popular Python library for NETCONF:

```bash
~$ pip install ncclient
```

Example:

```python
from ncclient import manager

with manager.connect(host='example.local', port=830,
                     username='admin', password='admin',
                     hostkey_verify=False) as m:
    # Get config
    c = m.get_config(source='running')
    print(c)
```

### Ansible

Ansible includes NETCONF modules for automation:

```yaml
- name: Get interface config
  netconf_get:
    source: running
    filter: <interfaces xmlns="urn:ietf:params:xml:ns:yang:ietf-interfaces"/>
```

### Cisco Tools

- **NSO (Network Services Orchestrator)**: Enterprise orchestration platform
- **YANG Suite**: Web-based YANG exploration and testing tool
- **Cisco pyATS**: Network test automation

## Troubleshooting

### Enable NETCONF Debugging

For netconf-client, use verbose mode:

```bash
~$ netconf-client --host example.local --user admin --verbose get-config
```

For netopeer2-cli, enable debug output:

```
> debug 1
> get-config --source running
```

### Common Issues

**Connection refused:**

- Verify SSH is running on port 830: `ssh -p 830 admin@example.local`
- Check firewall rules

**Authentication failed:**

- Verify credentials
- Check user has NETCONF access permissions

**Operation not supported:**

- Verify NETCONF capability: `netopeer2-cli` → `status` → check capabilities
- Some operations require specific YANG modules

## References

- [NETCONF Protocol (RFC 6241)](https://datatracker.ietf.org/doc/html/rfc6241)
- [netconf-client on GitHub](https://github.com/wires-se/netconf-client)
- [netopeer2 Documentation](https://github.com/CESNET/netopeer2)
- [YANG Data Modeling Language (RFC 7950)](https://datatracker.ietf.org/doc/html/rfc7950)
- [ietf-interfaces YANG module](https://datatracker.ietf.org/doc/html/rfc8343)
- [ietf-system YANG module](https://datatracker.ietf.org/doc/html/rfc7317)
- [ncclient Documentation](https://ncclient.readthedocs.io/)
