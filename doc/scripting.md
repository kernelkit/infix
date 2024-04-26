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
|-. v .-| https://kernelkit.github.io
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



```
admin@example:~$ sysrepocfg -X -fjson -d running -x "/ietf-interfaces:interfaces/interface[name='e1']/enabled"
{
  "ietf-interfaces:interfaces": {
    "interface": [
      {
        "name": "e1",
        "enabled": false
      }
    ]
  }
}
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

## Examples using SSH and sysrepocfg


[sysrepocfg][4] can be used to interact with the YANG models when logged
in to infix. Thus, *set config*, *read config*, *read status* and
*RPC* can be conducted using sysrepocfg for supported YANG models. 
It is possible to make configuration changes by operating on the
*startup* database.

See [sysrepocfg][4] for information. Examples below will utilize 



- `sysrepocfg -I FILE -fjson -d DATABASE` to import/write a JSON
  formatted configuration file to the specificed database.
- `sysrepocfg -E FILE -fjson -d DATABASE` to edit/merge JSON formatted
  configuration in FILE with the specificed database.
-  `sysrepocfg -R FILE -fjson` to execute remote procdure call (RPC) defined in
   FILE (JSON formatted). 
- `sysrepocfg -X -fjson -d DATABASE -x xpath` to read configuration or
  status from specified database.
  
For importing (-I) and editing (-E), `-d running` is typically used in
examples below. Specify `-d startup` to apply changes to startup
configuration. Exporting (-X) could operate on configuration (e.g.,
`-d running`) or status (`-d operational`).

Some commands require a file as input. In examples below we assume 
it been transfered to Infix in advance, e.g. using `scp` as shown below.

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

If you only wish to copy factory config to running config the
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

### Backup Configuration Using sysrepocfg And scp

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

### Restore Configuration Using sysrepocfg and ssh/scp


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

[1]: discovery.md
[2]: https://rauc.io/
[3]: boot.md#system-upgrade
[4]: https://netopeer.liberouter.org/doc/sysrepo/libyang1/html/sysrepocfg.html
