# Introduction

This document provides an introduction of key concepts, details how
the system boots, including failure modes, and provides links to
other documents for further study.

## CLI

The command line interface (CLI, see-ell-i) is the traditional way of
interacting with single network equipment like switches and routers.
Today users have come to expect more advanced graphical GUIs, like a web
interface, to manage a device or NETCONF-based tools that allow for
managing entire fleets of installed equipment.

Nevertheless, when it comes to initial deployment and debugging, it
is very useful to know how to navigate and use the CLI.

> Proceed to the [CLI Introduction](cli/introduction.md) or [CLI
> Configuration Tutorial](cli/configure.md).


## Key Concepts

The two modes in the CLI are the admin-exec and the configure context.

However, when logging in to the system, from the console port or SSH,
you land in a standard UNIX shell, Bash.  This is for advanced users
and remote scripting purposes (production equipment):

    Run the command 'cli' for interactive OAM
    
    admin@example:~$

To enter the CLI, follow the instructions, for interactive Operations,
Administration, and Management (OAM), type:

    admin@example:~$ cli
    admin@example:/>

The prompt, constructed from your username and the device's hostname,
changes slightly.  You are now in the admin-exec context of the CLI.
Here you can inspect system status and do operations to debug networking
issues, e.g. ping.  You can also enter configure context by typing:
`configure` followed by commands to `set`, `edit`, apply changes using
`leave`, or `abort` and return to admin-exec.

> The [CLI Introduction](cli/introduction.md) can be useful to skim
> through at this point.

The system has several datastores (or files):

 - `factory-config` consists of a set of default configurations, some
   static and others generated per-device, e.g., a unique hostname and
   number of ports/interfaces.   This file is generated at boot.
 - `failure-config` is also generated at boot, from the same YANG models
   as `factory-config`, and holds the system *Fail Secure Mode*
 - `startup-config` is created from `factory-config` at boot if it does
   not exist.  It is loaded as the system configuration on each boot.
 - `running-config` is what is actively running on the system.  If no
   changes have been made since the system booted, it is the same as
   `startup-config`.
 - `candidate-config` is created from `running-config` when entering the
   configure context.  Any changes made here can be discarded (`abort`,
   `rollback`) or committed (`commit`, `leave`) to `running-config`.

> Please see the [Branding & Releases](branding.md) document for more
> in-depth information on how `factory-config` and `failure-config` can
> be adapted to different customer requirements.  Including how you can
> override the generated versions of these files with plain per-product
> ones -- this may even protect against some of the failure modes below.


## System Boot

After the system firmware (BIOS or and [boot loader](boot.md) start
Linux the following happens.  The various failure modes, e.g., missing
password in VPD, are detailed later in this section.

![System boot flowchart](img/fail-secure.svg)

 1. Before mounting `/cfg` and `/var` partitions, hosting read-writable
    data like `startup-config` and container images, the system first
    checks if a factory reset has been requested by the user, if so it
    wipes the contents of these partitions
 2. Linux boots with a device tree which is used for detecting generic
    make and model of the device, e.g., number of interfaces.  It may
    also reference an EEPROM with [Vital Product Data](vpd.md).  That is
    where the base MAC address and per-device password hash is stored.
    (Generic builds use the same MAC address and password)
 3. On every boot the system's `factory-config` and `failure-config` are
    generated from the YANG[^1] models of the current firmware version.
    This ensures that a factory reset device can always boot, and that
    there is a working fail safe, or rather *fail secure*, mode
 4. On first power-on, and after a factory reset, the system does not
    have a `startup-config`, in which case `factory-config` is copied
    to `startup-config` -- if a per-product specific version exists it
    is preferred over the generated one
 5. Provided the integrity of the `startup-config` is OK, a system
    service loads and activates the configuration

### Failure Modes

So, what happens if any of the steps above fail?

**VPD Fail**

The per-device password cannot be read, or is corrupt, so the system
`factory-config` and `failure-config` are not generated:

 1. First boot, or after factory reset: `startup-config` cannot be
    created or loaded, and `failure-config` cannot be loaded.  The
    system ends up in an unrecoverable state, i.e., **RMA[^2] Mode**
 2. The system has booted (at least) once with correct VPD and password
    and already has a `startup-config`.  Provided the `startup-config`
    is OK (see below), it is loaded and system boots successfully

In both cases, external factory reset modes/button will not help, and
in the second case will cause the device to fail on the next boot.

> The second case does not yet have any warning or event that can be
> detected from the outside.  This is planned for a later release.

**Broken startup-config**

If loading `startup-config` fails for some reason, e.g., invalid JSON
syntax, failed validation against the system's YANG model, or a bug in
the system's `confd` service, the *Fail Secure Mode* is triggered and
`failure-config` is loaded (unless VPD Failure, see above).

> Again, please see the [Branding & Releases](branding.md) document for
> how to provide a per-product hard-coded `failure-config` to suit your
> products preferences.

*Fail Secure Mode* is a fail-safe mode provided for debugging the
system.  The default[^3] creates a setup of isolated interfaces with
communication only to the management CPU, SSH and console login using
the device's factory reset password, IP connectivity only using IPv6
link-local, and device discovery protocols: LLDP, mDNS-SD.  The login
and shell prompt are set to `failure-c0-ff-ee`, the last three octets of
the device's base MAC address.

[^1]: YANG is a modeling language from IETF, replacing that used for
    SNMP (MIB), used to describe the subsystems and properties of
	the system.
[^2]: Return Merchandise Authorization (RMA), i.e., broken beyond repair
    by end-user and eligible for return to manufacturer.
[^3]: Customer specific builds can define their own `failure-config`.
    It may be the same as `factory-config`, with the hostname set to
    `failure`, or a dedicated configuration that isolates interfaces, or
    even disables ports, to ensure that the device does not cause any
    security problems on the network.  E.g., start forwarding traffic
    between previously isolated VLANs.
