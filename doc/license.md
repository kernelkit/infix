Origin & Licensing
------------------

Infix is entirely built on Open Source components (packages).  Most of
them, as well as the build system with its helper scripts and tools, is
from [Buildroot][1], which is distributed under the terms of the GNU
General Public License (GPL).  See the file COPYING for details.

Some files in Buildroot contain a different license statement.  Those
files are licensed under the license contained in the file itself.

Buildroot and Infix also bundle patch files, which are applied to the
sources of the various packages.  Those patches are not covered by the
license of Buildroot or Infix.  Instead, they are covered by the license
of the software to which the patches are applied.  When said software is
available under multiple licenses, the patches are only provided under
the publicly accessible licenses.

Infix releases include the license information covering all Open Source
packages.  This is extracted automatically at build time using the tool
`make legal-info`.  Any proprietary software built on top of Infix, or
Buildroot, would need separate auditing to ensure it does not link with
any GPL[^2] licensed library.

[^2]: Infix image builds use GNU libc (GLIBC) which is covered by the
	[LGPL][8].  The LGPL *does allow* proprietary software, as long as
	said software is linking dynamically, [not statically][5], to GLIBC.

[1]: https://buildroot.org/
[2]: https://www.sysrepo.org/
[5]: https://lwn.net/Articles/117972/
[8]: https://en.wikipedia.org/wiki/GNU_Lesser_General_Public_License
