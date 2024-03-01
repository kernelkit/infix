# Introduction

The command line interface (CLI, see-ell-i) is the traditional way of
interacting with single network equipment like switches and routers.
Today more advanced graphical NETCONF-based tools are available that
allows for managing entire fleets of installed equipment.

Nevertheless, when it comes to initial deployment and debugging, it
is very useful to know how to navigate and use the CLI.  This very
short guide intends to help you with that.

## Key Concepts

The two modes in the CLI are the admin-exec and the configure context.
When logging in to the system, be it from console or SSH, you land in
admin-exec.  Here you can inspect the system status and do operations
to debug networking issues, e.g. ping.  You can also enter configure
context by typing: `configure`

The system has several datastores (or files):

 - `factory-config` consists of a set of default configurations, some
   static and others generated per-device, e.g., a unique hostname and
   number of ports/interfaces.   This file is generated at boot, if it
   does not exist, i.e., only on first boot or after factory reset.
 - `startup-config` is created from `factory-config` at boot if it does
   not exist.  It is loaded as the system configuration on each boot.
 - `running-config` is what is actively running on the system.  If no
   changes have been made since the system booted, it is the same as
   `startup-config`.
 - `candidate-config` is created from `running-config` when entering the
   configure context.  Any changes made here can be discarded (`abort`,
   `rollback`) or committed (`commit`, `leave`) to `running-config`.

To save configuration changes made to the `running-config` so the system
will use them consecutive reboots, use the `copy` command:

    admin@host-12-34-56:/> copy running-config startup-config

In *configure context* the following commands are available:

| **Command**       | **Description**                                        |
|-------------------|--------------------------------------------------------|
| `set foo bar val` | Set `bar` leaf node in `foo` subcontext to `val`       |
| `no foo bar`      | Clear/delete configuration made to `bar` in `foo`      |
| `edit foo baz`    | Enter `baz` sub-sub-context in `foo` subcontext        |
| `change password` | Start password dialogue to change a user's password    |
| `text-editor foo` | Open a text editor to edit binary setting `foo`        |
| `abort`           | Abort changes in configuration, return to admin-exec   |
| `exit`            | Exit one level sub-context, or abort from top-level    |
| `leave`           | Save changes to `running-config`, return to admin-exec |
| `show [foo]`      | Show configured values (optionally in subcontext)      |
| `diff [foo]`      | Show uncommitted changes in candidate                  |
| `do command`      | Call admin-exec command: `do show log`                 |
| `commit`          |                                                        |

### Example Session

> Remember to use the `TAB` and `?` keys to speed up your navigation.
> See `help keybindings` for more tips!

In this example we enter configure context to add an IPv4 address to
interface `eth0`, then we apply the changes using the `leave` command.

We inspect the system status to ensure the change took effect.  Then we
save the changes for the next reboot.

```
admin@host-12-34-56:/> configure
admin@host-12-34-56:/config/> edit interface eth0
admin@host-12-34-56:/config/interface/eth0/> set ipv4 <TAB>
      address     autoconf bind-ni-name      enabled
	  forwarding  mtu      neighbor
admin@host-12-34-56:/config/interface/eth0/> set ipv4 address 192.168.2.200 prefix-length 24
admin@host-12-34-56:/config/interface/eth0/> show
type ethernetCsmacd;
ipv4 address 192.168.2.200 prefix-length 24;
ipv6 enabled true;
admin@host-12-34-56:/config/interface/eth0/> diff
interfaces {
  interface eth0 {
+    ipv4 {
+      address 192.168.2.200 {
+        prefix-length 24;
+      }
+    }
  }
}
admin@host-12-34-56:/config/interface/eth0/> leave
admin@host-12-34-56:/> show interfaces brief
lo               UNKNOWN        00:00:00:00:00:00 <LOOPBACK,UP,LOWER_UP>
eth0             UP             52:54:00:12:34:56 <BROADCAST,MULTICAST,UP,LOWER_UP>
admin@host-12-34-56:/> show ip brief
lo               UNKNOWN        127.0.0.1/8 ::1/128
eth0             UP             192.168.2.200/24 fe80::5054:ff:fe12:3456/64
admin@host-12-34-56:/> copy running-config startup-config
```

One of the ideas behind a separate running and startup configuration is
to be able to verify a configuration change.  In case of an inadvertent
change that, e.g., breaks networking, it is trivial to revert back by:

```
admin@host-12-34-56:/> copy startup-config running-config
```

Or restarting the device.

