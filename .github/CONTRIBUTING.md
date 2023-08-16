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
stating the version the change is made against, what it does, and why.

Please take care to ensure you follow the project coding style and the
commit message format.  If you follow these recommendations you help
the maintainer(s) and make it easier for them to include your code.


Coding Style
------------

> **Tip:** Always submit code that follows the style of surrounding code!

First of all, lines are allowed to be longer than 72 characters these
days.  In fact, there exist no enforced maximum, but keeping it around
100 chars is OK.

The coding style itself is otherwise strictly Linux [KNF][].


Commit Messages
---------------

Commit messages exist to track *why* a change was made.  Try to be as
clear and concise as possible in your commit messages, and always, be
proud of your work and set up a proper GIT identity for your commits:

    git config --global user.name "Jane Doe"
    git config --global user.email jane.doe@example.com

Example commit message from the [Pro Git][gitbook] online book, notice
how `git commit -s` is used to automatically add a `Signed-off-by`:

    Brief, but clear and concise summary of changes
    
    More detailed explanatory text, if necessary.  Wrap it to about 72
    characters or so.  In some contexts, the first line is treated as
    the subject of an email and the rest of the text as the body.  The
    blank line separating the ummary from the body is critical (unless
    you omit the body entirely); tools like rebase can get confused if
    you run the two together.
    
    Further paragraphs come after blank lines.
    
     - Bullet points are okay, too
    
     - Typically a hyphen or asterisk is used for the bullet, preceded
       by a single space, with blank lines in between, but conventions
       vary here
    
    Signed-off-by: Jane Doe <jane.doe@example.com>


Code of Conduct
---------------

It is expected of everyone to respect the [Code of Conduct][conduct].
The *"maintainers have the right and responsibility to remove, edit,
or reject comments, commits, code, wiki edits, issues, and other
contributions that are not aligned to this Code of Conduct."*

[KNF]:      https://en.wikipedia.org/wiki/Kernel_Normal_Form
[gitbook]:  https://git-scm.com/book/ch5-2.html
[conduct]:  CODE-OF-CONDUCT.md
