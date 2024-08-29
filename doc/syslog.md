## Syslog Support

The system comes with native logging to internal storage, `/var/log/`,
or `/log` for a shortcut.  Depending on the device, this may be a RAM
disk, meaning logs are not retained across reboots.  This document
details how to log to external media or remote syslog servers.

It is also possible to set up the device to act as a syslog server (log
sink), this is covered briefly at the very end of this document.

> **Note:** the native logging cannot be modified, only the log file
> rotation can be changed.  Please see the `dir` admin-exec command for
> a listing of existing native log files.


### Log to File

Logging to a local file is useful when combined with an external media.
E.g., a USB stick with a log partition (named/labeled: "log").  Below is
an example.

For a list of available log facilities, see the table in a later section.

```bash
admin@example:/> configure
admin@example:/config/> edit syslog
admin@example:/config/syslog/> edit actions log-file file:/media/log/mylog
admin@example:/config/syslog/actions/log-file/file:/media/log/mylog/> set facility-list
      all    audit     auth authpriv  console     cron    cron2   daemon      ftp     kern
   local0   local1   local2   local3   local4   local5   local6   local7      lpr     mail
     news      ntp   syslog     user     uucp
admin@example:/config/syslog/actions/log-file/file:/media/log/mylog/> set facility-list all severity
     alert       all  critical     debug emergency     error      info      none    notice   warning
admin@example:/config/syslog/actions/log-file/file:/media/log/mylog/> set facility-list all severity critical
admin@example:/config/syslog/actions/log-file/file:/media/log/mylog/> set facility-list mail severity warning
admin@example:/config/syslog/actions/log-file/file:/media/log/mylog/> leave
admin@example:/>
```

> **Note:** the `log-file` syntax requires the leading prefix `file:`.
> If the path is not absolute, e.g., `file:mylog`, the file is saved to
> the system default path, i.e., `/log/mylog`.  In this case, verify
> that the filename is not already in use.


### Log Rotation

By default log files are allowed to grow to a size of 1 MiB after which
they are "rotated".  The whole reason for this is to not fill up the
disk with outdated logs.  A rotated file is saved in stages and older
ones are also compressed (using `gzip`).  Use the `show log` command in
admin-exec context to start the log file viewer:

    admin@example:/config/syslog/> do show log
    log  log.0  log.1.gz  log.2.gz  log.3.gz  log.4.gz  log.5.gz
    admin@example:/config/syslog/> do show log log.1.gz

> **Tip:** use the Tab key on your keyboard list available log files.
> The `do` prefix is also very useful in configure context to access
> commands from admin-exec context.

By default 10 compressed older files are saved.  Here the oldest is
`log.5.gz` and the most recently rotated one is `log.0`.

Log file rotation can be configured both globally and per log file.
Here we show the global settings, the set up is the same for per log
file, which if unset inherit the global settings:

```bash
admin@example:/> configure 
admin@example:/config/> edit syslog file-rotation
admin@example:/config/syslog/file-rotation/> show
admin@example:/config/syslog/file-rotation/>
```

The defaults are not shown.  We can inspect them by asking the YANG
model for the help texts:

```bash
admin@example:/config/syslog/file-rotation/> help
   max-file-size    number-of-files
admin@example:/config/syslog/file-rotation/> help max-file-size 
NAME
        max-file-size kilobytes

DESCRIPTION
        Maximum log file size (kiB), before rotation.

DEFAULT
        1024
admin@example:/config/syslog/file-rotation/> help number-of-files 
NAME
        number-of-files [0..4294967295]

DESCRIPTION
        Maximum number of log files retained.

DEFAULT
        10
```

To change the defaults to something smaller, 512 kiB and 20 (remember
everything after .0 is compressed, and text compresses well):

```bash
admin@example:/config/syslog/file-rotation/> set max-file-size 512
admin@example:/config/syslog/file-rotation/> set number-of-files 20
admin@example:/config/syslog/file-rotation/> show
number-of-files 20;
max-file-size 512;
admin@example:/config/syslog/file-rotation/> leave
admin@example:/> 
```


### Log Format

There are three major syslog log formats, the default is [RFC3164][] for
log files and BSD for remote logging.  Depending on time synchronization
and remote log server capabilities, or policies, the [RFC5424][] format
is often preferred since it not only has better time resolution but also
supports structured logging:

	BSD     : myproc[8710]: Kilroy was here.
	RFC3164 : Aug 24 05:14:15 192.0.2.1 myproc[8710]: Kilroy was here.
	RFC5424 : 2003-08-24T05:14:15.000003-07:00 192.0.2.1 myproc 8710 - - Kilroy was here.

The BSD format is only applicable to remote logging.  It remains the
default for compatibility reasons, and is recommended since the device
may not have proper time, making it better for the remote log server to
perform time stamping at the time of arrival.

Configuring the log format is the same for log files and remotes:

```bash
admin@example:/config/> edit syslog actions log-file file:foobar 
admin@example:/config/syslog/actions/log-file/file:foobar/> set log-format 
                  bsd               rfc3164              rfc5424
admin@example:/config/syslog/actions/log-file/file:foobar/> set log-format rfc5424 
admin@example:/config/syslog/actions/log-file/file:foobar/> leave
admin@example:/>
```

[RFC3164]: https://datatracker.ietf.org/doc/html/rfc3164
[RFC5424]: https://datatracker.ietf.org/doc/html/rfc5424

### Log to Remote Server

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

```bash
admin@example:/config/> edit syslog
       actions file-rotation        server
admin@example:/config/> edit syslog actions destination moon
admin@example:/config/syslog/actions/destination/moon/> set
 facility-list    log-format           udp
admin@example:/config/syslog/actions/destination/moon/> set udp
 address    port
admin@example:/config/syslog/actions/destination/moon/> set udp address 192.168.0.12
admin@example:/config/syslog/actions/destination/moon/> set facility-list container severity all
admin@example:/config/syslog/actions/destination/moon/> leave
admin@example:/>
```

> **Note:** the alternatives shown below each prompt in the example
> above can be found by tapping the Tab key.


### Acting as a Log Server

The syslog server can act as a log sink for other devices on a LAN.  For
this to work you need a static IP address, here we use 10.0.0.1/24.

```bash
admin@example:/> configure
admin@example:/config/> edit syslog server
admin@example:/config/syslog/server/> set enabled true
admin@example:/config/syslog/server/> set listen udp 514 address 10.0.0.1
admin@example:/config/syslog/server/> leave
admin@example:/>
```

See the above [Log to File](#log-to-file) section on how to set up
filtering of received logs to local files.  Please note, filtering based
on property, e.g., hostname, is not supported yet.


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
