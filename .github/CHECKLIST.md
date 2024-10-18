Checklists for Pull Requests and Releases
=========================================

Maintainer checklists for reviewing pull requests and doing releases.


Pull Requests
-------------

 - If applicable, is there a readable ChangeLog entry?
 - If any LICENSE file has been updated, has the `.hash` file been updated?
 - If any change to a Finit `.svc` file, does any run/task linger?  
   I.e., is there a runlevel and/or condition defined to prevent them
   from running outside of their intended runlevel?
 - If any change to grub or qemu/qeneth setup, has it been tested in GNS3?
   - If any change to u-boot/buildroot, has it been tested with `<booloader>_defconfig`
 - If any change to logging, have the resulting logs been audited?
   - Check for duplicate entries, misspellings
   - Check for sneaky severity changes, e.g., error vs note, error vs warning
 - If new subsystem, or major changes to a subsystem, have the docs been updated?
 - If change to mDNS, has it been tested with netbrowse?
 - If change to `_defconfig`, verify `local.mk` and sync with other archs
   - Test manually as well, e.g., CLI changes do not have ha regression tests
   - Build from distclean, or use artifacts built by build servers, for manual tests


Releases
--------

 - Make at least one -betaN release to verify the GitHub workflow well in time release day
   - Stuff happens, remember kernelkit/infix#735
 - Make at least one -rcN to flush out any issues in customer repos
   - Easy to forget adaptations/hacks in customer repos -- may need Infix change/support
 - Ensure the markdown link for the release diff is updated
 - Ensure subrepos are tagged (can be automated, see kernelkit/infix#393)
