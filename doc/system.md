# System Configuration

System settings in Infix are provided by the [ietf-system][1] YANG
model, augmented with Linux specific extensions in [infix-system][2],
like Message of the Day (login message) and user login shell.  More
on this later on in this document.

For the sake of brevity, the hostname in the following examples has been
shortened to `host`.  The default hostname is composed from a product
specific string followed by the last three octets of the system base MAC
address, e.g., `switch-12-34-56`.  An example of how to change the
hostname is included below.

> **Note:** when issuing `leave` to activate your changes, remember to
> also save your settings, `copy running-config startup-config`.  See
> the [CLI Introduction](cli/introduction.md) for a background.


## Changing Password

User management, including passwords, SSH keys, remote authentication is
available in the system authentication configuration context.

```
admin@host:/config/> edit system authentication user admin
admin@host:/config/system/authentication/user/admin/> change password
New password: 
Retype password: 
admin@host:/config/system/authentication/user/admin/> leave
```

The `change password` command starts an interactive dialogue that asks
for the new password, with a confirmation, and then salts and encrypts
the password with sha512crypt. 

It is also possible to use the `set password ...` command.  This allows
setting an already hashed password.  To manually hash a password, use
the `do password encrypt` command.  This launches the admin-exec command
to hash, and optionally salt, your password.  This encrypted string can
then be used with `set password ...`.

> **Tip:** if you are having trouble thinking of a password, Infix has a
> `password generate` command in admin-exec context which generates
> random passwords using the UNIX command `pwgen`.  Use the `do` prefix
> when inside any configuration context to access admin-exec commands.


### SSH Authorized Key

Logging in remotely with SSH is possible by adding a *public key* to a
user.  Here we add the authorized key to the admin user, multiple keys
are supported.

With SSH keys in place it is possible to disable password login, just
remember to verify SSH login and network connectivity before doing so.

```
admin@host:/config/> edit system authentication user admin 
admin@host:/config/system/authentication/user/admin/> edit authorized-key example@host
admin@host:/config/system/authentication/user/admin/authorized-key/example@host/> set algorithm ssh-rsa
admin@host:/config/system/authentication/user/admin/authorized-key/example@host/> set key-data AAAAB3NzaC1yc2EAAAADAQABAAABgQC8iBL42yeMBioFay7lty1C4ZDTHcHyo739gc91rTTH8SKvAE4g8Rr97KOz/8PFtOObBrE9G21K7d6UBuPqmd0RUF2CkXXN/eN2PBSHJ50YprRFt/z/304bsBYkDdflKlPDjuSmZ/+OMp4pTsq0R0eNFlX9wcwxEzooIb7VPEdvWE7AYoBRUdf41u3KBHuvjGd1M6QYJtbFLQMMTiVe5IUfyVSZ1RCxEyAB9fR9CBhtVheTVsY3iG0fZc9eCEo89ErDgtGUTJK4Hxt5yCNwI88YaVmkE85cNtw8YwubWQL3/tGZHfbbQ0fynfB4kWNloyRHFr7E1kDxuX5+pbv26EqRdcOVGucNn7hnGU6C1+ejLWdBD7vgsoilFrEaBWF41elJEPKDzpszEijQ9gTrrWeYOQ+x++lvmOdssDu4KvGmj2K/MQTL2jJYrMJ7GDzsUu3XikChRL7zNfS2jYYQLzovboUCgqfPUsVba9hqeX3U67GsJo+hy5MG9RSry4+ucHs=
admin@host:/config/system/authentication/user/admin/authorized-key/example@host/> show
algorithm ssh-rsa;
key-data AAAAB3NzaC1yc2EAAAADAQABAAABgQC8iBL42yeMBioFay7lty1C4ZDTHcHyo739gc91rTTH8SKvAE4g8Rr97KOz/8PFtOObBrE9G21K7d6UBuPqmd0RUF2CkXXN/eN2PBSHJ50YprRFt/z/304bsBYkDdflKlPDjuSmZ/+OMp4pTsq0R0eNFlX9wcwxEzooIb7VPEdvWE7AYoBRUdf41u3KBHuvjGd1M6QYJtbFLQMMTiVe5IUfyVSZ1RCxEyAB9fR9CBhtVheTVsY3iG0fZc9eCEo89ErDgtGUTJK4Hxt5yCNwI88YaVmkE85cNtw8YwubWQL3/tGZHfbbQ0fynfB4kWNloyRHFr7E1kDxuX5+pbv26EqRdcOVGucNn7hnGU6C1+ejLWdBD7vgsoilFrEaBWF41elJEPKDzpszEijQ9gTrrWeYOQ+x++lvmOdssDu4KvGmj2K/MQTL2jJYrMJ7GDzsUu3XikChRL7zNfS2jYYQLzovboUCgqfPUsVba9hqeX3U67GsJo+hy5MG9RSry4+ucHs=;
admin@host:/config/system/authentication/user/admin/authorized-key/example@host/> leave
```

