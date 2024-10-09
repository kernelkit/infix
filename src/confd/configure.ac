AC_PREREQ(2.61)
# confd version is same as system YANG model version, step on breaking changes
AC_INIT([confd], [1.2], [https://github.com/kernelkit/infix/issues])
AM_INIT_AUTOMAKE(1.11 foreign subdir-objects)
AM_SILENT_RULES(yes)

AC_CONFIG_FILES([
	Makefile
	bin/Makefile
	bin/gen-version
	share/Makefile
	share/factory.d/Makefile
	share/failure.d/Makefile
	share/test.d/Makefile
	share/migrate/Makefile
	share/migrate/1.0/Makefile
	share/migrate/1.1/Makefile
	share/migrate/1.2/Makefile
	src/Makefile
	yang/Makefile
])

AC_PROG_CC
AC_PROG_INSTALL
LT_INIT

AC_CONFIG_LIBOBJ_DIR(lib)
AC_REPLACE_FUNCS(vasprintf)
AC_REPLACE_FUNCS(asprintf)

#
# Check feature flags
#
AC_ARG_ENABLE(containers,
        AS_HELP_STRING([--enable-containers], [Enable support for containers]),,[
	enable_containers=no])

AC_ARG_WITH(login-shell,
        AS_HELP_STRING([--with-login-shell=shell], [Login shell for new users, default: /bin/false]),
	[login_shell=$withval], [login_shell=yes])

AC_ARG_WITH(crypt,
        AS_HELP_STRING([--with-crypt=crypt], [Default crypt for $0$cleartext, default: yescrypt]),
	[crypt=$withval], [crypt="yescrypt"])

AS_IF([test "x$enable_containers" = "xyes"], [
        AC_DEFINE(CONTAINERS, 1, [Built with container support])])

AS_IF([test "x$with_login_shell" != "xno"], [
	AS_IF([test "x$login_shell" = "xyes"], [login_shell=/bin/false])
	AC_DEFINE_UNQUOTED(LOGIN_SHELL, "$login_shell", [Default: /bin/false])],[
	AC_DEFINE_UNQUOTED(LOGIN_SHELL, "/bin/false")])

AS_IF([test "x$with_crypt" != "xno"], [
	AS_IF([test "x$crypt" = "xyes"], [crypt=yescrypt])
	AC_DEFINE_UNQUOTED(DEFAULT_CRYPT, "$crypt", [Default: yescrypt])],[
	AC_DEFINE_UNQUOTED(DEFAULT_CRYPT, "yescrypt")])

# Check for pkg-config first, warn if it's not installed
PKG_PROG_PKG_CONFIG

PKG_CHECK_MODULES([crypt],   [libxcrypt >= 4.4.27])
PKG_CHECK_MODULES([glib],    [glib-2.0 >= 2.50 gio-2.0 gio-unix-2.0])
PKG_CHECK_MODULES([jansson], [jansson >= 2.0.0])
PKG_CHECK_MODULES([libite],  [libite >= 2.5.0])
PKG_CHECK_MODULES([sysrepo], [sysrepo >= 2.2.36])
PKG_CHECK_MODULES([libsrx],  [libsrx >= 1.0.0])

# Control build with automake flags
AM_CONDITIONAL(CONTAINERS,   [test "x$enable_containers" != "xno"])

# Plugin installation path for sysrepo-plugind
PKG_CHECK_VAR([srpdplugindir], [sysrepo], [SRPD_PLUGINS_PATH])
AC_SUBST(srpdplugindir)

test "x$prefix" = xNONE && prefix=$ac_default_prefix
test "x$exec_prefix" = xNONE && exec_prefix='${prefix}'

DATAROOTDIR=`eval echo $datarootdir`
DATAROOTDIR=`eval echo $DATAROOTDIR`
AC_SUBST(DATAROOTDIR)

YANGDIR=`eval echo $datarootdir/yang/modules/confd`
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

Optional features:
  Container support ....: $enable_containers
  Login shell ..........: $login_shell
  Default crypt algo ...: $crypt

------------- Compiler version --------------
$($CC --version || true)
-------------- Linker version ---------------
$($LD --version || true)
---------------------------------------------

Check the above options and compile with:
 ${MAKE-make}

EOF
