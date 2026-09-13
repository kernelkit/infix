# Support Data Collection

When troubleshooting issues or seeking support, the `support` command
provides a convenient way to collect comprehensive system diagnostics.
This command gathers configuration files, logs, network state, and other
system information into a single compressed archive.

> [!NOTE]
> `support collect` needs root for kernel logs, hardware details and the
> full configuration, so run it with `sudo`. Without root it refuses;
> `--unprivileged` lets it run anyway and collect what your user may
> read, the rest is noted as missing in the archive.

## Collecting Support Data

On the device, collect to a file with `-o`. Progress goes to stderr and
the path of the archive is the only thing printed on stdout:

```bash
admin@host:~$ sudo support collect -o /var/lib/support
Starting support data collection from host...
Collecting to: /var/lib/support
This may take up to a minute. Please wait...
Tailing /var/log/messages for 30 seconds (please wait)...
Log tail complete.
Collection complete. Creating archive...
/var/lib/support/support-host-2026-09-11T13:05:42+02:00.tar.gz
```

Given a directory, the file gets the canonical name shown above. Given a
file name, that name is used. Either way the file is created with mode
0600. Secrets are redacted from the configuration, see below, but the
archive still holds every log on the device. Fetch it with `scp` and
remove it, or leave that to `support clean`.

Without `-o` the archive goes to stdout, which is what you want when
running the command from your workstation over SSH:

```bash
$ ssh admin@host 'sudo support collect' > support-data.tar.gz
...
```

On the device itself, prefer `-o`. A session that drops mid-way then
leaves the archive behind rather than taking the only copy with it.

The collection may take up to a minute depending on system load and the
amount of logging data.

Each command is run with a timeout, so a wedged driver or daemon cannot
stall the collection; the archive then holds a note in place of that
command's output. The logs are held in the collection and again in the
archive beside it, so collection measures them first and refuses when
the partition cannot hold both, rather than fill it. If the collection
itself fails, the log is kept next to the working directory, for
instance:

```
/var/lib/support/support-host-2026-09-11T13:05:42+02:00.log
```

It shows what was collected and what failed. Use `support clean` to
remove old collection directories and logs.


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
  with an explicit NACM permit rule can call it.
- Collection runs in `/var/lib/support`, like the `support` command,
  and the archive is removed once it has been returned, so a successful
  call leaves nothing behind on the device.
- An archive above 16 MiB is not returned inline. The reply then holds
  `size` and `filename` instead, and the file stays in `/var/lib/support`
  for you to fetch.
- Over NETCONF the archive arrives as one base64 text node, and libxml2
  rejects text nodes over 10 MB unless the parser is opened with
  `XML_PARSE_HUGE`. Clients built on it, lxml and ncclient among them,
  need that option (`huge_tree=True`) to receive an archive above about
  7.5 MB. RESTCONF returns JSON and has no such limit.
- The system log is tailed for 5 seconds, rather than the 30 the command
  line defaults to, so that the whole collection finishes inside the
  client's RPC timeout (`CONFD_TIMEOUT` in `/etc/default/confd`, 60
  seconds by default).
- Pass `password` to get the archive GPG encrypted, for handing on to
  someone else afterwards. The management session is already encrypted,
  so this is not needed to protect the transfer itself. The password
  must be a single line. It is handed to gpg on a private file
  descriptor and never appears in the process list. Devices built
  without the `BR2_PACKAGE_SUPPORT_ENCRYPT` option have no gpg and reject
  the request.
- `confd` is busy for the duration of the collection, like it is during a
  software upgrade, so a configuration change made at the same time has to
  wait for the collection to finish.
- The RPC has 60 seconds to finish. If the collection takes longer, the
  call fails with a timeout and nothing is kept, so there is no file to
  look for afterwards. On a device where that happens, with many ports
  or a lot of logging, log in over SSH and collect to a file instead:

  ```bash
  $ ssh admin@host 'sudo support collect -o /var/lib/support'
  /var/lib/support/support-host-2026-09-11T13:05:42+02:00.tar.gz
  $ scp admin@host:/var/lib/support/support-host-2026-09-11T13:05:42+02:00.tar.gz .
  ```
- A collection that fails keeps its log in `/var/lib/support`. The error
  message says why and names the log.
- Each call removes what earlier calls left behind once it is a week
  old, the same as `support clean` does.

From a shell on the device, use the `support` command rather than the
RPC. A base64 blob on your terminal is of no use to anyone.

## Encrypted Collection

For secure transmission of support data, the archive can be encrypted
with GPG using a password. This needs gpg on the device, which the
`BR2_PACKAGE_SUPPORT_ENCRYPT` build option adds.

```bash
admin@host:~$ sudo support collect -p mypassword -o /var/lib/support
Starting support data collection from host...
Collecting to: /var/lib/support
This may take up to a minute. Please wait...
...
Collection complete. Creating archive...
Encrypting with GPG...

WARNING: Remember to share the encryption password out-of-band!
         Do not send it in the same email as the encrypted file.
/var/lib/support/support-host-2026-09-11T13:05:42+02:00.tar.gz.gpg
```

Given a directory, `-o` appends `.gpg` to the canonical name. The
password may be left out, the command then prompts for it. That works
over SSH too, but the local ssh client may echo what you type, so pipe
it on stdin instead:

```bash
$ echo "$MYSECRET" | ssh admin@host 'sudo support collect -p' > support-data.tar.gz.gpg
```

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
