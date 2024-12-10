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


Factory & Failure Config
------------------------

To support booting the same image (CPU architecture) on multiple boards,
Infix by default generates the device's initial configuration every time
at boot.  This also ensures the device can always be restored to a known
state after a factory reset, since the `factory-config` is guaranteed to
be compatible with the YANG models for the given software version. (For
more information on how the system boots, please see the section [Key
Concepts](introduction.md#key-concepts) in the Introduction document.)

However, for custom builds of Infix it is possible to override this with
a single static `/etc/factory-config.cfg` (and failure-config) in your
rootfs overlay -- with a [VPD](vpd.md) you can even support several!


### Variables & Format Specifiers

Parts of the configuration you likely always want to generated, like the
SSH hostkey used by SSH server and NETCONF, a unique hostname, or the `admin` user's
unique (per-device with a VPD) password hash.  This section lists the
available keywords, see the next section for examples of how to use
them:

 - **Default password hash:** `$factory$` (from VPD, .dtb, or built-in)  
   XPath: `/ietf-system:system/authentication/user/password`
 - **Default SSH and NETCONF hostkey:** `genkey` (regenerated at factory reset)
   XPath: `/ietf-keystore:keystore/asymmetric-keys/asymmetric-key[name='genkey']`
 - **Hostname format specifiers:**  
   XPath: `/ietf-system:system/hostname`
   - `%i`: OS ID, from `/etc/os-release`, from Menuconfig branding
   - `%h`: Default hostname, from `/etc/os-release`, from branding
   - `%m`: NIC specific part of base MAC, e.g., to `c0-ff-ee`
   - `%%`: Literal %


### Static Files

> **Caveat:** maintaining static a factory-config and failure-config may
> seem like an obvious choice, but as YANG models evolve (even the IETF
> models get upgraded), you may need to upgrade your static files.

First, for one-off builds (one image per product), the simplest way is
to override the location where the system looks for the files, `/etc`
already at build time.  This can be done using a Buildroot rootfs
overlay providing, e.g., `/etc/factory-config.cfg`.  Example: [NanoPi
R2S][] in `${INFIX}/board/aarch64/r2s/rootfs/etc/factory-config.cfg`.

Second, to support multiple products in a single image, we can employ
another method to install a `/etc/factory-config.cfg` override -- at
runtime.  This relies on the very early system `probe` that detects the
specific product from VPD data.

The `probe` consists of several sequential steps that currently run from
`${INFIX}/board/common/rootfs/usr/libexec/infix/init.d/`.  One of them
check if `/usr/share/product/<PRODUCT>` exists, and if so attempts to
copy the entire contents to `/`.  Here, `<PRODUCT>` is determined from
the VPD, which is available in `/run/system.json` as `"product-name"`,
after `00-probe` has run. The lower case version of the string is used.

I.e., create a rootfs overlay that provides any combination of:

 - `/usr/share/product/<PRODUCT>/etc/factory-config.cfg`
 - `/usr/share/product/<PRODUCT>/etc/failure-config.cfg`


### Dynamically Generated

The generated `factory-config` and `failure-config` files consist of
both static JSON files and part generated files at runtime for each
device.  The resulting files are written to the RAM disk in `/run`:

 - `/run/confd/factory-config.gen`
 - `/run/confd/failure-config.gen`

Provided no custom overrides (see above) have been installed already,
these files are then copied to:

 - `/etc/factory-config.cfg`
 - `/etc/failure-config.cfg`

... where the bootstrap process expects them to be in the next step.

Examples of generated contents are the SSH hostkey and hostname.  The
latter is constructed from the file `/etc/hostname`, appended with the
last three octets of the system's base MAC address.  To override the
base hostname, set `BR2_TARGET_GENERIC_HOSTNAME` in your defconfig.

The static files are installed by Infix `confd` in `/usr/share/confd/`
at build time.  It contains two subdirectories:

    /usr/share/confd/
     |- factory.d/
     |  |- 10-foo.json
     |  |- 10-bar.json
     |  `- 10-qux.json
     `- failure.d/
        |- 10-xyzzy.json
        `- 10-garply.json

To override, or extend, these files in you br2-external, set up a rootfs
overlay and add it last in `BR2_ROOTFS_OVERLAY`.  Your overlay can look
something like this:

    ./board/common/rootfs/
      |- etc/
      |  |- confdrc             # See below
      |  `- confdrc.local
      `- usr/
         `- share/
            `- confd/
               |- 10-foo.json   # Override Infix foo
               |- 30-bar.json   # Extend, probably 10-bar.json
               `- 30-fred.json  # Extend, your own defaults

Using the same filename in your overlay, here `10-foo.json`, completely
replaces the contents of the same file provided by Infix.  If you just
want to extend, or replace parts of an Infix default, use `30-....json`.
Here the file `30-bar.json` is just a helpful hit to maintainers of your
br2-external that it probably extends Infix' `10-bar.json`.

The reason for the jump in numbers is that 20 is reserved for files
generated by Infix' `gen-function` scripts.  Your br2-external can
provide a few custom ones that the `bootstrap` knows about, e.g.,
`gen-ifs-custom` that overrides `20-interfaces.json`.  See the
bootstrap script for more help, and up-to-date information.

> **Note:** you may not need to provide your own `/etc/confdrc`.  The
> one installed by `confd` is usually enough.  However, if you want to
> adjust the behavior of `bootstrap` you may want to override it.  There
> is also `confdrc.local`, which usually is enough to change arguments
> to scripts like `gen-interfaces`, e.g., to create a bridge by default,
> you may want to look into `GEN_IFACE_OPTS`.


### Example Snippets

**IETF System:**

```hsib
  "ietf-system:system": {
    "hostname": "example-%m",
    "ntp": {
      "enabled": true,
      "server": [
        {
          "name": "ntp.org",
          "udp": {
            "address": "pool.ntp.org"
          }
        }
      ]
    },
    "authentication": {
      "user": [
        {
          "name": "admin",
          "password": "$factory$",
          "infix-system:shell": "bash"
        }
      ]
    },
    "infix-system:motd-banner": "Li0tLS0tLS0uCnwgIC4gLiAgfCBJbmZpeCAtLSBhIE5ldHdvcmsgT3BlcmF0aW5nIFN5c3RlbQp8LS4gdiAuLXwgaHR0cHM6Ly9rZXJuZWxraXQuZ2l0aHViLmlvCictJy0tLSctJwo="
  },            # <---- REMEMBER COMMA SEPARATORS IN SNIPPETS!
                # <---- ... and no comments.
```

The `motd-banner` is a binary type, which is basically a Base64 encoded
text file without line breaks (`-w0`):

```bash
$ echo "Li0tLS0tLS0uCnwgIC4gLiAgfCBJbmZpeCAtLSBhIE5ldHdvcmsgT3BlcmF0aW5nIFN5c3RlbQp8LS4gdiAuLXwgaHR0cHM6Ly9rZXJuZWxraXQuZ2l0aHViLmlvCictJy0tLSctJwo=" |base64 -d
.-------.
|  . .  | Infix -- a Network Operating System
|-. v .-| https://kernelkit.github.io
'-'---'-'
```

**IETF Keystore**

Notice how both the public and private keys are left empty here, this
cause them to be always automatically regenerated after each factory reset.
Keeping the `factory-config` snippet like this means we can use the same
file on multiple devices, without risking them sharing the same host
keys.  Sometimes you may want the same host keys, but that is the easy
use-case and not documented here.

```json
  "ietf-keystore:keystore": {
    "asymmetric-keys": {
      "asymmetric-key": [
        {
          "name": "genkey",
          "public-key-format": "ietf-crypto-types:ssh-public-key-format",
          "public-key": "",
          "private-key-format": "ietf-crypto-types:rsa-private-key-format",
          "cleartext-private-key": "",
          "certificates": {}
        }
      ]
    }
  },
```

**IETF NETCONF Server**

```json
  "ietf-netconf-server:netconf-server": {
    "listen": {
      "endpoints": {
        "endpoint": [
          {
            "name": "default-ssh",
            "ssh": {
              "tcp-server-parameters": {
                "local-address": "::"
              },
              "ssh-server-parameters": {
                "server-identity": {
                  "host-key": [
                    {
                      "name": "default-key",
                      "public-key": {
                        "central-keystore-reference": "genkey"
                      }
                    }
                  ]
                }
              }
            }
          }
        ]
      }
    }
  },
```

**Infix Services**
```json
  "infix-services:ssh": {
    "enabled": true,
    "hostkey": [
      "genkey"
    ],
    "listen": [
      {
        "name": "ipv4",
        "address": "0.0.0.0",
        "port": 22
      },
      {
        "name": "ipv6",
        "address": "::1",
        "port": 22
      }
    ]
  }
```



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


### Specifying Versioning Information

Two optional environment variables control the version information
recorded in images. Both of these **must be** a lower-case string (no
spaces or other characters outside of 0–9, a–z, '.', '_' and '-')
identifying the operating system version, excluding any OS name
information or release code name, and suitable for processing by
scripts or usage in generated filenames.

#### `INFIX_BUILD_ID`

Used for `BUILD_ID` in `/etc/os-release`.

**Default:** `$(git describe --always --dirty --tags)`, from the _top
directory_. By default, the top directory refers to the root of the
Infix source tree, but this can be changed by setting the branding
variable `INFIX_OEM_PATH`, e.g. in a `defconfig` file or via `make
menuconfig`, to the path of an enclosing br2-external.

#### `INFIX_RELEASE`

Used for `VERSION` and `VERSION_ID` in `/etc/os-release` and
generated file names like disk images, etc.

**Default:** `${INFIX_BUILD_ID}`

[NanoPi R2S]: https://github.com/kernelkit/infix/blob/main/board/aarch64/r2s/rootfs/etc/factory-config.cfg
