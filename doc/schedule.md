# Scheduling

Some Infix features run on a recurring calendar instead of on demand.
The recurrence itself lives in one place: a *named schedule*, which any
number of features can point at.

A schedule has no action of its own.  It only says when something should
happen, and the feature referencing it decides what happens.  Two features
can share the same schedule.

YANG support is defined in [infix-schedule][1], which augments
`ietf-system` with a `schedules` container and builds on the iCalendar
recurrence grouping from [ietf-schedule][2] (RFC 9922).


## Creating a Schedule

A schedule needs a name and a recurrence rule.  The example below fires
every night at 03:30.

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit system schedule nightly</b>
admin@example:/config/system/schedule/nightly/> <b>set description "Nightly maintenance window"</b>
admin@example:/config/system/schedule/nightly/> <b>set recurrence frequency daily</b>
admin@example:/config/system/schedule/nightly/> <b>set recurrence byhour 3</b>
admin@example:/config/system/schedule/nightly/> <b>set recurrence byminute 30</b>
admin@example:/config/system/schedule/nightly/> <b>leave</b>
</code></pre>

**Schedule parameters:**

- `name`: Unique identifier, 1-64 characters, starting with a letter or
  digit and otherwise limited to letters, digits, `_`, `.` and `-`.  The
  name is used verbatim by features referencing it
- `enabled`: Turn the schedule on or off (default: `true`).  When off,
  everything that uses it stops running, but the schedule is kept
- `description`: Optional human-readable note on the schedule's purpose
- `recurrence`: The recurrence rule.  A schedule without one is rejected
  at commit time


## Recurrence Rules

Schedules run in the system's local time.

`frequency` is mandatory and selects the base period:

| Frequency  | Fires                              |
|------------|------------------------------------|
| `minutely` | Every minute                       |
| `hourly`   | Every hour, on the hour            |
| `daily`    | Every day at midnight              |
| `weekly`   | Every week                         |
| `monthly`  | The 1st of every month at midnight |
| `yearly`   | January 1st at midnight            |

`interval` (default `1`) stretches the base period: `frequency hourly`
with `interval 6` fires every six hours.

The remaining fields refine that period by pinning one field to specific
values:

- `byminute`: Minutes within the hour, 0-59
- `byhour`: Hours of the day, 0-23
- `byday`: Days of the week, by `weekday` name (`monday` … `sunday`)
- `bymonthday`: Days of the month, 1-31
- `byyearmonth`: Months of the year, 1-12

Each accepts a list, so `byhour 8` plus `byhour 20` fires twice a day.

> [!TIP]
> Set `frequency` to the coarsest period you want, then refine it with the
> `by*` fields.  A weekly window on Sunday mornings is `frequency weekly`
> with `byday sunday` and `byhour 4`.  Writing the same window as
> `frequency daily` would fire every morning.


## Limitations

Infix turns each schedule into a five-field cron expression, and the YANG
model is pruned to the subset cron can express.  Everything below is
rejected at commit time, so a schedule never fires on the wrong days:

- **`secondly` frequency.**  Cron has no seconds field; the finest
  supported resolution is `minutely`
- **Combining `bymonthday` and `byday`.**  Cron fires on the *union* of
  day-of-month and day-of-week, where RFC 5545 specifies their
  intersection, so the combination is refused
- **Negative values.**  "The last Monday of the month" (`byday` with a
  direction) and "the last day of the month" (`bymonthday -1`) have no
  cron equivalent
- **Start and end bounds.**  There is no start anchor, no `until` date and
  no occurrence count.  A schedule recurs until it is disabled
- **Per-schedule time zones**, day-of-year, week-of-year and set-position

`frequency yearly` with an `interval` above 1 ("every other year") is also
not expressible; the interval is ignored in that case.


## Using a Schedule

Features reference a schedule through a leaf of type `schedule-ref`.  The
reference is validated, so a schedule cannot be deleted while something
still uses it, and a typo shows up at commit time instead of at the next
occurrence.

These features consume schedules today:

| Feature                    | Configuration path                  |
|----------------------------|-------------------------------------|
| Reboot on a schedule       | `system scheduled-reboot`           |
| Update checks              | `system software check-update`      |
| [Unattended updates][3]    | `system software unattended-update` |

The example below reboots the system on the `nightly` schedule created
above.  Note that `scheduled-reboot` has no `enabled` leaf.  It is active
as soon as it references a schedule; remove the reference or disable the
schedule to stop it.

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>set system scheduled-reboot schedule nightly</b>
admin@example:/config/> <b>leave</b>
</code></pre>


## Verifying

Active schedules become cron jobs owned by the `admin` user.  Infix starts
the cron daemon when at least one job is active and stops it when none
are.  To confirm a schedule took effect, look at the generated crontab
from the shell:

```sh
admin@example:~$ crontab -l
# Managed by infix-schedule
30 3 * * *	/usr/sbin/reboot
```

An empty crontab means nothing is scheduled.  Check that the consuming
feature is enabled, that it names the schedule correctly, and that the
schedule itself is enabled.

> [!NOTE]
> The crontab is generated and must not be edited by hand.  It is
> rewritten from the configuration on every change.

[1]: https://github.com/kernelkit/infix/blob/main/src/confd/yang/confd/infix-schedule.yang
[2]: https://www.rfc-editor.org/rfc/rfc9922
[3]: upgrade.md#unattended-updates
