AC_PREREQ(2.61)
AC_INIT([rauc-installation-status], [1.0], [https://github.com/kernelkit/infix/issues])
AM_INIT_AUTOMAKE(1.11 foreign subdir-objects)
AM_SILENT_RULES(yes)

AC_CONFIG_FILES([
	Makefile
])

AC_PROG_CC
AC_PROG_INSTALL
LT_INIT


PKG_PROG_PKG_CONFIG

test "x$prefix" = xNONE && prefix=$ac_default_prefix
test "x$exec_prefix" = xNONE && exec_prefix='${prefix}'

PKG_CHECK_MODULES([glib],    [glib-2.0 >= 2.50 gio-2.0 gio-unix-2.0])
PKG_CHECK_MODULES([jansson], [jansson >= 2.0.0])


LIBDIR=`eval echo $libdir`
LIBDIR=`eval echo $LIBDIR`

AC_SUBST(LIBDIR)

AC_OUTPUT
