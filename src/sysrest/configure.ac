AC_PREREQ(2.61)
AC_INIT([sysrest], [1.0.0],
	[https://github.com/kernelkit/infix/issues])
AM_INIT_AUTOMAKE(1.11 foreign subdir-objects)
AM_SILENT_RULES(yes)

LT_INIT

AC_CONFIG_FILES([
	Makefile
	src/Makefile
])

AC_PROG_CC
AC_PROG_INSTALL

# Check for pkg-config first, warn if it's not installed
PKG_PROG_PKG_CONFIG

PKG_CHECK_MODULES([sysrepo], [sysrepo >= 2.2.36])
PKG_CHECK_MODULES([fcgi],    [fcgi  >= 2.4.2])

# Plugin installation path for sysrepo-plugind
dnl PKG_CHECK_VAR([srpdplugindir], [sysrepo], [SRPD_PLUGINS_PATH])
dnl AC_SUBST(srpdplugindir)

dnl test "x$prefix" = xNONE && prefix=$ac_default_prefix
dnl test "x$exec_prefix" = xNONE && exec_prefix='${prefix}'

dnl DATAROOTDIR=`eval echo $datarootdir`
dnl DATAROOTDIR=`eval echo $DATAROOTDIR`
dnl AC_SUBST(DATAROOTDIR)

dnl LIBDIR=`eval echo $libdir`
dnl LIBDIR=`eval echo $LIBDIR`
dnl AC_SUBST(LIBDIR)

dnl LOCALSTATEDIR=`eval echo $localstatedir`
dnl LOCALSTATEDIR=`eval echo $LOCALSTATEDIR`
dnl AC_SUBST(LOCALSTATEDIR)

dnl RUNSTATEDIR=`eval echo $runstatedir`
dnl RUNSTATEDIR=`eval echo $RUNSTATEDIR`
dnl AC_SUBST(RUNSTATEDIR)

dnl SYSCONFDIR=`eval echo $sysconfdir`
dnl SYSCONFDIR=`eval echo $SYSCONFDIR`
dnl AC_SUBST(SYSCONFDIR)

AC_OUTPUT
