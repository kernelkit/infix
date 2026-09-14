# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in Infix, please use GitHub's
built-in [Report a Vulnerability][report] feature for a private and secure
disclosure.

When reporting, include:

- A clear description of the vulnerability.
- Steps to reproduce the issue.
- Potential impact of the vulnerability.

## Supported Versions

Security fixes are made on `main` and reach users in the next release,
roughly once a month.  The supported version of the project is therefore
the latest release.  Each release also carries the latest Linux kernel
LTS and Buildroot LTS patch level, so following the releases keeps the
whole foundation current.

Older release series live on their `vYY.MM.x` branches.  They receive a
patch release only when there is a reason to make one, and when that
happens the kernel and Buildroot LTS patch bumps are ported from `main`
along with the fix.  A branch that exists is not the same as a branch
that is maintained -- there is no promise of updates for a series once
the next one is out.

Products that must stay on one version for years need a support contract.
The company [*Wires*][wires] maintains individual release series on a
schedule, with security fixes backported to the series a product shipped
on.

The full policy, including what may and may not go into a patch release,
is documented in [Releases & Support][releases].  Contract options are
described in [Support][support].

## Acknowledgments

We appreciate the efforts of the security community to help improve the security
of Infix. Thank you for your responsible disclosure.

[report]:   https://github.com/kernelkit/infix/security/advisories/new
[releases]: https://github.com/kernelkit/infix/blob/main/doc/releases.md
[support]:  https://github.com/kernelkit/infix/blob/main/.github/SUPPORT.md
[wires]:    https://wires.se
