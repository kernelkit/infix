AC_PREREQ(2.61)
AC_INIT([klish-plugin-infix], [1.0.0],
	[https://github.com/kernelkit/infix/issues])
AM_INIT_AUTOMAKE(1.11 foreign subdir-objects)
AM_SILENT_RULES(no)

LT_INIT

AC_CONFIG_FILES([
	Makefile
	src/Makefile
	xml/Makefile
])

AC_PROG_CC
AC_PROG_INSTALL

#
# Check feature flags
#
AC_ARG_ENABLE(containers,
        AS_HELP_STRING([--enable-containers], [Enable support for containers]),,[
	enable_containers=no])

AC_ARG_ENABLE(shell,
        AS_HELP_STRING([--enable-shell], [Enable shell access from CLI]),,[
	enable_shell=no])

AC_ARG_WITH(shell,
        AS_HELP_STRING([--with-shell=shell], [Path to shell to use, default: /bin/sh]),
	[shell=$withval], [shell=yes])

# Check for pkg-config first, warn if it's not installed
PKG_PROG_PKG_CONFIG

PKG_CHECK_MODULES([sysrepo], [sysrepo >= 2.2.60])
PKG_CHECK_MODULES([libyang], [libyang >= 2.1.80])
AC_CHECK_LIB([klish], [kplugin_new],, AC_MSG_ERROR([Klish not found]))

# Control build with automake flags
AM_CONDITIONAL(CONTAINERS,   [test "x$enable_containers" != "xno"])
AM_CONDITIONAL(SHELL,        [test "x$enable_shell" != "xno"])

AS_IF([test "x$with_shell" != "xno"], [
	AS_IF([test "x$shell" = "xyes"], [shell=/bin/sh])
	with_shell=$shell
	AC_DEFINE_UNQUOTED(SHELL, "$shell", [Default: /bin/sh])],[
	AC_DEFINE_UNQUOTED(SHELL, "/bin/sh")])

test "x$prefix" = xNONE && prefix=$ac_default_prefix
test "x$exec_prefix" = xNONE && exec_prefix='${prefix}'

KLISHCONFDIR=`eval echo $sysconfdir/klish`
AC_SUBST(KLISHCONFDIR)

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

Optional features:
  Container support ....: $enable_containers
  Allow shell access ...: $enable_shell
  Shell to use .........: $with_shell

------------- Compiler version --------------
$($CC --version || true)
-------------- Linker version ---------------
$($LD --version || true)
---------------------------------------------

Check the above options and compile with:
 ${MAKE-make}

EOF
