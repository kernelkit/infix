# Issues and Features TODO

## Reminder: Working Locally

```
~/src/infix/src/webui(web)$ make clean; sudo make dev ARGS="--restconf https://192.168.0.1/restconf --insecure-tls"  
```

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

## YANG tree pruning (Phase 5)

- Sysrepo-internal modules: `sysrepo`, `sysrepo-*`, `sysrepo-factory-default`
- NETCONF/RESTCONF protocol modules: `ietf-netconf*`, `notifications`,
  `nc-notifications`, `ietf-restconf*`, `ietf-yang-patch`, etc.
- YANG library/type utility modules: `ietf-yang-library`, `ietf-yang-types`,
  `ietf-yang-metadata`, `yang`, `default`, etc.
- Nodes with an active `deviate not-supported` deviation

Approach: maintain a module deny-list (or better, an allow-list seeded from the
modules that actually appear in the running datastore), combined with an
Entry.Config check and a deviation walk in `topLevelNodes`/`dirToNodes`.

## Later, investigate statd/copy behavior

- Why does /ietf-routing:routing/interfaces XPath not return anything, but /ietf-routing:routing does?
- Command `copy operational` fails hard (segfault?) for invalid XPath:

        admin@rpi-42-a6-03:~$ copy operational -x /system-data
        Error: (null) (5)
        Error: failed retrieving operational-state data
