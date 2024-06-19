Configuration Daemon
====================

`confd` is the Infix configuration daemon that serves as the glue
between sysrepo and netopeer2 (NETCONF) and the UNIX system under
it all.


Factory & Failure Config
------------------------

Infix supports both static and dynamically generated factory-config.  At
boot the dynamic is always generated to `/run/confd/factory-config.gen`,
which is installed to `/etc/factory-config.cfg` and used by default, if
that file does not already exist.  The same applies to the failure mode
configuration.

The following describes how a vendor/product specific config is found
and installed into `/etc/factory-config.cfg` before the dynamic one is
installed.

 1. `/etc/factory-config.cfg`: built into the image, e.g., r2s
 2. `/usr/share/product/<PRODUCT>/etc/factory-config.cfg`, where the
    `<PRODUCT>` is determined from the VPD, which is available after
	`probe` has run, in `/run/system.json` as `"product-name"`.  The
	lower case version of the string is used

In the second option a script running just after `probe` will in fact
`cp` the complete product specific directory to `/`, meaning any file
and directory that is writable at runtime can be overloaded with device
specific versions.


Origin & References
-------------------

Based on the Open Source [dklibc/sysrepo_plugin_ietf_system][0]
project by Denis Kalashnikov.

[0]: https://github.com/dklibc/sysrepo_plugin_ietf_system
