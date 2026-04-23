# Issues and Features TODO

## Reminder: Working Locally

```
~/src/infix/src/webui(web)$ make clean; sudo make dev ARGS="--restconf https://192.168.0.1/restconf --insecure-tls"  
```

## New TODO

Minor changes:

 1. Rename YANG Tree back to Advanced
 1. The tree view should never list leafs, only nodes that can be expanded.
 1. The tree should be a tree, not just foldouts in a list
 1. The top of the tree could be a single '/'

### Graphics & Design

Use smaller logo, without the three pillars, on top-bar and raise top-bar.
Similar to the WebUI v1 design. The left side-bar should not cover the
top-bar.

Review all the icons.  DHCP, Keystore, Interfaces, Routing, and Advanced look
weird.  Advanced + Routing look like a swastika ... Tobias says the macOS
network/interface icon could be a good fit.

### Refactor Save/Save all/Apply/Apply & Save/Abort

It's a bit of a mess currently on auto-generated pages that collapse any
sub-containers or lists, they show multiple Save and Save all buttons.

That in combination with the Apply, Apply & Save, and Abort buttons at the
bottom of the screen.  One user told me; "What do they do, what if I do
something wrong?"

So we had a discussion in the team and we all agreed we want to mimic the
semantics of the CLI.  A user building up new candidate config should be able
to see the diff between running and candidate before applying (CLI `leave`) or
aborting (CLI `abort`).  When applied, regardless of context, the WebUI should
display a permanent "status" of sorts, reminding the user they've got unsaved
activated changes.  From that status the user should be able to, again, view
the diff (this time running vs startup) and/or save to startup-config.  This
status should also show if a CLI user makes a change in the background to the
running-config.

We have very different opinions on how this should be implemented, so we are
very open to design ideas and discussions around this topic before we go ahead
and make a change.

### Ideas for auto-generated pages

Some pages, like IPv4 addresses could be shown similar to how the curated
Users configuration page looks.  I.e., when a container has a list, the
complexity of the list items decide if it deserves a separate new page or can
be shown on the current page.  Q: how should this "complexity score" be
calculated?

## Ideas for Improvement for Configure Advanced Tree View

### Filter out /test

The /infix-test:test/ subtree should be filtered out.

### Reduce meta-data from leaf view

All the meta-data presented for leaf nodes is quite useless to an end-user.
Sure, for understanding YANG, the tree, node types and limitations, or getting
the full XPath of a leaf node, it can be useful.  But it should all be hidden
from the user by default in a "Detail" view, right-hand side pane foldout, or
something similar.

Maybe we can look upon all this meta-data from the schema as Online Help?
There should be quite a few Web design patterns for online help in interfaces
like this one.  In the CLI I added support for the 'help [node]' command to
show the YANG description(s) in a "man page" style, listing also any default
value.

### Presenting Leaf Nodes with YANG leafref's

Any leaf node that is s a leafref should present a drop-down list of available
values from that reference.  This is what klish-plugin-sysrepo does as well
with Tab completion for settings with leafref.

### The saga of the booleans with/without YANG schema defaults

Currently we use radio buttons to illustrate boolean leaf nodes.  However, the
current value of an unset leaf node is not show at all, which makes it very
difficult for a user to know what its value is currently.

We must show the current value, show what the default is, even if there's no
default in the YANG model, so that when a user clicks on "Remove" button

### Resetting a configure setting using "Remove" button

The naming of the "Remove" button is very unfortunate.  Maybe more logical in
the CLI, but in the WebUI it's not obvious what it does.  Reset or Clear would
probably be better names, but I'm willing to hear ideas on this.  In any case
we need some sort of tooltip (hover) so users can get an explanation what it
does.

### "Final container" inline leaf rendering

The first prototype of this turned out great, it worked perfectly on the
/mdns/reflector container, for example.  I'm only missing the "Remove" or
"Reset" button per leaf and a way to get more details about th

The original requirement I made was too vague.  I've realized it would be a
great improvement if we could create these "intermediate" auto-generated pages
of multiple leaf nodes for every level we encounter.  A good example is NACM
actually, which on the top-level has a set of global options, which you then
can refine per group and per user in lists below.  This pattern is common in
YANG so it's quite possible we could create something really useful and also
reusable here.

Another good example is /system which contains a lot of global system settings
and then goes into users, ntp client, and other settings below that which all
deserve their own auto-generated pages per "level" so to speak.

### LEAF value Presentation

Instead of having a card title "LEAF <node>" we should just present the
simplified Path, i.e., XPath without model prefixes.

### The Details view

- It should call Path XPath since it holds YANG model prefixes
- It should also list the YANG model Description as the help text for the node
- The Description as help text is missing also from YANG container views

## Important

### Fork goyang in kernelkit org

The webui currently carries a local copy of goyang in `internal/goyang/` with
three patches that fix genuine upstream bugs (unresolved since 2015–2024):

1. `Uses.Augment *Augment` → `[]*Augment` — multiple `augment` inside `uses`
   (upstream Issue #75, PR #272, open since Aug 2024)
2. `Value` struct: add `Reference *Value` field — `when { reference "..."; }`
   (not reported upstream)
3. `Input`/`Output` structs: add `Must []*Must` — `must` in rpc input/output
   (upstream PR #270, open since Aug 2024)

The right long-term fix is to fork openconfig/goyang into the kernelkit org,
create a `v1.6.3-kkit` branch, apply the patches there, `git format-patch
v1.6.3` and add them to the Infix `patches/` directory.  Then point go.mod at
the kernelkit fork instead of the local `internal/goyang` copy.

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

## YANG tree pruning

The Configure > Advanced tree currently shows everything goyang loads, which
includes noise that libyang-based tools normally filter:

**Bug (fix before/during Phase 2):**
- Duplicate top-level nodes: `ms.Modules` in goyang includes both modules *and*
  submodules as separate map entries, causing submodules (e.g. `infix-if-base`)
  to appear twice — once inlined under their parent and once as a standalone root.
  Fix: skip entries in `topLevelNodes()` where the module is a submodule
  (`mod.BelongsTo != nil`).

**Phase 5 filtering:**
- Sysrepo-internal modules: `sysrepo`, `sysrepo-*`, `sysrepo-factory-default`
- NETCONF/RESTCONF protocol modules: `ietf-netconf*`, `notifications`,
  `nc-notifications`, `ietf-restconf*`, `ietf-yang-patch`, etc.
- YANG library/type utility modules: `ietf-yang-library`, `ietf-yang-types`,
  `ietf-yang-metadata`, `yang`, `default`, etc.
- Nodes with an active `deviate not-supported` deviation
- `config false` subtrees when browsing in Configure (write) mode

Approach: maintain a module deny-list (or better, an allow-list seeded from the
modules that actually appear in the running datastore), combined with an
Entry.Config check and a deviation walk in `topLevelNodes`/`dirToNodes`.

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
