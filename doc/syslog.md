# Syslog Support

The system comes with native logging to internal storage, `/var/log/`,
or `/log` for a shortcut.  Depending on the device, this may be a RAM
disk, meaning logs are not retained across reboots.  This document
details how to log to external media or remote syslog servers.

It is also possible to set up the device to act as a syslog server (log
sink), this is covered briefly at the very end of this document.

> [!NOTE]
> The default logging setup in the system cannot be modified, only the
> log file rotation.  Please see the `dir` admin-exec command for a
> listing of existing log files.

## Log to File

Logging to a local file is useful when combined with an external media.
E.g., a USB stick with a log partition (named/labeled: "log").  Below is
an example.

For a list of available log facilities, see the table in a later section.

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit syslog actions log-file file:/media/log/mylog</b>
admin@example:/config/syslog/…/file:/media/log/mylog/> <b>set facility-list</b>
      all    audit     auth authpriv  console     cron    cron2   daemon      ftp     kern
   local0   local1   local2   local3   local4   local5   local6   local7      lpr     mail
     news      ntp   syslog     user     uucp
admin@example:/config/syslog/…/file:/media/log/mylog/> <b>set facility-list all severity</b>
     alert       all  critical     debug emergency     error      info      none    notice   warning
admin@example:/config/syslog/…/file:/media/log/mylog/> <b>set facility-list all severity critical</b>
admin@example:/config/syslog/…/file:/media/log/mylog/> <b>set facility-list mail severity warning</b>
admin@example:/config/syslog/…/file:/media/log/mylog/> <b>leave</b>
admin@example:/>
</code></pre>

> [!IMPORTANT]
> The `log-file` syntax requires the leading prefix `file:`.  If the
> path is not absolute, e.g., `file:mylog`, the file is saved to the
> system default path, i.e., `/log/mylog`.  In this case, verify that
> the filename is not already in use.

## Log Rotation

By default log files are allowed to grow to a size of 1 MiB after which
they are "rotated".  The whole reason for this is to not fill up the
disk with outdated logs.  A rotated file is saved in stages and older
ones are also compressed (using `gzip`).  Use the `show log` command in
admin-exec context to start the log file viewer:

<pre class="cli"><code>admin@example:/config/syslog/> <b>do show log</b>
log  log.0  log.1.gz  log.2.gz  log.3.gz  log.4.gz  log.5.gz
admin@example:/config/syslog/> <b>do show log log.1.gz</b>
</code></pre>

> [!TIP]
> Use the Tab key on your keyboard list available log files.  The `do`
> prefix is also very useful in configure context to access commands
> from admin-exec context.

By default 10 compressed older files are saved.  Here the oldest is
`log.5.gz` and the most recently rotated one is `log.0`.

Log file rotation can be configured both globally and per log file.
Here we show the global settings, the set up is the same for per log
file, which if unset inherit the global settings:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit syslog file-rotation</b>
admin@example:/config/syslog/file-rotation/> <b>show</b>
admin@example:/config/syslog/file-rotation/>
</code></pre>

The defaults are not shown.  We can inspect them by asking the YANG
model for the help texts:

<pre class="cli"><code>admin@example:/config/syslog/file-rotation/> <b>help</b>
   max-file-size    number-of-files
admin@example:/config/syslog/file-rotation/> <b>help max-file-size</b>
<b>NAME</b>
        max-file-size kilobytes

<b>DESCRIPTION</b>
        Maximum log file size (kiB), before rotation.

<b>DEFAULT</b>
        1024
admin@example:/config/syslog/file-rotation/> <b>help number-of-files</b>
<b>NAME</b>
        number-of-files [0..4294967295]

<b>DESCRIPTION</b>
        Maximum number of log files retained.

<b>DEFAULT</b>
        10
</code></pre>

To change the defaults to something smaller, 512 kiB and 20 (remember
everything after .0 is compressed, and text compresses well):

<pre class="cli"><code>admin@example:/config/syslog/file-rotation/> <b>set max-file-size 512</b>
admin@example:/config/syslog/file-rotation/> <b>set number-of-files 20</b>
admin@example:/config/syslog/file-rotation/> <b>show</b>
number-of-files 20;
max-file-size 512;
admin@example:/config/syslog/file-rotation/> <b>leave</b>
admin@example:/>
</code></pre>

