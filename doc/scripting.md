# Scripting Infix

In some situations a user cannot, or does not want to, use the NETCONF
API for interacting with Infix.  Examples include production tasks and
simpler remote scripting jobs to one or more remote devices.

This document assumes you have the password for the `admin` user, and
that you can connect to the device.  Please see [Finding my Device][1]
for help on locating it.

Furthermore, the example commands shown here that are execute from a PC
to a remote device over SSH, use Linux/UNIX.  With advances lately in
both Windows and macOS, many of the user friendly tools previously only
available in Linux are now available there too.

 - The shell prompt for the PC laptop side:
 
   ```shell
   ~$
   ```

 - The shell prompt when logged in to an Infix device:

   ```shell
   admin@example:~$
   ```

> **Note:** the shell script commands used here are the raw variants
> which the CLI usually wraps in a warm and snugly blanket.  Meaning
> they may change over time, while the CLI wrappers do *not*.  That
> being said, please let us know if you find any inconsistencies.


## Tips

 - Ensure the `admin` user does *not* have `clish` as login shell
 - Enable [SSH key authentication](system.md#ssh-authorized-key)
 - Deploy same SSH *public* key to many Infix devices
 - Secure your *private* SSH key using, e.g., `ssh-agent`

The `ssh-keygen` command, used to create the private/public key pair,
asks for a passphrase and although this is *technically optional* it is
highly recommended to set one.  For ease of use, in particular when
scripting, use `ssh-agent` to avoid retyping the passphrase for every
command.

Useful links on SSH, keys, and using `ssh-agent`:

 - https://en.wikipedia.org/wiki/Ssh-agent
 - https://www.cyberciti.biz/faq/how-to-use-ssh-agent-for-authentication-on-linux-unix/
 - https://goteleport.com/blog/how-to-use-ssh-agent-safely/


## Admin User Not Authorized?

All system services and critical configuration files are owned by the
locked-down `root` user.  It is not possible to activate the `root` user
account for remote logins.  Instead, use `admin` user and the `sudo`
command prefix.

Here we are logged in to an example device:

```
admin@example:~$ cp /cfg/startup-config.cfg /cfg/backup-config.cfg
cp: can't create '/cfg/backup-config.cfg': Permission denied
admin@example:~$ sudo cp /cfg/startup-config.cfg /cfg/backup-config.cfg
``` 

## Examples

The following example commands are run from the PC over SSH.  The
following is a *very brief* introduction.

The notation is `ssh username@address`, where address can be an IPv4 or
IPv6 address, a DNS name, or an mDNS name, e.g. infix.local.  In the
case of IPv6: `address%interface`, where interface differs between
operating systems.  On Linux and macOS the interface name is used, but
on Windows the interface index[^1] is used.

[^1]: Press Win-r to bring up the Run dialog, enter `cmd.exe` and press
    enter.  Then type in `ipconfig /all` to list all interfaces, their
    status, as well as interface index.

**Logging in to a device**

```
~$ ssh admin@fe80::ff:fe00:0%eth0
The authenticity of host 'fe80::ff:fe00:0%eth0 (fe80::ff:fe00:0%eth0)' can't be established.
ED25519 key fingerprint is SHA256:5/9mw64jhmYyD8MD+SwrsG3RXMBbP48pDe2T8bg14RQ.
This key is not known by any other names
Are you sure you want to continue connecting (yes/no/[fingerprint])? yes
Warning: Permanently added 'fe80::ff:fe00:0%eth0' (ED25519) to the list of known hosts.
admin@fe80::ff:fe00:0%eth0's password: *****
.-------.
|  . .  | Infix -- a Network Operating System
|-. v .-| https://kernelkit.org
'-'---'-'

Run the command 'cli' for interactive OAM
```

**Executing a command on a device**

```
~$ ssh admin@fe80::ff:fe00:0%eth0 echo hej
admin@fe80::ff:fe00:0%eth0's password: *****
hej
```

**Made Easy**

Connecting to networked devices using IP addresses is the way many
people are used to.  The above example with IPv6 tend to scare off
people, so for the rest of this document we'll use the mDNS name
instead:

```
~$ ssh admin@infix.local%eth0
The authenticity of host 'infix.local%eth0 (infix.local%eth0)' can't be established.
``` 

### Factory Reset

The command option `-y` disables any "are you sure?" interaction and
immediately triggers a factory reset and reboot of the device.  It is
when the device boots up it erases all writable storage.

```
~$ ssh admin@infix.local%eth0 factory -y
admin@infix.local%eth0's password: *****
```

### System Reboot 

```
~$ ssh admin@infix.local%eth0 reboot
admin@infix.local%eth0's password: *****
```

### Set Date and Time

Devices running Infix may have their system time completely off and this
can cause problems for upgrading and when accessing the web interface
over HTTPS (certificate validation looks at start and end dates).

To set the device's system time, *and* sync that with the RTC:
use the PCs current time as argument:

```
~$ ssh admin@infix.local%eth0 "sudo date -s '2024-03-20 18:14+01:00' && sudo hwclock -w -u"
admin@infix.local%eth0's password: *****
```

> The `-u` option ensures saving system time to the RTC in UTC time.

Verify that the change took:

```
~$ ssh admin@infix.local%eth0 date
admin@infix.local%eth0's password: *****
Wed Mar 20 17:14:47 UTC 2024
```

### Remote Control of Ethernet Ports

There are two ways to do it:

 1. Change the configuration without saving it to `startup-config`
 2. Change the operational state

The first involves sending a NETCONF command/config in XML. The second
we will cover here. We start by querying available interfaces (ports) on
the remote system:

```
~$ ssh admin@infix.local%qtap0 ip -br a
admin@infix.local%qtap0's password: 
lo               UP             127.0.0.1/8 ::1/128 
e0               UP             fe80::ff:fe00:0/64 
e1               UP             
e2               UP             
e3               UP             
e4               UP             
e5               UP             fe80::ff:fe00:5/64 
e6               UP             fe80::ff:fe00:6/64 
e7               UP             fe80::ff:fe00:7/64 
e8               UP             fe80::ff:fe00:8/64 
e9               UP             192.168.2.200/24 fe80::ff:fe00:9/64 
br0              UP             
```

Here we see a loopback interface (lo), ten Ethernet ports (e0-e9) and a
bridge (br0).  From this quick glance we can guess that the ports e1-e4
are bridged (you can verify this with the remote command `bridge link`)
because they do not have a link-local IPv6 address.

I know it's port e6 that I want to take down:

```
~$ ssh admin@infix.local%qtap0 ip link set e6 down
admin@infix.local%qtap0's password: 
RTNETLINK answers: Operation not permitted
~$ ssh admin@infix.local%qtap0 sudo ip link set e6 down
admin@infix.local%qtap0's password: 
```

Changing the operational link state of a port is a privileged command,
so we have to prefix our command with `sudo`.

Inspecting the link state again show the port is now down:

```
~$ ssh admin@infix.local%qtap0 ip -br a
admin@infix.local%qtap0's password: 
lo               UP             127.0.0.1/8 ::1/128 
e0               UP             fe80::ff:fe00:0/64 
e1               UP             
e2               UP             
e3               UP             
e4               UP             
e5               UP             fe80::ff:fe00:5/64 
e6               DOWN           
e7               UP             fe80::ff:fe00:7/64 
e8               UP             fe80::ff:fe00:8/64 
e9               UP             192.168.2.200/24 fe80::ff:fe00:9/64 
br0              UP  
```

### Check Device's Network Connectivity

Say you want to perform a [System Upgrade][#system-uprgade] and it just
doesn't work, then you might want to ensure the device actually can
reach the upgrade server.

```
~$ ssh admin@infix.local%eth0 ping -c 3 server.local
admin@infix.local%qtap0's password: *****
PING server.local (192.168.2.42) 56(84) bytes of data.
64 bytes from server.local: icmp_seq=1 ttl=64 time=0.201 ms
64 bytes from server.local: icmp_seq=2 ttl=64 time=0.432 ms
64 bytes from server.local: icmp_seq=3 ttl=64 time=0.427 ms

--- server.local ping statistics ---
3 packets transmitted, 3 received, 0% packet loss, time 2050ms
rtt min/avg/max/mdev = 0.201/0.353/0.432/0.107 ms
```

Here we get a reply, so whatever is the issue with the upgrade was
not hiding behind a connectivity issue at least.

### System Upgrade

The underlying software that handles upgrades is called [RAUC][2].  To
trigger an upgrade you (currently) need an FTP/TFTP or HTTP/HTTPS server
where RAUC can fetch the upgrade from.  In this example we use an FTP
server to upgrade the currently inactive "slot":

```
~$ ssh admin@infix.local%eth0 rauc install ftp://server.local/infix-aarch64-24.06.0.pkg
admin@infix.local%eth0's password: *****
installing
  0% Installing
  0% Determining slot states
 20% Determining slot states done.
 20% Checking bundle
 20% Verifying signature
 40% Verifying signature done.
 40% Checking bundle done.
 40% Checking manifest contents
 60% Checking manifest contents done.
 60% Determining target install group
 80% Determining target install group done.
 80% Updating slots
 80% Checking slot rootfs.1
 90% Checking slot rootfs.1 done.
 90% Copying image to rootfs.1
 99% Copying image to rootfs.1 done.
 99% Updating slots done.
100% Installing done.
idle
Installing `ftp://server.local/infix-aarch64-24.06.0.pkg` succeeded
~$
```

The inactive slot is now marked active and will be used on the next
boot.  To upgrade the partition we booted from, we must first reboot.

For more information, see the [Boot Procedure][3] document.

**Alternative:**

If you know your device has sufficient storage, eMMC or RAM disk (check
with the remote command `df -h`), you can also copy the `.pkg` file to
the device instead of having to set up an FTP/TFTP or HTTP/HTTPS server.

Create an upload directory where `admin` has write permission:

```
~$ ssh admin@infix.local%eth0 "sudo mkdir /var/tmp/upload; sudo chown admin /var/tmp/upload"
admin@infix.local%eth0's password: 
```

Copy the file with secure copy, first we show the nasty IPv6 version of
the command:

```
~$ scp infix-aarch64-24.06.0.pkg admin@\[fe80::ff:fe00:0%eth0\]:/var/tmp/upload/
admin@fe80::ff:fe00:0%eth0's password: 
infix-aarch64-24.06.0.pkg                                    100%  296   601.4KB/s   00:00 
```

And the upgrade command itself:

```
~$ ssh admin@infix.local%eth0 rauc install /var/tmp/upload/infix-aarch64-24.06.0.pkg
admin@infix.local%eth0's password: *****
.
.
.
```

Remember to remove the file from the upload directory when you are done,
this can be done before or after the reboot to activate the upgrade.  If
you want to upgrade both "slots", then you can of course keep the file
until you are done (provided the upload directory was created on
persistent storage).

```
~$ ssh admin@infix.local%eth0 rm /var/tmp/upload/infix-aarch64-24.06.0.pkg
admin@infix.local%eth0's password: *****
~$ 
```

### Controlling LEDs for Production Tests

As part of production testing, LED verification is often expected to
be performed. Infix uses standard [Linux support for LED
management][6], where LEDs appear in the file system under
/sys/class/leds and can be controlled using *echo* command. `sudo`
privileges are required.

When interacting with LEDs this way, first disable the Infix *iitod*
daemon to avoid conflicting LED control.

```
~$ ssh admin@example.local 'initctl stop iitod'
```

Then run the test, e.g., visually control that a red LED labeled
'LAN' is working.

```
~$ ssh admin@example.local 'echo none | sudo tee /sys/class/leds/red\:lan/trigger'
~$ ssh admin@example.local 'echo 1    | sudo tee /sys/class/leds/red\:lan/brightness'
```

To turn off the same LED, run the following commands.

```
~$ ssh admin@example.local 'echo none | sudo tee /sys/class/leds/red\:lan/trigger'
~$ ssh admin@example.local 'echo 0    | sudo tee /sys/class/leds/red\:lan/brightness'
```
When done with LED testing, enable Infix *iitod* daemon again.

```
~$ ssh admin@example.local 'initctl start iitod'
```

### Reading Power Feed Status for Production Tests

As part of production tests, verification of Power Feed sensors is
often expected to be performed. Infix uses standard [Linux support for
Power management][7], where power sources appear in the file system
under /sys/class/power_supply. The following example reads status of
two power supplies named *pwr1* and *pwr2*.

```
~$ ssh admin@example 'cat /sys/class/power_supply/pwr1/online' 
1
~$ ssh admin@example 'cat /sys/class/power_supply/pwr2/online' 
0
~$ 
```
Here, only *pwr1* happened to have power. 


## Examples using SSH and sysrepocfg


[sysrepocfg][4] can be used to interact with the YANG models when logged
in to infix. Thus, *set config*, *read config*, *read status* and
*RPC* can be conducted using sysrepocfg for supported YANG models. 
It is possible to make configuration changes by operating on the
*startup* database.

See [sysrepocfg][4] for information. Examples below will utilize 



- `sysrepocfg -I FILE -fjson -d DATABASE` to import/write a JSON
  formatted configuration file to the specified database.
- `sysrepocfg -E FILE -fjson -d DATABASE` to edit/merge JSON formatted
  configuration in FILE with the specified database.
-  `sysrepocfg -R FILE -fjson` to execute remote procedure call (RPC) defined in
   FILE (JSON formatted). 
- `sysrepocfg -X -fjson -d DATABASE -x xpath` to read configuration or
  status from specified database.
  
For importing (-I) and editing (-E), `-d running` is typically used in
examples below. Specify `-d startup` to apply changes to startup
configuration. Exporting (-X) could operate on configuration (e.g.,
`-d running`) or status (`-d operational`).

Some commands require a file as input. In examples below we assume 
it been transferred to Infix in advance, e.g. using `scp` as shown below.

```
~$ cat file.json 
{
   "ietf-factory-default:factory-reset": {
   }
}
~$ scp file.json admin@example.local:/tmp/file.json
~$
```

### Factory Reset Using sysrepocfg


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
See [Factory Reset](#factory-reset) for another (simpler) alternative.

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


### System Reboot Using sysrepocfg


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
See [System Reboot](#system-reboot) for another (simpler) alternative.



### Set Date and Time Using sysrepocfg


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
See [Set Date and Time](#set-date-and-time) for another (simpler) alternative.

### Remote Control of Ethernet Ports Using sysrepocfg


Reading administrative status of interface *e0* of running configuration.

```
~$ ssh admin@example.local 'sysrepocfg -X -fjson -d running -e report-all -x \"/ietf-interfaces:interfaces/interface[name='e0']/enabled\"'
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
> Note: Without `-e report-all` argument the line `"enabled: true`
> would not be shown as `true` is default.

```
~$ ssh admin@example.local "sysrepocfg -X -fjson -d running -x \"/ietf-interfaces:interfaces/interface[name='e0']/enabled\""
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

### Enable/Disable DHCPv4 client


Enabling DHCPv4 client on interface *e0*, with current default options.

```
~$ cat /tmp/file.json 
{
  "infix-dhcp-client:dhcp-client": {
    "enabled": true,
    "client-if": [
      {
        "if-name": "e0"
      }
    ]
  }
}
~$ scp file.json admin@example.local:/tmp/file.json
~$ ssh admin@example.local 'sysrepocfg -E /tmp/file.json -fjson -d running' 
~$
```

Disabling DHCPv4 client. 

```
~$ cat /tmp/file.json 
{
  "infix-dhcp-client:dhcp-client": {
    "enabled": false
  }
}
~$ scp file.json admin@example.local:/tmp/file.json
~$ ssh admin@example.local 'sysrepocfg -E /tmp/file.json -fjson -d running' 
~$
```

Configuration for client interface *e0* remains, but does not apply as
DHCPv4 is disabled. 

```
admin@example:~$ sysrepocfg -X -fjson -d running -x "/infix-dhcp-client:dhcp-client" 
{
  "infix-dhcp-client:dhcp-client": {
    "enabled": false,
    "client-if": [
      {
        "if-name": "e0"
      }
    ]
  }
}
admin@example:~$ 
```

To fully remove the DHCPv4 client configuration or a specific
*client-if* with sysrepocfg, one would need to read out the full
configuration, remove relevant parts and read back.


### Enable/Disable IPv6

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

### Change a Binary Setting

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


### <a id="backup"></a> Backup Configuration Using sysrepocfg And scp 

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

A final example is to only use `scp`. This is simpler, but only works to backup the
startup configuration (not running).

```
~$ scp admin@example.local:/cfg/startup-config.cfg backup.json
~$
```

### <a id="restore"></a> Restore Configuration Using sysrepocfg and ssh/scp 


To restore a backup configuration to startup, the simplest way is to
use `scp` and reboot as shown below

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

> Note: admin login credentials (hash) are stored as part of the
> configuration file. When replacing a switch and applying the backed 
> up configuration from the former switch, the password on the
> replacement unit will also change.

### Copy Running to Startup Using sysrepocfg

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

### Read Out Hardware Information Using sysrepocfg

Infix supports IETF Hardware YANG with augments for ONIE formatted
production data stored in EEPROMs, if available. See Infix [VPD
documentation][5], as well as *ietf-hardware* and *infix-hardware* YANG
models for details.


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

## Examples using RESTCONF

### Factory Reset

```
~$ curl -kX POST -H "Content-Type: application/yang-data+json" https://example.local/restconf/operations/ietf-factory-default:factory-reset -u admin:admin
curl: (56) OpenSSL SSL_read: error:0A000126:SSL routines::unexpected eof while reading, errno 0
```

### System Reboot

```
~$ curl -kX POST -H "Content-Type: application/yang-data+json" https://example.local/restconf/operations/ietf-system:system-restart -u admin:admin
```

### Set Date and Time

Here's an example of an RPC that takes input/argument:

```
~$ curl -kX POST -H "Content-Type: application/yang-data+json" https://example.local/restconf/operations/ietf-system:set-current-datetime -u admin:admin -d '{"ietf-system:input": {"current-datetime": "2024-04-17T13:48:02-01:00"}}'
```

You can verify that the changes took by a remote SSH command:

```
~$ ssh admin@example.local 'date'
Wed Apr 17 14:48:12 UTC 2024
~$
```


## Miscellaneous

### <a id="port-test-intro"></a> Port Configuration Example for Production Tests 

As part of production tests, verification Ethernet ports are expected
to be performed. A common way is to connect a test PC to two ports and
send a *ping* traversing all ports.  This can be achieved by using
VLANs on the switch as described in this section. The resulting
configuration file can be applied to the running configuration of the
produced unit, e.g, use config file restore as described
[above](#restore).

In this example we assume a 10 port switch, with ports e1-e10. 

The following VLAN configuration and cable connections will be used:

| VLAN & Ports      | Connect   |
|:------------------|:----------|
| VLAN 10: e1 & e2  | e2 <=> e3 |
| VLAN 20: e3 & e4  | e4 <=> e5 |
| VLAN 30: e5 & e6  | e6 <=> e7 |
| VLAN 40: e7 & e8  | e8 <=> e9 |
| VLAN 50: e9 & e10 |           |

The test PC is connected to e1 and e10 via different interfaces
(alternatively, two different PCs are used).

> Configuration here is done via console. When configuring remotely
> over SSH, remember to keep one IP address (the one used for the SSH
> connection)! I.e., set a static IP address first, then perform the
> VLAN configuration step."

#### Configuration at Start

Starting out, we assume a configuration where all ports are network
interfaces (possibly with IPv6 enabled).

``` shell
admin@example:/> show interfaces
lo              ethernet   UP          00:00:00:00:00:00                        
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
e1              ethernet   LOWER-DOWN  00:53:00:06:11:01                        
e2              ethernet   LOWER-DOWN  00:53:00:06:11:02                        
e3              ethernet   LOWER-DOWN  00:53:00:06:11:03                        
e4              ethernet   LOWER-DOWN  00:53:00:06:11:04                        
e5              ethernet   LOWER-DOWN  00:53:00:06:11:05                        
e6              ethernet   LOWER-DOWN  00:53:00:06:11:06                        
e7              ethernet   LOWER-DOWN  00:53:00:06:11:07                        
e8              ethernet   LOWER-DOWN  00:53:00:06:11:08                        
e9              ethernet   LOWER-DOWN  00:53:00:06:11:09                        
e10             ethernet   UP          00:53:00:06:11:0a                        
                ipv6                   fe80::0053:00ff:fe06:110a/64 (link-layer)
admin@example:/> 
```

#### Creating Bridge and Adding Ports

The example below uses Infix documentation on [creating bridges][8]. 

``` shell
admin@example:/> configure
admin@example:/config/> edit interface br0
admin@example:/config/interface/br0/> end
admin@example:/config/> set interface e1 bridge-port bridge br0
admin@example:/config/> set interface e2 bridge-port bridge br0
admin@example:/config/> set interface e3 bridge-port bridge br0
admin@example:/config/> set interface e4 bridge-port bridge br0
admin@example:/config/> set interface e5 bridge-port bridge br0
admin@example:/config/> set interface e6 bridge-port bridge br0
admin@example:/config/> set interface e7 bridge-port bridge br0
admin@example:/config/> set interface e8 bridge-port bridge br0
admin@example:/config/> set interface e9 bridge-port bridge br0
admin@example:/config/> set interface e10 bridge-port bridge br0
admin@example:/config/> 
```

The interface status can be viewed using "show interface" after
leaving configuration context. If configuration via SSH, first assign
an IP address to br0 before *leaving* configuration context, e.g.,
`set interface br0 ipv6 enabled` to get auto-configured IPv6
address. Or skip 'leave' and stay in configuration context until done
with all sections, including the one on [Add IP on
Switch](#ip-on-switch).

``` shell
admin@example:/config/> leave
admin@example:/> 
admin@example:/> show interfaces
INTERFACE       PROTOCOL   STATE       DATA                                     
br0             bridge                 
│               ethernet   UP          00:53:00:06:11:01                        
├ e1            bridge     LOWER-DOWN  
├ e2            bridge     LOWER-DOWN  
├ e3            bridge     LOWER-DOWN  
├ e4            bridge     LOWER-DOWN  
├ e5            bridge     LOWER-DOWN  
├ e6            bridge     LOWER-DOWN  
├ e7            bridge     LOWER-DOWN  
├ e8            bridge     LOWER-DOWN  
├ e9            bridge     LOWER-DOWN  
└ e10           bridge     FORWARDING  
lo              ethernet   UP          00:00:00:00:00:00                        
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
admin@example:/> 
```

#### Assign VLANs to Ports

Then configure VLANs as outlined [above](#port-test-intro):
default VID for ingress (PVID), which is done per port, and egress
mode (untagged), which is done at the bridge level. See Infix
[documentation for VLAN bridges][9] for more information.


``` shell
admin@example:/> 
admin@example:/> configure
admin@example:/config/> set interface e1 bridge-port pvid 10
admin@example:/config/> set interface e2 bridge-port pvid 10
admin@example:/config/> set interface e3 bridge-port pvid 20
admin@example:/config/> set interface e4 bridge-port pvid 20
admin@example:/config/> set interface e5 bridge-port pvid 30
admin@example:/config/> set interface e6 bridge-port pvid 30
admin@example:/config/> set interface e7 bridge-port pvid 40
admin@example:/config/> set interface e8 bridge-port pvid 40
admin@example:/config/> set interface e9 bridge-port pvid 50
admin@example:/config/> set interface e10 bridge-port pvid 50
admin@example:/config/> edit interface br0
admin@example:/config/interface/br0/> edit bridge vlans
admin@example:/config/interface/br0/bridge/vlans/> set vlan 10 untagged e1
admin@example:/config/interface/br0/bridge/vlans/> set vlan 10 untagged e2
admin@example:/config/interface/br0/bridge/vlans/> set vlan 20 untagged e3
admin@example:/config/interface/br0/bridge/vlans/> set vlan 20 untagged e4
admin@example:/config/interface/br0/bridge/vlans/> set vlan 30 untagged e5
admin@example:/config/interface/br0/bridge/vlans/> set vlan 30 untagged e6
admin@example:/config/interface/br0/bridge/vlans/> set vlan 40 untagged e7
admin@example:/config/interface/br0/bridge/vlans/> set vlan 40 untagged e8
admin@example:/config/interface/br0/bridge/vlans/> set vlan 50 untagged e9
admin@example:/config/interface/br0/bridge/vlans/> set vlan 50 untagged e10
admin@example:/config/interface/br0/bridge/vlans/> leave
admin@example:/> 
```

Interface status would now should something like the following

``` shell
admin@example:/> show interfaces 
INTERFACE       PROTOCOL   STATE       DATA                                     
br0             bridge                 
│               ethernet   UP          00:53:00:06:11:01                        
├ e1            bridge     LOWER-DOWN  vlan:10u pvid:10                         
├ e2            bridge     LOWER-DOWN  vlan:10u pvid:10                         
├ e3            bridge     LOWER-DOWN  vlan:20u pvid:20                         
├ e4            bridge     LOWER-DOWN  vlan:20u pvid:20                         
├ e5            bridge     LOWER-DOWN  vlan:30u pvid:30                         
├ e6            bridge     LOWER-DOWN  vlan:30u pvid:30                         
├ e7            bridge     LOWER-DOWN  vlan:40u pvid:40                         
├ e8            bridge     LOWER-DOWN  vlan:40u pvid:40                         
├ e9            bridge     LOWER-DOWN  vlan:50u pvid:50                         
└ e10           bridge     FORWARDING  vlan:50u pvid:50                         
lo              ethernet   UP          00:00:00:00:00:00                        
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
admin@example:/> 
```

#### Connect Cables and Test

We can now connect the PC to e1 and e10, and the other ports are
patched according to plan [above](#port-test-intro). We should get
link up on all ports.

``` shell
admin@example:/> show interfaces 
INTERFACE       PROTOCOL   STATE       DATA                                     
br0             bridge                 
│               ethernet   UP          00:53:00:06:11:01                        
├ e1            bridge     FORWARDING  vlan:10u pvid:10                         
├ e2            bridge     FORWARDING  vlan:10u pvid:10                         
├ e3            bridge     FORWARDING  vlan:20u pvid:20                         
├ e4            bridge     FORWARDING  vlan:20u pvid:20                         
├ e5            bridge     FORWARDING  vlan:30u pvid:30                         
├ e6            bridge     FORWARDING  vlan:30u pvid:30                         
├ e7            bridge     FORWARDING  vlan:40u pvid:40                         
├ e8            bridge     FORWARDING  vlan:40u pvid:40                         
├ e9            bridge     FORWARDING  vlan:50u pvid:50                         
└ e10           bridge     FORWARDING  vlan:50u pvid:50                         
lo              ethernet   UP          00:00:00:00:00:00                        
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
admin@example:/> 
```

Here we use IPv6 ping all hosts (ff02::1) on PC interface eth1 to
check reachability to the other interface of the PC. 

> A recommendation is to use network name spaces on PC to ensure
> traffic really goes out to switch, instead of being looped
> internally. Or use two PCs. 


``` shell
~ $ ping -L ff02::1%eth1
PING ff02::1%eth1(ff02::1%eth1) 56 data bytes
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=1 ttl=64 time=0.496 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=2 ttl=64 time=0.514 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=3 ttl=64 time=0.473 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=4 ttl=64 time=0.736 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=5 ttl=64 time=0.563 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=6 ttl=64 time=0.507 ms
^C
--- ff02::1%eth1 ping statistics ---
6 packets transmitted, 6 received, 0% packet loss, time 5108ms
rtt min/avg/max/mdev = 0.473/0.548/0.736/0.088 ms
~ $
```

We can verify that traffic goes through the switch by disconnecting
one of the patch cables, e.g., between e4 and e5

``` shell
~ $ ping -L ff02::1%eth1
PING ff02::1%eth1(ff02::1%eth1) 56 data bytes
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=1 ttl=64 time=0.510 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=2 ttl=64 time=0.448 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=3 ttl=64 time=0.583 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=4 ttl=64 time=0.515 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=5 ttl=64 time=0.521 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=6 ttl=64 time=0.495 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=7 ttl=64 time=0.743 ms
... Disconnecting patch cable, thus losing packets
... and reconnecting again. Connectivity resumes.
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=16 ttl=64 time=0.961 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=17 ttl=64 time=0.513 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=18 ttl=64 time=0.794 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=19 ttl=64 time=0.755 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=20 ttl=64 time=0.779 ms
^C
--- ff02::1%eth1 ping statistics ---
20 packets transmitted, 12 received, 40% packet loss, time 19432ms
rtt min/avg/max/mdev = 0.448/0.634/0.961/0.156 ms
~ $ 
```

#### <a id="ip-on-switch"></a> Add IP Address on Switch 

The configuration so far does not provide a means to connect to the
switch management via SSH or NETCONF, as the switch has no IP
address. The example below shows how to add the switch to VLAN 10 (as
used for ports e1 and e2) and enables IPv6.


``` shell
admin@example:/config/> edit interface vlan10
admin@example:/config/interface/vlan10/> set vlan lower-layer-if br0
admin@example:/config/interface/vlan10/> set ipv6 enabled 
admin@example:/config/interface/vlan10/> show
type vlan;
ipv6 {
  enabled true;
}
vlan {
  tag-type c-vlan;
  id 10;
  lower-layer-if br0;
}
admin@example:/config/interface/vlan10/> 
admin@example:/config/interface/vlan10/> end
admin@example:/config/> edit interface br0 bridge vlans 
admin@example:/config/interface/br0/bridge/vlans/> set vlan 10 tagged br0
admin@example:/config/interface/br0/bridge/vlans/> leave
admin@example:/> 
```

Interface *vlan10* with an auto-configured IPv6 address should appear.

``` shell
admin@example:/> show interfaces
INTERFACE       PROTOCOL   STATE       DATA                                     
br0             bridge                 vlan:10t                                 
│               ethernet   UP          00:53:00:06:11:01                        
├ e1            bridge     FORWARDING  vlan:10u pvid:10                         
├ e2            bridge     FORWARDING  vlan:10u pvid:10                         
├ e3            bridge     FORWARDING  vlan:20u pvid:20                         
├ e4            bridge     FORWARDING  vlan:20u pvid:20                         
├ e5            bridge     FORWARDING  vlan:30u pvid:30                         
├ e6            bridge     FORWARDING  vlan:30u pvid:30                         
├ e7            bridge     FORWARDING  vlan:40u pvid:40                         
├ e8            bridge     FORWARDING  vlan:40u pvid:40                         
├ e9            bridge     FORWARDING  vlan:50u pvid:50                         
└ e10           bridge     FORWARDING  vlan:50u pvid:50                         
lo              ethernet   UP          00:00:00:00:00:00                        
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
vlan10          ethernet   UP          00:53:00:06:11:01                        
│               ipv6                   fe80::0053:00ff:fe06:1101/64 (link-layer)
└ br0           ethernet   UP          00:53:00:06:11:01                        
admin@example:/> 
```

When pinging "IPv6 all hosts" from the PC, there should be two
responses for every ping, one from the switch and one from the PC
attached to e10.

``` shell
~ $ ping -L ff02::1%eth1
PING ff02::1%eth1(ff02::1%eth1) 56 data bytes
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=1 ttl=64 time=0.508 ms
64 bytes from fe80::0053:00ff:fe06:1101%eth1: icmp_seq=1 ttl=64 time=0.968 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=2 ttl=64 time=0.866 ms
64 bytes from fe80::0053:00ff:fe06:1101%eth1: icmp_seq=2 ttl=64 time=0.867 ms
64 bytes from fe80::0053:00ff:fe06:1101%eth1: icmp_seq=3 ttl=64 time=0.467 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=3 ttl=64 time=0.469 ms
64 bytes from fe80::488a:a35f:9d41:ac9c%eth1: icmp_seq=4 ttl=64 time=0.452 ms
64 bytes from fe80::0053:00ff:fe06:1101%eth1: icmp_seq=4 ttl=64 time=0.453 ms
^C
--- ff02::1%eth1 ping statistics ---
4 packets transmitted, 4 received, +4 duplicates, 0% packet loss, time 3031ms
rtt min/avg/max/mdev = 0.452/0.631/0.968/0.211 ms
~ $ 
```

It should now be possible to access the switch from the PC via SSH (or NETCONF).

``` shell
~ $ ssh admin@fe80::0053:00ff:fe06:1101%eth1
admin@fe80::0053:00ff:fe06:1101%eth1's password: 
.-------.
|  . .  | Infix -- a Network Operating System
|-. v .-| https://kernelkit.org
'-'---'-'

Run the command 'cli' for interactive OAM

admin@example:~$ exit
~ $
```

See previous sections on [backup](#backup) and [restore](#restore) of
the created configuration.




[1]: discovery.md
[2]: https://rauc.io/
[3]: boot.md#system-upgrade
[4]: https://netopeer.liberouter.org/doc/sysrepo/libyang1/html/sysrepocfg.html
[5]: vpd.md
[6]: https://docs.kernel.org/leds/leds-class.html
[7]: https://docs.kernel.org/power/power_supply_class.html
[8]: networking.md#bridging
[9]: networking.md#vlan-filtering-bridge
