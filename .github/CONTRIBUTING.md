Contributing to Infix
=====================

We welcome any and all help in the form of bug reports, fixes, patches
to add new features -- *preferably as GitHub pull requests*.

If you are unsure of what to do, or how to implement an idea or bug fix,
open an issue with `"[RFC]: Unsure if this is a bug ... ?"`, or use the
[GitHub discussions forum](https://github.com/orgs/kernelkit/discussions).
Talking about code and problems first is often the best way to get started
before submitting a pull request.

When submitting a bug report, patch, or pull request, please start by
stating the version the change is made against, what it does, *and why*.

Please take care to ensure you follow the project coding style and the
commit message format.  If you follow these recommendations you help
the maintainer(s) and make it easier for them to include your code.


Coding Style
------------

Before jumping into code, remember to **document new features** and bug
fixes.  Both the manual and ChangeLog are in the `doc/` sub-directory
and it is expected that you provide a human-readable summary for the
release notes (ChangeLog) and at least a configuration example in the
manual for new features.

> **Tip:** consider ["Readme driven development"][RDD] for new features.
> It is amazing how many flaws in your own bright ideas come to bare
> when you suddenly have to explain them to someone else!

We expect code contributions for:

 - C code in [Linux Coding Style][Linux]
 - Python code should follow [PEP-8][]

> **However,** always submit code that follows the style of surrounding
> code!  Legacy takes precedence, and remember, we read code a lot more
> than write it, so legibility is important.

As a final note, lines are allowed to be longer than 72 characters these
days.  There is no enforced maximum, but the team usually keep it around
100 characters for both C and Python.


Commit Messages
---------------

Commit messages exist to track *why* a change was made.  Try to be as
clear and concise as possible in your commit messages, and always, be
proud of your work and set up a proper GIT identity for your commits:

    $ git config --global user.name "Jacky Linker"
    $ git config --global user.email jacky.linker@example.com

Example commit message from the [Pro Git][gitbook] online book, notice
how `git commit -s` is used to automatically add a `Signed-off-by`:

    subsystem: brief, but clear and concise summary of changes
    
    More detailed explanatory text, if necessary.  Wrap it to about 72
    characters or so.  In some contexts, the first line is treated as
    the subject of an email and the rest of the text as the body.  The
    empty line separating summary from body is critical.  Tools like
    rebase can get confused if the empty line is missing.
    
    Further paragraphs should be separated with empty lines.
    
     - Bullet points are okay, too
    
     - Typically a hyphen or asterisk is used for the bullet, preceded
       by a single space, with blank lines in between, but conventions
       vary here
    
    Signed-off-by: Jacky Linker <jacky.linker@example.com>


Code of Conduct
---------------

It is expected of everyone to respect the [Code of Conduct][conduct].
The *"maintainers have the right and responsibility to remove, edit, or
reject comments, commits, code, discussion forum threads, issues, and
other contributions that are not aligned to this Code of Conduct."*

[Linux]:   https://www.kernel.org/doc/html/v6.9/process/coding-style.html
[PEP-8]:   https://peps.python.org/pep-0008/
[RDD]:     https://tom.preston-werner.com/2010/08/23/readme-driven-development
[gitbook]: https://git-scm.com/book/ch5-2.html
[conduct]: CODE-OF-CONDUCT.md
