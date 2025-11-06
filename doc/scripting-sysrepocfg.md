> [!NOTE]
> This method is a legacy "simple and human-friendly" way to manage the
> system.  These days we strongly recommend using [RESTCONF][1] instead.

# Legacy Scripting

Although not the primary interface for Infix, it is possible to interact
with the system using raw [sysrepocfg][0] commands.  This way you get to
interact directly with the YANG models when logged in to Infix.  Thus, a
*set config*, *read config*, *read status* and an *RPC* can be conducted
using `sysrepocfg` for any supported YANG model.

See [sysrepocfg][0] for more information.  Examples below will utilize:

- `sysrepocfg -I FILE -fjson -d DATABASE` to import/write a JSON
  formatted configuration file to the specified database.
- `sysrepocfg -E FILE -fjson -d DATABASE` to edit/merge JSON formatted
  configuration in FILE with the specified database.
- `sysrepocfg -R FILE -fjson` to execute remote procedure call (RPC)
   defined in FILE (JSON formatted).
- `sysrepocfg -X -fjson -d DATABASE -x xpath` to read configuration or
  status from specified database.

For importing (-I) and editing (-E), `-d running` is typically used in
examples below. Specify `-d startup` to apply changes to startup
configuration. Exporting (-X) could operate on configuration (e.g.,
`-d running`) or status (`-d operational`).

Some commands require a file as input.  In the examples below we assume
it has been transferred to the device in advance, e.g. using `scp`:

```
~$ cat file.json
{
   "ietf-factory-default:factory-reset": {
   }
}
~$ scp file.json admin@example.local:/tmp/file.json
~$
```

## Factory Reset

```
~$ cat file.json
{
   "ietf-factory-default:factory-reset": {
   }
}
~$ scp file.json admin@example.local:/tmp/file.json
~$ ssh admin@example.local 'sysrepocfg -fjson -R /tmp/file.json'
^C
~$
```

