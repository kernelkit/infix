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
by device discovery protocols like SSDP, mDNS/SD and LLDP.

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


Releases
--------

A release build requires the global variable `INFIX_RELEASE` to be set.
It can be derived from GIT, if the source tree is kept in GIT VCS.

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
