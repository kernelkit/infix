Upgrading the Software
----------------------

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
