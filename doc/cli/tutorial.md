User Guide
==========

The command line interface (CLI, see-el-i) is built on the open source
component [klish][1], which implements a CISCO like, or Juniper Networks
JunOS-like CLI on a UNIX system.

New users always get the CLI as the default "shell" when logging in, but
the default `admin` user logs in to the Bash.  To access the CLI, type:

    cli

Key commands available in any context are:

    help
    show

For each command it also possible to press the `?` key and `TAB` to get
more help and suggestions for completion.

> **Note:** for the sake of brevity, the hostname in the following
> examples has been shortened to `host`.  The default name is composed
> from a product specific string followed by the last three octets of
> the system base MAC address, e.g., `infix-12-34-56`.  An example of
> how to change the hostname is included below.


Admin Exec
----------

The top-level context after logging in and starting the CLI is the
admin-exec or "main" context.  It is used for querying system status,
managing configuration files/profiles and doing advanced debugging.

Available commands can be seen by pressing `?` at the prompt:

```
admin@host:/>
  configure  Create new candidate-config based on running-config
  copy       Copy
  exit       Exit
  logout     Alias for exit
  shell      Enter system shell
  show       Show
```

Configure Context
-----------------

Enter the configure context from admin-exec by typing `configure`
followed by Enter.  Available commands, press `?` at the prompt:

```
admin@host:/>
admin@host:/config/>
  abort     Abandon candidate
  check     Validate candidate
  commit    Commit current candidate to running-config
  delete    Delete configuration setting(s)
  diff      Summarize uncommitted changes
  do        Execute operational mode command
  edit      Descend to the specified configuration node
  exit      Ascend to the parent configuration node, or abort (from top)
  leave     Finalize candidate and apply to running-config
  no        Alias for delete
  rollback  Restore candidate to running-config
  set       Set configuration setting
  show      Show configuration
  top       Ascend to the configuration root
  up        Ascend to the parent configuration node
```

The `edit` command lets you change to a sub-configure context, e.g.:

```
admin@host:/config/> edit interfaces interface eth0
admin@host:/config/interfaces/interface/eth0/>
```

Use `up` to go up one level.

```
admin@host:/config/interfaces/interface/eth0/> up
admin@host:/config/interfaces/>
```

> **Note:** the tree structure in the configure context is automatically
> generated from the system's supported NETCONF YANG models, which may
> vary between products.  However, the `ietf-interfaces.yang` and
> `ietf-ip.yang` models, for instance, that provide basic networking
> support are common to all systems.


### Set IP Address on an Interface

```
admin@host:/config/> edit interfaces interface eth0
admin@host:/config/interfaces/interface/eth0/>
admin@host:/config/interfaces/interface/eth0/> set ipv4 address 192.168.2.200 prefix-length 24
```

From anywhere in configure context you can see the changes you have
made by typing `diff`:

```
admin@host:/config/interfaces/interface/eth0/> diff
interfaces {
  interface eth0 {
+    ipv4 {
+      address 192.168.2.200 {
+        prefix-length 24;
+      }
+    }
  }
}
```


### Saving Changes

Apply the changes (from candidate to `running-config`):

```
admin@host:/config/interfaces/> leave
admin@host:/> show running-config
...
interfaces {
  interface eth0 {
	type ethernetCsmacd;
	ipv4 {
	  address 192.168.2.200 {
		prefix-length 24;
	  }
	}
  }
...
```

Since we did not get any warnings we can save the running (RAM only)
configuration to startup, so the changes are made persistent across
reboots:

```
admin@host:/> copy running-config startup-config
```

The `startup-config` can also be inspected with the `show` command to
verify the changes are saved.

> **Note:** most (all) commands need to be spelled out, no short forms
> are allowed at the moment.  Use the `TAB` key to make this easier.


### Changing Hostname

Settings like hostname are located in the `ietf-system.yang` model.
Here is how it can be set.

