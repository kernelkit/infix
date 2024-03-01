User Guide
==========

The command line interface (CLI, see-ell-aye) implements a CISCO-like,
or Juniper Networks JunOS-like, CLI for UNIX systems.

New users always get the CLI as the default "shell" when logging in, but
the default `admin` user logs in to `bash`.  To access the CLI, type:

    cli

Key commands available in any context are:

    help
    show

For each command it also possible to press the `?` key and `TAB` to get
more help and suggestions for completion.

> **Note:** for the sake of brevity, the hostname in the following examples
> has been shortened to `host`.  The default name is composed from a product
> specific string followed by the last three octets of the system base MAC
> address, e.g., `switch-12-34-56`.  An example of how to change the hostname
> is included below.


Admin Exec
----------

The top-level context after logging in and starting the CLI is the
admin-exec or "main" context.  It is used for querying system status,
managing configuration files/profiles and doing advanced debugging.

Available commands can be seen by pressing `?` at the prompt:

```
admin@host:/>
  configure      Create new candidate-config based on running-config
  copy           Copy configuration, e.g., copy running-config startup-config
  exit           Exit from CLI (log out)
  factory-reset  Restore the system to factory default state
  follow         Monitor a log file, use Ctrl-C to abort
  help           Help system (also try '?' key)
  logout         Alias to exit
  netcalc        IP subnet calculator
  password       Password tools
  ping           Ping a network host or multicast group
  poweroff       Poweroff system (system policy may yield reboot)
  reboot         Reboot system
  set            Set operations, e.g., current date/time
  shell          Enter system shell
  show           Show system status and configuration files
  tcpdump        Capture network traffic
  upgrade        Install a software update bundle
```

To get more help for a given command, type the command, a space, and
then tap `?` again.  You can also tap the `Tab` key to see available
argument completions.

### Upgrading the Software

The admin-exec command `upgrade` can be used to install software images, or
bundles.  A bundle is a signed and self-contained package that carries all the
information necessary to determine if it holds a bootloader, a Linux image, or
even both.

To install a new software image to the currently *inactive* partition[^1], we
use the `upgrade` command and a URI to a ftp/tftp/sftp or http/https server
that hosts the file:

```
admin@host:/> upgrade tftp://192.168.122.1/firmware-x86_64-v23.11.pkg
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
Installing `tftp://192.168.122.1/firmware-x86_64-v23.11.pkg` succeeded
admin@host:/>
```

The secondary partition (`rootfs.1`) has now been upgraded and will be used as
the *active* partition on the next boot.  Leaving the primary partition, with
the version we are currently running, intact in case of trouble.

[^1]: It is not possible to upgrade the partition we booted from.  Thankfully
    the underlying "rauc" subsystem keeps track of this.  Hence, to upgrade
    both partitions you must reboot to the new version (to verify it works)
    and then repeat the same command.


Configure Context
-----------------

Enter the configure context from admin-exec by typing `configure`
followed by Enter.  Available commands, press `?` at the prompt:

```
admin@host:/> configure
admin@host:/config/>
  abort        Abandon candidate
  change       Interactively change setting, e.g. password
  check        Validate candidate
  commit       Commit current candidate to running-config
  delete       Delete configuration setting(s)
  diff         Summarize uncommitted changes
  do           Execute operational mode command
  edit         Descend to the specified configuration node
  end          Alias to up, leave this subsection/node
  exit         Ascend to the parent configuration node, or abort (from top)
  leave        Finalize candidate and apply to running-config
  no           Alias for delete
  rollback     Restore candidate to running-config
  set          Set configuration setting
  show         Show configuration
  text-editor  Modify binary content in a text editor
  top          Ascend to the configuration root
  up           Ascend to the parent configuration node
