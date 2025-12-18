# Support Data Collection

When troubleshooting issues or seeking support, the `support` command
provides a convenient way to collect comprehensive system diagnostics.
This command gathers configuration files, logs, network state, and other
system information into a single compressed archive.

> [!NOTE]
> The `support collect` command should be run with `sudo` to collect
> complete system information (kernel logs, hardware details, etc.).
> Use the `--unprivileged` option to run as a regular user in degraded
> data collection mode.

## Collecting Support Data

To collect support data and save it to a file:

```bash
admin@host:~$ sudo support collect > support-data.tar.gz
Starting support data collection from host...
Collecting to: /var/lib/support
This may take up to a minute. Please wait...
Tailing /var/log/messages for 30 seconds (please wait)...
Log tail complete.
Collection complete. Creating archive...
admin@host:~$ ls -l support-data.tar.gz
-rw-rw-r-- 1 admin admin 508362 nov 30 13:05 support-data.tar.gz
```

The command can also be run remotely via SSH from your workstation:

```bash
$ ssh admin@host 'sudo support collect' > support-data.tar.gz
...
```

The collection process may take up to a minute depending on system load
and the amount of logging data. Progress messages are shown during the
collection process.

## Encrypted Collection

For secure transmission of support data, the archive can be encrypted
with GPG using a password:

```bash
admin@host:~$ sudo support collect -p mypassword > support-data.tar.gz.gpg
Starting support data collection from host...
Collecting to: /var/lib/support
This may take up to a minute. Please wait...
...
Collection complete. Creating archive...
Encrypting with GPG...
```

The `support collect` command even supports omitting `mypassword` and
will then prompt interactively for the password.  This works over SSH too,
but the local ssh client may then echo the password.

> [!TIP]
> To hide the encryption password for an SSH session, the script supports
> reading from stdin:
> `echo "$MYSECRET" | ssh user@device 'sudo support collect -p' >
> file.tar.gz.gpg`

After transferring the resulting file to your workstation, decrypt it
with the password:

```bash
$ gpg -d support-data.tar.gz.gpg > support-data.tar.gz
$ tar xzf support-data.tar.gz
...
```

or

```bash
$ gpg -d support-data.tar.gz.gpg | tar xz
...
```

> [!IMPORTANT]
> Make sure to share `mypassword` out-of-band from the encrypted data
> with the recipient of the data.  I.e., avoid sending both in the same
> plain-text email for example.

## What is Collected

The support archive includes:

- System identification (hostname, uptime, kernel version)
- Running and operational configuration (sysrepo datastores)
- System logs (`/var/log` directory and live tail of messages log)
- Network configuration and state (interfaces, routes, neighbors, bridges)
- FRRouting information (OSPF, BFD status)
- Container information (podman containers and their configuration)
- System resource usage (CPU, memory, disk, processes)
- Hardware information (PCI, USB devices, network interfaces)
