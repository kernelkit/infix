TFTP Server
===========

The TFTP server hands out files to devices on the local network, for
example a fallback boot image for devices whose own firmware partition
has failed, or configuration files for IP phones and similar equipment.
It is read-only, so clients cannot upload files.

Files are served from a root directory, by default `/var/lib/tftpboot`.
This directory is persistent on all supported boards and writable by
admin users, so files can be placed there from the CLI or a shell.  A
directory on USB media, e.g., `/media/usb/tftp`, can be used instead.

> [!IMPORTANT]
> Only world-readable files are served.  Files copied with the CLI
> `copy` command are made world-readable automatically, files copied
> from a shell must be given mode `0644` or similar.


## Basic Configuration

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>set tftp enabled true</b>
admin@example:/config/> <b>leave</b>
</code></pre>

The server listens on all interfaces by default.  To restrict it to a
subset, list the interfaces to serve on:

<pre class="cli"><code>admin@example:/config/> <b>edit tftp</b>
admin@example:/config/tftp/> <b>set interface eth1</b>
admin@example:/config/tftp/> <b>set interface eth2</b>
admin@example:/config/tftp/> <b>leave</b>
</code></pre>

When the firewall is enabled, the `tftp` service must also be allowed
in the zone facing the clients, see [Firewall](firewall.md).


## Uploading Files

Files can be fetched to the TFTP root with the `copy` command from any
of the supported remote sources, or copied from USB media.  A directory
destination keeps the source file name:

<pre class="cli"><code>admin@example:/> <b>copy tftp://192.168.1.1/fallback.itb /var/lib/tftpboot/</b>
admin@example:/> <b>copy /media/usb/phones.cfg /var/lib/tftpboot/</b>
admin@example:/> <b>dir /var/lib/tftpboot</b>
/var/lib/tftpboot directory
fallback.itb   phones.cfg
</code></pre>

Files are removed with the `remove` command, which asks for
confirmation:

<pre class="cli"><code>admin@example:/> <b>remove /var/lib/tftpboot/phones.cfg</b>
Remove /var/lib/tftpboot/phones.cfg, are you sure? (y/N)? y
</code></pre>


## Per-Client Directories

Some devices, IP phones in particular, expect a configuration file with
a fixed name that differs per device.  With `client-directory` set, the
server first looks for the requested file in a subdirectory of the root
named after the client, and falls back to the root itself if there is
none:

<pre class="cli"><code>admin@example:/config/tftp/> <b>set client-directory mac</b>
</code></pre>

With this setting a request for `config.xml` from the device with MAC
address `00:11:22:33:44:55` is answered with
`/var/lib/tftpboot/00-11-22-33-44-55/config.xml` if that file exists,
otherwise with `/var/lib/tftpboot/config.xml`.  Use `ip` instead of
`mac` to name the directories after the client IP address.


## Network Boot

Devices that boot over the network learn the boot file name and TFTP
server address from the DHCP server.  See [Network Boot](dhcp.md#network-boot)
in the DHCP server documentation for how to hand these out.


## Monitoring

<pre class="cli"><code>admin@example:/> <b>show tftp</b>
Root directory   : /var/lib/tftpboot
Interfaces       : all
Client directory : none

<span class="header">NAME          SIZE  MODIFIED        </span>
fallback.itb  7.0M  2026-09-18 05:18
phones.cfg    812B  2026-09-17 12:00
</code></pre>

The file list is the operational view of the root directory and shows
only files the server can actually hand out.  A file missing from the
list is either not world-readable or outside the configured root.
