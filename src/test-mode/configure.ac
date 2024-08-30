AC_PREREQ(2.61)
AC_INIT([test-mode], [1.0], [https://github.com/kernelkit/infix/issues])
AM_INIT_AUTOMAKE(1.11 foreign subdir-objects)
AM_SILENT_RULES(yes)

AC_CONFIG_FILES([
	Makefile
	src/Makefile
	yang/Makefile
])

AC_PROG_CC
AC_PROG_INSTALL
LT_INIT

PKG_PROG_PKG_CONFIG

PKG_CHECK_MODULES([sysrepo], [sysrepo >= 2.2.36])
PKG_CHECK_MODULES([libite],  [libite >= 2.5.0])

# Plugin installation path for sysrepo-plugind
PKG_CHECK_VAR([srpdplugindir], [sysrepo], [SRPD_PLUGINS_PATH])
AC_SUBST(srpdplugindir)

test "x$prefix" = xNONE && prefix=$ac_default_prefix
test "x$exec_prefix" = xNONE && exec_prefix='${prefix}'

YANGDIR=`eval echo $datarootdir/yang/modules/test-mode`
AC_SUBST(YANGDIR)

LIBDIR=`eval echo $libdir`
LIBDIR=`eval echo $LIBDIR`
AC_SUBST(LIBDIR)

LOCALSTATEDIR=`eval echo $localstatedir`
LOCALSTATEDIR=`eval echo $LOCALSTATEDIR`
AC_SUBST(LOCALSTATEDIR)

RUNSTATEDIR=`eval echo $runstatedir`
RUNSTATEDIR=`eval echo $RUNSTATEDIR`
AC_SUBST(RUNSTATEDIR)

SYSCONFDIR=`eval echo $sysconfdir`
SYSCONFDIR=`eval echo $SYSCONFDIR`
AC_SUBST(SYSCONFDIR)

AC_OUTPUT

cat <<EOF

------------------ Summary ------------------
 $PACKAGE_NAME version $PACKAGE_VERSION
  Prefix................: $prefix
  Exec prefix...........: $eprefix
  Sysconfdir............: `eval echo $sysconfdir`
  Localstatedir.........: `eval echo $localstatedir`
  Default fstab.........: `eval echo $fstab`
  System environment....: ${sysconfig_path:-${sysconfig}}
  C Compiler............: $CC $CFLAGS $CPPFLAGS $LDFLAGS $LIBS
  Linker................: $LD $LLDP_LDFLAGS $LLDP_BIN_LDFLAGS $LDFLAGS $LIBS

------------- Compiler version --------------
$($CC --version || true)
-------------- Linker version ---------------
$($LD --version || true)
---------------------------------------------

Check the above options and compile with:
 ${MAKE-make}

EOF
