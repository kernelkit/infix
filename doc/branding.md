Branding & Releases
===================

This document is for projects using Infix as a br2-external, i.e., OEMs.


Branding
--------

Branding is done in menuconfig, there are several settings affecting
it, most are in the Infix external subsection called "Branding", but
there is also `BR2_TARGET_GENERIC_HOSTNAME`, which deserves a
special mention.

The hostname is used for the system default `/etc/hostname`, which
is the base name for the "unique:ified" hostname + the last three
octets of the base MAC[^1] address, e.g., `infix-c0-ff-ee`.  This in
turn is the hostname that is set at first boot and also advertised
by device discovery protocols like mDNS-SD and LLDP.

See the help texts for the *Infix Branding* settings to understand
which ones are mandatory and which are optional, menuconfig does not
check this for you and you may end up with odd results.

Verify the result after a build by inspecting:

  - `output/images/*`: names, missing prefix, etc.
  - `output/target/etc/os-release`: this file is sourced by
    other build scripts, e.g., `mkgns3a.sh`.  For reference, see
	https://www.freedesktop.org/software/systemd/man/os-release.html

> **Note:** to get proper GIT revision (hash) from your composed OS,
> remember in menuconfig to set `INFIX_OEM_PATH`.  When unset the
> Infix `post-build.sh` script defaults to the Infix base path.  The
> revision is stored in the file `/etc/os-release` as BUILD_ID and
> is also in the file `/etc/version`.  See below for more info.

[^1]: The base MAC address is defined in the device's Vital Product
    Data (VPD) EEPROM, or similar, which is used by the kernel to
    create the system interfaces.  This MAC address is usually also
    printed on a label on the device.


Integration
-----------

When integrating your software stack with Infix there may be protocols
that want to change system settings like hostname and dynamically set
IP address and default gateway, e.g. PROFINET.  This section detail a
few recommendations for maintaining co-existence in this scenario of
the multiple producers problem.

First, there's a clear difference between "singleton" like hostsname
and an interface IP address.  Consider the case of a static IP and a
DHCP assigned IP, these can co-exist because of the `proto NUM` field
available in iproute2.  This is used in Infix so that static addresses
can be flushed independently of DHCP addresses.  The same can be done
by other "address providers", e.g., PROFINET.

Changing properties like hostname should be done by injecting a change
into Infix, by for example calling `sysrepocfg -Ediff.xml`.  Here is an
example of how to get the current hostname and apply an XML diff:

```
root@infix-00-00-00:~# sysrepocfg -X -x "/system/hostname" > hostnm.xml
root@infix-00-00-00:~# cat hostnm.xml
<system xmlns="urn:ietf:params:xml:ns:yang:ietf-system">
  <hostname>infix-00-00-00</hostname>
</system>
root@infix-00-00-00:~# edit hostnm.xml
root@infix-00-00-00:~# sysrepocfg -Ehostnm.xml
root@example:~# 
```

Second, perform all changes on `running-config`, the running datastore.
That way you have a clear state to return to if your application needs
to do a factory reset.  E.g., in PROFINET a type 1/2 factory reset will
reset only the PROFINET specific settings.  That way you can actually
have your system `startup-config` disable all physical ports and the
PROFINET application enables only ports that are not deactivated.  (On
factory reset it will not know of any ports to deactivate so it will
activate all.)

You can consider the system composed of two entities:

  - NETCONF starts up the system using `startup-config`, then
  - Hands over control to your application at runtime

Infix is prepared for this by already having two "runlevels" for these
two states.  The `startup-config` is applied in runlevel S (bootstrap)
and the system then enters runlevel 2 for normal operation.

This allow you to keep a set of functionality that is provided by the
underlying system, and another managed by your application.  You can
of course in your br2-external provide a sysrepo plugin that block 
operations on certain datastores when your application is enabled.
E.g., to prevent changes to startup after initial deployment.  In
that case a proper factory reset would be needed to get back to a
"pre-deployment" state where you can reconfigure your baseline.


Releases
--------

A release build requires the global variable `INFIX_RELEASE` to be set.
It can be derived from GIT, if the source tree is kept in GIT VCS.  First,
let us talk about versioning in general.

### Versioning

Two popular scheme for versioning a product derived from Infix:

 1. Track Infix major.minor, e.g. *Foobar v23.08.z*, where `z` is
    your patch level.  I.e., Foobar v23.08.0 could be based on Infix
    v23.08.0, or v23.08.12, it is up to you.  Maybe you based it on
    v23.08.12 and then back ported changes from v23.10.0, but it was
    the first release you made to your customer(s).
 2. Start from v1.0.0 and step the major number every time you sync
    with a new Infix release, or every time Infix bumps to the next
    Buildroot LTS.

The important thing is to be consistent, not only for your own sake,
but also for your end customers.  The *major.minor.patch* style is
the most common and often recommended style, which usually maps well
to other systems, e.g. PROFINET GSDML files require this (*VX.Y.Z*).
But you can of course use only two numbers, *major.minor*, as well.

> What could be confusing, however, is if you use the name *Infix*
> with your own versioning scheme.


### `INFIX_RELEASE`

This global variable **must be** a lower-case string (no spaces or
other characters outside of 0–9, a–z, '.', '_' and '-') identifying
the operating system version, excluding any OS name information or
release code name, and suitable for processing by scripts or usage
in generated filenames.

Used for `VERSION` and `VERSION_ID` in `/etc/os-release` and
generated file names like disk images, etc.

**Default:** generated using `git describe --always --dirty --tags`,
with an additional `-C $infix_path`.  This variable defaults to the
Infix tree and can be changed by setting the menuconfig branding
variable `INFIX_OEM_PATH` to that of the br2-external.  It is also
possible to set the `GIT_VERSION` variable in your `post-build.sh`
script to change how the VCS version is extracted.

