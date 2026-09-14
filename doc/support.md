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

Each command is run with a timeout, so a wedged driver or daemon cannot
stall the collection; the archive then holds a note in place of that
command's output. If the collection itself fails, the log is kept next
to the working directory, for instance:

```
/var/lib/support/support-host-2026-09-11T13:05:42+02:00.log
```

It shows what was collected and what failed. Use `support clean` to
remove old collection directories and logs.

## Collecting to a File

With `-o` the archive is written to a file instead of stdout, and the
path is printed:

```bash
admin@host:~$ sudo support collect -o /var/lib/support
...
/var/lib/support/support-host-2026-09-11T13:05:42+02:00.tar.gz
```

Given a directory, the file gets the canonical name shown above. Given a
file name, that name is used. Either way the file is created with mode
0600, since the archive contains password hashes and keys.

## Collecting over NETCONF or RESTCONF

Clients that only speak the management API can call the
`infix-system:support-collect` RPC, which runs the same collection and
returns the archive base64 encoded:

```bash
$ curl -ku admin:admin -X POST \
       -H "Content-Type: application/yang-data+json" \
       https://host/restconf/operations/infix-system:support-collect \
    | jq -r '."infix-system:output".data' | base64 -d > support-data.tar.gz
```

Add a password to get it encrypted, then decrypt it with the same
password after passing it on:

```bash
$ curl -ku admin:admin -X POST \
       -H "Content-Type: application/yang-data+json" \
       -d '{"infix-system:input":{"password":"mypassword"}}' \
       https://host/restconf/operations/infix-system:support-collect \
    | jq -r '."infix-system:output".data' | base64 -d > support-data.tar.gz.gpg
```

A few things to know about this path:

- The RPC is denied by default (`nacm:default-deny-all`), so only groups
  with an explicit NACM permit rule can call it. In the factory
  configuration that is the `admin` group.
- Collection runs in `/tmp`, and the archive is removed once it has been
  returned, so nothing is left behind on the device.
- An archive above 16 MiB is not returned inline. The reply then holds
  `size` and `filename` instead, and the file stays in `/tmp` for you to
  fetch and remove.
- The system log is tailed for 5 seconds, rather than the 30 the command
  line defaults to, so that the whole collection finishes inside the
  client's RPC timeout (`CONFD_TIMEOUT` in `/etc/default/confd`, 60
  seconds by default).
- Pass `password` to get the archive GPG encrypted, for handing on to
  someone else afterwards. The management session is already encrypted,
  so this is not needed to protect the transfer itself. The password
  reaches gpg on stdin and never appears in the process list. Devices
  built without gpg reject the request.
- `confd` is busy for the duration of the collection, like it is during a
  software upgrade, so a configuration change made at the same time has to
  wait for the collection to finish.
- If the client gives up before the collection finishes, the archive is
  discarded along with it, so call again rather than looking for a
  leftover file. On a device with many ports, where collection can
  outlast the 60 second timeout, collect over SSH with `-o` instead.
- A collection that fails leaves its log in the work directory, `/tmp`
  for this path, which is RAM and therefore cleared on reboot. Elsewhere
  use `support clean` to remove old logs and directories.

From a shell on the device, use the `support` command rather than the
RPC. A base64 blob on your terminal is of no use to anyone.

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
