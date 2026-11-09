---
name: webui-gui-test
description: >-
  Walk through the Infix WebUI in a headless Chromium before a release and
  report what a user would hit: pages that fail to load, dialogs that leave
  the page unusable, saves that do not apply, script errors. Use when asked
  to GUI test, browser test or review the WebUI of a running box.
---

# WebUI release walk

An exploratory pass over the WebUI of one running box, done in a real
browser, with a written report at the end.  It is a review, not a gate:
it runs on a developer machine once per release, and finds the breakage
nobody wrote a test for.  The deterministic tests live in `make test`.

## Setup

1. A box to test: `make run` in the tree, real hardware, or a QEMU with
   a routable address.  Note its address and the admin password.
2. Chromium on PATH, `chromium` or `chromium-browser`.
3. A venv with websockets 11 or later, the system package is often older:

       python3 -m venv ~/.infix/gui-venv && ~/.infix/gui-venv/bin/pip install 'websockets>=12'

4. Never run the pass against a box someone else is using; the pass
   creates and deletes configuration.

## Driving the browser

`webui.py` next to this file is the only tool.  Write one Python script in
the scratchpad per sidebar group, import `Browser` from it by path, and
run it with the venv's python.  Each page runs in its own `try`, a
failure is recorded as a finding and the script moves on to the next
page; the report is rendered from that list at the end, so one crash
does not throw away the pages already walked:

```python
import importlib.util
spec = importlib.util.spec_from_file_location("webui", "<repo>/.claude/skills/webui-gui-test/webui.py")
mod = importlib.util.module_from_spec(spec); spec.loader.exec_module(mod)

with mod.Browser("192.168.122.152", password="admin") as web:
    web.login()
    web.goto("/interfaces")
    web.wait("document.querySelector('.data-table')")
    web.click(".key-row-toggle", text="e1")
    web.fill("input[name=description]", "walk")
    web.click("button[type=submit]", text="Save")
    print(web.invalid(), web.restconf("ietf-interfaces:interfaces/interface=e1"))
    print(web.eval("document.activeElement.tagName"), web.errors)
```

- `goto(path)`, `wait(expr, timeout)`, `eval(expr)` return what the page
  says.  `click(selector, text=)` is a real mouse click at the element's
  centre, so a covered or inert element does not react, the same as for a
  user.  `fill(selector, text)` types into a control, so a form that
  reacts to input or change sees the change; never set `.value` from
  `eval`, nothing listens to that.
- `errors` collects uncaught exceptions, `console.error`, every htmx
  request that failed or was swallowed by a handler, and every HTTP
  response of 400 or above, each prefixed with the page it happened on.
  Read it after every page and after every save; a 403 or 500 behind a
  green notice is a finding on its own.
- A button that does nothing is not dead until `invalid(form)` says the
  form is valid: a `required` control the page hides still blocks the
  submit, and the browser cannot say so.  Report the names it returns.
- `screenshot(path)` writes a PNG of the viewport; take one for every
  finding and name it in the report.
- `upload(selector, path)` chooses a local file in a file input, the
  way the file picker does, and fires its change event.
- `download(selector, text=)` clicks and returns the path and size of
  the file that arrived; the file lives until the browser is closed.  A
  download is not verified until it has.
- `restconf(path, datastore=)` is a GET on the box with the same
  credentials, `None` on 404.  Use it to confirm a value after a save
  and to check that a created thing exists and a deleted one is gone.
- A link-local address needs `interface=`; the helper then forwards from
  localhost, since Chromium refuses a zone id in a URL.
- Check state in the DOM, not in the server: a row is open when its
  detail row has `is-open` and a height, a field works when `focus()`
  makes it `document.activeElement`.
- After every save, read the notice the page shows and then confirm the
  value over RESTCONF or on the box.  A green notice with no change is a
  finding.

## The walk

The pages and how they look change between releases, so nothing here
names a page or a CSS class to rely on.  Discover, then act:

1. Read the sidebar of the logged-in page: every link in it is a page to
   walk, grouped the way the sidebar groups them, status, configure and
   maintenance.  Then, on every page reached, every same-origin link and
   every tab is a page too, at any depth, until nothing new appears.
   Say in the report which sidebar links you found and how many pages
   were reached beyond them.
2. On each page, read the DOM before acting: the tables, the rows that
   toggle something, the links and tabs that lead on, the forms and
   their fields, the buttons that open a dialog, the buttons that run
   something.  Derive selectors from what is there.  A page that expands
   nodes is walked two levels deep from every top node.  If a page looks
   nothing like the others, say so, that is a finding too.
3. A page that lists or shows data (the status group):
   - renders data, not a spinner or an error alert, within 30 s;
   - stays sane through two auto-refresh periods, the period is the one
     picked in the user menu and shown next to the page title, set the
     shortest one first: content updates in place, no flicker to empty,
     scroll position and an open fold-out kept;
   - every fold-out opens and closes on a real click;
   - a few numbers the CLI also shows, counters and addresses, agree.
4. A page that edits something (the configure group), including every
   tab of every detail page:
   - every row opens and a field in each takes focus; every select has
     its options read;
   - one harmless value per form changed, saved, the notice read, the
     value confirmed in the running configuration, then changed back;
   - when the UI shows pending changes, one change is made, the diff
     read, the change abandoned, and the box confirmed unchanged; then
     made again and applied;
   - every dialog opened, cancelled, opened again and submitted with its
     defaults; afterwards the page must still take clicks and focus;
   - every add button at any depth is used, the thing created, confirmed
     on the box, and deleted again: interfaces of every type offered, a
     user, a route, a rule, and the entries that live inside a detail
     tab; the user is created without admin rights and kept until step 7;
   - one form submitted with an invalid value: the error names the
     field, "Save failed" alone is a finding.
5. Every button on any page that is not a form submit runs something:
   press it and read what comes back, unless its label says reboot,
   shutdown, reset, restart, upgrade, install or factory.  Those are
   listed in the report as not run, and never run unless asked.
6. A download is verified with `download()`, a non-empty file must
   arrive.  Every upload form is given a wrong file, a text file, and
   must refuse it with a reason, not a stack trace and not a silent
   success.
7. Log in as the user created in step 4 and repeat the status group: no
   configure action may work, and a denial must be a denial, not an
   error page.  Then log back in as admin and delete the user.
8. Log out with the real button.  Load a data page: the login page must
   come back.  Press the browser back button: no data may show.

## The report

Lead with the findings, one per bullet: page, what was done, what
happened, what should have happened, and the evidence (an error text,
the DOM state, a RESTCONF value, the screenshot).  Then one line per
page walked without findings.  Every pass starts from scratch: do not read old reports or
bug lists, and do not record this pass's findings in the skill.
