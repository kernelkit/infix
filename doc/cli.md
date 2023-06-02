CLI User Guide
==============

The Infix CLI is built on the [klish project][1], which is a framework
for implementing a CISCO, or Juniper Networs JunOS-like CLI on a UNIX
systems.

Currently, when the `admin` user logs in the default shell is Bash.  To
access the CLI, type:

    cli

Key commands available in any context are:

    help
	show

For each command it is also possible to press the `?` key and `TAB` to
get more help and suggestions for completion.


Admin Exec
----------

The top-level context after logging in and starting the CLI is the
admin-exec or "main" context.  It is used for querying system status,
managing configuration files/profiles and doing advanced debugging.

Available commands can be seen by pressing `?` at the prompt:

```
root@infix-12-34-56:exec> 
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
root@infix-12-34-56:configure> 
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
root@infix-12-34-56:configure> edit interfaces interface eth0 
[edit interfaces interface eth0]
```

Notice the `[edit ...]` displayed, it shows your current location.
Use `up` to go back to the previous context.

```
root@infix-12-34-56:configure> up
[edit]
```

### Set IP Address on an Interface

```
root@infix-12-34-56:configure> 
[edit]
root@infix-12-34-56:configure> edit interfaces interface eth0 
[edit interfaces interface eth0]
root@infix-12-34-56:configure> set ipv4 address 192.168.2.200 prefix-length 24
[edit interfaces interface eth0]
```

From anywhere in configure context you can see the changes you have
made by typing `diff`:

```
root@infix-12-34-56:configure> diff
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

Apply the changes (from candidate to running):

```
root@infix-12-34-56:configure> leave
root@infix-12-34-56:exec> show running-config 
interfaces {
  interface eth0 {
    type ethernetCsmacd;
    ipv4 {
      address 192.168.2.200 {
        prefix-length 24;
      }
    }
  }
```

Since we did not get any warnings we can save the running (RAM only)
configuration to startup, so the changes are made persistent across
reboots:

```
root@infix-12-34-56:exec> copy running-config startup-config 
```

> **Note:** most (all) commands need to be spelled out, no short forms
> are allowed at the moment.  Use the `TAB` key to make this easier.


[1]: https://src.libcode.org/pkun/klish