See [Factory Reset](scripting.md#factory-reset) for another (simpler)
alternative.

If it is only wished to copy factory config to running config the
following RPC is available

```
~$ cat file.json
{
   "infix-factory-default:factory-default": {
   }
}
~$ scp file.json admin@example.local:/tmp/file.json
~$ ssh admin@example.local 'sysrepocfg -fjson -R /tmp/file.json'
^C
~$
```

## System Reboot

```
~$ cat /tmp/file.json
{
   "ietf-system:system-restart": {
   }
}
~$ scp file.json admin@example.local:/tmp/file.json
~$ ssh admin@example.local 'sysrepocfg -fjson -R /tmp/file.json'
~$
```

See [System Reboot](scripting.md#system-reboot) for another (simpler)
alternative.

## Set Date and Time

```
~$ ssh admin@example.local 'date'
Sun Nov 20 10:20:23 UTC 2005
~$ cat file.json
{
   "ietf-system:set-current-datetime": {
      "current-datetime": "2024-04-17T13:48:02-01:00"
   }
}
~$ scp file.json admin@example.local:/tmp/file.json
~$ ssh admin@example.local 'sysrepocfg -fjson -R /tmp/file.json'
~$ ssh admin@example.local 'date'
Wed Apr 17 14:48:12 UTC 2024
~$
```

See [Set Date and Time](scripting.md#set-date-and-time) for another
(simpler) alternative.

## Remote Control of Ethernet Ports

Reading administrative status of interface *e0* of running configuration.

```
~$ ssh admin@example.local 'sysrepocfg -X -fjson -d running -e report-all \
       -x \"/ietf-interfaces:interfaces/interface[name='e0']/enabled\"'
{
  "ietf-interfaces:interfaces": {
    "interface": [
      {
        "name": "e0",
        "enabled": true
      }
    ]
  }
}
~$
```

> [!NOTE]
> Without `-e report-all` argument the line `"enabled: true` would not
> be shown as `true` is default.

```
~$ ssh admin@example.local "sysrepocfg -X -fjson -d running \
       -x \"/ietf-interfaces:interfaces/interface[name='e0']/enabled\""
{
  "ietf-interfaces:interfaces": {
    "interface": [
      {
        "name": "e0"
      }
    ]
  }
}
~$
```

Setting the administrative status of interface *e0* of running configuration.

```
$ cat file.json
{
  "ietf-interfaces:interfaces": {
    "interface": [
      {
        "name": "e0",
        "enabled": false
      }
    ]
  }
}
~$ scp file.json admin@example.local:/tmp/file.json
~$ ssh admin@example.local 'sysrepocfg -E /tmp/file.json -fjson -d running'
~$
```

## Enable/Disable DHCPv4 client

Enabling DHCPv4 client on interface *e0*, with current default options.

```
~$ cat /tmp/file.json
{
  "ietf-interfaces:interfaces": {
    "interface": [
      {
        "name": "e0",
        "ietf-ip:ipv4": {
          "infix-dhcp-client:dhcp": {}
        }
      }
    ]
  }
}
~$ scp file.json admin@example.local:/tmp/file.json
~$ ssh admin@example.local 'sysrepocfg -E /tmp/file.json -fjson -d running'
~$
```

Disabling DHCPv4 client on interface *e0* (remove the dhcp container).

```
~$ cat /tmp/file.json
{
  "ietf-interfaces:interfaces": {
    "interface": [
      {
        "name": "e0",
        "ietf-ip:ipv4": {}
      }
    ]
  }
}
~$ scp file.json admin@example.local:/tmp/file.json
~$ ssh admin@example.local 'sysrepocfg -E /tmp/file.json -fjson -d running'
~$
```

To fully remove the DHCPv4 client configuration, remove the `infix-dhcp-client:dhcp`
container from the interface's ipv4 configuration.

## Enable/Disable IPv6

IPv6 is typically enabled on all interfaces by default. The example
below shows IPv4 and IPv6 addresses assigned on *e0*.

```
~$ ssh admin@example.local 'ip addr show dev e0'
2: e0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc pfifo_fast state UP group default qlen 1000
    link/ether 02:00:00:00:00:00 brd ff:ff:ff:ff:ff:ff
    inet 10.0.2.15/24 scope global proto dhcp e0
       valid_lft forever preferred_lft forever
    inet6 fec0::ff:fe00:0/64 scope site dynamic mngtmpaddr proto kernel_ra
       valid_lft 86380sec preferred_lft 14380sec
    inet6 fe80::ff:fe00:0/64 scope link proto kernel_ll
       valid_lft forever preferred_lft forever
~$
```

IPv6 is enabled/disabled per interface. The example below disables IPv6
on interface *e0*.

```
~$ cat /tmp/file.json
{
  "ietf-interfaces:interfaces": {
    "interface": [
      {
        "name": "e0",
        "ietf-ip:ipv6": {
          "enabled": false
        }
      }
    ]
  }
}
~$ scp file.json admin@example.local:/tmp/file.json
~$ ssh admin@example.local 'sysrepocfg -E /tmp/file.json -fjson -d running'
~$ ssh admin@example.local 'ip addr show dev e0'
2: e0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc pfifo_fast state UP group default qlen 1000
    link/ether 02:00:00:00:00:00 brd ff:ff:ff:ff:ff:ff
    inet 10.0.2.15/24 scope global proto dhcp e0
       valid_lft forever preferred_lft forever
~$
```

## Change a Binary Setting

A YANG `binary` type setting is Base64 encoded and requires a little bit
more tricks.  We take the opportunity to showcase a shell script helper:
`/usr/bin/text-editor`, which works just like the `text-editor` command
in the CLI, but this one takes an XPath argument to the binary leaf to
edit.

Stripped down, it looks something like this:

```bash
if tmp=$(sysrepocfg -G "$xpath"); then
    file=$(mktemp)

    echo "$tmp" | base64 -d > "$file"
    if edit "$file"; then
        tmp=$(base64 -w0 < "$file")
        sysrepocfg -S "$xpath" -u "$tmp"
    fi

    rm -f "$file"
else
    echo "Failed to retrieve value for $xpath"
    exit 1
fi
```

An example container configuration, with an embedded file that is
mounted to `/var/www/index.html` can look like this:

```json
  "infix-containers:containers": {
    "container": [
      {
        "name": "web",
        "image": "oci-archive:/lib/oci/curios-httpd-latest.tar.gz",
        "hostname": "web",
        "network": {
          "interface": [
            {
              "name": "veth-sys0"
            }
          ]
        },
        "mount": [
          {
            "name": "index.html",
            "content": "PCFET0NUWVBFIGh0bWwjibberish.shortened.down==",
            "target": "/var/www/index.html"
          }
        ]
      }
    ]
  }
```

The command to edit this file, and restart the container with the new
contents, look like this:

```
admin@infix:~$ cfg edit "/infix-containers:containers/container[name='web']/mount[name='index.html']/content"
```

## Backup Configuration

Displaying running or startup configuration is possible with
`sysrepocfg -X`, as shown below.

```
~$ ssh admin@example.local 'sysrepocfg -X -fjson -d running'
{
  "ieee802-dot1ab-lldp:lldp": {
    "infix-lldp:enabled": true
...
~$
```

An example for backing up startup configuration from remote PC.

```
~$ ssh admin@example.local 'sysrepocfg -X -fjson -d startup > /tmp/backup.json'
~$ scp admin@example.local:/tmp/backup.json .
~$
```

Or possibly skip intermediate storage of file

```
~$ ssh admin@example.local 'sysrepocfg -X -fjson -d startup' > backup.json
~$
```

A final example is to only use `scp`. This is simpler, but only works to
backup the startup configuration (not running).

```
~$ scp admin@example.local:/cfg/startup-config.cfg backup.json
~$
```

## Restore Configuration

To restore a backup configuration to startup, the simplest way is to use
`scp` and reboot as shown below

```
~$ scp admin@example.local:/cfg/startup-config.cfg backup.json
~$ ssh admin@example.local 'reboot'
Connection to switch.local closed by remote host.
~$
```

An alternative method to restore a backup configuration is to use the
`sysrepocfg -I FILE` (import) command.

The example below imports the backup configuration to startup, and
reboots the unit.

```
~$ scp backup.json admin@example.local:/tmp/
~$ ssh admin@example.local 'sudo sysrepocfg -I /tmp/backup.json -fjson -d startup'
~$ ssh admin@example.local 'reboot'
Connection to switch.local closed by remote host.
~$
```

> [!NOTE]
> The login credentials (hash) for the `admin` user are stored as part
> of the configuration file.  When replacing a switch and applying the
> backed up configuration from the former switch, the password on the
> replacement unit will also change.

## Copy Running to Startup

The following command reads out the running config via `sysrepocfg -X`
and writes the result to the startup configuration.

```
~$ ssh admin@example.local 'sysrepocfg -X -fjson -d running > /cfg/startup-config.cfg'
~$
```

An alternative is to write it to a temporary file, and use `sysrepocfg
-I` to import it to startup.

```
~$ ssh admin@example.local 'sysrepocfg -X -fjson -d running > /tmp/running.json'
~$ ssh admin@example.local 'sysrepocfg -I /tmp/running.json -fjson -d startup'
~$
```

## Read Hardware Information

The IETF Hardware YANG model has been augmented for ONIE formatted
production data stored in EEPROMs, if available.  For details, see the
[VPD documentation][2] and the *ietf-hardware* and *infix-hardware*
YANG models.

```
~$ ssh admin@example.local 'sysrepocfg -X -fjson -d operational -x /ietf-hardware:hardware'
{
  "ietf-hardware:hardware": {
    "component": [
      {
        "name": "product",
        "class": "infix-hardware:vpd",
        "serial-num": "12345",
        "model-name": "Switch2010",
        "mfg-date": "2024-01-30T16:42:37+00:00",
        "infix-hardware:vpd-data": {
          "product-name": "Switch2010",
          "part-number": "ABC123-001",
          "serial-number": "007",
          "mac-address": "00:53:00:01:23:45",
          "manufacture-date": "01/30/2024 16:42:37",
          "num-macs": 11,
          "manufacturer": "ACME Production",
          "vendor": "SanFran Networks"
        }
      },
      {
        "name": "USB",
        "class": "infix-hardware:usb",
        "state": {
          "admin-state": "unlocked",
          "oper-state": "enabled"
        }
      }
    ]
  }
}
~$
```

[0]: https://netopeer.liberouter.org/doc/sysrepo/libyang1/html/sysrepocfg.html
[1]: scripting-restconf.md
[2]: vpd.md