## Log Format

There are three major syslog log formats, the default is [RFC3164][] for
log files and BSD for remote logging.  Depending on time synchronization
and remote log server capabilities, or policies, the [RFC5424][] format
is often preferred since it not only has better time resolution but also
supports structured logging:

```
BSD     : myproc[8710]: Kilroy was here.
RFC3164 : Aug 24 05:14:15 192.0.2.1 myproc[8710]: Kilroy was here.
RFC5424 : 2003-08-24T05:14:15.000003-07:00 192.0.2.1 myproc 8710 - - Kilroy was here.
```

The BSD format is only applicable to remote logging.  It remains the
default for compatibility reasons, and is recommended since the device
may not have proper time, making it better for the remote log server to
perform time stamping at the time of arrival.

Configuring the log format is the same for log files and remotes:

<pre class="cli"><code>admin@example:/config/> <b>edit syslog actions log-file file:foobar</b>
admin@example:/config/syslog/…/file:foobar/> <b>set log-format</b>
                  bsd               rfc3164              rfc5424
admin@example:/config/syslog/…/file:foobar/> <b>set log-format rfc5424</b>
admin@example:/config/syslog/…/file:foobar/> <b>leave</b>
admin@example:/>
</code></pre>

## Log to Remote Server

Logging to a remote syslog server is the recommended way of supervising
the system.  This way all login attempts (console, SSH, or web) and any
configuration changes can be traced, even in cases of a remote attacker
tries to cover their traces by deleting logs.

The recommended setup involves using a fixed IP address, default BSD log
format, and the default Internet port (514).  This is the most reliable,
because your device may not have DNS set up or even available, and some
remote syslog servers do not support receiving time stamped log messages
-- this is of course entirely dependent on how the remote server is set
up, as well as local policy.

<pre class="cli"><code>admin@example:/config/> <b>edit syslog</b>
       actions file-rotation        server
admin@example:/config/> <b>edit syslog actions destination moon</b>
admin@example:/config/syslog/…/moon/> <b>set</b>
 facility-list    log-format           udp
admin@example:/config/syslog/…/moon/> <b>set udp</b>
 address    port
admin@example:/config/syslog/…/moon/> <b>set udp address 192.168.0.12</b>
admin@example:/config/syslog/…/moon/> <b>set facility-list container severity all</b>
admin@example:/config/syslog/…/moon/> <b>leave</b>
admin@example:/>
</code></pre>

> [!TIP]
> The alternatives shown below each prompt in the example above can be
> found by tapping the Tab key.

## Acting as a Log Server

The syslog server can act as a log sink for other devices on a LAN.  For
this to work you need a static IP address, here we use 10.0.0.1/24.

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit syslog server</b>
admin@example:/config/syslog/server/> <b>set enabled true</b>
admin@example:/config/syslog/server/> <b>set listen udp 514 address 10.0.0.1</b>
admin@example:/config/syslog/server/> <b>leave</b>
admin@example:/>
</code></pre>

