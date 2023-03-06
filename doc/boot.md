Disk Layout
-----------

Infix runs from a block device (e.g. eMMC or virtio disk) with the
following layout. The disk is expected to use the GPT partitioning
scheme. Partitions marked with an asterisk are optional.

    .-----------.
	| GPT Table |
	:-----------:
	|    boot*  |
	:-----------:
	|    aux    |
	:-----------:
	|           |
	|  primary  |
	|           |
	:-----------:
	|           |
	| secondary |
	|           |
	:-----------:
	|    cfg    |
	:-----------:
	|           |
	|    var*   |
	|           |
	'-----------'

### `boot` - Bootloader

| Parameter | Value                                   |
|-----------|-----------------------------------------|
| Required  | No                                      |
| Size      | 4 MiB                                   |
| Format    | Raw binary, as dictated by the hardware |

Optional partition containing the system's bootloader. May also reside
in a separate storage device, e.g. a serial FLASH.


### `aux` - Auxiliary Data

| Parameter | Value           |
|-----------|-----------------|
| Required  | Yes             |
| Size      | 4 MiB           |
| Format    | EXT4 filesystem |

Holds information that is shared between Infix and its bootloader,
such as image signatures required to validate the chain of trust,
bootloader configuration etc.

Typical layout when using U-Boot bootloader:

    /
    ├ primary.itbh
	├ secondary.itbh
	└ uboot.env

During boot, an ITB header along with the corresponding root
filesystem image are concatenated in memory, by U-Boot, to form a
valid FIT image that is used to verify its integrity and origin before
any files are extracted from it.

Note that the bootloader's primary environment is bundled in the
binary - `uboot.env` is only used to import a few settings that is
required to configure the boot order.


### `primary`/`secondary` - Infix Root Filesystem Images

| Parameter | Value             |
|-----------|-------------------|
| Required  | Yes               |
| Size      | >= 256 MiB        |
| Format    | Squash filesystem |

The main Infix image - the `rootfs.squashfs` image in your `images/`
directory. Two copies exist so that an incomplete upgrade does not
brick the system, and to allow fast rollbacks when upgrading to a new
version.


### `cfg` - Configuration Data

| Parameter | Value           |
|-----------|-----------------|
| Required  | Yes             |
| Size      | >= 16 MiB       |
| Format    | EXT4 filesystem |

Non-volatile storage of the system configuration and user
data. Concretely, user data is everything stored under `/root` and
`/home`. Depending on the operating mode, the configuration is either
the NETCONF databases from `/cfg`, or the contents of `/etc` when
operating in native mode.


### `var` - Variable Data

| Parameter | Value           |
|-----------|-----------------|
| Required  | No              |
| Size      | >= 16 MiB       |
| Format    | EXT4 filesystem |

Persistent storage for everything under `/var`. This is maintained as
a separate filesystem from the data in `cfg`, because while the system
can funtion reasonably well without a persistent `/var`, loosing
`/cfg` or `/etc` is much more difficult.

If `var` is not available, Infix will still persist `/var/lib` using
`cfg` as the backing storage.