```
admin@host:/config/> edit system
admin@host:/config/system/> set hostname example
admin@host:/config/system/> leave
admin@example:/> 
```

Notice how the hostname in the prompt does not change until the change
is committed.

> **Note:** critical services like syslog, mDNS, LLDP, and similar that
> advertise the hostname, are restarted when the hostname is changed.


### Changing Password

User management, including passwords, is also a part of `ietf-system`.

```
admin@host:/config/> edit system authentication
admin@host:/config/system/authentication/> do password encrypt
Password: ******
$1$oVHGR0AP$6Pad1Pm8jPwPsan5WHULS1
admin@host:/config/system/authentication/> set user admin password $1$oVHGR0AP$6Pad1Pm8jPwPsan5WHULS1
admin@host:/config/system/> leave
```

The call to `do password encrypt` brings up the helpful admin-exec
command to hash, and optionally salt, your password.  This encrypted
string is what goes into the system configuration.

> **Tip:** if you are having trouble thinking of a password, there is
> also `do password generate`, which generates random but readable
> strings using the UNIX command `pwgen`.


### Creating a VETH Pair

The following example creates a `veth0a <--> veth0b` virtual Ethernet
pair which is useful for connecting, e.g., a container to the physical
world.  Here we also add an IPv4 address to one end of the pair.

```
admin@host:/config/> edit interfaces interface veth0a
admin@host:/config/interfaces/interface/veth0a/> set veth peer veth0b
admin@host:/config/interfaces/interface/veth0a/> set ipv4 address 192.168.0.1 prefix-length 24
admin@host:/config/interfaces/interface/veth0a/> up
admin@host:/config/interfaces/> diff
interfaces {
+  interface veth0a {
+    type veth;
+    ipv4 {
+      address 192.168.0.1 {
+        prefix-length 24;
+      }
+    }
+    veth {
+      peer veth0b;
+    }
+  }
+  interface veth0b {
+    type veth;
+    veth {
+      peer veth0a;
+    }
+  }
}
admin@host:/config/interfaces/> leave
```

See the bridging example below for more.

> **Note:** in the CLI you do not have to create the `veth0b` interface.
> The system _infers_ this for you.  When setting up a VETH pair using
> NETCONF, however, you must include the `veth0b` interface.


### Creating a Bridge

Building on the previous example, we now create a non-VLAN filtering
bridge (`br0`) that forwards any, normally link-local, LLDP traffic
between both its bridge ports: `eth0` and `vet0b`.

```
admin@host:/> configure
admin@host:/config/> edit interfaces interface br0
admin@host:/config/interfaces/interface/br0/> set bridge ieee-group-forward lldp
admin@host:/config/interfaces/interface/br0/> up
admin@host:/config/interfaces/> set interface eth0 bridge-port bridge br0
admin@host:/config/interfaces/> set interface veth0b bridge-port bridge br0
admin@host:/config/interfaces/> diff
interfaces {
+  interface br0 {
+    type bridge;
+    bridge {
+      ieee-group-forward lldp;
+    }
+  }
  interface eth0 {
+    bridge-port {
+      bridge br0;
+    }
  }
+  interface veth0a {
+    type veth;
+    ipv4 {
+      address 192.168.0.1 {
+        prefix-length 24;
+      }
+    }
+    veth {
+      peer veth0b;
+    }
+  }
+  interface veth0b {
+    type veth;
+    veth {
+      peer veth0a;
+    }
+    bridge-port {
+      bridge br0;
+    }
+  }
}
```

Both a physical port `eth0` and a virtual port `veth0b` (bridge side of
the VETH pair from the previous example) are now bridged.  Any traffic
ingressing one port will egress the other.  Only reserved IEEE multicast
is filtered, except LLDP frames as shown above.

> **Note:** the bridge can be named anything, provided the interface
> name is not already taken.  However, for any name outside the pattern
> `br[0-9]+`, you have to set the interface type manually to `bridge`.



[1]: https://src.libcode.org/pkun/klish
