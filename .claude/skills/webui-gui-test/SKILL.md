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
the scratchpad per pass, import `Browser` from it by path, and run it with
the venv's python:

```python
import importlib.util
spec = importlib.util.spec_from_file_location("webui", "<repo>/.claude/skills/webui-gui-test/webui.py")
mod = importlib.util.module_from_spec(spec); spec.loader.exec_module(mod)

with mod.Browser("192.168.122.152", password="admin") as web:
    web.login()
    web.goto("/interfaces")
    web.wait("document.querySelector('.data-table')")
    web.click(".key-row-toggle", text="e1")
    print(web.eval("document.activeElement.tagName"), web.errors)
```

- `goto(path)`, `wait(expr, timeout)`, `eval(expr)` return what the page
  says.  `click(selector, text=)` is a real mouse click at the element's
  centre, so a covered or inert element does not react, the same as for a
  user.  `errors` collects uncaught exceptions and `console.error`.
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
   maintenance.  Say in the report which links you found.
2. On each page, read the DOM before acting: the tables, the rows that
   toggle something, the forms and their fields, the buttons that open a
   dialog.  Derive selectors from what is there; if a page looks nothing
   like the others, say so, that is a finding too.
3. A page that lists or shows data (the status group):
   - renders data, not a spinner or an error alert, within 30 s;
   - stays sane through two auto-refresh periods: content updates in
     place, no flicker to empty, scroll position and an open fold-out
     kept;
   - every fold-out opens and closes on a real click;
   - a few numbers the CLI also shows, counters and addresses, agree.
4. A page that edits something (the configure group):
   - every row opens and a field in each takes focus;
   - one harmless value per form changed, saved, the notice read, the
     value confirmed in the running configuration, then changed back;
   - every dialog opened, cancelled, opened again and submitted with its
     defaults; afterwards the page must still take clicks and focus;
   - whatever the page can create is created and deleted again,
     interfaces of every type offered, a user, a route, a rule;
   - one form submitted with an invalid value: the error names the
     field, "Save failed" alone is a finding.
5. Maintenance pages: load them and run what is safe, a backup
   download, a log view, a ping.  No reboot, upgrade or factory reset
   unless asked.
6. Log in as a non-admin user at the end and repeat the status group: no
   configure action may work, and a denial must be a denial, not an
   error page.

## The report

Lead with the findings, one per bullet: page, what was done, what
happened, what should have happened, and the evidence (an error text,
the DOM state, a RESTCONF value).  Then one line per page walked without
findings.  Every pass starts from scratch: do not read old reports or
bug lists, and do not record this pass's findings in the skill.
