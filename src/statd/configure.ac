AC_PREREQ(2.61)
AC_INIT([statd], [1.0.0], [https://github.com/kernelkit/infix/issues])
AM_INIT_AUTOMAKE(1.11 foreign subdir-objects)
AM_SILENT_RULES(yes)

AC_CONFIG_FILES([
	Makefile
])

AC_PROG_CC
AC_PROG_INSTALL

#
# Check feature flags
#
AC_ARG_ENABLE(containers,
        AS_HELP_STRING([--enable-containers], [Enable support for containers]),,[
	enable_containers=no])

# C defines
AS_IF([test "x$enable_containers" = "xyes"], [
        AC_DEFINE(CONTAINERS, 1, [Built with container support])])

# Control build with automake flags
AM_CONDITIONAL(CONTAINERS,   [test "x$enable_containers" != "xno"])

# Check for pkg-config first, warn if it's not installed
PKG_PROG_PKG_CONFIG

PKG_CHECK_MODULES([jansson], [jansson >= 2.0.0])
PKG_CHECK_MODULES([libite],  [libite  >= 2.5.0])
PKG_CHECK_MODULES([libyang], [libyang >= 2.1.80])
PKG_CHECK_MODULES([sysrepo], [sysrepo >= 2.2.36])
PKG_CHECK_MODULES([libsrx],  [libsrx  >= 1.0.0])

AC_CHECK_HEADER([ev.h],
    [AC_CHECK_LIB([ev], [ev_loop_new],
        [],
	[AC_MSG_ERROR("libev not found")] )],
    [AC_MSG_ERROR("ev.h not found")]
)

test "x$prefix" = xNONE && prefix=$ac_default_prefix
test "x$exec_prefix" = xNONE && exec_prefix='${prefix}'

DATAROOTDIR=`eval echo $datarootdir`
DATAROOTDIR=`eval echo $DATAROOTDIR`
AC_SUBST(DATAROOTDIR)

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
