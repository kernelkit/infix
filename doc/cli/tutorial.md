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


Admin Exec
----------

The top-level context after logging in and starting the CLI is the
admin-exec or "main" context.  It is used for querying system status,
managing configuration files/profiles and doing advanced debugging.

Available commands can be seen by pressing `?` at the prompt:

```
admin@infix-12-34-56:exec>
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
admin@infix-12-34-56:configure>
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
[edit]
```

The `edit` command lets you change to a sub-configure context, e.g.:

```
admin@infix-12-34-56:configure> edit interfaces interface eth0
[edit interfaces interface eth0]
```

Notice the `[edit ...]` displayed, it shows your current location.
Use `up` to go back to the previous context.

```
admin@infix-12-34-56:configure> up
[edit]
```

### Set IP Address on an Interface

```
admin@infix-12-34-56:configure>
[edit]
admin@infix-12-34-56:configure> edit interfaces interface eth0
[edit interfaces interface eth0]
admin@infix-12-34-56:configure> set ipv4 address 192.168.2.200 prefix-length 24
[edit interfaces interface eth0]
```

From anywhere in configure context you can see the changes you have
made by typing `diff`:

```
admin@infix-12-34-56:configure> diff
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

### Creating a VETH Pair

The following example creates a `veth0a <--> veth0b` virtual Ethernet
pair which is useful for connecting, e.g., a container to the physical
world.  Here we also add an IPv4 address to one end of the pair.

```
admin@infix-12-34-56:configure>
[edit]
admin@infix-12-34-56:configure> edit interfaces interface veth0a
[edit interfaces interface veth0a]
admin@infix-12-34-56:configure> set veth peer veth0b
[edit interfaces interface veth0a]
admin@infix-12-34-56:configure> set ipv4 address 192.168.0.1 prefix-length 24
[edit interfaces interface veth0a]
admin@infix-12-34-56:configure> up
[edit interfaces]
admin@infix-12-34-56:configure> diff
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
[edit interfaces]
admin@infix-12-34-56:configure> leave
```

See the bridging example below for more.

> **Note:** in the CLI you do not have to create the `veth0b` interface.
> The system _infers_ this for you.  When setting up a VETH pair using
> NETCONF, however, you must include the `veth0b` interface.


### Creating a Bridge

Here we create a non-VLAN filtering bridge that forwards any, normally
link-local, LLDP traffic.

```
admin@infix-12-34-56:exec> configure
[edit]
admin@infix-12-34-56:configure> edit interfaces interface br0
[edit interfaces interface br0]
admin@infix-12-34-56:configure> set bridge ieee-group-forward lldp
[edit interfaces interface br0]
admin@infix-12-34-56:configure> up
[edit interfaces]
admin@infix-12-34-56:configure> set interface eth0 bridge-port bridge br0
[edit interfaces]
admin@infix-12-34-56:configure> set interface veth0b bridge-port bridge br0
[edit interfaces]
admin@infix-12-34-56:configure> diff
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
[edit interfaces]
admin@infix-12-34-56:configure> leave
```

Both a physical port `eth0` and a virtual port `veth0b` (bridge side of
the VETH pair from the previous example) are now bridged.  Any traffic
ingressing one port will egress the other.  Only reserved IEEE multicast
is filtered, except LLDP frames as shown above.


### Saving Changes

Apply the changes (from candidate to running):

```
admin@infix-12-34-56:configure> leave
admin@infix-12-34-56:exec> show running-config
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
admin@infix-12-34-56:exec> copy running-config startup-config
```

The `startup-config` can also be inspected with the `show` command to
verify the changes are saved.

> **Note:** most (all) commands need to be spelled out, no short forms
> are allowed at the moment.  Use the `TAB` key to make this easier.


[1]: https://src.libcode.org/pkun/klish