See the above [Log to File](#log-to-file) section on how to set up
filtering of received logs to local files.  Advanced filtering based
on hostname and message properties is also available, see the next
section for details.

## Advanced Filtering

The syslog subsystem supports several advanced filtering options that
allow fine-grained control over which messages are logged.  These can
be combined with facility and severity filters to create sophisticated
logging rules.

### Pattern Matching

Messages can be filtered using regular expressions (POSIX extended regex)
on the message content.  This is useful when you want to log only messages
containing specific keywords or patterns:

<pre class="cli"><code>admin@example:/config/> <b>edit syslog actions log-file file:errors</b>
admin@example:/config/syslog/…/file:errors/> <b>set pattern-match "ERROR|CRITICAL|FATAL"</b>
admin@example:/config/syslog/…/file:errors/> <b>set facility-list all severity info</b>
admin@example:/config/syslog/…/file:errors/> <b>leave</b>
admin@example:/>
</code></pre>

This will log all messages containing ERROR, CRITICAL, or FATAL.

### Advanced Severity Comparison

By default, severity filtering uses "equals-or-higher" comparison,
meaning a severity of `error` will match error, critical, alert, and
emergency messages.  You can change this behavior:

<pre class="cli"><code>admin@example:/config/> <b>edit syslog actions log-file file:daemon-errors</b>
admin@example:/config/syslog/…/file:daemon-errors/> <b>set facility-list daemon</b>
admin@example:/config/syslog/…/daemon/> <b>set severity error</b>
admin@example:/config/syslog/…/daemon/> <b>set advanced-compare compare equals</b>
admin@example:/config/syslog/…/daemon/> <b>leave</b>
admin@example:/>
</code></pre>

This will log only `error` severity messages, not higher severities.

You can also block specific severities:

<pre class="cli"><code>admin@example:/config/syslog/…/daemon/> <b>set advanced-compare action block</b>
</code></pre>

This will exclude `error` messages from the log.

### Hostname Filtering

When acting as a log server, you can filter messages by hostname.  This
is useful for directing logs from different devices to separate files:

<pre class="cli"><code>admin@example:/config/> <b>edit syslog actions log-file file:router1</b>
admin@example:/config/syslog/…/file:router1/> <b>set hostname-filter router1</b>
admin@example:/config/syslog/…/file:router1/> <b>set facility-list all severity info</b>
admin@example:/config/syslog/…/file:router1/> <b>leave</b>
admin@example:/>
</code></pre>

Multiple hostnames can be added to the filter list.

### Property-Based Filtering

For more advanced filtering, you can match on specific message properties
using various comparison operators:

<pre class="cli"><code>admin@example:/config/> <b>edit syslog actions log-file file:myapp</b>
admin@example:/config/syslog/…/file:myapp/> <b>edit property-filter</b>
admin@example:/config/syslog/…/property-filter/> <b>set property programname</b>
admin@example:/config/syslog/…/property-filter/> <b>set operator isequal</b>
admin@example:/config/syslog/…/property-filter/> <b>set value myapp</b>
admin@example:/config/syslog/…/property-filter/> <b>leave</b>
admin@example:/>
</code></pre>

Available properties:
- `msg`: Message body
- `msgid`: RFC5424 message identifier
- `programname`: Program/tag name
- `hostname`: Source hostname
- `source`: Alias for hostname
- `data`: RFC5424 structured data

Available operators:
- `contains`: Substring match
- `isequal`: Exact equality
- `startswith`: Prefix match
- `regex`: Basic regular expression
- `ereregex`: Extended regular expression (POSIX ERE)

The comparison can be made case-insensitive:

<pre class="cli"><code>admin@example:/config/syslog/…/property-filter/> <b>set case-insensitive true</b>
</code></pre>

Or negated to exclude matching messages:

<pre class="cli"><code>admin@example:/config/syslog/…/property-filter/> <b>set negate true</b>
</code></pre>

### Facilities

| **Code** | **Facility** | **Description**                           |
|----------|--------------|-------------------------------------------|
| 0        | kern         | Kernel log messages                       |
| 1        | user         | User-level messages                       |
| 2        | mail         | Mail system                               |
| 3        | daemon       | General system daemons                    |
| 4        | auth         | Security/authorization messages           |
| 5        | syslog       | Messages generated by syslogd             |
| 6        | lpr          | Line printer subsystem                    |
| 7        | news         | Network news subsystem                    |
| 8        | uucp         | UNIX-to-UNIX copy                         |
| 9        | cron         | Clock/cron daemon (BSD, Linux)            |
| 10       | authpriv     | Security/authorization messages (private) |
| 11       | ftp          | FTP daemon                                |
| 12       | ntp          | NTP subsystem                             |
| 13       | audit        | Log audit (security)                      |
| 14       | console      | Log alert                                 |
| 15       | cron2        | Clock/cron daemon (Solaris)               |
| 16       | rauc*        | local0, reserved for RAUC                 |
| 17       | container*   | local1, reserved for containers           |
| 18       | local2       | Currently unused                          |
| 19       | local3       | Currently unused                          |
| 20       | local4       | Currently unused                          |
| 21       | local5       | Currently unused                          |
| 22       | reserved*    | local6, reserved for industrial Ethernet  |
| 23       | web*         | local7, reserved for nginx web server     |

Facilities marked `*` are local augments to the model.

[RFC3164]: https://datatracker.ietf.org/doc/html/rfc3164
[RFC5424]: https://datatracker.ietf.org/doc/html/rfc5424
