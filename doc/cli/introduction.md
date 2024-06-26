# Introduction

The command line interface (CLI, see-ell-aye) implements a CISCO-like,
or Juniper Networks JunOS-like, CLI.  It is the traditional way of
interacting with single network equipment like switches and routers.
Today more advanced graphical NETCONF-based tools are available that
allows for managing entire fleets of installed equipment.

Nevertheless, when it comes to initial deployment and debugging, it
is very useful to know how to navigate and use the CLI.  This very
short guide intends to help you with that.


## About

New users usually get the CLI as the default "shell" when logging in,
but the default `admin` user logs in to `bash`.  To access the CLI,
type:

```
admin@host-12-34-56:~$ cli

See the 'help' command for an introduction to the system

admin@host-12-34-56:/>
```

The prompt (beginning of the line) changes slightly.  Key commands
available in any context are:

```
admin@host-12-34-56:/> help                   # Try: Tab or ?
...
admin@host-12-34-56:/> show                   # Try: Tab or ?
admin@host-12-34-56:/>                        # Try: Tab or ?
```

> **Tip:** Even on an empty command line you can tap the Tab or ? keys.
> See `help keybindings` for more tips!


## Key Concepts

The two modes in the CLI are the admin-exec and the configure context.

The top-level context after logging in and starting the CLI is the
admin-exec or "main" context.  It is used for querying system status,
managing configuration files/profiles and doing advanced debugging.

Available commands can be seen by pressing `?` at the prompt:

```
admin@host:/>
  configure      Create new candidate-config based on running-config
  copy           Copy file or configuration, e.g., copy running-config startup-config
  dir            List available configuration files
  exit           Exit from CLI (log out)
  factory-reset  Restore the system to factory default state
  follow         Monitor a log file, use Ctrl-C to abort
  help           Help system (try also the '?' key)
  logout         Alias to exit
  netcalc        IP subnet calculator, with subnetting
  password       Password tools
  ping           Ping a network host or multicast group
  poweroff       Poweroff system (system policy may yield reboot)
  reboot         Reboot system
  remove         Remove a configuration file
  set            Set operations, e.g., current date/time
  show           Show system status and configuration files
  tcpdump        Capture network traffic
  upgrade        Install a software update bundle from remote or local file
```

The system has three *main datastores* (or files): *factory*, *startup*,
and *running* that can be managed and inspected using the `copy`,
`show`, and `configure` commands.  The traditional names used in the CLI
for these are listed below:

 - `factory-config` the default configuration from factory for the
   device, i.e., what the system returns to after a `factory-reset`
 - `startup-config` created from `factory-config` at first boot after
   factory reset.  Loaded as the system configuration on each boot
 - `running-config` what is actively running on the system.  If no
   changes have been made since boot, it is the same as `startup-config`
 - `candidate-config` is created from `running-config` when entering the
   configure context.  Any changes made here can be discarded (`abort`,
   `rollback`) or committed (`commit`, `leave`) to `running-config`

Edit the *running* configuration using the `configure` command.  This
copies *running* to *candidate*, a temporary datastore, where changes
are made:

```
admin@host-12-34-56:/> configure
admin@host-12-34-56:/config/> ...             # Try: Tab or ?
admin@host-12-34-56:/config/> leave
```

The `leave` command activates the changes by issuing a transaction to,
essentially, copy the *candidate* back to *running*.  Depending on the
changes made, this can take a few seconds.  If the changes are invalid,
i.e., not correct according to the underlying YANG models, a warning is
shown and the session remains in configure context.  Use the `abort`
command to cancel your changes, or investigate further with the `diff`
command (see more below).

To save configuration changes made to the `running-config` to persistent
store, so the system will use them for consecutive reboots, use the
`copy` command:

```
admin@host-12-34-56:/> copy running-config startup-config
```

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