```

The `edit` command lets you change to a sub-configure context, e.g.:

```
admin@host:/config/> edit interface eth0
admin@host:/config/interface/eth0/>
```

Use `up` to go up one level.

```
admin@host:/config/interface/eth0/> up
admin@host:/config/>
```

> **Note:** the tree structure in the configure context is automatically
> generated from the system's supported NETCONF YANG models, which may
> vary between products.  However, the `ietf-interfaces.yang` and
> `ietf-ip.yang` models, for instance, that provide basic networking
> support are common to all systems.


### Set IP Address on an Interface

```
admin@host:/config/> edit interface eth0
admin@host:/config/interface/eth0/>
admin@host:/config/interface/eth0/> set ipv4 address 192.168.2.200 prefix-length 24
```

From anywhere in configure context you can see the changes you have
made by typing `diff`:

```
admin@host:/config/interface/eth0/> diff
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
admin@host:/config/interface/eth0/> leave
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
admin@host:/config/> edit system authentication user admin
admin@host:/config/system/authentication/user/admin/> change password
New password: 
Retype password: 
admin@host:/config/system/authentication/user/admin/> leave
```

The `change password` command starts an interactive dialogue that asks
for the new password, with a confirmation, and then salts and encrypts
the password with sha512crypt. 

It is also possible to use the `set password ...` command.  This allows
setting an already hashed password.  To manually hash a password, use
the `do password encrypt` command.  This launches the admin-exec command
to hash, and optionally salt, your password.  This encrypted string can
then be used with `set password ...`.

> **Tip:** if you are having trouble thinking of a password, there is
> also `do password generate`, which generates random but readable
> strings using the UNIX command `pwgen`.


### SSH Authorized Key

Logging in remotely with SSH is possible by adding a *public key* to a
user.  Here we add the authorized key to the admin user, multiple keys
are supported.

With SSH keys in place it is possible to disable password login, just
remember to verify SSH login and network connectivity before doing so.

```
admin@host:/config/> edit system authentication user admin 
admin@host:/config/system/authentication/user/admin/> edit authorized-key example@host
admin@host:/config/system/authentication/user/admin/authorized-key/example@host/> set algorithm ssh-rsa
admin@host:/config/system/authentication/user/admin/authorized-key/example@host/> set key-data AAAAB3NzaC1yc2EAAAADAQABAAABgQC8iBL42yeMBioFay7lty1C4ZDTHcHyo739gc91rTTH8SKvAE4g8Rr97KOz/8PFtOObBrE9G21K7d6UBuPqmd0RUF2CkXXN/eN2PBSHJ50YprRFt/z/304bsBYkDdflKlPDjuSmZ/+OMp4pTsq0R0eNFlX9wcwxEzooIb7VPEdvWE7AYoBRUdf41u3KBHuvjGd1M6QYJtbFLQMMTiVe5IUfyVSZ1RCxEyAB9fR9CBhtVheTVsY3iG0fZc9eCEo89ErDgtGUTJK4Hxt5yCNwI88YaVmkE85cNtw8YwubWQL3/tGZHfbbQ0fynfB4kWNloyRHFr7E1kDxuX5+pbv26EqRdcOVGucNn7hnGU6C1+ejLWdBD7vgsoilFrEaBWF41elJEPKDzpszEijQ9gTrrWeYOQ+x++lvmOdssDu4KvGmj2K/MQTL2jJYrMJ7GDzsUu3XikChRL7zNfS2jYYQLzovboUCgqfPUsVba9hqeX3U67GsJo+hy5MG9RSry4+ucHs=
admin@host:/config/system/authentication/user/admin/authorized-key/example@host/> show
algorithm ssh-rsa;
key-data AAAAB3NzaC1yc2EAAAADAQABAAABgQC8iBL42yeMBioFay7lty1C4ZDTHcHyo739gc91rTTH8SKvAE4g8Rr97KOz/8PFtOObBrE9G21K7d6UBuPqmd0RUF2CkXXN/eN2PBSHJ50YprRFt/z/304bsBYkDdflKlPDjuSmZ/+OMp4pTsq0R0eNFlX9wcwxEzooIb7VPEdvWE7AYoBRUdf41u3KBHuvjGd1M6QYJtbFLQMMTiVe5IUfyVSZ1RCxEyAB9fR9CBhtVheTVsY3iG0fZc9eCEo89ErDgtGUTJK4Hxt5yCNwI88YaVmkE85cNtw8YwubWQL3/tGZHfbbQ0fynfB4kWNloyRHFr7E1kDxuX5+pbv26EqRdcOVGucNn7hnGU6C1+ejLWdBD7vgsoilFrEaBWF41elJEPKDzpszEijQ9gTrrWeYOQ+x++lvmOdssDu4KvGmj2K/MQTL2jJYrMJ7GDzsUu3XikChRL7zNfS2jYYQLzovboUCgqfPUsVba9hqeX3U67GsJo+hy5MG9RSry4+ucHs=;
admin@host:/config/system/authentication/user/admin/authorized-key/example@host/> leave
```

> **Note:** the `ssh-keygen` program already base64 encodes the public
> key data, so there is no need to use the `text-editor` command, `set`
> does the job.

### Creating a VETH Pair

The following example creates a `veth0a <--> veth0b` virtual Ethernet
pair which is useful for connecting, e.g., a container to the physical
world.  Here we also add an IPv4 address to one end of the pair.

```
admin@host:/config/> edit interface veth0a
admin@host:/config/interface/veth0a/> set veth peer veth0b
admin@host:/config/interface/veth0a/> set ipv4 address 192.168.0.1 prefix-length 24
admin@host:/config/interface/veth0a/> up
admin@host:/config/> diff
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
admin@host:/config/> leave
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
admin@host:/config/> edit interface br0
admin@host:/config/interface/br0/> set bridge ieee-group-forward lldp
admin@host:/config/interface/br0/> up
admin@host:/config/> set interface eth0 bridge-port bridge br0
admin@host:/config/> set interface veth0b bridge-port bridge br0
admin@host:/config/> diff
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
