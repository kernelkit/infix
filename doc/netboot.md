Netboot HowTo
=============

This document describes how to set up network booting U-Boot devices on
a LAN, e.g., when working with an evaluation board or other embedded
system.  The most secure way to do this is with a local LAN between a PC
and the device.

Instead of setting up everything in U-Boot to download the Linux Image,
device tree, and initramfs, we will let U-Boot download a script with
instructions to run.  When you have multiple systems (boards) this
quickly becomes a lot easier to manage.

> [!NOTE]
> Instructions in this HowTo assume a Debian based development system,
> e.g., Ubuntu or Linux Mint.


## Network Interface Setup

For two dedicated network interfaces, here `eth2` and `eth3` (ymmv), we
create an old-style interfaces config[^1] with the following content:

```
# /etc/network/interfaces.d/gimli
auto eth2
iface eth2 inet static
        address 192.168.0.1
        netmask 255.255.255.0

auto eth3
iface eth3 inet manual
```

> [!TIP]
> Use configuration file names in `.d/` directories that make sense and
> can easily be remembered.  Here we use the hostname of the PC.


## DHCP/TFTP Server Setup

The examples given here use `dnsmasq`, which provides both DHCP and TFTP
server support.  The same can be achieved with other implementations.

Similar to `interfaces.d`, dnsmasq has an `/etc/dnsmasq.d` directory so
we can use "snippets" instead of modifying `/etc/dnsmasq.conf` directly.
Add a file called `/etc/dnsmasq.d/gimli`.

Initial content:

```
# Remember IP address handed out to BOOTP clients
bootp-dynamic
# Disable check-if-ip-address-is-already-used
no-ping

# Enable TFTP server, use /srv/ftp, same as any FTP server, useful
# when using the same images for system upgrade as for netbooting.
enable-tftp
tftp-root=/srv/ftp
```

> [!CAUTION]
> First of all, make sure you DO NOT accidentally set up dnsmasq so that
> it starts acting as a DHCP server also on your office LAN!  Follow the
> instructions below for more details.

If you have many interfaces used for lab equipment and only one office
LAN interface, then use something like this:

```
# Disable DHCP server on loopback and office LAN (eth0)
except-interface=lo
except-interface=eth0
```

To further lock this down, we only run the DHCP server on each of the
interfaces used for lab equipment, with a dedicated IP range as well:

```
# Currently I have an imx8mp-evk on eth2, so on my system I have a
# symlink bootfile-eth2 -> netboot.scr
interface=eth2
dhcp-range=192.168.0.100,192.168.0.199,1h
dhcp-boot=tag:eth2,bootfile-eth2
```

## Bootfile netboot.scr

The bootfile U-Boot retrieves from the TFTP server is a script that
looks like this, `netboot.sh`:

```sh
setenv ramdisk_addr_r 0x58000000
setenv fdt_addr_r     0x50400000

setenv autoboot off
tftp ${fdt_addr_r}     imx8mp-evk/imx8mp-evk.dtb
tftp ${kernel_addr_r}  imx8mp-evk/Image
tftp ${ramdisk_addr_r} imx8mp-evk/rootfs.squashfs

setenv bootargs console=ttymxc1,115200 root=/dev/ram0 brd.rd_size=500000 rauc.slot=net
booti ${kernel_addr_r} ${ramdisk_addr_r}:${filesize} ${fdt_addr_r}
```

U-Boot cannot read script files directly, so we need to wrap it with a
FIT format header, this is done by first converting it on the PC:

```
$ mkimage -T script -d netboot.sh netboot.scr
```

The output is `netboot.scr` which we symlink to above in the dnsmasq
setup step.


## U-Boot Commands

U-Boot is a maze of environment variables, some with values, some wrap
commands, and most are undocumented.  We will use a prefix for our
variables to ensure we do not overwrite anything you may want to use
later.

```sh
==> setenv ixboot 'dhcp && source \${fileaddr}'
==> saveenv
```


[^1]: To prevent NetworkManager from automatically managing the interfaces.
