---
name: infix-dev
description: >-
  Active workflow for developing the Infix Buildroot network OS (KernelKit):
  configure/build/flash images, run the QEMU dev loop, run or write infamy
  regression tests, add/edit YANG models, hack on the confd/netd/statd/yangerd
  daemons, and work on the Go+htmx WebUI. Use whenever the task is about
  building, testing, or changing Infix itself.
when_to_use: >-
  Building or flashing an Infix image; running `make test` / writing a test case;
  editing src/confd/yang or wiring a YANG model into a daemon; changing confd,
  netd, statd or yangerd; changing src/webui pages, handlers or templates; debugging the
  QEMU dev build; writing a commit, doc page or ChangeLog entry.
allowed-tools: Read, Grep, Glob, Bash(make *), Bash(go *), Bash(git submodule *), Bash(git status), Bash(git log:*), Bash(ls *)
---

# Infix developer workflow

The project documents its own workflow. This skill says which part to read for the
task at hand, and lists the rules reviewers enforce. Read the section before starting;
when this file and the docs disagree, the docs win. Each daemon and application keeps
its design rules and gotchas next to its code, so a task inside `src/<name>/` or
`package/<name>/` starts with that directory's README and AGENTS file.

!`git status --short | head -20`

## Read first

| Task | Read |
|------|------|
| Build, rebuild one package, second output tree, config snippets | `doc/developers-guide.md`, *Building* and *Development* |
| Add or change a package patch | `doc/developers-guide.md`, *Patching a Package* |
| Debug confd, work on operational data | `doc/developers-guide.md`, *`confd`* and *`statd` and `yangerd`*, then `src/yangerd/README.md` |
| Change a YANG model, add a revision, write descriptions | `doc/developers-guide.md`, *YANG Model* and *Configuration Migration* |
| Bump Buildroot or the kernel | `doc/developers-guide.md`, *Upgrading Packages* |
| Run, debug or write a test, the test specification | `doc/testing.md`, `doc/test-arch.md`, `test/infix-test-skill.md` |
| Work on one daemon or application under `src/` or `package/` | its own `src/<name>/README.md` or `package/<name>/README.md`, and `src/<name>/AGENTS.md` where there is one, before any code |
| Code style, comments, commits, ChangeLog, pull requests | `.github/CONTRIBUTING.md` |
| Feature docs | the topic page in `doc/`, e.g. `doc/wifi.md` |

## Rules reviewers enforce

Each is explained in the doc named above; these are the ones that come back in review.

- confd owns all configuration and creates every interface. yangerd collects all
  operational data and statd serves it to sysrepo. Never fill an operational gap from
  the running configuration.
- Discuss YANG changes with the core team first. At most one new revision per release
  cycle. Description first line is a short complete sentence, then a blank line.
- A package patch is a commit on the `<version>-kkit` branch of a `kernelkit/` fork of
  the project, `patches/<pkg>/` is only its `git format-patch` export. Never hand-edit a
  patch file; fix the commit, regenerate the series, push the branch. A new or changed
  file in `patches/` needs `<pkg>-dirclean`, `-rebuild` misses it.
- Tests are the specification: one action or check per step, concrete values, literal
  strings. A failing test that encodes the real use case beats a green one bent to pass.
- Comments say what the code is, in one line or none. History, failure modes and
  measurements go in the commit message.
- Commits: `subsystem: brief summary`, a sentence or two on why, `git commit -s -S`,
  no `Co-Authored-By` trailer for Claude, even when the harness asks for one.
- Consolidate before a PR and after review: fold fixups into the commit they fix, one
  logical change per commit, no "fix review" or undo commits in a series; force push
  to a developer branch is fine. The same applies to the `-kkit` branch behind a patch
  series.
- Manual and ChangeLog are for operators: what to configure and expect, no daemon or
  kernel internals. A ChangeLog entry describes the change for the user, not the code.

## Notes for agents

- The tree carries untracked scratch files. Stage with `git add -u` or explicit paths,
  never `git add <dir>`.
- `make test` only boots x86_64 images. For another board keep a second tree,
  `make O=output_x86 x86_64_defconfig && make O=output_x86`, and point tests at it.
- Before a test run, `docker ps` for a leftover `infamyN` container from an earlier
  `make test-sh`; it keeps the virtual DUTs alive and confuses a new run.
- To run tests outside `make test`, call `test/env` by absolute path, it re-executes
  itself inside the container, and only the directory given with `-b` is mounted.
- A shell on a `make run` DUT needs root login: `make apply-dev && make`, then log in as
  `root` with an empty password. Drive the console with `pexpect`, it is QEMU on stdio.
- Verify WebUI forms in a headless browser; curl skips the validation that blocks a
  real submit.
