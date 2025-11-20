Contributing Guidelines
=======================

Thank :heart: you for taking the time to read this!

We welcome any help in the form of bug reports, fixes, or patches to add
new features.  We *prefer* GitHub pull requests, but are open to other
forms of collaboration as well.  [Let's talk!][support] :handshake:

If you are unsure how to start implementing an idea or fix:

- :bug: open an issue, there are human friendly templates for _bugs_
  and _feature requests_ at <https://github.com/kernelkit/infix/issues>
- :speech_balloon: use the [Q&A Forum][discuss]
- :technologist: The [Developer's Guide][devguide] is also a useful start

> [!IMPORTANT]
> Talking about code and problems first is often the best way to get
> started before submitting a pull request.  We have found it always
> saves time, yours and ours.

:sparkles: General Guidelines
-----------------------------

When submitting bug reports or patches to bugs, please state which
version the change is made against, what it does, and, more importantly
*why* -- from your perspective, why is it a bug, why does the code need
changing in this way.  Start with why.

- :bug: Bug reports need metadata like Infix version or commit hash
- :adhesive_bandage: Bug fixes also need version, and (preferably) a
  corresponding issue number for the ChangeLog
- :new: New features, you need to get approval of the YANG model first!  
  :speech_balloon: Please use the [Forum][discuss], e.g., category:
  *Ideas*, or open a :pray: feature request issue
- :white_check_mark: New features also need new regression tests, this
  can be basic tests or more complex use-case tests comprising multiple
  subsystems, see [Testing Changes](#test_tube-testing-changes), below

Please take care to ensure you follow the project coding style and the
commit message format.  If you follow these recommendations you help
the maintainers and make it easier for them to include your code.

:woman_technologist: Coding Style
---------------------------------

Before jumping into code, remember to **document new features** and bug
fixes.  Both the manual and ChangeLog are in the `doc/` sub-directory
and it is expected that you provide a human-readable summary for the
release notes (ChangeLog) and at least a configuration example in the
manual for new features.

> [!TIP]
> Consider ["Readme driven development"][RDD] for new features.  It is
> amazing how many flaws in your own bright ideas come to bare when you
> suddenly have to explain them to someone else!

We expect code contributions for:

- C code in [Linux Coding Style][Linux]
- Python code should follow [PEP-8][]

> [!IMPORTANT]
> **However,** always submit code that follows the style of surrounding
> code!  Legacy takes precedence, and remember, we read code a lot more
> than write it, so legibility is important.

The ChangeLog deserves a separate mention:

- Releases are listed in reverse chronological order order, so the
  latest/next release is at the beginning of the file
- Only *user-facing bugs and features* are detailed, so code refactor,
  new tests, etc. are not listed.
- Add your changes/features to the Changes section
- Add your Fix line in the Fixes section, in numeric order
- Changes and fixes without an issue number are listed after all
  numbered ones
- YANG model changes are documented in their respective model, for
  standard models, e.g., for `ietf-interfaces.yang`, the corresponding
  `infix-interfaces.yang` detail augments/deviations as revisions.

A final note, lines of code are allowed to be longer than 72 characters
these days, unless you live by PEP-8 (see above).  There is no enforced
maximum, but the team usually keep it around 100 characters for both C
and Python.

:test_tube: Testing Changes
---------------------------

Please test your changes, no matter how trivial or obviously correct
they may seem.  Nobody is infallible, making mistakes is only human.
It is also the best insurance policy for your feature, it ensures your
use-case will remain functional as the source base evolves.

For new functionality we expect new regression tests to be added in
the same pull request.

For help getting started with testing, see the following resources:

- [Developer's Guide][devguide]
- [Regression Testing][testing]

:memo: Commit Messages
----------------------

Commit messages exist to track *why* a change was made.  Try to be as
clear and concise as possible in your commit messages, and always, be
proud of your work and set up a proper GIT identity for your commits:

<img src="../doc/jack.png" width=70 align="right">

```bash
$ git config --global user.name "Jacky Linker"
$ git config --global user.email jacky.linker@example.com
```

Example commit message from one of many [online guides][cbeams].  Use
`git commit -s` to automatically add a `Signed-off-by` for proof of
origin, see [DCO][] for more info.

```text
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

If you use an issue tracker, put references to them at the bottom,
like this:

Resolves: #123
See also: #456, #789

Signed-off-by: Jacky Linker <jacky.linker@example.com>
```

This is an example of how to [automatically close][closing] an issue
when the commit is merged to mainline.  Several keywords are available.

:lock_with_ink_pen: Signing Commits with GPG
---------------------------------------------

To ensure the authenticity and integrity of your contributions, we
**require** all commits to be signed with GPG.  This cryptographically
verifies that commits come from a trusted source.

### Generating a GPG Key

If you don't already have a GPG key, generate one:

```bash
$ gpg --full-generate-key
```

When prompted, choose:
- Key type: `RSA and RSA` (default)
- Key size: `4096` bits (recommended for security)
- Expiration: `0` (key does not expire)
- Real name and email: Use the same email as your Git configuration

> [!NOTE]
> We recommend keys that do not expire for signing commits.  Expiration
> creates a "usability time bomb" without providing meaningful security
> benefits for code signing.  See [this article][pgpfan] for details.

### Configuring Git to Sign Commits

First, find your GPG key ID:

```bash
$ gpg --list-secret-keys --keyid-format=long
```

Look for the line starting with `sec`, the key ID is the part after the `/`.
For example, in `sec   rsa4096/ABCD1234EFGH5678`, the key ID is `ABCD1234EFGH5678`.

Configure Git to use your GPG key:

```bash
$ git config --global user.signingkey ABCD1234EFGH5678
$ git config --global commit.gpgsign true
```

The second command enables automatic signing for all commits.  Alternatively,
you can sign individual commits with `git commit -S`.

### Publishing Your Public Key

To allow others to verify your signatures, publish your public key to a
keyserver:

```bash
$ gpg --keyserver hkps://keys.openpgp.org --send-keys ABCD1234EFGH5678
```

> [!IMPORTANT]
> The keyserver will send a verification email to the address associated
> with your key.  You **must** click the link in that email to confirm
> ownership before your key becomes searchable by email address.

Alternative keyservers you can use:
- `hkps://keyserver.ubuntu.com`
- `hkps://pgp.mit.edu`

### Adding Your GPG Key to GitHub

For GitHub to show your commits as "Verified", you need to add your public
key to your account:

1. Export your public key:
   ```bash
   $ gpg --armor --export ABCD1234EFGH5678
   ```

2. Copy the entire output, including the `-----BEGIN PGP PUBLIC KEY BLOCK-----`
   and `-----END PGP PUBLIC KEY BLOCK-----` lines.

3. Go to [GitHub Settings → SSH and GPG keys](https://github.com/settings/keys)

4. Click **New GPG key** and paste your public key.

Now your signed commits will display a "Verified" badge on GitHub! :white_check_mark:

For more details, see GitHub's [official documentation on commit signature verification][gpg-verify].

:twisted_rightwards_arrows: Pull Requests
-----------------------------------------

> [!NOTE]
> _The git repository is the canonical location for all information._

A pull request should preferably address a single issue or change.  This
may of course include multiple related changes, but what is important to
remember is the bandwidth of the maintainers.  A smaller well documented
PR is more likely to get attention quicker in some reviewer's busy day.

Well documented here means that each commit message stands on its own,
telling the complete story of the change.  In fact each commit should,
as much as possible, be independent of other changes.  This is very
important, not only when digging though logs to understand why a piece
of code exists, but also when bisecting a problem -- each single commit
should also compile and be possible to run.

If you've worked on projects that send patches, like the Linux kernel or
Buildroot, consider the pull request message body similar to the cover
letter for a series of patches -- it's a summary of changes, and it is
lost when the changes are merged to the mainline branch.

:balance_scale: Code of Conduct
-------------------------------

It is expected of everyone to respect the [Code of Conduct][conduct].
The *"maintainers have the right and responsibility to remove, edit, or
reject comments, commits, code, discussion forum threads, issues, and
other contributions that are not aligned to this Code of Conduct."*

[support]:  https://github.com/kernelkit/infix/blob/main/.github/SUPPORT.md
[discuss]:  https://github.com/orgs/kernelkit/discussions
[testing]:  https://github.com/kernelkit/infix/blob/main/doc/testing.md
[devguide]: https://github.com/kernelkit/infix/blob/main/doc/developers-guide.md
[Linux]:    https://www.kernel.org/doc/html/v6.9/process/coding-style.html
[PEP-8]:    https://peps.python.org/pep-0008/
[RDD]:      https://tom.preston-werner.com/2010/08/23/readme-driven-development
[cbeams]:   https://cbea.ms/git-commit/#seven-rules
[conduct]:    CODE-OF-CONDUCT.md
[DCO]:        https://developercertificate.org/
[closing]:    https://docs.github.com/en/get-started/writing-on-github/working-with-advanced-formatting/using-keywords-in-issues-and-pull-requests
[gpg-verify]: https://docs.github.com/en/authentication/managing-commit-signature-verification
[pgpfan]:     https://articles.59.ca/doku.php?id=pgpfan:expire