> **Note:** the `ssh-keygen` program already base64 encodes the public
> key data, so there is no need to use the `text-editor` command, `set`
> does the job.


## Multiple Users

The system supports multiple users and has by default three user levels,
or groups, that a user can be a member of.  Access control is handled by
["NACM"][3], which provides granular access to configuration, data, and
RPC commands over NETCONF.

By default the system comes with three user groups: guest, operator, and
admin.  The default user `admin` is by default part of the group `admin`
and is granted full permissions to the system.  There is no restrictions
on the number of users with administrator privileges, nor is the `admin`
user reserved or protected in any way -- it is completely possible to
remove it from the configuration.  However, it is recommended to keep at
least one user with administrator privileges in the system, otherwise
the only way to regain full access is to perform a *factory reset*.

### Adding a User

Similar to how to change password, adding a new user is done using the
same set of commands:

```
admin@host:/config/> edit system authentication user jacky
admin@host:/config/system/authentication/user/jacky/> change password
New password:
Retype password:
admin@host:/config/system/authentication/user/jacky/> leave
```

An authorized SSH key is added the same way as presented previously.

### Adding a User to the Admin Group

The following commands add user `jacky` to the `admin` group.

```
admin@host:/config/> edit nacm group admin
admin@host:/config/nacm/group/admin/> set user-name jacky
admin@host:/config/nacm/group/admin/> leave
```

### Security Aspects

The three default user levels apply primarily to NETCONF, with exception
of the `admin` group which is granted full access to the underlying UNIX
system with the following ACL rules:

```json
   ...
   "module-name": "*",
   "access-operations": "*",
   "action": "permit",
   ...
```

A user in the `admin` group is allowed to also use a POSIX login shell.


## Changing Hostname

Notice how the hostname in the prompt does not change until the change
is committed by issuing the `leave` command.

```
admin@host:/config/> edit system
admin@host:/config/system/> set hostname example
admin@host:/config/system/> leave
admin@example:/> 
```

The hostname is advertised over mDNS-SD in the `.local` domain.  If
another device already has claimed the `example.local` CNAME, in our
case, mDNS will advertise a "uniqified" variant, usually suffixing with
an index, e.g., `example-1.local`.  Use an mDNS browser to scan for
available devices on your LAN.

> **Note:** critical services like syslog, mDNS, LLDP, and similar that
> advertise the hostname, are restarted when the hostname is changed.


## Changing Login Banner

The `motd-banner` setting is an Infix augment and an example of a
`binary` type setting that can be changed interactively with the
built-in [`text-editor` command](cli/text-editor.md).

> **Tip:** see the next section for how to change the editor used
> to something you may be more familiar with.

```
admin@host:/config/> edit system
admin@host:/config/system/> text-editor motd-banner
admin@host:/config/system/> leave
admin@host:/>
```

Log out and log back in again to inspect the changes.


## Changing the Editor

The system has three different built-in editors that can be used
as the `text-editor` command:

 - `emacs` (Micro Emacs)
 - `nano` (GNU Nano)
 - `vi` (Visual Editor)

To change the editor to GNU Nano:

```
admin@host:/> configure
admin@host:/config/> edit system
admin@host:/config/system/> set text-editor nano
admin@host:/config/system/> leave
admin@host:/>
```

> **Note:** as usual, configuration changes only take effect after
> issuing the `leave` command.  I.e., you must change the editor first,
> and then re-enter configure context to use your editor of choice.


[1]: https://www.rfc-editor.org/rfc/rfc7317
[2]: https://github.com/kernelkit/infix/blob/main/src/confd/yang/infix-system%402024-02-29.yang
[3]: https://www.rfc-editor.org/rfc/rfc8341
