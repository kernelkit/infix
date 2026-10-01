# Scheduling

Some things happen on a timer rather than on demand: checking for new
software, installing it, or rebooting in a maintenance window.  The timer
is a *named schedule*, which says when something happens.  Features point
at a schedule by name and decide what happens, so one schedule can drive
several features.


## Schedules

A schedule has a name and a recurrence rule.  A fresh unit has two, from
the factory configuration.  Written as CLI commands, they look like this:

<pre class="cli"><code>admin@example:/> <b>configure system</b>
admin@example:/config/system/> <b>edit schedule nightly</b>
admin@example:/config/system/…/nightly/> <b>set description "Every night at 03:00"</b>
admin@example:/config/system/…/nightly/> <b>set recurrence frequency daily</b>
admin@example:/config/system/…/nightly/> <b>set recurrence byhour 3</b>
admin@example:/config/system/…/nightly/> <b>end</b>
admin@example:/config/system/> <b>edit schedule weekly</b>
admin@example:/config/system/…/weekly/> <b>set description "Sunday nights at 03:00"</b>
admin@example:/config/system/…/weekly/> <b>set recurrence frequency weekly</b>
admin@example:/config/system/…/weekly/> <b>set recurrence byday sunday</b>
admin@example:/config/system/…/weekly/> <b>set recurrence byhour 3</b>
admin@example:/config/system/…/weekly/> <b>leave</b>
</code></pre>

The name is how features refer to the schedule.  It is 1 to 64
characters, starts with a letter or digit, and otherwise uses letters,
digits, `_`, `.` and `-`.  The description is a free-form note.  A
schedule without a recurrence rule is refused at commit time.

To stop a schedule, and everything that uses it, set `enabled false`.
The schedule stays in the configuration for when it is needed again.


## Recurrence Rules

A recurrence rule is evaluated in the system's local time.  The
mandatory `frequency` picks the base period:

| Frequency  | Fires                              |
|------------|------------------------------------|
| `minutely` | Every minute                       |
| `hourly`   | Every hour, on the hour            |
| `daily`    | Every day at midnight              |
| `weekly`   | Every week                         |
| `monthly`  | The 1st of every month at midnight |
| `yearly`   | January 1st at midnight            |

`interval` stretches the period, so `frequency hourly` with `interval 6`
fires every six hours.  The default is 1.

The `by*` fields pin one part of the period to given values:

- `byminute`: minutes within the hour, 0-59
- `byhour`: hours of the day, 0-23
- `byday`: days of the week, by name (`monday` … `sunday`)
- `bymonthday`: days of the month, 1-31
- `byyearmonth`: months of the year, 1-12

Each takes a list, so `byhour 8` and `byhour 20` together fire twice a
day.  This is how the factory `nightly` schedule becomes 03:00 rather
than midnight, and how `weekly` lands on Sunday.

Pick the coarsest frequency that fits, then narrow it.  A window every
Sunday morning is `frequency weekly` with `byday sunday` and `byhour 4`.
With `frequency daily` the same `byday` and `byhour` would fire every
morning.


## Limitations

A schedule is turned into a five-field cron expression, and the model is
cut down to what cron can express.  The following are refused at commit
time:

- `secondly` frequency, since cron has no seconds field.  The finest
  resolution is `minutely`
- `bymonthday` together with `byday`.  Cron fires on the union of the two
  where RFC 5545 specifies their intersection
- negative values, such as the last Monday of the month (`byday` with a
  direction) or the last day of the month (`bymonthday -1`)
- start and end bounds.  There is no start date, no `until` date and no
  occurrence count, so a schedule recurs until it is disabled
- per-schedule time zones, day of year, week of year and set position

`frequency yearly` with an `interval` above 1, as in every other year,
cannot be expressed either.  The interval is ignored in that case.


## Using a Schedule

A feature uses a schedule through a leaf of type `schedule-ref`.  The
reference is validated, so a schedule in use cannot be deleted, and a
typo is caught at commit time rather than at the next occurrence.

These features run on a schedule:

| Feature                          | Configuration path                  |
|----------------------------------|-------------------------------------|
| Reboot on a schedule             | `system scheduled-reboot`           |
| Update checks                    | `system software check-update`      |
| [Unattended software updates][3] | `system software unattended-update` |

A feature is active as soon as it references a schedule.  This reboots
the system every night at 03:00:

<pre class="cli"><code>admin@example:/> <b>configure system</b>
admin@example:/config/system/> <b>set scheduled-reboot schedule nightly</b>
admin@example:/config/system/> <b>leave</b>
</code></pre>

To pause one feature and keep its settings, set its `enabled` leaf to
`false`.  Disabling the schedule itself stops every feature using it.


## Verifying

Active schedules become cron jobs owned by the `admin` user, and the cron
daemon runs only while there is at least one.  The generated crontab is
visible from the shell:

```sh
admin@example:~$ crontab -l
# Managed by infix-schedule
0 3 * * *	/usr/sbin/reboot
```

An empty crontab means nothing is scheduled.  Check that the feature is
enabled, that it names the schedule correctly, and that the schedule is
enabled.  For update checks and unattended updates, `show software` shows
the trigger and the outcome of the last occurrence.

> [!NOTE]
> The crontab is generated from the configuration on every change.  Do
> not edit it by hand.

[3]: upgrade.md#unattended-software-updates
