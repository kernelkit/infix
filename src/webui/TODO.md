# Issues and Features TODO

## Important

- N/A

## Refactor to use YANG schema

In Configure System, we should not hard-code the timezones, but instead
use identities from the YANG model.  This was dismissed by Claude
earlier, but since we're steaming ahead and will soon add more pages, as
well as learning more about default values in YANG, we seem to need
something to carry us forward.  If this is YANG, fetched from the target
system when starting up, and/or a translated to JSON schema at runtime,
I do not know yet. But we should look into it.

The idea from the team, when we discussed Configure mode in the WebUI
was to have dedicated pages for core features where we compose a good
user experience, and for the rest we just present a generic
configuration tree based on parsing the yang tree.  Similar to how
~/src/klish-plugin-sysrepo/ does for the CLI (klish).

I more comprehensive description is in the file `yang-tree.md`.

Enter plan mode to think about this super task carefully.

## Minor, annoying but fixable with schema

- Configure > Users, the shell field show "CLI Shell" for admin user while it really is the YANG default
- Configure > System, the timezone should show the current one, which might be YANG default
- Configure > System, the Text editor field shows "--not set--", should be YANG default

## Later, investigate statd/copy behavior

- Why does /ietf-routing:routing/interfaces XPath not return anything, but /ietf-routing:routing does?
- Command `copy operational` fails hard (segfault?) for invalid XPath:

        admin@rpi-42-a6-03:~$ copy operational -x /system-data
        Error: (null) (5)
        Error: failed retrieving operational-state data
