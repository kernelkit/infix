# Introduction

![Infix - Linux <3 NETCONF](logo.png){ align=right  width="480" }

Welcome to Infix, your immutable, friendly, and secure operating system!
On these pages you can find both user and developer documentation.

Most topics on configuring the system include CLI examples, but every
setting, as well as status read-back from the operational datastore, is
also possible to perform using NETCONF or RESTCONF.  In fact, the Infix
regression test system solely relies on NETCONF and RESTCONF.

> [!TIP]
> The CLI documentation is also available from inside the CLI itself
> using the `help` command in admin-exec mode.

This document provides an introduction of key concepts, details how
the system boots, including failure modes, and provides links to
other documents for further study.

## Command Line Interface

The command line interface (CLI, see-ell-i) is the traditional way of
interacting with single network equipment like switches and routers.
Today users have come to expect more advanced graphical GUIs, like a web
interface, to manage a device or NETCONF-based tools that allow for
managing entire fleets of installed equipment.

Nevertheless, when it comes to initial deployment and debugging, it
is very useful to know how to navigate and use the CLI.

> [!INFO]
> For more information, see the [CLI Introduction](cli/introduction.md)
> and the [CLI Configuration Tutorial](cli/configure.md).

## Key Concepts

The two modes in the CLI are the admin-exec and the configure context.

However, when logging in to the system, from the console port or SSH,
you land in a standard UNIX shell, Bash.  This is for advanced users
and remote scripting purposes (production equipment):

```
    Run the command 'cli' for interactive OAM

    admin@example:~$
```

To enter the CLI, follow the instructions, for interactive Operations,
Administration, and Management (OAM), type:

```
    admin@example:~$ cli
    admin@example:/>
```

The prompt, constructed from your username and the device's hostname,
changes slightly.  You are now in the admin-exec context of the CLI.
Here you can inspect system status and do operations to debug networking
issues, e.g. ping.  You can also enter configure context by typing:
`configure` followed by commands to `set`, `edit`, apply changes using
`leave`, or `abort` and return to admin-exec.

> [!TIP]
> If you haven't already, the [CLI Introduction](cli/introduction.md)
> would be useful to skim through at this point.

## Datastores

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

> [!TIP]
> Please see the [Branding & Releases](branding.md) document for more
> in-depth information on how `factory-config` and `failure-config` can
> be adapted to different customer requirements.  Including how you can
> override the generated versions of these files with plain per-product
> ones -- this may even protect against some of the failure modes below.
